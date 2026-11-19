# 12 分阶段技术改造路线图

## 1. 执行规则

- 本路线图是建议，未修改代码或生产。每个处理包均需独立 Issue/ADR/评审和明确授权。
- 先安全与正确性，再通用写能力，再产品体验，最后真实规模；不以大重构替代小步证据。
- 数据库变更默认 append-only；旧二进制回滚必须先验证 schema 兼容，不能盲目切旧镜像。
- 真实 20K+、长稳、HTTP 吞吐、PG 负载和网盘压力统一放到授权部署后。
- PR #191 的 per-user UserSpace 仅在 Q00 产品路线明确、生产备份恢复 Gate 通过、schema21/22 兼容发布完成后施工；文档审查不构成代码/生产授权。
- 当前 main 的 migrator 严格校验 migration history；schema22 不能假定可由 v1.0.3 直接回滚，必须先准备双版本兼容发布或采用经演练的整库恢复方案。

## 2. P0：安全止血

### R0-1 — 115driver 凭证边界与危险默认行为

- 开发目标：关闭 DR-01/02/03；下载不跨源带长期 Cookie，禁用 HTTP API，debug 全脱敏。
- 影响模块：115driver `download.go/api.go/option.go/client.go`、CLI/MCP 下载调用；CloudSite 作为依赖消费者做协议回归。
- 数据库变更：无。
- API 变更：`DownloadInfo.Header` 改 allowlist；`WithMultiUrls` 移除或仅 HTTPS；保持取链方法签名可提供兼容包装。
- 兼容性：依赖“下载必须带账号 Cookie”的调用方需改为显式短时下载令牌；属于必要安全行为变更，需 release note。
- 测试要求：跨 host Cookie 零发送、HTTP 零发送、日志金丝雀扫描、取链/下载成功解密、CLI/MCP 回归。
- 风险：不同 CDN 的 Header 合同不完全已知，过度删除可能破坏下载。
- 回滚方案：保留上一版本二进制；不得回滚到允许明文/泄密 Header 的配置作为长期方案，失败时关闭受影响下载能力。
- 验收标准：所有非 API host 请求无长期 UID/CID/SEID/KID 或长期 Authorization；仅允许经批准、短时、单用途、单 host 的下载凭证，并记录 host allowlist 与 expiry/expires_at（若可得）；日志无秘密。

### R0-2 — 生产秘密文件与备份暴露面

- 开发目标：处理 DEP-01/02，确认秘密读取权限、备份内容和远端保护。
- 影响模块：`/opt/my-servers/.env`、rclone-backup、Compose、秘密轮换和备份策略。
- 数据库变更：无；可能需要秘密引用配置变更。
- API 变更：无；setup/master key 轮换需明确会话/数据兼容。
- 兼容性：轮换旧秘密可能使 session/setup/加密数据失效，必须按用途逐项设计。
- 测试要求：非 owner 读取失败、服务配置加载成功、归档清单、远端 ACL/加密、隔离恢复和旧秘密失效。
- 风险：权限/轮换/备份排除错误可导致服务启动或恢复失败。
- 回滚方案：授权窗口前备份配置元数据和秘密到既有受保护位置；只回滚配置，不恢复已公开旧秘密为长期有效。
- 验收标准：最小文件权限、秘密不进入不应包含的归档、远端加密/ACL 有证据、隔离恢复通过。

## 3. P1：核心正确性与可靠性

### R1-1 — 离线下载、分页、详情和上传边界修复

- 开发目标：关闭 CS-01/02、DR-09/10/11/12，建立真实 pinned SDK 协议测试。
- 影响模块：CloudSite `providers/115`、`runtime`、acquisition outcome；115driver `offline.go/dir.go/response.go/op.go/upload.go`。
- 数据库变更：建议仅为 recovery reason/error kind 增加受控枚举/字段，先评估现有文本兼容；不无理由改 schema。
- API 变更：Adapter 区分任务不存在、页预算耗尽、协议错误和暂时故障；对外任务 DTO 增加恢复类别，不暴露原始响应。
- 兼容性：旧 `PollUnknown` 映射需迁移，历史 REFERENCE_KNOWN 不重放；保持既有 Manifest/Job ID。
- 测试要求：多/少/重复离线结果、第四页任务、9GiB±1、CID mismatch、详情空 data、完整 Worker 和崩溃恢复。
- 风险：真实 115 排序/分页/任务最终一致性未知，错误分类不当会造成新误判。
- 回滚方案：功能开关恢复旧轮询仅用于只读诊断，不自动重放历史 Start；代码回滚需保持 schema 兼容。
- 验收标准：无 panic、页外任务可恢复、无效 scope 不假空、无第二次 Provider Start、错误均 typed。

### R1-2 — 不可变会话 + typed Outcome Provider Adapter

- 开发目标：关闭 CS-03、DR-04—08/13，形成可支撑普通文件写的确定性 Provider 基座。
- 影响模块：115driver client/login/upload/offline/op/recycle；CloudSite provider contracts、bootstrap、secret storage、Job fence。
- 数据库变更：新增 `provider_operations/provider_operation_items`（设计）、`secret_references` 或复用受保护 secret ref；source 迁移为 digest+opaque ref 需 append-only 方案。
- API 变更：引入 `APPLIED/NOT_APPLIED/UNKNOWN/PARTIAL`、八类 typed error、BatchResult、OperationEnvelope；不暴露秘密。
- 兼容性：保留旧 DownloaderProvider 适配层，逐方法迁移；历史 source 读取支持 legacy，不能盲目删审计记录。
- 测试要求：多账号 race、A/B/A Cookie、429/Retry-After、提交后断连、根/空删除零 HTTP、上传 resume、source 全存储面扫描。
- 风险：会话和 source 存储是架构边界变更，需所有者批准 supersede 旧条款；UNKNOWN 对账可能无法唯一识别 Copy。
- 回滚方案：新 Provider 能力 default-off，保留只读和现有 acquisition；数据库只追加，关闭新写入口，旧秘密迁移按恢复预案。
- 验收标准：无共享账号状态、无文本错误分类、写 UNKNOWN 不盲重试、批量逐项可恢复、秘密不明文入新表/日志。

### R1-3 — IndexCore 身份冲突与诊断闭合

- 开发目标：关闭 IC-01—04，确保冲突不授权删除、R8 可解释收敛、重复稳定 ID 失败关闭。
- 影响模块：`kernel/reconcile`、PostgreSQL `tx/decision/diagnostic`、相关测试。
- 数据库变更：为 conflicts/counts 持久化做兼容列/JSONB 结构扩展；如需 replacement lineage，先 ADR，不直接大改 Canonical。
- API 变更：诊断输出有界冲突计数/类别；Q5/Q8 语义不自行选 winner；必要时新增 conflict receipt。
- 兼容性：保留历史 Journal/Canonical，不回写伪造身份；旧 reason-only 诊断可读。
- 测试要求：重复稳定 ID、R8 A→PARTIAL(B)→mirror/COMPLETE、UNRESOLVED 连续 COMPLETE、crash/replay、race。
- 风险：简单冻结所有冲突会造成无关资源无法删除，简单选 newest 又会误删。
- 回滚方案：默认禁用有争议的新 absence 路径，保留 additive-only；不以 SQL 删除新状态来回滚。
- 验收标准：对抗矩阵无错误 REMOVED、冲突持久化可诊断、合法删除仍按 coverage 收敛。

### R1-4 — Journal 合同与所有消费者统一

- 开发目标：关闭 IC-05、CS-04，统一 gap 处理、版本化 payload 和回读兼容。
- 影响模块：IndexCore `decision.go/query/httpapi`；CloudSite `projector/libraryview/integrations/indexcore`。
- 数据库变更：新事件写 `payload_version` 和版本化 old/new path/attributes；旧事件不改写；可增设 projection recovery 状态。
- API 变更：Q8 增加版本化字段，保留 reason-only legacy；gap/过期返回显式错误，不静默跳过。
- 兼容性：消费者先支持 legacy+v2，再让生产者写 v2；分阶段 rollout，避免新旧互读失败。
- 测试要求：ADD/UPDATE/MOVE+UPDATE/RENAME/REMOVE 重放、gap 不推进、legacy 混合、跨 Root cursor vector。
- 风险：事件 payload 增大和双版本消费错误；不能把 payload 变成无界大对象。
- 回滚方案：生产者停止写 v2 回到兼容通知模式，消费者用 Q6 基线重建；不删除 Journal。
- 验收标准：任何 gap 都停止游标，Q8+必要回读可重建与 Q6 一致，旧事件可消费。

### R1-5 — 生产恢复、权限与版本证据链

- 开发目标：关闭 DEP-03/05/07 的可控部分，建立源码→镜像→二进制→schema→备份恢复回执。
- 影响模块：Docker Compose、PostgreSQL 备份、Portainer/OpenList 权限、只读运维账号和发布 runbook。
- 数据库变更：不直接改业务 schema；新增备份 catalog/恢复元数据可存运维系统。
- API 变更：无业务 API；提供只读 build/version/health 证据时需避免暴露敏感配置。
- 兼容性：非 root OpenList 和备份格式改变需验证旧数据/卷权限；回滚 schema 另行检查。
- 测试要求：`go version -m`、镜像 digest、schema SELECT、隔离 PG restore、服务只读查询、Portainer/socket 权限矩阵。
- 风险：卷 owner/权限改变可导致 OpenList/PG 无法启动；恢复覆盖新数据是重大风险。
- 回滚方案：仅在网络隔离环境先演练；生产回滚保留原配置/卷/镜像，禁止在线盲目恢复旧备份。
- 验收标准：实际 SHA/schema/flags/日志可只读核验，隔离恢复通过，管理面和容器权限最小化。

### R1-6 — IndexCore 配置 fence、故障边界与容量门槛

- 开发目标：修复 IC-06 的配置 fence；对 IC-08 的高影响 fail-closed 设计按 Q15 评审局部故障隔离，不预设吞错；对 IC-07 保持 NO-GO 并建立真实容量验收，不承诺未验证优化。
- 影响模块：full scan、incremental exec/runtime、Watch/Work、F1/F2/F3 与 Draft F4 评审。
- 数据库变更：full-scan adapter digest/revision provenance；容量所需的 budget/lifecycle 另 ADR，Draft schema6—8 不视为已接受。
- API 变更：STALE_INPUT/typed item failure/debt 指标；Q9 增强 freshness debt，不把 ready 当 fresh。
- 兼容性：保留 serial writer 和 claimed/pending；F1/F2/新读池 default-off，独立 flag。
- 测试要求：adapter ABA、item/system 故障矩阵、真实 20K+ traversal、多 Root churn、无 Hint 晚到文件、长稳和端到端 Q5/Q8。
- 风险：吞掉 INTERNAL 会破坏 fail-closed；加并发会放大 2 vCPU 主机 I/O 和 Provider 限流。
- 回滚方案：关闭新 runtime/读池/F4，保留只读 Query 和既有安全观察；旧 schema 回滚须按 migration 兼容执行。
- 验收标准：配置变化拒绝旧 Snapshot，系统错误仍 fatal；容量指标达到事先批准的 debt/freshness 门限，否则维持 NO-GO。

### R1-7 — 每用户累计 1500 次云下载上限

- 开发目标：按项目所有者 2026-11-16 指示，将每个 CloudSite 用户 UUID 的累计新下载操作上限设为 1500 次。
- 边界：UserSpace 不设置个人存储容量配额；1500 仅是云下载提交次数上限，不是容量、文件大小或并发限制。
- 影响模块：CloudSite product admission、PostgreSQL operation ledger、usage DTO 和 downloads UI。
- 数据库变更：优先复用现有 operation ledger/quota lock；不为统一 1500 上限单独新增 schema 或改写历史 migration。若实施证明必须变更，另行 ADR。
- API 变更：`operation_limit=1500`、`used`、`remaining`；`unlimited=false`；并发不设每用户上限，DTO 必须明确表达 concurrency unlimited。
- 兼容性：当前 main 仍不限次数；新策略只在独立 release 生效。先查 exact replay，再查额度；原操作重放不重复计数。
- 测试要求：1499/1500/1501 边界、并发新提交、同 key replay/conflict、commit UNKNOWN、历史已超限用户、RECOVERY_REQUIRED 不占并发槽、超限零 Provider 请求。
- 风险：把 1500 错当并发/文件数/容量，或把失败重试重复计数；错误计数会锁死用户或绕过上限。
- 回滚方案：关闭新额度策略需经所有者明确批准；不得删除历史 operation ledger。该策略无 schema migration 时保持数据库兼容。
- 验收标准：每用户最多 1500 个不同的已提交操作意图；幂等重放不增加计数；无 per-user concurrency cap；全局背压独立有效。

## 4. P2：通用文件管理与产品能力

**入口前置决策（Q00/Q01）**：IndexCore 当前项目上下文将 CloudSite 1.0 标为 Legacy/Frozen Product，并把正式 successor product/Gate 5 标为未授权。因此，R2 系列施工前，所有者/Architect 必须明确选择“扩展 CloudSite 1.0”还是“在新 successor product 中实现”，并书面 supersede 受影响的旧边界；本路线图只提供两种选择下的需求与风险研究，不构成开工授权。R2-3 还必须先获得 Q01 的 Copy 基数决策。PR190/191 与 R1/R2 存在重叠，开工前需评审其未合并内容，避免重复实现。

### R2-1 — 通用 FileOperation Job Engine（延后决策）

- 开发目标：仅当多个已验收的物理写操作确实重复出现相同状态/恢复需求时，再评审是否在现有 acquisition fence 上抽象统一命令状态机；不作为 UserSpace 首期前置条件。
- 影响模块：仅在单独 ADR 批准后涉及 CloudSite jobs/acquisition/providers/contracts/store/postgres；IndexCore Hint outbox。
- 数据库变更：append-only `file_operations/file_operation_items/indexcore_hint_outbox`，阶段 deadline、request_digest、expected_version、item outcome。
- API 变更：`operations:preview/create/cancel/confirm` 和 item/status；通用 Cancel 拒绝不能直接取消的 acquisition。
- 兼容性：现有 ACQUISITION 保持原状态机或由适配器映射，不迁移/重放历史未知任务。
- 测试要求：永久 pending、429/503、配对取消、重启、多 worker、部分成功、Canonical TTL、上传读回。
- 风险：过度泛化会破坏已验证 acquisition；必须先从窄接口和两个操作证明。
- 回滚方案：如获批准，采用新 job type/default-off，旧 acquisition 独立运行；停止新命令入队，保留恢复读取。
- 验收标准：每副作用有 fence、UNKNOWN 可恢复、无无限 pending、取消不分裂、Hint/Canonical 可追踪。

### R2-2 — 文件管理中心与下载/分享闭环

- 开发目标：交付只读树→普通写→高风险分阶段 UI，处理 CS-09、DR-16/17。
- 影响模块：CloudSite producthttp/fileaccess/fileshare、新 storage handlers、前端、Provider Adapter。
- 数据库变更：复用 R2-1；新增下载/分享审计或回收站对账投影，避免存签名 URL。
- API 变更：v2 storage entries/operations/recycle；DownloadLease；分享能力按 Provider capability 明示。
- 兼容性：现有内容/分享路由保留，302 撤销限制有明确文案；新文件写 default-off。
- 测试要求：大目录分页、Q5 ambiguous、Range/错误页/OOM、回收站 rid、根保护、批量预览、权限撤销。
- 风险：真实 Provider 递归/分享语义未知；高风险 UI 不能先于后端保护上线。
- 回滚方案：按 capability 关闭 mutation，保留只读树和现有下载/分享；数据库只追加。
- 验收标准：用户可完成授权范围内管理并看到逐项回执；危险输入零请求；UNKNOWN/等待可见。

### R2-3 — 业务资源模型、分类与元数据

- 开发目标：实现 `06-RESOURCE-MANAGEMENT.md`，保持物理/业务分类解耦并补 CS-11。
- 影响模块：CloudSite catalog/resourceview/adminops、搜索投影、migration。
- 数据库变更：append-only categories/tags/aliases/summaries/collections/topics 关联表；历史数据盘点后补 CHECK/唯一约束。
- API 变更：Asset/Release/Variant 读取和管理、分类/标签/别名、expected_version/CAS。
- 兼容性：以 Q01 批准为入口条件；若批准正式冻结 Copy 一对一，则保持现有 UNRESOLVED_COPY 和 overlay 可读并逐步映射，若否决则先另交基数迁移/兼容 ADR，不按本方案施工。
- 测试要求：多分类不移动文件、循环分类拒绝、版本冲突、移动后收藏/标签不变、历史 migration。
- 风险：自动规则误绑定和全站/私有分类权限未决策。
- 回滚方案：新元数据 default 不发布，旧文件库继续；不物理删除新表作常规回滚。
- 验收标准：三种资源路径独立可用，一个 Asset 多检索入口无物理复制，所有变更有审计。

### R2-4 — 搜索与资源发现

- 开发目标：实现 `07-SEARCH-AND-DISCOVERY.md`，解决只记文件名的问题。
- 影响模块：CloudSite search projection/producthttp、libraryview、合集/专题/个人行为模块、前端。
- 数据库变更：SearchDocument 投影、FTS/trigram 索引、favorites/recent/聚合表；全部可重建派生数据与私有数据分表。
- API 变更：v2 search/categories/collections/topics/favorites/recent；游标和 revision。
- 兼容性：现有 `/api/v1/library` 保留，搜索投影失败回退旧库；不引入 AI 强制依赖。
- 测试要求：别名/筛选/合集三路径、结果去重、权限计数、ETag、投影失败回退、50K 授权性能验证。
- 风险：中文分词、权限泄漏和热门防刷；过早引入搜索集群增加运维。
- 回滚方案：停止新投影发布，保留上一完整 revision 或旧 library；私有行为功能可独立关闭。
- 验收标准：知道名称/用途/完全探索均能找到资源，私有结果不泄漏，查询无 N+1。

### R2-5 — 查询、存储保留和背压

- 开发目标：关闭 CS-08、IC-09—12，建立长期运行成本边界。
- 影响模块：CloudSite admission/library；IndexCore prior/query/collector/Snapshot/Watch retention。
- 数据库变更：最新 observation 索引、批量 SQL、retention 元数据；保留策略不能删除法定 Journal/必要身份历史。
- API 变更：最大 limit、非法参数 400、队列/债务/429 背压 DTO。
- 兼容性：旧合法分页可用；非法参数从默认回退变 400 是行为修正，需 release note。
- 测试要求：EXPLAIN/WAL/锁、重复页/大 body、长期 NOOP、崩溃 DRAFT、入队水位和公平性。
- 风险：索引构建/清理会压垮小主机，错误 retention 破坏恢复证据。
- 回滚方案：在线低峰滚动索引/功能 flag；清理先 dry-run 和备份，可停止但不盲恢复已清理数据。
- 验收标准：查询/内存/锁有界，非法请求明确，长期重复扫描成本受控，背压不重复副作用。

### R2-6 — 生产可维护性与安全运行

- 开发目标：关闭 IC-13/14、DEP-04/06/08，形成预算、单写、镜像、监控和保留闭环。
- 影响模块：IndexCore adapter config/writer lock、Compose、监控/告警/备份/镜像发布。
- 数据库变更：严格配置校验无需新表；writer fencing 若改协议需 ADR/migration。
- API 变更：诊断和告警增加 debt/backup/queue/writer 状态，不暴露秘密。
- 兼容性：拒绝历史非法 secret 配置前提供只读扫描和迁移；latest pin digest 保持应用版本兼容。
- 测试要求：非法配置、writer 锁连接失效、容器 stats、备份失败告警、磁盘满、异机重建。
- 风险：单写 HA 改造复杂，资源调整可能改变现有性能基线。
- 回滚方案：保留原镜像 digest/配置；writer 新协议 default-off，监控变更无副作用。
- 验收标准：镜像可复现、非法秘密配置拒绝、单写边界明确、告警与恢复演练有证据。

## 5. P3：一致性与维护性

### R3-1 — 小缺陷、指标和发布一致性

- 开发目标：关闭 CS-10/12、IC-15/16、DR-14/18。
- 影响模块：panta-db、README/release checks、IndexCore diagnostics/migrations、115driver 小函数。
- 数据库变更：append-only trigger/最小权限评审；不修改历史 migration checksum。
- API 变更：指标分类修正，发布文档同步；`errors.Is` 兼容 sentinel。
- 兼容性：保留 deprecated 拼写和旧 sentinel 包装；错误输出改封闭类别。
- 测试要求：WithAsc query、Cookie fuzz、mkdir 包装错误、指标矩阵、append-only 权限、README/schema 发布静态检查。
- 风险：新增 DB trigger 可能阻碍合法运维迁移。
- 回滚方案：代码/文档小步回滚，trigger 通过独立受控 migration 管理。
- 验收标准：版本来源一致、错误分类一致、小缺陷防回归、历史 migration 字节不变。

## 6. 明确不在近期核心路线

- AI Agent 自主移动、删除、重命名或分类真实文件。
- 第二 Canonical writer、公网 Hint、浏览器直写 PostgreSQL。
- 未经 ADR 的分布式 HA、多 writer、搜索集群或缓存系统。
- 未经真实授权部署的 20K/吞吐/长稳“完成”声明。

## 7. 阶段放行门槛

```text
安全止血回执
  → 核心对抗测试与 pinned 协议测试
  → Provider/Job 通用能力 default-off
  → 隔离环境普通写与高风险演练
  → 业务分类/搜索投影
  → 生产只读版本/备份/权限证据闭合
  → 授权真实容量与长稳验收
```

任一阶段出现身份错误、凭证泄露、盲重试或不可恢复备份，应停止后续能力扩张，而不是以功能数量抵消基础风险。