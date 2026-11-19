# 05 115 网盘文件管理体系设计

本文是目标设计，不是当前功能声明。实施必须沿用 CloudSite Job/Provider fence 和 IndexCore Canonical 边界，逐项获得授权；AI 不参与自主物理操作。

## 1. 产品目标与原则

目标是让用户在 CloudSite 内完成“找到—理解—管理—验证—追溯”115 文件，而不是在多个官方客户端之间切换。

原则：

1. **先只读、后写入、最后高风险**：先统一树/详情/搜索/任务，再逐项开放普通写入和删除。
2. **Provider ID 是 Provider 对象身份，路径只是某时刻位置**。
3. **物理目录与业务分类解耦**，打标签不移动/复制文件。
4. **每次写操作都有持久化意图、预检、回执、结果核验、审计和恢复路径**。
5. **IndexCore 负责最终物理真相**；Provider read-after-write 只证明操作回执，不直接改 Canonical。
6. **所有写能力都有预算、取消/UNKNOWN 语义和部分成功模型**。

## 2. 文件管理中心信息架构

### 2.1 一级页面

| 页面 | 核心内容 | 初始数据来源 |
|---|---|---|
| 全局目录树 | Root、挂载点、懒加载子目录、刷新状态、ambiguous/conflict | 当前 IndexCore Q4 + Q9 Root Status；observed_at/freshness debt 等为未来扩展契约，不是当前 Q9 已有字段 |
| 文件列表 | 名称、类型、大小、mtime、业务分类、物理位置、选择/批量 | Q4/Q6 + CloudSite Catalog |
| 文件详情 | Provider/IndexCore/Copy 身份、元数据、版本、副本、操作历史 | Q3/Q5 + CloudSite |
| 全局搜索 | 文件名/别名/简介/厂商/标签，结构化筛选 | CloudSite 搜索投影，物理精确解析回退 Q5 |
| 上传/离线下载 | 目标、进度、分片恢复、任务回执 | CloudSite Job + Provider |
| 下载中心 | 短 lease、Range/恢复、内容校验、撤销边界 | Provider/OpenList 内容车道 |
| 回收站 | rid、原位置、删除时间、恢复/永久清理风险 | Provider 回收站 API + 对账 |
| 操作历史 | 操作者、意图、影响预览、回执、UNKNOWN、Canonical 确认 | CloudSite 审计表 |
| 任务中心 | 排队/运行/等待观察/恢复/部分成功，按阶段筛选 | CloudSite Job Engine |

### 2.2 全局树的正确语义

- 树只表示物理观察结果，不表示业务分类。
- 节点同时保存 `root_id + indexcore_resource_id + parent_resource_id + generation`，路径用于显示和 Q5 精确解析。
- Q5 `ambiguous=true` 时展示冲突，不允许前端自行选择一项。
- 页面显示 `observed_at`、Root generation、projection checkpoint 和 stale/freshness 状态；不能拿 HTTP 200 当 fresh。
- 大目录采用服务端分页和懒加载；不在浏览器构造全量树。
- Provider 直接列表只用于命令预检/UNKNOWN 对账，不另建一套与 IndexCore 竞争的物理库存。

## 3. 能力分阶段开放

| 阶段 | 能力 | 上线门槛 |
|---|---|---|
| A 只读基线 | 树、列表、详情、搜索、下载、操作历史读取 | generation/cursor、权限、分页、freshness 可观测 |
| B 低爆炸半径写入 | Mkdir、单文件 Rename、Offline Add | 幂等预检、UNKNOWN 对账、审计、Hint outbox |
| C 普通管理 | Move、Copy、Upload、Recycle Restore | 目标冲突策略、部分成功、上传 checkpoint、Canonical 等待 |
| D 高风险 | 入回收站、目录递归、批量操作、永久清理 | 影响预览、二次确认、爆炸半径上限、专门恢复演练 |
| E 分享 | 115 原生能力或 CloudSite 产品分享的明确选择 | 创建/撤销/权限/有效期/访问日志真实合同验证 |

任何阶段都不能因为 SDK 存在同名函数就跳过真实协议合同和故障注入。

## 4. 统一 FileOperation 状态机

```text
INTENT_RECORDED
  → AUTHORIZED
  → PRECHECKED（目标/源/预算/影响快照）
  → DISPATCH_RESERVED（side-effect fence）
  → PROVIDER_APPLIED | PROVIDER_NOT_APPLIED | RESULT_UNKNOWN | PARTIAL
  → VERIFYING_PROVIDER（必要时对账）
  → PROVIDER_CONFIRMED
  → HINT_PENDING → HINT_ACCEPTED
  → AWAITING_CANONICAL
  → SUCCEEDED

任意可安全重试错误 → RETRY_WAIT
不可安全自动恢复 → RECOVERY_REQUIRED
明确业务拒绝/校验失败 → FAILED（不得假称远端未执行，除非有证据）
```

约束：

- 数据库事务只覆盖本地状态，不跨 Provider HTTP。
- `DISPATCH_RESERVED` 后同一 operation 不得被另一 worker 再发写请求。
- UNKNOWN 必须保存 request_digest、目标身份、发送时间、Provider request/correlation ID（若有）和对账游标。
- 202 Hint 不等于完成；只有规定 Query/Canonical 条件满足才 SUCCEEDED。
- Provider 成功但 Canonical 永不出现，应进入可见恢复，不得无限 pending。
- 批量操作保存每项 `succeeded/failed/notAttempted/verified/result_id`；不能只存一个总状态。

## 5. Provider Adapter 设计

### 5.1 会话与预算

- 每账号一个不可变 Session，`account_id + credential_ref + session_generation` 绑定；重认证原子替换，不叠加 Cookie。
- 所有方法接收 `context.Context`，统一 timeout、账号速率、并发、每日/每分钟请求预算和 429 Retry-After。
- 默认禁止 Resty/底层 HTTP 自动重试；重试只由 Adapter 根据 Outcome 拥有。
- typed error 固定为 `AUTH_OR_PERMISSION`、`THROTTLED`、`TRANSIENT_PROVIDER`、`SCOPE_TOO_LARGE`、`INVALID_SCOPE`、`CONFIG_INVALID`、`ROOT_INACTIVE`、`INTERNAL`。
- 错误分类使用 HTTP status、endpoint 和结构化 provider code；禁止错误文本匹配。

### 5.2 写操作通用输入

```text
OperationEnvelope
  operation_id / idempotency_key / request_digest
  account_id / connection_id / session_generation
  actor_id / risk_level / approval_id
  source_objects[]（Provider ID + prechange name/parent/size/mtime/hash-if-known）
  destination_scope（Provider Folder ID + expected identity）
  expected_preconditions / batch_policy / budget / deadline
```

Provider 返回：

```text
OperationReceipt
  outcome: APPLIED | NOT_APPLIED | UNKNOWN | PARTIAL
  provider_operation_id / provider_task_id（若存在）
  item_results[]：source_id、result_id、outcome、verified、typed_error
  observed_at / verification_evidence / retry_after
```

`verification_evidence` 在实施前必须冻结可序列化 schema，至少包含：`operation_id/request_digest/account_id/session_generation`、逐项 Provider outcome/result_id/verified/typed_error、`root_id/resource_id` 及 read visibility、可得的 `snapshot_id/admission_seq/canonical_generation/last_confirmed_generation/journal_event_seq/committed_at`、CloudSite projection cursor、`observed_at/verified_at/evidence_source` 和无法取证原因。projection cursor 只能与投影结果在同一事务推进。上述证据不得保存 Cookie、签名 URL、原始 magnet 或不必要响应正文。

## 6. 各操作的确定性与核验

| 操作 | 预检 | UNKNOWN 后对账 | 成功核验 |
|---|---|---|---|
| Mkdir | 父目录存在且为目录、同名策略、名称合法 | 按 parentID+name 查询，唯一且类型正确才可关联 | 返回非空 Folder ID，父 ID/名称一致 |
| Rename | 源 ID、原名称、目标冲突策略 | 按源 ID Stat；不得仅按新名称猜 | 源 ID 保持与否以响应+读回为准，名称/父 ID 符合 |
| Move | 源/目标 Folder ID、循环移动、跨 scope 权限 | 按源 ID 和目标目录读回；多项分别对账 | 目标 parent 正确，源位置不再存在或已明确终态 |
| Copy | 目标空间、同名、循环复制、预算 | 目标前后差异+name/size/SHA1；不能唯一识别则保持 UNKNOWN | 新 File ID 与源 ID 不同，目标和内容元数据符合 |
| Upload | 目标、大小、SHA1、重名策略、checkpoint 匹配 | ListUploadedParts/OSS object/115 目标读回；Complete 后不盲重放 | 最终 File ID、parent、name、size、可用 SHA1 一致 |
| Download | 权限、目标本地路径、磁盘、lease | 403 过期最多刷新一次；Range/ETag 续传 | 长度和可用 SHA1 校验后原子 rename |
| Offline Add | URI、目标 Folder、重复任务策略、配额 | 按 URI/hash/saveDir/时间窗查任务；无法唯一关联则恢复 | 每 URI 有明确 info_hash/任务状态，不多结果越界 |
| Delete→Recycle | ID 非根、影响预览、目录递归授权 | 查源和回收站 rid 映射，考虑延迟 | 源不可见且有回收站证据；不将 rid 当 fid |
| Restore | rid、原目标是否占用、恢复目标策略 | 查回收站与目标，考虑延迟 | 目标对象可见，身份映射有证据 |
| Permanent Clean | 明确 rid 集、二次确认、不可恢复提示 | 只能读回确认，不自动重放 | 指定 rid 消失；空集合绝不表示“全部” |
| Share | 权限、范围、有效期、提取码、访问级别 | 按分享 ID/目标列表对账 | 分享 ID、权限、目标和撤销状态一致 |

## 7. 批量与高风险操作

### 7.1 批量协议

1. 固定服务端最大批次，客户端选择一万项不代表一次 Provider 请求。
2. 预检生成逐项计划；预算耗尽后剩余项为 `notAttempted`，不能静默丢弃。
3. 每项独立回执和错误；支持“仅重试可安全重试项”，不整批重放。
4. 原始选择快照与执行期间变化分离；源 mtime/parent/ID 不匹配时该项转冲突。
5. 任务页展示总数、成功、失败、未知、未执行、Canonical 已确认/等待数。

### 7.2 风险等级

| 等级 | 示例 | 必备控制 |
|---|---|---|
| 只读 | List/Search/Stat/Preview/DownloadLease | 认证、对象 scope、速率、下载链接保护、日志脱敏 |
| 普通写入 | Mkdir/Rename/Move/Copy/Upload/Offline/Restore | 预检、幂等、UNKNOWN 对账、审计、Hint/Canonical 核验 |
| 高风险写入 | 删除目录、批量删除、永久清理、连带删除离线文件、大规模移动 | 目标确认、影响预览、二次确认、爆炸半径上限、审计、结果核验和恢复方案 |

高风险预览至少包含：文件/目录数、总大小（已知时）、递归深度、跨 Root/挂载情况、目标清单抽样、完整清单安全导出、不可逆项、当前观察时间和冲突项。预览不能用过期 IndexCore 数据单独授权删除，必须有新鲜 Provider 预检。

## 8. 上传与下载可靠性

### 8.1 上传 checkpoint

保存：账号、目标 Folder ID、文件名、内容 SHA1、大小、chunk planner 版本、bucket/object、UploadID、每片编号/ETag/SHA1、token expiration、callback 状态、创建时间。

恢复前必须重新匹配所有身份；任何不一致都 abort/转人工，不得把旧 UploadID 用于新内容。每片仅对同 UploadID+PartNumber+内容摘要重传。Complete UNKNOWN 后先查 OSS/115 最终状态。

### 8.2 下载 lease

`DownloadLease` 包含 URL、allowlist Header、issued_at、expires_at/expiry_known、size、SHA1、pickcode/file_id。禁止把长期账号 Cookie 发到 CDN。浏览器 302 撤销边界必须可见；高敏感内容优先短 TTL 或受控代理。

## 9. 大规模管理

- **分页**：所有列表/搜索/回收站/任务都有服务端最大页大小和稳定游标。
- **增量扫描**：Hint 只加速，Watch/周期基线负责兜底；不得假定截至当前已审计 API 表面存在 115 native delta。
- **目录缓存**：只缓存可失效的派生视图，key 包含 account/root/generation/scope；不是身份真相。
- **本地索引**：物理索引归 IndexCore；CloudSite 只存业务投影与检索文档。
- **请求预算**：账号级速率/并发/每日额度、全局队列水位、429 背压，区分前景用户操作与后台扫描。
- **优先级**：用户交互 > 有明确副作用的验证 > 业务投影 > 后台全量；同时保留老化防饥饿。
- **重复检测**：内容 hash 只作 duplicate candidate，不自动合并/删除；同内容不同路径是不同物理副本。
- **失败续扫**：保存 scoped cursor、最后成功页、provider offset/response fingerprint，重复页/无进展显式失败。
- **容量验证**：20K+ 真实规模、长稳和真实 Provider 压测放到授权部署后。

## 10. API 建议（目标，不是现有路由）

```text
GET    /api/v2/storage/roots
GET    /api/v2/storage/roots/{root}/entries
GET    /api/v2/storage/entries/{resource_id}
POST   /api/v2/storage/operations:preview
POST   /api/v2/storage/operations
GET    /api/v2/storage/operations/{operation_id}
POST   /api/v2/storage/operations/{operation_id}:cancel
POST   /api/v2/storage/operations/{operation_id}:confirm
GET    /api/v2/storage/operations/{operation_id}/items
GET    /api/v2/storage/recycle-bin
```

所有 mutation 只接受服务端配置 Binding/Root，不能让浏览器任意指定 credential_ref/native scope。Preview 与 Execute 使用同一 request_digest 和过期时间；Confirm 必须绑定预览版本。API 不返回 Cookie、签名 URL 到日志或不必要 DTO。

## 11. 可观测性与验收

指标：`provider_requests_total{op,kind}`、`provider_throttled_total`、`operation_outcome_total`、`unknown_operations`、`canonical_wait_seconds`、`hint_lag_seconds`、`projection_lag`、`freshness_debt_age/slope`、`batch_partial_total`、`queue_depth/oldest_age`。

基础验收不靠函数名：每项能力至少通过协议 fixture、服务端提交后断连、429、部分成功、重复 ID、路径 ABA、进程重启、Hint 失败、Canonical 不可见、权限撤销和审计脱敏测试。真实接口验证只使用专用账号、隔离目录和逐项授权。