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
3. **逐表行数比对**：恢复副本与生产**完全一致**（判定 `ROWCOUNTS_IDENTICAL`）→ 仅证明该**非冻结（live）测试快照**可恢复到一致的表计数（近似一致性），**不等于**生产恢复正确性（见下方「结论」）。
4. `panta-db status`（迁移前）：`compatible=false, current=15, latest=16, pending=[16]`。
5. `panta-db migrate`：`compatible=true, current=16, latest=16`。
6. 迁移后校验：`head=16, rows=16`；三张新表 `new_tables=3`；`admin_version` 列存在。
7. **新镜像启动冒烟**：`cloudsite:ui3a-c43408bb serve-product-readonly` 对接 head16 隔离库 →
   - 容器 `running=true, exit=0`（无错误日志）
   - `GET /`（Host `cloud.netioi.com`）= `200`
   - `GET /api/v1/auth/capabilities` = `{"registration_enabled":true}`

> **结论（Round 1 修正）**：在**非冻结（live）**的生产数据副本上，v15→v16 迁移逻辑跑通、恢复副本可读，且新镜像可在 head16 上正常启动与服务。
>
> 该副本为 live 快照，因此这**仅**证明「该测试快照可恢复 + 近似表计数一致」，**不构成**：
> - 并发写入条件下**生产恢复正确性**的证明；
> - v16→v15 **反向迁移演练**的证明。
>
> 因此生产回滚在 Round 1 时为 `NOT_VERIFIED`；**隔离反向演练已于 2026-10-22 通过**（见 §9 与 `06-reverse-cutover-drill.md`）。最终仍须以「冻结 writer 备份 + 验证恢复」的可复核证据为准。

## 6. 备份方案（生产权威备份）

> **状态：`NOT_RUN`。** 下列为**要求方案**，尚未执行。在产出「冻结 writer 备份 + 验证恢复」的可复核证据之前，不得宣称生产备份/恢复已验证。

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

## 7. 回滚步骤（gate6）— 状态：`DRILL-VERIFIED（隔离演练通过）` / 生产切换未授权

> **反向演练已于 2026-10-22 在隔离容器内通过（见 `06-reverse-cutover-drill.md`）**：v15 备份恢复到全新空库 → `head=15`、**v16-only 对象不存在**、关键表行数一致；**旧镜像在该库上正常启动**（`GET /`=200）。
> 但生产切换**仍未授权**；实际回滚须在**冻结 writer 的生产窗口**内执行，且权威备份须在冻结后重取并验证。

### 7.1 为什么不能用 `pg_restore --clean` 直接就打到现有 v16 库

- `--clean` 只会删除 **schema15 备份中记录的对象**，**不会**删除 v16 新增的
  `product_admin_actions` / `product_admin_action_results` / `product_admin_probe_cooldowns`，
  也不会移除新增列 `product_users.admin_version`。
- v16 审计表对 v15 对象存在**外键依赖**；非事务的 `--clean --exit-on-error` 可能在删除/重建中途失败，使**唯一的生产库处于不可用状态**。
- 因此 in-place `pg_restore --clean` 既不安全，也**不能证明**回到了精确的 schema15。

### 7.2 受控恢复方案（要求；须演练通过后才可执行）

1. **fence 所有 product DB writers 与连接**：停止 `cloudsite` 产品容器（含 acquisition worker），并确认无其他写入者。
2. 目标为一个**全新的空数据库**（或对 `public` schema 做**完整 owned 重建**），**保留 DB 的 grants / extensions / owner**。
3. 使用**匹配的 schema15 备份**恢复到该空/重建目标（例如恢复到新库，而**非覆盖现有库**）。
4. 验证：`schema_migrations` 回到 head=15、历史与 checksum 精确匹配、**v16-only 对象完全不存在**、关键表数据与备份一致、**旧镜像可启动**。
5. 仅在上述验证通过后，执行**经过验证的连接切换**（将服务指向恢复库，或采用其他已测试、经 operator 批准的流程）。

> 备选：任何**已完整测试**、经 operator 批准的恢复流程。**禁止**把未演练的破坏性 in-place 命令当作安全步骤。

- 关键约束：旧镜像要求 schema15，故「恢复备份 + 旧镜像」必须成对；**禁止**让旧镜像运行在 schema16 上。
- 触发条件：迁移/切换/验收任一步失败、健康检查不通过、旧数据缺失或出现回归。

## 8. 单向门风险说明

- 一旦生产切到 schema16 且运行新镜像，回退到旧镜像必须先恢复 schema15 备份（迁移后产生的写入将丢失）。
- 该风险已通过第 5 节的隔离 dry-run（真实数据 + 新镜像启动）**部分**降低，但**真实生产切换仍属不可逆窗口**，必须按第 6/7 节准备。

## 9. 反向演练（reverse-cutover drill）— 状态：`PASS（隔离）`（证据：`06-reverse-cutover-drill.md`）

在授予生产切换之前，须在隔离环境完成并留证：

1. 构造一个 **v16** 目标库（含代表性的新 ADMIN 审计记录 `product_admin_actions`/`product_admin_action_results` 与产品会话）。
2. 将 **schema15 备份**恢复到一个**全新的空库**（或完整 owned 重建），保留 grants/extensions/owner。
3. 验证：
   - `schema_migrations` 精确回到 **head=15**，历史与 checksum 全部匹配，且 **v16-only 对象已完全不存在**；
   - 关键表数据与备份一致；
   - **旧镜像**（`cloudsite:deleted-preview-bf89f5a`）能在该库上正常启动。
4. 演练已于 2026-10-22 **通过**（见 `06-reverse-cutover-drill.md`）；§7.2 的回滚流程**已在隔离环境验证**，但生产切换**仍未授权**。

> 已于 2026-10-22 在生产主机**隔离容器**内完成（`head=15`、`v16_objects_present=0`、`admin_version_col=0`、行数一致、旧镜像启动 `200`；见 `06-reverse-cutover-drill.md`）。**未在生产环境执行任何变更**；生产切换仍未授权。