# 06 · 反向恢复演练（v16 → 恢复 v15 备份 → 旧镜像启动）

> 结论：**PASS（隔离）**。在生产主机上的**隔离容器**中完成，**未触碰任何生产服务或生产数据库**。
> 本演练通过**不等于**生产切换已授权；生产切换仍为 **BLOCKED / 未授权**（见 §4）。

- 日期：2026-10-22（Asia/Shanghai）
- 上游缺口：`02-migration-v15-to-v16.md` §9「反向演练要求（`NOT_RUN`）」
- 脱敏：不含主机 IP、口令、Token、Cookie、DSN、受保护配置内容或二进制镜像。

## 1. 目的

验证 §9 要求：**从 v16 环境恢复 v15 备份到全新空库后**，(a) 回到精确的 schema15 且 **v16-only 对象完全不存在**，(b) 关键表数据一致，(c) **旧镜像能够正常启动**；并验证旧镜像在 schema16 上 fail-closed。

## 2. 方法（隔离、自清理）

- 材料（生产主机上，均**非**生产服务）：
  - v15 备份：生产产品库 `pg_dump -Fc`，sha256 `cd0350235b008a84c98f5d2a8059cb873850a30ede28e832e813b570c8fb78dd`
  - `panta-db` 一次性工具（本 rollout 构建）
  - 旧镜像 `cloudsite:deleted-preview-bf89f5a`（revision `bf89f5ab658aacecd3775e310ed488b7a69136da`）
- 容器（均**自终止、自清理**）：
  - `ui3a-revdrill-pg`：`postgres:16-alpine`，容器内临时 `initdb`（trust），监听 `127.0.0.1:55480`
  - `ui3a-revdrill-old`：旧镜像，host 网络
- 演练所需的受保护启动配置由**容器内在命名卷中生成**（不落主机、命令中不含任何 DSN/secret）。
- **全程未停止、未修改**任何生产服务或生产数据库。

## 3. 步骤与结果

### 3.1 构造 v16 环境

- `panta_v16` = 恢复 v15 备份 → `panta-db migrate`
- 结果：`head=16 rows=16`；`v16_objects=3`（三张审计表）；`admin_version_col=1`
- 插入 1 条**代表性 ADMIN 审计**：
  - `product_admin_actions`（`action=PROBE_115`）
  - `product_admin_action_results`（`outcome={"status":"SUCCEEDED","code":"PROBE_RECORDED"}`）
  - `product_admin_probe_cooldowns`（1 行）
  - → `audit_rows=1`

### 3.2 反向恢复（关键）

- `panta_rollback` = **全新空库** → `pg_restore` v15 备份（`--no-owner --no-privileges --exit-on-error`）
- 校验结果：
  - `head=15 rows=15` ✓
  - `v16_objects_present=0`（三张 v16 审计表**不存在**）✓
  - `admin_version_col=0`（`product_users.admin_version` **不存在**）✓
  - 关键表行数与生产一致：`copies=1535, product_users=7, product_sessions=98, storage_bindings=6, storage_connections=7, product_file_shares=23, product_file_share_grants=24, jobs=10, acquisition_manifests=10, acquisition_provider_tasks=10, product_operations=10, product_operation_quotas=2, indexcore_projection_cursors=1, assets=0, releases=0, variants=0` ✓

### 3.3 旧镜像启动验证

| 目标 | 期望 | 实测 |
| --- | --- | --- |
| 旧镜像 on **head15**（恢复库） | 正常启动 | `running=true, exit=0`，`GET /`=**200** ✅ |
| 旧镜像 on **head16**（v16 库） | fail-closed | `exit=1`，日志 `panta stopped: product process unavailable` ✅ |

结论：旧镜像确能在**恢复的 schema15 库**上正常启动；且在 schema16 上**按约束 fail-closed**（证明「旧镜像必须配 schema15」）。

### 3.4 自清理

- 演练容器 `ui3a-revdrill-pg`、`ui3a-revdrill-old` 均**自行退出并被清除**（无需外部 `stop`/`rm`）。
- 遗留：命名卷 `ui3a-revdrill-out`（仅含容器内生成的临时配置，**不含生产数据**）；其删除被环境安全护栏拦截，待授权处理。

## 4. 状态更新（相对 PR #151 的 Round 1 审查）

| 项 | Round 1 | 现在 |
| --- | --- | --- |
| 反向演练 | `NOT_RUN` | **`PASS（隔离）`** |
| rollback 判定 | `NOT_VERIFIED` | **`DRILL-VERIFIED（隔离演练通过）`** |
| 生产切换 | `BLOCKED` | **`BLOCKED`（仍未授权）** |
| 生产库/服务 | 未变更 | **未变更** |

## 5. 边界（UNKNOWN）

- 演练使用 **live（非冻结）** v15 快照；权威生产备份仍须在**冻结 writer** 后重取并验证。
- 演练在**隔离容器**进行，**不等于**生产回滚已执行；生产回滚命令只有在冻结窗口内才可实际运行。
- 未执行真实生产迁移/镜像切换、真实 115 探针、真实 USER 启停（仍见 `03` / `05`）。
- 本演练**不构成**生产部署或切换授权。

## 6. 复现要点（脱敏）

- 演练脚本与流程见 `02-migration-v15-to-v16.md` §9、`04-operator-runbook.md` §8；受保护配置经命名卷在容器内生成，命令与主机均不落 secret。
- 复用 v15 备份（sha256 `cd035023…`）作为唯一"回滚源"。