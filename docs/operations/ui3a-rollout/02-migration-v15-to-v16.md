# 02 · schema v15 → v16 迁移准备、dry-run 与回滚

## 1. 迁移内容（append-only）

迁移文件 `migrations/0016_product_admin_actions.sql`（24 行，**append-only，未修改**）：

- `ALTER TABLE product_users ADD COLUMN admin_version bigint NOT NULL DEFAULT 0 CHECK (admin_version >= 0);`
- 新建三张表：
  - `product_admin_actions`
  - `product_admin_action_results`
  - `product_admin_probe_cooldowns`
- v16 内嵌 checksum：`40344bd66dc4a9070b24fc61ccaa8088a2839f12d153038468a3225adef0536d`

性质：**纯增量**（新增列带默认值 + 新增表），不删除/重命名既有对象，对旧进程保持向后兼容。

## 2. 迁移前生产实测状态（只读）

- `schema_migrations`：`head=15`，`rows=15`
- 不存在 `product_users.admin_version`；不存在 `product_admin_actions`
- 关键表行数（迁移前）：
  - `product_users=7`、`product_sessions=98`
  - `storage_bindings=6`、`storage_connections=7`
  - `copies=1535`、`product_file_shares=23`、`product_file_share_grants=24`
  - `jobs=10`、`acquisition_manifests=10`、`acquisition_provider_tasks=10`
  - `product_operations=10`、`product_operation_quotas=2`、`indexcore_projection_cursors=1`
  - `assets=0`、`releases=0`、`variants=0`

## 3. 迁移器机制

- `internal/store/postgres/migrator.go`：迁移与 `schema_migrations` 记录同一事务内执行，并以 `pg_advisory_xact_lock(0x50414E5441)` 串行化。
- `panta-db status` 输出兼容性状态：`compatible / current_version / latest_version / pending / modified / history_gaps / out_of_order / unknown`。
- checksum 采用内嵌 SQL 文件内容的 SHA-256；任何对已应用迁移的改动会被识别为 `modified` 而**拒绝迁移**。

## 4. 单元/集成测试（既有，非本次新增）

```
go test ./internal/store/postgres -run TestPostgresAdminOperationsMigration15To16AndRollback -count=1 -v
```

| 目标 PG | 结果 |
| --- | --- |
| PostgreSQL 16 | PASS |
| PostgreSQL 18 | PASS |

覆盖：应用 1..15 → 播种 legacy `product_users` → 事务内应用 0016 → `admin_version` 默认 0 → 事务 rollback → `product_admin_actions` 不存在（事务性 DDL rollback）→ legacy 行未变。

## 5. 隔离 dry-run（对生产数据副本，无停机）

步骤与结果：

1. 生产产品库 **live** 快照：`pg_dump -Fc`，sha256 = `cd0350235b008a84c98f5d2a8059cb873850a30ede28e832e813b570c8fb78dd`。
2. 恢复到生产主机上的隔离 PG 容器（库名以 `_test`/专用名，测试 helper 拒绝破坏非测试库）。
3. **逐表行数比对**：恢复副本与生产**完全一致**（判定 `ROWCOUNTS_IDENTICAL`）→ 备份可恢复性成立。
4. `panta-db status`（迁移前）：`compatible=false, current=15, latest=16, pending=[16]`。
5. `panta-db migrate`：`compatible=true, current=16, latest=16`。
6. 迁移后校验：`head=16, rows=16`；三张新表 `new_tables=3`；`admin_version` 列存在。
7. **新镜像启动冒烟**：`cloudsite:ui3a-c43408bb serve-product-readonly` 对接 head16 隔离库 →
   - 容器 `running=true, exit=0`（无错误日志）
   - `GET /`（Host `cloud.netioi.com`）= `200`
   - `GET /api/v1/auth/capabilities` = `{"registration_enabled":true}`

> 结论：迁移逻辑在**真实生产数据**上成功；新镜像可在 head16 上正常启动与服务。schema16 单向门风险已显著降低。

## 6. 备份方案（生产权威备份，**尚未执行**）

在 writer 冻结下进行，避免非一致快照：

1. 冻结写入：停止 `cloudsite` 产品容器（仅 product；catalog/catalog-cloud 只读，保持）。
2. 完整备份：
   - `pg_dump -Fc`（含历史 reservations 等全部业务数据）
   - 受保护配置（product config / 数据库 DSN 文件 / acquisition bootstrap）
   - `formal.env` 与 compose 文件
   - 容器 inspect（切换前状态）
   - 记录全部 `sha256sum`
3. 记录切换前 `head-before`（期望 15）。
4. 恢复验证：将备份恢复到隔离 PG，校验 `head=15` 且**逐表行数与生产一致**。

## 7. 回滚步骤（gate6）

- 回滚**不是** down-migrator。回滚 = writer freeze 下 **恢复 schema15 备份 + 还原旧镜像/配置**：
```
docker compose --env-file <formal.env> -f <compose> stop cloudsite
# 恢复 schema15 备份到产品库（drop/recreate 或 --clean）
docker exec -i <product-db> pg_restore -U <user> -d <db> --no-owner --no-privileges --exit-on-error < <backup>.dump
# compose 还原 image: cloudsite:deleted-preview-bf89f5a
docker compose --env-file <formal.env> -f <compose> up -d --no-deps --force-recreate cloudsite
```
- 关键约束：旧镜像要求 schema15，故「恢复备份 + 旧镜像」必须成对；**禁止**让旧镜像运行在 schema16 上。
- 触发条件：迁移/切换/验收任一步失败、健康检查不通过、旧数据缺失或出现回归。

## 8. 单向门风险说明

- 一旦生产切到 schema16 且运行新镜像，回退到旧镜像必须先恢复 schema15 备份（迁移后产生的写入将丢失）。
- 该风险已通过第 5 节的隔离 dry-run（真实数据 + 新镜像启动）显著降低，但**真实生产切换仍属不可逆窗口**，必须按第 6/7 节准备。