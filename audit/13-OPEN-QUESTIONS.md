# 13 需要项目所有者确认的决策与未知材料

## 1. 架构与产品决策

### Q00 — 后续平台是在 CloudSite 1.0 上扩展，还是建设 successor product？

- 背景：CloudSite 仓库当前处于 maintenance，但允许新的明确请求；IndexCore `PROJECT-CONTEXT.md:104-112` 和 `PROJECT-STATE.md:222-256` 则将 CloudSite 1.0 标为 Legacy/Frozen Product，并将正式 successor product、Gate 5、auth/search/catalog、115 集成标为需要新 Architect 计划明确授权。
- 建议：所有者/Architect 先二选一并冻结责任边界。若继续扩展 CloudSite 1.0，需明确 supersede IndexCore 项目中“do not inherit CloudSite architecture”等受影响旧边界；若建设 successor，则只复用 CloudSite 的需求、经验和可独立复用模块，不默认继承其架构。
- 为什么：这是 R2 数据库、API、任务和前端工作的前置分叉，不能用“每个处理包单独授权”替代。
- 阻塞：R2 全系列以及正式产品 Gate/实施路线。

### Q01 — Copy 是否正式保持一对一物理资源？

- 背景：当前 DB 一对一，项目上下文写“one or more”。
- 建议：保持一对一，用 Variant→多个 Copy 表达多文件，用 Collection 表达合集。
- 为什么：避免无谓身份迁移，保持 IndexCore 投影简单。
- 阻塞：R2-3 的正式领域合同和 API 冻结。

### Q02 — 首批允许哪些物理写操作？

- 建议：Mkdir→Rename→Offline 已加固→Move/Copy/Upload→Recycle Restore→Delete；永久清理最后单独批准。
- 为什么：不同操作的 UNKNOWN、递归和部分成功风险差异大。
- 阻塞：R1-2/R2-1/R2-2 的实施范围。

### Q03 — 是否允许目录递归删除和批量永久清理？

- 建议：默认不允许；确需时独立权限、爆炸半径上限、新鲜影响预览、二次确认和恢复演练。
- 为什么：115 服务端递归语义与空列表清理语义尚未真实验证。
- 阻塞：任何高风险 API 上线。

### Q04 — CloudSite 产品分享与 115 原生分享谁为主？

- 背景：当前只有 CloudSite 自管单文件分享；上游只有快照/下载表面。
- 建议：短期保留产品分享，原生分享先做能力验证，不并行隐式创建两套分享。
- 阻塞：R2-2 分享路线。

### Q05 — source 明文入库的现行架构条款如何 supersede？

- 背景：当前实现存 original/execution source，AGENTS 条款写不存用户 URL/magnet。
- 建议：新命令 digest+opaque secret ref；历史数据保留、加密、清理和备份策略需明确 ADR。
- 阻塞：R1-2 数据迁移，不能只改文档掩盖现状。

### Q06 — 1500 累计上限与系统背压的边界（目标政策已明确，待实现）

- 项目所有者要求（2026-11-16）：每个 CloudSite 用户 UUID 的累计云下载提交次数上限为 **1500 次**。
- 计数单位为已持久化的新 `(user_id, operation_key)`；相同 key+digest 的重放、状态查询、轮询和 Worker claim 不重复计数。
- 已提交的失败、取消或 UNKNOWN 操作计数一次，不自动返还；达到上限后保留历史数据和原操作恢复能力，只拒绝新的不同意图。
- **不恢复每用户并发任务上限**，避免重现 PR #186 中 RECOVERY_REQUIRED 任务占满并发名额、阻塞新提交的问题。
- 账号/IP 速率、全局队列水位、Provider 预算和公平调度仍独立存在；不得把 1500 当作系统容量背压。
- 当前 main 仍执行 v1.0.2 起的不限累计次数政策；1500 仅是目标政策，须经独立实现、测试和发布后才生效。
- 阻塞：实现 PR 的幂等/事务/并发回归、超限零副作用、历史超限账号行为和 API DTO 兼容验收。

### Q07 — 业务分类全站共享还是按用户/空间隔离？

- 建议：现有公共库先全站共享；私有空间的标签、搜索计数、收藏、热门单独设计，不复用公共投影。
- 阻塞：R2-3/R2-4 权限模型。

### Q08 — 哪些规则可自动绑定 Variant？

- 建议：初期自动绑定仅限高置信、唯一、可回放规则；冲突和人工已改数据永不自动覆盖。
- 阻塞：元数据自动化程度与审核工作量。

### Q09 — 物理删除后 Asset 的生命周期？

- 建议：Copy 标记 REMOVED，Asset 默认保留并可显示“无可用副本”；是否自动归档由所有者决定。
- 阻塞：资源详情和搜索默认过滤规则。

### Q10 — 去重是否只展示、不自动清理？

- 建议：是。hash 只生成候选，同内容不同物理副本不自动合并/删除。
- 阻塞：未来存储优化功能。

## 2. 技术实施选择

### Q11 — 是否采用 Next.js + TypeScript reference consumer？

- 建议：若确需独立消费者，采用 App Router + server-only typed client，以只读搜索/详情做 strangler；现有原生 JS 不必推倒。
- 为什么：这是实施选择，不应被描述为新冻结架构或绕过 CloudSite 授权的理由。
- 阻塞：R2-4 前端工作量与部署形态。

### Q12 — 搜索是否先坚持 PostgreSQL？

- 建议：是，先类型化关联表+FTS/trigram；中文分词和 50K 实测不足时再评审独立搜索引擎。
- 阻塞：搜索基础设施与运维成本。

### Q13 — IndexCore R8/冲突解决接受哪种语义？

- 建议：先对抗测试和 ADR，冲突不授权删除；不能简单 newest/path winner，也不能全局永久冻结所有删除。
- 阻塞：R1-3 核心身份修复。

### Q14 — Draft F4 的哪些子能力可独立评审？

- 建议：将 latest observation 索引、配置 fence、debt 指标等小步拆分；不把 #147/148/149 整包当稳定方案。
- 阻塞：R1-6 与后续 schema6+ 决策。

### Q15 — item-local INTERNAL 是否允许隔离继续？

- 建议：只有可证明的 Provider/scope 局部错误可评审隔离；DB/invariant/systemic 错误仍 fail closed。
- 阻塞：R1-6 服务可用性策略。

### Q16 — freshness 是否有业务 SLO？

- 建议：先定义可测指标（P50/P95 debt age、最大可接受未观察时长），再谈 SLO；当前不设虚构数字。
- 阻塞：Watch 周期、后台预算、UI stale 文案和容量验收。

### Q17 — 备份 RPO/RTO 与保留期？

- 建议：所有者明确可接受数据丢失/恢复目标、远端保留、加密和异机副本；在决定前不宣称备份合格。
- 阻塞：R1-5/R2-6 生产恢复设计。

### Q18 — 单点是否接受？

- 建议：若接受单机，应明确这是成本决策，并优先可靠备份/恢复；不要以单机 advisory lock 冒充 HA。
- 阻塞：是否值得设计多 writer/HA；默认不做。

**并发工作核对（截至 2026-11-16T11:45:01Z）**：CloudSite PR190（open/non-draft）处理云下载会话健康前置检查，PR191（open/draft）研究 per-user 云下载 CRUD。二者均未合并、不是 main 能力；R1-1/R1-2/R2-1/R2-3 开工前必须评审重叠和迁移边界，Q00/Q07 的选择也需考虑 PR191 的方向。

## 3. 生产与验证授权

### Q19 — 如何提供真正的只读生产证据？

需要的不是秘密，而是脱敏回执或专用只读账号：

- Docker ps/inspect/logs、镜像 digest、重启/OOM；
- CloudSite/IndexCore `go version -m` 或 build manifest；
- schema_migrations 与聚合队列/连接/锁/尺寸 SELECT；
- alpha Compose/systemd/启动 flags（秘密值脱敏）；
- 备份远端加密/ACL/保留与隔离 restore 回执。

当前 DEP-07 阻断源码与生产完全一致、schema21、无积压和长稳结论。

### Q20 — 是否有专用 115 测试账号和隔离目录？

- 建议：真实接口验证仅在专用账号、隔离 Root、限定预算下进行；危险写测试还需逐项许可。
- 阻塞：分页、递归删除、回收站 rid、Copy 新 ID、分享和离线部分成功的线上 UNKNOWN。

### Q21 — 是否授权后续真实容量验证？

需另行授权 20K+、长稳、HTTP 吞吐、PG 负载、真实网盘操作和增量索引。当前任务明确未做，不能默认下一阶段已授权。

### Q22 — 生产主机压力先调预算还是扩资源？

- 建议：先取得容器/PG/磁盘/PSI 长窗口证据定位原因，不盲目加 Reader/并发或直接扩配。
- 阻塞：R1-6/R2-6 容量措施。

## 4. 当前 UNKNOWN 清单（无需立即产品决策，但必须补证）

1. CloudSite/IndexCore 实际二进制 Git SHA 和启动 flags。
2. CloudSite schema21 与 library_snapshots 实际状态。
3. IndexCore schema、Root、Watch/Work、F1/F2/读池/F4 实际配置和债务。
4. PostgreSQL 实际角色、连接池、锁、长事务、临时文件、表/索引尺寸和备份一致性。
5. OpenList 实际 image/config、115 拓扑凭证健康和容器权限。
6. 生产日志脱敏、错误率、重启/OOM 和监控告警。
7. 备份远端是否加密、ACL、保留和最近隔离恢复结果。
8. 真实 115 页大小/排序、429、Cookie 有效期、任务多结果、Copy/Move/Rename/Delete 的最终一致性和返回语义。

上述 UNKNOWN 不妨碍完成只读研究，但会阻止对应生产验收和真实接口能力标记为“已验证”。