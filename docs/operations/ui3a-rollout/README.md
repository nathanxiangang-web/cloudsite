# CloudSite UI-3A 生产 Rollout — 部署准备工作与验收证据

> 状态：**部署准备（pre-cutover）已完成并通过隔离验证；生产变更尚未执行。**
> 本文档仅供架构审查与部署准入评审，**不包含任何凭据**（无口令、Token、Cookie、密钥、数据库连接串、受保护配置内容、二进制）。

- 日期：2026-10-22（Asia/Shanghai）
- 目标主机：生产 VPS（`/opt/my-servers`，compose 项目 `cloudsite-v100-alpha`）
- 基线提交：`c43408bb61722fb3d575063b8da79c04d736abdc`（`main` HEAD，PR #150）
- 审查分支：`docs/ui3a-production-rollout-evidence`
- 授权范围：**仅证据归档与部署准入审查**。不授权执行生产数据库迁移、服务切换、重启或真实账号修改。

## 1. 结论摘要（严格区分三种状态）

| 类别 | 内容 | 状态 |
| --- | --- | --- |
| **① 已完成的隔离 dry-run** | v15→v16 迁移脚本在**生产数据快照的隔离恢复副本**上跑通；新镜像在 head16 隔离库上启动成功 | ✅ PASS |
| **② 尚未执行的真实联机测试** | 真实 115 `GetInfo` 探针、真实 USER 创建/停用/启用验收 | ⛔ NOT EXECUTED |
| **③ 生产当前实际运行版本** | 镜像 `cloudsite:deleted-preview-bf89f5a`（revision `bf89f5ab658aacecd3775e310ed488b7a69136da`），schema **v15** | 未变更 |

**生产环境当前仍运行旧镜像 + schema15；UI-3A 尚未启用。**

## 2. 完成度矩阵

| 步骤 | 结果 |
| --- | --- |
| 生产只读侦察 | ✅ 完成（见 `01`） |
| 权威源码构建 + 镜像构建并落盘生产 | ✅ 完成（见 `01`） |
| 隔离恢复副本上 v15→v16 迁移 dry-run | ✅ 通过（见 `02`） |
| 新镜像在 head16 隔离库启动冒烟 | ✅ 通过（见 `02`） |
| 反向演练（v16→v15 恢复，rollback 验证） | ⛔ `NOT_RUN` → 回滚 `NOT_VERIFIED`（见 `02` §9） |
| 生产冻结 writer + 权威备份 | ⛔ 未执行（被环境安全护栏阻断，见 `05`） |
| 生产 schema15→16 迁移 | ⛔ 未执行 |
| 生产镜像切换 | ⛔ 未执行 |
| 真实 115 联机验收 | ⛔ 未执行（见 `03`） |
| 真实 USER 启停验收 | ⛔ 未执行（见 `03`） |

> 注：隔离快照「v15→v16 迁移成功」**不等于**生产回滚已验证。生产回滚（v16→v15）是独立演练项，当前 **`NOT_RUN`**，故 rollback 判定 **`NOT_VERIFIED`**、生产切换 **BLOCKED**。

## 3. 文档索引

| 文件 | 内容 |
| --- | --- |
| `01-source-and-image.md` | 源码版本、镜像标识、构建过程、校验哈希 |
| `02-migration-v15-to-v16.md` | 迁移准备、dry-run 结果、备份/恢复方案、回滚步骤、测试命令与结果 |
| `03-live-acceptance.md` | 真实 115 与用户管理的**联机**验收条件（未执行） |
| `04-operator-runbook.md` | 剩余生产步骤的可执行 runbook（脱敏占位） |
| `05-findings-and-blockers.md` | 发现的问题、阻断点、遗留项与 UNKNOWN |

## 4. 脱敏声明

- 未提交任何凭据、受保护配置文件内容或二进制镜像。
- 文中出现的口令/Token/Cookie/DSN 一律以占位符或"由操作者持有"表述。
- 生产数据仅以**计数/聚合**形式出现，不含真实用户敏感数据。
- 受保护绑定、凭据引用等以 `<binding-id>`、`<credential-ref>` 形式遮蔽。
## 5. Round 1 审查修正记录（2026-10-22）

依据 PR #151 正式审查（`FIX REQUIRED`）修正三处，**仅改 Markdown，不改业务代码、不执行远程命令**：

1. **回滚安全**：移除 in-place `pg_restore --clean`（`02` §7、`04` §8），改为「恢复到**全新空库** + 验证 + 连接切换」的受控方案；并新增 `02` §9 **反向演练（v16→v15）要求**，状态 `NOT_RUN`，rollback 标记 **`NOT_VERIFIED` / 生产切换 BLOCKED**。
2. **115 canary 契约**：修正为「顶层 `status=SUCCEEDED` 且 `code=PROBE_RECORDED`」+「嵌套 `capacity.status=AUTHENTICATED` 且 `capacity.source=115_GET_INFO`」；补充 `Origin`/`Accept` 等必需请求头；UNKNOWN/NOT_AUTHENTICATED 明确不得视为成功（`03` §2）。
3. **证据精度与受保护数据留存**：`02` §5 结论改为「仅证明**非冻结测试快照**可恢复 + 近似计数一致，不等于生产恢复正确性，也非反向演练」；`05` §3 明确隔离容器内**生产数据副本**的访问控制 / custodian / 处理与留存 / **授权处置 gate**。