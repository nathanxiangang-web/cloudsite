# 02 CloudSite 深度审计

基线：`1b5b560bb8006ea69f05c61926040c3d9ad7679f`。已阅读实际 Go 实现、21 个 migration、前端和测试结构；未执行数据库迁移、真实 Provider 请求或生产写操作。

## 1. 技术架构评价

- 后端为 Go 1.27.1、pgx/v5，前端为嵌入 Go 的原生 HTML/CSS/JavaScript，不是 Next.js/TypeScript。
- 主要边界为 `producthttp → product/acquisition/admin* → store/postgres 或 integrations/*`。
- 配置和密钥通过受保护 bootstrap 加载；Worker 不从 CLI 接收明文 Cookie。
- 迁移器具有嵌入 SQL、SHA-256、历史前缀、缺口/乱序/未来 schema 拒绝、事务与 advisory lock：`migrations/migrations.go:15-66`、`internal/store/postgres/migrator.go:17-125,133-223`。
- 静态测试规模：339 个 Go 文件、182 个 Go `_test.go`、15 个顶层 Node 测试文件、21 个 migration。该计数不是覆盖率。
- 审计 SHA 的 GitHub CI 与 release workflow 均 success；本审计未重新运行完整测试。

## 2. 领域模型与产品差距

| 模型 | 当前事实 | 评价 |
|---|---|---|
| Asset | `canonical_name/category/status` | 逻辑资源入口合理，但缺少公开 CRUD、别名和丰富元数据 |
| Release | 版本原始/规范化值、scheme、channel | 能表达软件版本；Asset 内版本唯一性约束不足 |
| Variant | `variant_key + attributes JSONB` | 适合平台/架构/封装差异；需确定性 schema 校验 |
| Copy | 可空 Variant；一个 root+resource 唯一 | 物理副本与逻辑资源分离正确；当前不是一对多 |
| Provider/StorageConnection | `credential_ref` 与 Binding 分离 | 观察连接与受权连接可区分，方向正确 |
| ResourceView | 区分 presence、availability、分类状态 | 应保留并作为统一 API 语义 |
| Copy presentation | alias/category/hidden + CAS 审计 | 只是 Copy overlay，未进入消费者完整分类/检索体系 |
| Library snapshot | 版本化派生快照、分页、条件读 | 改善整库重读；不是业务搜索/分类替代品 |

未实现或不完整：业务标签、别名/同义词、厂商、结构化平台/架构筛选、资源简介、合集、专题、收藏、最近浏览、热门、统一检索、完整物理文件管理。

## 3. Job Engine 评价

| 维度 | 实际实现 |
|---|---|
| 创建 | operation intent/digest 先落库，Manifest PENDING，Activation 同事务创建并链接 Job |
| 认领 | `FOR UPDATE SKIP LOCKED LIMIT 1`，类型过滤，QUEUED/到期 RETRY_WAIT |
| 多进程 | 随机 owner、行锁、claim generation、Provider fence advisory lock |
| 并发 | 当前单 Worker、每 claim 一个 persisted stage，避免长事务跨 HTTP |
| 超时 | HTTP 有单次超时，Job 有 lease；缺少全任务/阶段总 TTL |
| 重试 | Provider poll 有 DB-time 指数退避；claim 与 failure 计数分离 |
| 幂等 | operation key、start fence、精确 outcome replay、READY result_copy_id |
| 恢复 | ACQUISITION 过期配对原子转 RECOVERY_REQUIRED，普通 expired sweep 排除它 |
| 取消 | 通用 Job.Cancel 与 acquisition 配对状态存在隔离缺口；运行中取消未闭环 |
| 预算 | 提交次数/账号并发业务上限已移除；缺少系统水位和全任务等待预算 |

关键证据：`internal/store/postgres/job_repository.go:116-152`、`acquisition_activation_repository.go:32-98`、`acquisition_provider_task_repository.go:67-131`、`acquisition_lease_recovery_repository.go:20-159`。

## 4. 问题清单

### CS-01 — P1 — 固定版 SDK 多结果可先于本地校验 panic

- 类型：FACT（崩溃代码路径）；真实服务是否返回多结果 UNKNOWN。
- 项目/位置：CloudSite `internal/providers/115/adapter.go:141-159` `StartDownload`；115driver v1.3.5 `pkg/driver/offline.go:148-151`。
- 数据/API：`acquisition_provider_tasks`；115 `add_task_urls`；无直接 SQL 缺陷。
- 触发：一个 URI 的 `taskInfos.Result` 超过一个元素，SDK 向长度 1 的 `hashes` 写第二项。
- 影响：INFERENCE——goroutine panic 可终止产品进程；Manifest 已有 START_RESERVED 时结果变为不确定。
- 建议：升级不能替代契约检查；在真实 SDK 边界防 panic 并转 typed protocol/UNKNOWN，不自动重放 StartDownload。
- 难度：中。
- 测试：以 pinned driver + fake HTTP 返回两个结果，断言无 panic、无第二次 Start、进入恢复而非成功。

### CS-02 — P1 — 三页轮询把页外任务推向人工恢复

- 类型：FACT（配置和状态链）；线上页大小/排序 UNKNOWN。
- 位置：`internal/runtime/runtime.go:293-298` MaxPages=3；`internal/providers/115/adapter.go:215-256` `findTask`；`poll.go:39-60`。
- 数据/API：`jobs`、`acquisition_manifests`；离线任务列表 API。
- 触发：目标在第四页之后，或新建任务暂未出现；`ErrPaginationUnbounded`/`ErrTaskNotFound` 经 `PollUnknown` 停止。
- 影响：INFERENCE——ACTIVE 任务无法进入 Hint/Canonical/READY；持续提交和历史任务增加时更明显。
- 建议：区分 `TASK_NOT_FOUND_CONFIRMED`、`PAGINATION_LIMIT_EXCEEDED`、`TRANSIENT_PROVIDER`；验证精确状态 API，否则做有预算的游标对账。
- 难度：中到大。
- 测试：四页、页外目标、刚创建不可见、确认删除分别跑完整 Worker，禁止盲重试和误永久失败。

### CS-03 — P1 — 原始/执行 source 明文入库，可能含签名秘密

- 类型：FACT（表与写入）；是否已有真实泄露 UNKNOWN。
- 位置：`migrations/0015_product_operations.sql:7-18`、`0005_acquisition_manifests.sql:4-9`；`internal/acquisition/source.go:31-41`。
- 数据/API：`product_operations.original_source/execution_source`、`acquisition_manifests.source_ref`；`POST /api/v1/cloud-downloads`。
- 触发：用户提交带 query signature/token 或私密名称的 URL/magnet；HTTP URL 仅明确拒绝 userinfo。
- 影响：INFERENCE——秘密进入两张表和备份，与现行 `AGENTS.md:653-660` 的硬边界表述冲突。
- 建议：产品表保存 digest + opaque secret ref，执行源由 SecretResolver 提供；确需可逆存储须单独批准加密和保留策略，并明确 supersede 旧条款。
- 难度：大。
- 测试：提交含 `signature=SECRET` 的 source，扫描所有表、审计、DTO、日志和备份，验证 replay/重启/删除策略。

### CS-04 — P2 — 旧 Journal Projector 接受 gap 并推进游标

- 类型：FACT。
- 位置：`internal/projector/projector.go:150-183` `mapBatch`；`internal/store/postgres/projection_store.go:109-119`。
- 数据/API：`indexcore_projection_cursors`、`copies`；IndexCore Q8。
- 触发：cursor=42，事件为 44 或 43、45；代码只检查递增，不检查 `previous+1`。
- 影响：INFERENCE——缺失 add/remove 被跳过且游标越过，Copy 可长期错误 PRESENT 或缺失。
- 建议：非 baseline 强制连续；gap 拒绝 ApplyBatch，进入显式重建/恢复；与 `libraryview/engine.go:318-343` 共用规则。
- 难度：小。
- 测试：gap、重复、倒序、缺 removal 时断言零 Copy 变化、零 cursor 推进和 typed journal gap。

### CS-05 — P2 — acquisition 缺少全任务与阶段 TTL

- 类型：FACT。
- 位置：`internal/acquisition/provider_outcome.go:72-79`、`canonical.go:275-306`；Manifest/Job migrations。
- 数据/API：Provider status、Q5、Q8；`jobs/acquisition_manifests` 无总 deadline。
- 触发：Provider 永远 RUNNING、Q5 永无匹配、Copy 永不投影。
- 影响：INFERENCE——无限轮询和行/外部请求积累；正常 pending 不消耗失败预算。
- 建议：分别增加 provider、visibility、canonical deadline 和等待预算，超时进 recovery，保留证据且不重 Start。
- 难度：中。
- 测试：可控 DB 时钟跨越 TTL，断言停止外部请求、状态可恢复、无重复副作用。

### CS-06 — P2 — Hint/Q5/Projector 暂时错误未安全分类重试

- 类型：FACT。
- 位置：`internal/acquisition/stage_dispatch.go:247-282`、`canonical.go:266-299`、`internal/integrations/indexcore/hint_client.go:344-366`。
- 数据/API：Hint、Q5、Q8；Job lease/recovery。
- 触发：429、503、短暂 transport reset 或 Q8 暂不可用。
- 影响：INFERENCE——本可重试阶段等待 lease 过期后转 RECOVERY_REQUIRED；已有 `ErrHintBusy/RetryAfter` 未用于调度。
- 建议：引入 `AUTH_OR_PERMISSION/THROTTLED/TRANSIENT_PROVIDER/...` typed error，DB-time fence 提交安全 RETRY_WAIT，尊重 Retry-After。
- 难度：中。
- 测试：第一次 429/reset、第二次成功，验证同 Manifest/Job 安全重试、无新 Provider start。

### CS-07 — P2 — 通用 Cancel 可制造 ACQUISITION 配对状态分裂

- 类型：FACT（接口行为）；当前生产 HTTP 未暴露该通用入口。
- 位置：`internal/store/postgres/job_repository.go:221-242` `Cancel`；`admin_pending_cancellation.go:97-136`。
- 数据/API：`jobs` 单表更新；Manifest 不同步；运行中 115 取消未接线。
- 触发：未来内部调用者直接取消 QUEUED/RETRY_WAIT acquisition Job。
- 影响：INFERENCE——Manifest ACTIVE 与 Job CANCELED 分裂；Worker 停止也不等于远端任务停止。
- 建议：通用 Cancel 立即拒绝 ACQUISITION；另做有 owner/reference/审计/UNKNOWN 对账的配对取消命令。
- 难度：中。
- 测试：直接调用通用 Cancel 应拒绝且双边不变；覆盖取消成功、响应丢失、重启和重复取消。

### CS-08 — P2 — 无限提交缺少系统容量背压

- 类型：FACT（政策与代码）；实际滥用程度 UNKNOWN。
- 位置：`internal/product/admission.go:26-32`、`internal/producthttp/cloud_download.go:52-89`、`registration.go:16-58`。
- 数据/API：公开注册、`POST /api/v1/cloud-downloads`；Manifest/Job 表。
- 触发：开放注册环境中多账号快速入队；串行 Worker 不限制入队速率。
- 影响：INFERENCE——DB 膨胀、长队列、115 限流和存储压力。
- 建议：保留业务“无限”，增加 IP/账号速率、全局水位、磁盘预算、Provider 429 背压与公平调度；不要偷恢复旧业务上限。
- 难度：中。
- 测试：突发至水位，验证新命令 429、幂等 replay 可用、历史任务不重 Start、队列公平。

### CS-09 — P2 — 撤销产品授权不能回收已签发上游 302 URL

- 类型：FACT（边界）；上游 URL 实际 TTL UNKNOWN。
- 位置：`internal/fileaccess/delivery.go:60-95`、`internal/producthttp/content.go:166-173`、`admin_share_lifecycle.go:14-40`。
- 数据/API：分享撤销、账号禁用/删除；CDN 302。
- 触发：URL 已签发后再撤销或禁用。
- 影响：INFERENCE——持有 URL 者可能在上游过期前继续访问。
- 建议：高风险内容使用短 TTL 或产品代理；UI 明确两个失效时间线；不得假定上游支持 revoke。
- 难度：中到大。
- 测试：签发→撤销/删号→旧 URL，分别验证产品拒绝与 CDN 预期行为。

### CS-10 — P3 — `panta-db` 错误输出脱敏弱于产品服务

- 类型：FACT；必现密码泄露未证明。
- 位置：`cmd/panta-db/main.go:38-40,58-63`、`internal/store/postgres/database.go:19-26`。
- 数据/API：DB DSN/迁移错误；无 HTTP。
- 触发：异常 DSN、Ping 或 migration 失败。
- 影响：INFERENCE——特殊底层错误可能进入运维日志。
- 建议：仅输出封闭错误类别，详细诊断受保护并结构化脱敏。
- 难度：小。
- 测试：含 SECRET 和异常字符 DSN，扫描 stderr 不出现秘密或完整 DSN。

### CS-11 — P3 — Catalog 数据库不变量弱于领域状态机

- 类型：FACT。
- 位置：`migrations/0001_catalog.sql:1-47`；`internal/store/postgres/catalog_repository.go:145-179,246-253,310-325`。
- 数据/API：`assets/releases/variants/copies` 状态字段无完整 CHECK；Asset/version 唯一性不足。
- 触发：受信任运维写入或未来新调用者插入非法状态/重复逻辑身份。
- 影响：INFERENCE——读取 503、重复 Catalog 语义。
- 建议：先盘点历史数据，再补领域验证、CHECK 和明确唯一约束。
- 难度：中。
- 测试：非法 status/availability、重复 Asset/version 被拒绝，旧合法数据可升级。

### CS-12 — P3 — 发布文档版本漂移

- 类型：FACT。
- 位置：`README.md:3,14` 写 1.0.2/schema20；`VERSION:1` 为 1.0.3；`migrator.go:19-21` 为 21。
- 数据/API：schema 兼容与回滚，无运行时 API。
- 触发：运维按 README 判断二进制/schema 兼容。
- 影响：错误升级或回滚决策。
- 建议：同步文档，增加 VERSION/README/migration head/runtime schema 发布一致性检查。
- 难度：小。
- 测试：静态发布测试比较四个版本来源。

## 5. 安全与前端正面证据

- Session 为 256-bit 随机值，数据库仅 SHA-256 digest；Cookie HttpOnly、HTTPS Secure、SameSite=Strict，Protect 每次重查用户状态：`internal/auth/auth.go:161-245`。
- Host/Origin/Sec-Fetch-Site、严格 JSON、CSP、frame deny、no-referrer、nosniff 均有实现：`internal/producthttp/workspace.go:247-280`。
- 已审 SQL 使用 pgx 参数绑定；路径拒绝反斜杠、NUL、`.`、`..`，结果名只允许直接子段。
- OpenList 内容客户端有 redirect/dial IP 防护：`internal/integrations/openlist/content/client.go:43-117`。
- 首页/列表使用 `textContent`，未发现 `innerHTML/eval/document.write` 的对应危险用法。
- 新版 libraryview 有 50 行服务端分页、分类/搜索、revision、stale/ready 和私有 ETag；生产静态资源哈希与本地一致。

## 6. 测试缺口

1. pinned SDK 多结果 panic 没有经过真实 SDK 边界的测试。
2. 生产 MaxPages=3 页外任务没有完整 Worker 恢复测试。
3. 旧 Projector 没有连续递增中的 gap 测试。
4. 永久 pending、Hint/Q5 暂时错误、通用 Cancel 配对隔离缺少防回归测试。
5. 带签名 URL 入库及历史迁移策略缺少安全测试。
6. 没有 20K、长稳、多副本 Worker + 真实 115 压测；本阶段也不应补做生产压测。