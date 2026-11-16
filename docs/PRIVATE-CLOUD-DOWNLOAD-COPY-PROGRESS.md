# 私有云下载与原生复制：开发进度和后续任务

日期：2026-11-16
状态：开发暂存，未合并、未部署、未做真实 Provider 验收

## 1. 当前基线

- 开发分支：`feat/private-cloud-download-copy`
- 基线：CloudSite main `1b5b560bb8006ea69f05c61926040c3d9ad7679f` + PR #190 `15f3c71c4269faa62284de873e843aca209dbd18`
- PR #191 仅作为 UserSpace 设计输入，PR #192 仅作为审计输入。
- 当前 Schema head：22；旧 0001—0021 migration 未修改。

## 2. 本次暂存成果

### 架构与持久化

- 选择 **115 原生复制到各用户独立目录**，不共享跨用户 Canonical resource 或 CloudSite Copy。
- `info_hash`/BTIH 不作为文件 SHA1；hash 只作复制前后一致性证据。
- 新增 Schema 22：UserSpace、Manifest 冻结目标、donor 结果证据和 CopyOperation。
- 历史 Manifest 的私有字段保持 NULL，不回填、不自动重放。
- UserSpace owner 删除 tombstone、Connection 维度 native DirID 唯一性和 CAS。

### 115 与复制执行

- 新增可选 ObjectCopyProvider、DirectoryProvider、严格对象证据和 SHA1/size/parent 校验。
- 严格分页：响应 CID、offset、count、实际页进度、重复页和空目录证明。
- Copy 单次调用，复制前后目标目录差异和精确 Stat；零匹配等待，多匹配进入恢复。
- Copy/Start 共用 advisory fence，双向拒绝既有预约；新预约按数据库时间复核 lease/claim。
- donor 必须满足同源、READY、SUCCEEDED Job、REFERENCE_KNOWN、PRESENT Copy 和完整证据。
- CopyCoordinator 按当前 Manifest 的 Provider Session 创建 CopyExecutor并接入 Runtime。

### UserSpace、API 与 ACL

- `GET /api/v2/cloud-downloads/space`：只读，不创建目录。
- `POST /api/v2/cloud-downloads/space/provision`：显式创建或只读对账；UNKNOWN 不盲目 Mkdir。
- `POST /api/v2/cloud-downloads`：只允许 ACTIVE 空间，由服务端注入冻结的 user/binding/connection/scope/prefix。
- 私有 digest domain，旧 v1 digest 与旧请求保持不变。
- Resolve/Browse 按 owner 过滤私有路径；共享 libraryview 在计数、搜索和分页前排除私有树。
- 目录统一为 `/<base>/_users/<normalized-login>__<full-user-uuid>`。
- 前端渐进增强：GET 不创建目录，ACTIVE 才使用 v2，旧端点作为兼容回退。

### 安全边界

- 复用 Runtime 中已注册的同一 Provider Session，不创建第二份 Cookie 会话。
- 浏览器不能提交 native FileID/DirID、ProviderScope、Binding、Root 或 donor。
- 原生 ID 不进入公开 DTO、Job payload、日志或 IndexCore 身份。
- 未使用真实账号密码，未执行真实 115/OpenList 写操作。

## 3. 已执行验证

- 全仓串行 Go 回归：`go test -p 1 ./...`，使用本地隔离 PostgreSQL 18 `panta_test`，通过。
- `go vet ./...`：通过。
- 全部 Node 测试：217/217 通过。
- Python 只读服务和发布安全测试：14/14 通过。
- 受影响包 race：通过；新增 Copy/Start 竞争、lease fence、UserSpace、donor 和 Copy 仓储 PG race 通过。
- 新增覆盖：Schema 22、私有目标、严格分页、复制 UNKNOWN、donor 失效恢复、共同副作用栅栏、lease fence、v2 provision/submit、A/B 授权和前端模块。
- `git diff --check`：通过。

以上仅证明本地代码与受控测试，不证明真实 115 行为、生产容量、真实双用户同内容复制或长稳。

## 4. 当前未完成

1. 完整 A/B/ADMIN/匿名矩阵：文件列表、Resolve、Content GET/HEAD/Range、Preview、缓存、ETag、Share/Redeem/Open 和 302。
2. 私有文件专用列表与前端完整交互，以及公开 API 的 donor 信息防泄漏验收。
3. Schema 21→22 的生产兼容发布、备份/恢复、迁移与回滚方案。
4. 本地完整 Worker E2E：真实 PostgreSQL + fake Provider + Q5/Q8/Copy/READY 全链。
5. 公网完全隔离部署与真实 115/OpenList 双用户小文件验证。
6. 真实 115 Copy ACK、延迟可见、重复同 hash、Copy UNKNOWN 和目标冲突行为验证。
7. 生产发布、20K、长稳和真实吞吐测试均未开展。

## 5. 后续任务顺序

### T1 完成 ACL 与产品行为

- 为私有文件专用列表补充服务端 owner scope，不允许通过 parent/cursor/search/count 侧漏。
- 验证私有文件禁止新分享，历史分享在 Redeem 和 Open 均重新鉴权。
- 私有文件禁用外部 302，缓存命中也必须重新授权。
- 验收：A/B/ADMIN/匿名均无法跨用户读取、计数、猜测 donor 或获得路径。

### T2 完成本地端到端测试

- admission → provision → donor 证据 → Copy → Hint → Q5/Q8 → Copy → READY。
- 同用户重放、跨用户相同来源、Copy/Start 竞争、RPC 成功后 DB 失败、进程重启和 UNKNOWN 恢复。
- 同 hash 不同路径必须产生不同 resource_id/CopyID；A 删除不影响 B。

### T3 公网隔离验证

- 独立 CloudSite 进程、PostgreSQL、IndexCore Root、Binding、Connection、测试父目录和 A/B 用户。
- 关闭公开注册，禁止连接生产数据库、Root、队列和用户目录。
- 首轮仅创建测试目录和小文件，不开放删除、递归、批量操作或任务取消。
- 保存精确 HEAD、隔离拓扑、测试日志和零生产副作用证据。

### T4 评审与发布

- 独立代码审查和 exact-head CI 全部通过后，再评审 Draft PR。
- 另立生产迁移/发布任务，完成备份、隔离恢复、schema 兼容和回滚演练。
- 未获明确生产授权前，不切流、不执行生产 migration、不修改真实用户文件。

## 6. 恢复开发

- 工作树：`/home/nathan/桌面/cloudsite-private-copy`
- 本地隔离数据库容器：`cloudsite-copy-pg18-test`
- Go 工具链：`/home/nathan/.local/share/go1.27.1/bin`
- 先阅读 `PRODUCT-PRIVATE-CLOUD-DOWNLOAD-COPY-IMPLEMENTATION.md` 与本文件。
- 不恢复个人容量配额、1500 次限制、跨用户共享 Copy、完整文件 CRUD 或 AI 自主文件操作。