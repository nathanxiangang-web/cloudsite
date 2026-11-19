# 私有云下载与 115 原生复制实施合同

状态：开发中，未合并且未部署。

## 1. 基线与授权

- 代码基线：CloudSite main `1b5b560bb8006ea69f05c61926040c3d9ad7679f` 加 PR #190 `15f3c71c4269faa62284de873e843aca209dbd18`。
- PR #191 仅作为 UserSpace 设计输入；PR #192 仅作为审计与风险输入。
- 最新所有者指令恢复正式开发，允许先本地实现，再在公网完全隔离环境验证。
- 当前 main 尚无 UserSpace、私有 v2 API、原生复制状态机或 Schema 22。
- 本任务不引入每用户 GB/字节配额，不改变现有下载次数与并发政策。

## 2. 相同文件的架构决策

采用 **115 原生 Copy 到各用户独立目录**，不采用跨用户共享同一 IndexCore resource/CloudSite Copy。

原因：

1. 每个用户保留独立 native object、Canonical resource、Copy、ACL 和删除生命周期。
2. 同 hash 不同路径在 IndexCore 中本来就是独立资源，无需修改 IndexCore 内核或新增共享授权模型。
3. 复用现有 `Provider 成功 → Hint → Q5/Q8 → Projector → result_copy_id → READY`。
4. donor 的移动、删除或账号生命周期不会直接改变接收者已取得的独立对象。
5. 底层物理块是否去重由 115 决定，CloudSite 不承诺容量节省。

`info_hash`、BTIH、AdmissionDigest 均不等于通用单文件内容 SHA1。hash 只作为复制前后的一致性证据，不作为对象身份、授权或 Canonical 查询依据。

首版只对候选 donor 做自动复制：同一受控 Provider Connection、相同离线来源候选、已有完整结果证据的单文件。不同 URL 碰巧具有相同内容、目录、多文件结果、SHA1 缺失或来源歧义不自动识别为 donor。

## 3. 不可破坏的边界

- 一个运营方 115 账号，不为用户分配 Cookie，不复制 Worker/Root/Binding。
- 所有权只认不可变 `user_id`；login 只用于目录显示名。
- 浏览器不能指定 user_id、native FileID/DirID、ProviderScope、Binding、Root 或 donor。
- 原生 FileID/DirID 不进入公开 DTO、日志、Job payload 或 IndexCore 身份。
- 旧 Manifest、Job、ProviderTask、START_RESERVED 不回填、不自动重放。
- 私有文件默认禁止分享和外部 302；Content/Preview/Range 每次重新授权。
- Copy 与 StartDownload 共用同一副作用决策，任何 UNKNOWN 都禁止盲目 Copy、重试 Start 或删除“多余”文件。
- Provider Copy 的整体 ACK 不等于新对象身份已知，也不等于 READY。

## 4. 开发任务

### T0 基线与合同

- 梳理 PR #190/#191/#192，保留 #190 的 Session 健康门禁。
- 固化本合同、API/Schema 兼容边界和验收矩阵。
- 验收：基线 SHA、任务范围和“不包含”清单均可审查，无文档冒充实现。

### T1 UserSpace 与冻结目标持久化

- 追加 Schema 22，不修改 0001—0021。
- `cloud_user_spaces` 保存 owner、Binding、Connection、Root、native parent/dir、canonical prefix、状态和 CAS 版本。
- Manifest 增加 nullable `user_space_id`、`target_provider_scope`；历史行保持 NULL。
- 新私有 Manifest 必须同时冻结 UserSpace、Provider scope 与 canonical target path。
- 增加 donor 结果证据和 copy operation 持久化，记录 SHA1/size/parent/object 证据、request digest、状态与恢复原因。
- 验收：21→22、重复 migration、checksum、并发唯一性、历史 NULL、删号 tombstone、无个人容量字段。

### T2 私有准入、空间 provision 与 ACL

- 实现 default-off UserSpace 领域、显式 provision 和最小 v2 API。
- 目录创建 UNKNOWN 只读对账，禁止第二次盲目 Mkdir。
- 私有提交使用独立 digest domain，旧 v1 历史读取与重放规则不混用。
- 在列表、搜索、计数、cursor、Resolve、Content、Preview、HEAD、Range、Share/Redeem/Open 前执行 owner ACL。
- 验收：A/B/ADMIN/匿名矩阵无跨用户数据、计数、缓存或 donor 信息泄漏。

### T3 115 donor 证据与原生 Copy Adapter

- 只读采集 donor FileID、parent、name、size、SHA1；缺字段、目录、空 FileID 或多候选 fail closed。
- Copy 前重新核验 donor 未移动、未替换、内容证据未变。
- Copy 使用精确 `pid + fid[0]`，禁用底层自动重试，错误按封闭类别和副作用确定性处理。
- Copy 后通过目标目录前后差异和 Stat/GetFile 找唯一新对象；零匹配保持 UNKNOWN，多匹配进入恢复。
- 验收：协议 fixture、空响应、429、超时、CID mismatch、重复 hash、目的端零/一/多匹配，均无猜测和盲目第二次写。

### T4 Worker 复制状态机与 READY

- donor 必须为 READY + Job SUCCEEDED + REFERENCE_KNOWN + PRESENT Copy + 完整原始结果证据。
- 在锁类 3401 下原子决定 Copy 或 StartDownload，持久化 COPY_RESERVED 后才允许一次 Copy。
- `COPY_RESERVED / PROVIDER_ACCEPTED / PROVIDER_CONFIRMED / PROVIDER_UNKNOWN` 与现有 Manifest/Job 状态配对。
- 只有目的对象唯一且 SHA1/size/parent/name 匹配，才进入既有 AWAITING_VISIBILITY。
- 验收：Copy 与 Start 竞争仅一个获胜；RPC 成功 DB 失败、提交后断连、进程崩溃、延迟可见均不重复副作用；最终仍经 Q5/Q8/Copy 才 READY。

### T5 最小用户界面

- 显示空间未 provision/provisioning/active/recovery 状态。
- 新提交仍保留 #190 的登录健康门禁、输入保留、原 operation 查询和精确重放。
- 不显示 donor 用户、donor 路径、native ID 或内部复制诊断。
- 验收：A/B 页面无交叉信息；失败/UNKNOWN 可恢复可见；无自动重试。

### T6 验证与隔离部署

- 本地：unit、PG16/18、race、HTTP ACL、Worker fault-injection、Node、vet、build。
- 公网：独立进程、数据库、Root、Binding、Connection、测试父目录和 A/B 用户，关闭公开注册。
- 隔离 Provider 首轮仅创建测试目录和小文件；不连接生产 DB/Root/队列，不删除或迁移真实文件。
- 验收证据包括精确 HEAD、测试收据、隔离拓扑、A/B 不同对象、同 hash 安全恢复和零生产写副作用。

## 5. 首版 API 目标

```text
GET  /api/v2/cloud-downloads/space
POST /api/v2/cloud-downloads/space/provision
POST /api/v2/cloud-downloads
```

复用现有 owner-only 任务、usage、library、content、preview 和 session health 读取。文件 Rename/Move/Delete、任务取消、批量/递归操作、完整文件 CRUD 均不在首版。

## 6. 核心验收矩阵

| 场景 | 必须结果 |
|---|---|
| 两个用户相同候选来源 | 各自 Manifest/Job、native 目标、Canonical path、resource_id、CopyID 独立 |
| donor 单文件证据完整 | 至多一次 Copy；复制前后 SHA1/size/name/parent 一致 |
| donor 是目录、多文件、SHA1 缺失或证据过旧 | 不复制；未预约 Copy 时才允许普通离线下载 |
| donor 同 info_hash 多任务或对象不唯一 | 不认领、不复制、不取消，进入封闭恢复 |
| Copy 请求超时 | UNKNOWN，只读对账；不得自动 Copy 或 StartDownload |
| Copy 成功但 DB 提交失败 | 重启后按 request digest 对账，不生成第二份对象 |
| 目的端尚未可见 | 保持等待，不伪造成功 |
| 目的端多个候选 | RECOVERY_REQUIRED，不选 winner、不删除 |
| Canonical 未投影 | 不得 READY |
| A 删除/移动 donor | 已完成的 B 对象不受影响 |
| A 访问 B 的任务/文件/计数/缓存 | 全部拒绝，ADMIN 无默认跨用户内容权限 |
| 历史 Manifest/未知 START_RESERVED | 保持原状态，不自动回填或重放 |
| Schema 21 数据 | 升级前不变；升级后历史字段 NULL，旧 binary 不盲目启动于未知 schema |

## 7. 明确不做

- 不做跨用户 Canonical/Copy 共享、引用计数或共享物理删除。
- 不做每用户容量配额、1500 次政策切换、完整 CRUD、取消任务或永久删除。
- 不修改 IndexCore 身份规则，不新增第二物理真相写入者。
- 不把 SDK 方法存在、115 Copy ACK、Hint 202 或静态测试描述当作生产验收。
- 不在本轮进行生产切流、20K、长稳或真实压力测试。
## 8. 当前实施状态（2026-11-16）

已完成后端基础实现：

- Schema 22：UserSpace、Manifest 冻结目标、donor 结果证据和 CopyOperation。
- UserSpace 仓储、owner 删除 tombstone、native Connection 唯一性和 CAS。
- Manifest 私有目标提交/执行校验，历史字段保持 NULL。
- 115 对象证据、严格分页、Copy 单次调用和复制前后只读核验。
- donor 资格、请求摘要、目标基线差异、CopyExecutor 与 UNKNOWN 恢复。
- Copy 与 StartDownload 共用 advisory fence，双向拒绝既有预约。
- 本地 unit、race、PG18 integration、全仓串行测试、Node、Python、vet 和 build。

Worker 侧后续已补齐：

- donor 查询同时要求同源、READY、SUCCEEDED Job、REFERENCE_KNOWN、PRESENT Copy 和结果证据。
- Copy/Start 的共同 advisory fence 与数据库时刻 lease/claim fence。
- CopyCoordinator 按当前 Manifest Provider Session 创建 CopyExecutor，并接入 Runtime。
- 严格分页的 CID、offset、count、实际进度和空目录证明。
- Copy/Start 双向及并发竞争、donor 查询、lease fence、Coordinator 和全仓回归测试。

原生 provision 已增加：

- product CloudSpaceService：GET 不创建目录，显式 provision 才允许 Mkdir，UNKNOWN 只读对账。
- `GET /api/v2/cloud-downloads/space` 与 `POST /api/v2/cloud-downloads/space/provision`。
- 115 FindDirectory/单次 Mkdir 与 Runtime 同一 Provider Session 复用，不创建第二份 Cookie 会话。
- v2 provision 的创建、重放、已存在目录、UNKNOWN 和严格 JSON 测试。

私有提交与 ACL 已增加：

- `POST /api/v2/cloud-downloads` 只允许 ACTIVE 空间，由服务端冻结 user_id、Binding、Connection、native scope 和 canonical prefix。
- 私有 digest domain、Submission/Manifest 冻结字段及 PostgreSQL 准入核验。
- owner-only 私有路径 Resolve/Browse 过滤，共享 libraryview 在计数、搜索和分页前排除私有树。
- 私有空间前端渐进增强：GET 不创建目录，ACTIVE 才使用 v2，保留旧端点兼容回退。
- 私有目录统一使用 `/<base>/_users/<normalized-login>__<full-user-uuid>`，与 ACL 判断一致。

尚未完成，不能宣称端到端功能可用：

- 完整 A/B/ADMIN/匿名文件、缓存、分享和 HTTP 行为验收矩阵。
- 公网隔离环境部署与真实 115/OpenList 验证。
- 生产 schema 迁移、发布、回滚和真实双用户同内容复制验收。

因此当前改动仅是开发暂存切片，不代表生产可用。详细进度和后续任务见 `PRIVATE-CLOUD-DOWNLOAD-COPY-PROGRESS.md`。