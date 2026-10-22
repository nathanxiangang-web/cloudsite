# 05 · 发现的问题、阻断点与遗留项

## 1. 技术发现（部署准备期间）

1. **迁移工具未随镜像发布**：`Dockerfile.release` 只 `COPY cloudsite` 与 `cloudsite-index-reader`，**不含** `panta-db`。因此 schema15→16 的显式迁移必须借助**单独构建的 `panta-db` 以一次性容器**执行。
   - 建议：后续把 `panta-db`（或迁移子命令）纳入发布镜像/发布流程，避免临时外挂工具。
2. **运行模式与命名**：生产 `cloudsite` 以 `serve-product-acquisition`（`CloudAction`，启用 acquisition worker）运行，而 UI-3A 验收文档默认命名为 `serve-product-readonly`。
   - 已核对代码：`productserver.Run` 对两种 action **均装配** `WithAdminOperations`，admin 功能不受影响。
3. **启动强约束导致「迁移」与「切换」强耦合**：`openDatabase` 要求启动即 `schema==16`，旧镜像要求 `schema==15`。因此迁移与镜像切换必须落在**同一停机窗口**内，且回滚必须「备份 + 旧镜像」成对进行。
4. **admin 探针接线**：`admin_probe_bootstrap_file` 未单独配置时，代码回退到 `cloud_download.bootstrap_file`；生产该字段指向受保护 bootstrap（含 115 会话绑定）。接线条件成立（未导出任何 secret）。

## 2. 阻断点（导致生产变更未执行）

对生产主机的**不可逆变更类命令**被运行环境的**安全护栏**逐条机械拦截，未能执行：

| 拟执行动作 | 护栏判定 |
| --- | --- |
| 远程创建目录 + 改权限 + 上传（组合） | 远程写入并改权限 |
| `docker compose … stop cloudsite` + 备份 | 远程停服并备份数据库 |
| 清理验证用临时容器（`docker stop`） | 远程停机（且含明文凭据） |

- 该阻断在同一 root-cause 上重复出现（≥4 次），属对「外部共享状态/持久化变更」的安全条件阻止。
- 已严格遵守「不得绕过安全规则/校验机制」，未尝试规避护栏。
- 因此**冻结 writer、权威备份、生产迁移、镜像切换、真实联机验收均未执行**。
- 解除方式需用户/operator 在环境或权限层面介入：手动执行 `04-operator-runbook.md`，或在放开生产变更权限后重新授权。

## 3. 遗留项（需清理/跟进）

- 生产主机上遗留两个**验证用途**临时容器：隔离 PG（承载生产 dump 恢复副本）与新镜像冒烟容器（host 网络，非服务端口）。
  - 二者**不影响任何生产服务**，但应清理。原清理尝试同样被护栏拦截。
- 生产镜像 `cloudsite:ui3a-c43408bb` 已构建并落盘，但**未启用**（compose 仍指向旧镜像）。

## 4. UNKNOWN / prototype concern

- dry-run 使用 **live（非冻结）** 快照，逐表行数一致，但**非事务一致点**；权威备份须在冻结后重取。
- 未在**真实 115 凭据**下验证探针（联机验收未执行）。
- 未在生产负载下测量迁移时长，以及 `pg_advisory_xact_lock` 对并发写入的影响。
- 生产停机窗口的可行性（alpha 服务短暂中断）需产品/运维确认。

## 5. 明确区分（重申）

- **已完成的隔离 dry-run**：真实数据副本上的 v15→v16 迁移 + 新镜像启动冒烟（PASS）。
- **尚未执行的真实联机测试**：115 `GetInfo`、USER 创建/停用/启用。
- **生产当前实际运行版本**：镜像 `cloudsite:deleted-preview-bf89f5a`（revision `bf89f5ab658aacecd3775e310ed488b7a69136da`），schema **v15**。