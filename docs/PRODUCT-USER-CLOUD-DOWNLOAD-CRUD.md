# CloudSite 多用户独立云下载空间与 CRUD 开发方案

> **架构审查修订（2026-11-16）：** 已补充安全过渡、v1/v2 API 兼容、schema21→22 回滚门禁，以及每用户累计 1500 次云下载提交上限。跨 PR 审查意见见 [CloudSite 安全过渡与架构审查](https://github.com/nathanxiangang-web/cloudsite/blob/docs/system-audit-20261116/audit/15-ARCHITECTURE-TRANSITION-REVIEW.md)。本修订仍是设计文件，不表示 main 已实施 1500 上限或 UserSpace 功能。

> **文档状态：代码调研完成；开发方案待项目所有者评审。**
>
> 本文是 D-065 / Issue #138 的最新代码审计补充和实施交接文档，不代表功能已经实现，也不授权执行数据库迁移、真实 115 写操作或生产部署。
>
> **本轮授权范围（2026-11-16）：** 阅读当前代码、核对 115driver 能力、形成开发方案并提交 GitHub 评审。Issue #139 原有代码实施暂停状态不因本文自动解除；本轮不写业务代码、不调用真实 115、不创建目录、不执行文件操作。
>
> **代码基线：** CloudSite main = 1b5b560bb8006ea69f05c61926040c3d9ad7679f；VERSION = 1.0.3；PostgreSQL CurrentSchemaVersion = 21；Go 依赖锁定 github.com/SheltonZhu/115driver v1.3.5。

关联资料：

- [总控 Issue #138](https://github.com/nathanxiangang-web/cloudsite/issues/138)
- [Stage A Issue #139（当前仍暂停实施）](https://github.com/nathanxiangang-web/cloudsite/issues/139)
- [原 D-065 私有云空间蓝图](PRODUCT-PRIVATE-CLOUD-SPACES.md)
- [CloudSite 115driver v1.3.5](https://github.com/SheltonZhu/115driver/tree/v1.3.5)
- [IndexCore 仓库](https://github.com/nathanxiangang-web/index-core)

---

## 1. 产品目标

CloudSite 的每个登录用户拥有一个独立的 115 云下载目录，并在 CloudSite 的 /downloads/ 页面中管理自己的下载任务和文件。

目标不是给现有任务列表简单增加用户名过滤，而是建立完整的用户空间隔离：

1. 每个 CloudSite 用户在同一个运营方管理的 115 账号下拥有独立目录。
2. 用户只能创建、查询、管理自己目录中的下载任务、文件和子目录。
3. 用户 A 不能查看、搜索、预览、播放、下载、移动或删除用户 B 的内容。
4. 所有权以 CloudSite 不可变的 user_id 为准，用户名只用于目录的人类可读名称。
5. 用户通过 CloudSite 页面完成云下载任务和个人文件空间的增删改查，不需要进入 115 客户端操作。
6. 115driver 负责实际 115 目录、任务和文件操作；IndexCore 继续负责物理资源观察与 Canonical 真相。
7. 保留现有 CloudSite 登录、任务、Worker、Provider Session、OpenList 和 IndexCore 架构，不新建第二套下载引擎。

本方案按**一个共享的运营方 115 账号 + 每个 CloudSite 用户一个独立子目录**设计，不是给每个 CloudSite 用户分配独立 115 Cookie 或独立 115 账号。

## 2. 当前代码审计结论

### 2.1 已经具备的能力

| 模块 | 当前代码证据 | 当前能力 |
|---|---|---|
| 用户身份 | internal/auth/auth.go | UUID UserID、规范化 login、USER/ADMIN、登录会话 |
| 用户名规则 | auth.NormalizeLogin | 小写 ASCII，3–64 位，允许字母、数字、点、下划线、连字符 |
| 用户改名 | internal/store/postgres/admin_user_lifecycle.go | 改 login，不改变 user_id |
| 用户删除 | internal/store/postgres/admin_user_delete.go | 账号删除时清理 sessions、shares、operations、quota 和 product_users；保留 Manifest、Job、ProviderTask 历史 |
| 云下载入口 | internal/producthttp/cloud_download.go | 固定运营方 CloudDownloadTarget；用户提交 source 和 operation_key |
| 下载任务归属 | internal/acquisition/manifest.go | Manifest 已有可空 UserID、BindingID、TargetPath |
| 用户任务读取 | internal/product/reads.go、internal/store/postgres/product_read_repository.go | SQL 按 user_id 和允许的 Binding 查询任务 |
| 下载执行 | internal/acquisition/execution_input.go | 从 Manifest 和 StorageBinding 生成 provider-neutral DownloadRequest |
| 115 离线下载 | internal/providers/115/adapter.go、driver.go | 已接入新增、状态查询和取消离线任务 |
| 下载页面 | internal/producthttp/web/downloads.html、downloads.js | 链接检查、提交、自己的任务列表、已下载文件入口 |
| 文件观察 | internal/integrations/indexcore、internal/libraryview | 使用 IndexCore 读取物理资源与快照 |
| 文件交付 | internal/fileaccess | 通过 Canonical 身份校验后读取内容、预览或直链交付 |

现有任务查询已经按 user_id 过滤，这是可以复用的基础。但任务归属不等于物理目录隔离。

### 2.2 尚未实现的关键能力

| 缺口 | 当前证据 | 影响 |
|---|---|---|
| 用户专属 115 目录 | 当前 CloudDownloadTarget 只有统一 BindingID、TargetPath、Label | 所有用户仍使用同一固定下载目标 |
| 用户空间持久化映射 | 当前没有 cloud_user_spaces / UserSpace 实体 | 无法证明 native directory 属于哪个 user_id |
| Manifest 目标空间 | Manifest 没有 UserSpaceID | 历史任务无法冻结到用户专属目录 |
| 动态 Provider Scope | ExecutionInputResolver 使用 Binding.ProviderScope | 仍会把同一个原生 saveDirID 传给不同用户 |
| 115 目录管理 | 115 Adapter 只声明 Downloader capability | 没有创建、浏览、重命名、移动、删除目录的应用接口 |
| 115 文件管理 | 当前 Adapter 没有文件 CRUD 接口 | 页面不能管理下载完成后的文件 |
| Provider 任务目录校验 | OfflineTask 投影只保留 InfoHash、Status、Name | 丢弃 115 返回的 DirId，无法在适配层核验任务目标目录 |
| 私有资源后端 ACL | ReadService 只校验 Binding allowlist | 知道资源 ID 或路径的用户可能访问同一 Binding 下其他用户的资源 |
| 私有文件分享限制 | fileshare 通过 fileaccess 读取文件 | 需要防止私有资源通过旧分享、匿名兑换绕过权限 |
| 私有 CDN 交付策略 | fileaccess 可返回外部 DirectURL | 已签发的外部 URL 可能在有效期内无法撤销 |
| 用户空间前端 CRUD | downloads.js 只有提交、查询、刷新 | 没有创建文件夹、取消任务、重命名、移动和删除操作 |

### 2.3 任务归属已经存在，但不能代替空间归属

当前 ProductReadRepository.ListTasks 的查询条件包含：

- acquisition_manifests.user_id = 当前 principal.UserID
- target_storage_binding_id 属于当前产品进程允许的 Binding 集合。

因此当前 GET /api/v1/tasks 是用户自己的任务列表。

但当前 SQL 没有 UserSpaceID 条件。新个人空间启用后，应额外要求任务属于当前用户的有效 UserSpace。没有 UserSpaceID 的历史任务不能自动归入新空间。

### 2.4 当前下载目标是服务端固定值

internal/producthttp/cloud_download.go 中：

- CloudDownloadTarget 由运营方配置。
- POST /api/v1/cloud-downloads 不接受客户端提供目标目录。
- HTTP 层把配置中的 BindingID 和 TargetPath 写入 AdmissionRequest。

这是正确的安全原则，应继续保留。

新实现不是让浏览器提交 username、folder ID 或 target_path，而是由服务端根据已经认证的 Principal.UserID 查找 UserSpace，再生成本次下载目标。

### 2.5 当前执行目标存在协议缺口

internal/acquisition/execution_input.go 当前执行逻辑：

- Target.Scope 来自 StorageBinding.ProviderScope。
- Target.Path 来自 Manifest.TargetPath。

这两个字段代表不同坐标：

- Scope：115 原生目录 ID，即实际离线下载保存位置。
- Path：IndexCore Canonical 观察路径。

不能只把 Manifest.TargetPath 拼接用户名，就认为下载已经保存到对应的 115 目录。

必须新增服务端 UserSpace 目标解析能力，使每条新 Manifest 都能绑定其 UserSpace，并在 Worker 执行时解析出该空间的原生目录 ID。历史 Manifest 不应重新计算或覆盖原目标。

## 3. 115driver v1.3.5 源码能力核验

CloudSite 的 go.mod 当前锁定：

github.com/SheltonZhu/115driver v1.3.5

本次直接检查了该版本的 Go 源码。以下是**SDK 源码已实现**的能力，不代表本次进行了真实账号调用或生产验证。

| 115driver 方法 | 能力 | 计划用途 |
|---|---|---|
| Mkdir(parentID, name) | 创建目录并返回目录 ID | 为用户创建专属目录、创建用户子目录 |
| ListPage(dirID, offset, limit) | 分页列出目录内容 | 目录浏览、操作前核验 |
| List(dirID) | 列出目录内容 | 小目录辅助操作，不用于无界全盘扫描 |
| DirName2CID(path) | 按路径解析目录 ID | 服务器内部解析用户目录 |
| GetFile(fileID) | 获取文件信息 | 操作前读取文件信息 |
| Stat(fileID) | 获取文件/目录状态及 Parents 链 | 验证目标是否处于当前用户目录树 |
| Rename(fileID, newName) | 重命名文件或目录 | 文件和子目录更新 |
| Move(dirID, fileIDs...) | 移动文件或目录 | 仅允许在当前用户空间内部移动 |
| Copy(dirID, fileIDs...) | 复制文件或目录 | 可作为后续增强，不作为首版必要条件 |
| Delete(fileIDs...) | 删除文件或目录 | 用户空间内的受控删除 |
| ListOfflineTask(page) | 查询离线任务 | 读取 115 任务状态 |
| AddOfflineTaskURIs(uris, saveDirID) | 新建离线任务并指定保存目录 | 创建用户自己的云下载任务 |
| DeleteOfflineTasks(hashes, deleteFiles) | 删除离线任务 | 取消用户自己的未完成任务 |
| ListRecycleBin(offset, limit) | 查询回收站 | 后续核验删除恢复能力 |
| RevertRecycleBin(rIDs...) | 恢复回收站项目 | 仅在真实语义验证后考虑开放 |

源码位置：

- [offline.go](https://github.com/SheltonZhu/115driver/blob/v1.3.5/pkg/driver/offline.go)
- [dir.go](https://github.com/SheltonZhu/115driver/blob/v1.3.5/pkg/driver/dir.go)
- [op.go](https://github.com/SheltonZhu/115driver/blob/v1.3.5/pkg/driver/op.go)
- [recylcle.go](https://github.com/SheltonZhu/115driver/blob/v1.3.5/pkg/driver/recylcle.go)

### 3.1 当前 CloudSite Adapter 的限制

当前 internal/providers/115/driver.go 中的 OfflineClient 仅声明：

- AddOfflineTaskURIs
- ListOfflineTask
- DeleteOfflineTasks

当前投影后的 OfflineTask 仅包含：

- InfoHash
- Status
- Name

而 115driver 原始 OfflineTask 还包含：

- DirId（wp_path_id）
- FileId
- DelFileId
- Percent
- Size
- AddTime / UpdateTime
- Url

当前 Adapter 没有把 DirId 投影出来，DownloadStatus 仅按 InfoHash 搜索任务，没有校验任务的实际目标目录。

新实现必须补齐目标目录核验，但绝不能把原始 URL、Cookie、FileId、DirId、PickCode 或 InfoHash 直接返回浏览器。

### 3.2 任务更新能力的边界

115driver v1.3.5 没有独立的离线任务编辑/更新接口。

因此：

- 下载任务创建：支持。
- 下载任务查询：支持。
- 未完成任务取消：支持。
- 修改已有任务的源链接或原生保存目录：SDK 没有直接更新接口。

用户希望改变下载源或保存位置时，产品流程应为：在权限核验后取消原任务，再提交新任务。已完成任务的文件可以通过 Rename / Move 管理。

不能在文档或页面中承诺“直接修改正在运行的 115 离线任务”。

### 3.3 取消任务与删除文件必须分开

当前 CloudSite 的 DeleteOfflineTask 固定调用：

DeleteOfflineTasks([]string{infoHash}, false)

这意味着取消控制面任务，不删除已经形成的文件。

新实现必须继续遵守：

- 取消下载任务：只删除任务记录，deleteFiles 必须为 false。
- 删除已下载文件：使用独立的文件删除接口，先证明文件属于当前 UserSpace。
- 禁止把 DeleteOfflineTasks(..., true) 当作普通取消接口。
- 禁止使用 ClearOfflineTasks 清理共享账号全部任务。
- 禁止把任务取消成功直接当成文件已删除。

115driver 提供回收站列表和恢复方法，但仅凭 SDK 方法存在，不能推断生产删除一定可恢复。真实删除语义需要在单独授权的测试账号/目录中核验；禁止调用 CleanRecycleBin。

### 3.4 115driver 调用约束

当前 115 Adapter 已使用有界 HTTP Client，默认 5 秒，最大 10 秒；其注释明确说明 Pan115Client 有可变的用户 ID 和请求状态，并通过锁串行化调用。

新文件管理能力必须：

- 复用现有受保护 115 Session / CredentialRef 边界。
- 使用同一连接级客户端和共享调用锁，不能为文件操作另建不受控的并发客户端。
- 为 SDK 不支持 context 的方法设置有界 HTTP timeout。
- 对外部写请求超时按 UNKNOWN 处理，先只读核验，不盲目重试。
- 不调用 115driver CLI 子进程，不要求用户提交自己的 Cookie，不开放任意 SDK 代理接口。

## 4. 目标 115 目录和身份模型

### 4.1 目录结构

历史已验收的路径关系记录为：

- 115 原生目录：/CloudSite/云下载
- OpenList 账号挂载基础：/115open/CloudSite
- IndexCore Canonical 路径：/云下载

建议在原生下载根目录下新增统一用户容器：

~~~text
115 原生目录：
/CloudSite/云下载/_users/
    zhangsan__<full-user-uuid>/
        video/
        documents/
        file.iso
    lisi__<full-user-uuid>/
        software/
        package.zip

IndexCore Canonical：
/云下载/_users/zhangsan__<full-user-uuid>/...
/云下载/_users/lisi__<full-user-uuid>/...
~~~

**重要：** 上述是基于历史验收文档的拟议目录结构。当前保护配置中的实际 native parent ID、Binding、Connection、IndexCore Root 和 canonical prefix 必须在实施前重新只读核验。本文不创建目录，也不把历史 ID 当作当前生产证据。

旧版 PRODUCT-CLOUD-RUNTIME.md 示例中出现过 /downloads，而历史 acquisition acceptance 记录的是 /云下载。这属于需要在实施前统一的配置/文档坐标差异，不能直接复制旧示例作为生产目标。

### 4.2 目录名与所有权

目录名规则：

**规范化 login + "__" + 完整 user_id(UUID)**

例如：

zhangsan__8f7c...完整UUID

规则：

1. login 必须经过现有 auth.NormalizeLogin。
2. 不允许昵称、中文展示名或客户端任意路径片段参与目录路径。
3. 完整 UUID 用于避免同名用户、删除后重注册和用户名重用造成冲突。
4. 真正所有权始终由 user_id 决定，不由目录名决定。
5. 管理员修改 login 时，保留原 UserSpace、native directory ID 和原目录名，不自动重新建目录。
6. 原账号被删除后，同名新账号拥有新的 UUID，必须创建新的空间，不得继承旧目录。

### 4.3 用户空间根目录不可由普通用户操作

用户可以在自己的空间内创建子目录、移动和重命名文件。

用户不能：

- 重命名自己的空间根目录。
- 删除自己的空间根目录。
- 把根目录移动到其他用户或运营目录。
- 移动文件到其他用户空间。
- 通过传入其他目录 ID 绕过服务端权限。

根目录的 native ID 和 canonical prefix 是服务器内部的安全边界。

## 5. UserSpace 身份与生命周期

建议新增 UserSpace 领域对象和持久化映射。

### 5.1 建议的 cloud_user_spaces

以下是设计字段，不是当前数据库表：

| 字段 | 用途 |
|---|---|
| space_id | UserSpace 稳定 UUID |
| owner_user_id | 不可变 CloudSite 用户 UUID |
| binding_id | 当前产品存储 Binding |
| connection_id | 受保护的 115 StorageConnection |
| provider_account_identity | 115 账号身份快照，不是 Cookie |
| native_parent_id | 运营方指定的父目录 ID，服务端私有 |
| native_dir_id | 用户专属目录 ID，服务端私有 |
| indexcore_root_id | 对应 IndexCore Root |
| canonical_prefix | 用户专属 Canonical 路径 |
| directory_name | 创建时的 login + UUID 名称 |
| status | 空间创建/可用/待核验状态 |
| owner_state | ACTIVE / OWNER_DELETED |
| version | CAS/并发更新版本 |
| provisioning_key | 创建目录操作幂等标识 |
| last_verified_at | 最近一次目录归属核验时间 |
| created_at / updated_at | 审计时间 |

唯一约束建议：

- 同一个 owner_user_id + binding_id 只能有一个有效空间。
- 同一个 binding 下一个 native_dir_id 不能归属多个 UserSpace。
- 同一个 binding 下一个 canonical_prefix 不能分配给多个 UserSpace。
- ACTIVE 空间的 native_dir_id 和 canonical_prefix 不允许普通更新。

### 5.2 重要：当前用户删除是物理删除

当前 internal/store/postgres/admin_user_delete.go 的 deleteAccount 会物理删除 product_users，但保留历史 Manifest、Job、ProviderTask。

因此原 D-065 设计中简单地给 UserSpace.owner_user_id 添加 REFERENCES product_users(user_id)，会导致账号删除时产生外键冲突。

新设计必须处理这个生命周期：

- UserSpace 的 owner_user_id 应作为不可变历史归属保留。
- 不能因账号删除而级联删除 115 目录。
- 账号删除事务中把空间标记为 OWNER_DELETED。
- OWNER_DELETED 空间不允许任何普通用户访问，也不能被新账号领用。
- 同名新账号有新的 user_id，因此会获得新空间。

实现上可使用独立身份 tombstone 表，或在 cloud_user_spaces 中保留无 product_users 外键的 owner UUID 与 owner_state。具体选择由架构评审确定；不能照搬旧 D-065 外键字段。

### 5.3 用户生命周期

| 用户事件 | UserSpace 行为 |
|---|---|
| 新用户注册 | 不必同步调用 115；空间可延迟开通 |
| 首次开通 | 创建唯一 UserSpace 和 115 专属目录 |
| 用户改名 | 保留 user_id、native_dir_id、原目录名 |
| 用户禁用 | 立即失去访问权限，目录保留 |
| 用户退出/会话失效 | 不再访问，目录保留 |
| 用户删除 | 标记 OWNER_DELETED；不自动删除 115 内容 |
| 同名重新注册 | 新 user_id，创建全新目录 |

## 6. 目录创建和未知结果恢复

不建议在普通 GET 请求中创建 115 目录。

推荐流程：

1. GET /downloads 页面时读取空间状态。
2. 未开通时显示“创建我的云下载空间”。
3. 用户点击后调用明确的 POST provision 接口。
4. 服务端校验登录用户、父目录、Binding、Connection 和 provider account identity。
5. 数据库先写入唯一创建意图。
6. 使用 115driver Mkdir 创建目录。
7. 使用 Stat / DirName2CID / 父目录 ListPage 做只读核验。
8. 只有唯一目录、预期父目录、native ID 和数据库映射一致后才标记 ACTIVE。

建议状态机：

~~~text
NOT_PROVISIONED
    |
    v
PROVISIONING
    | 成功且核验一致
    v
ACTIVE

PROVISIONING
    | 外部请求超时/响应未知
    v
RECONCILE_REQUIRED
    | 唯一目录匹配且归属证明成立
    v
ACTIVE

RECONCILE_REQUIRED
    | 多个候选/父目录变化/账号变化
    v
BLOCKED / UNAVAILABLE
~~~

外部 Mkdir 超时不能直接再次 Mkdir。必须先通过只读目录查询核验是否已经创建。

禁止：

- 未知结果盲目重试创建。
- 多个同名目录时自动任选一个。
- 把其他用户已有目录自动认领。
- 在没有确认父目录 ID 时创建目录。
- 将 115 返回的目录 ID 暴露给浏览器。

## 7. 下载目标与 Worker 改造

### 7.1 请求目标由服务端解析

新的下载提交仍使用现有 AdmissionService、operation_key、Manifest、Job 和单一 Worker。

建议调用链：

~~~text
登录 Session
   |
   v
Principal.UserID
   |
   v
UserSpaceResolver
   |
   +--> ACTIVE UserSpace
   |      binding_id
   |      native_dir_id
   |      canonical_prefix
   |
   v
AdmissionService
   |
   +--> Manifest.user_space_id
   +--> Manifest.user_id
   +--> Manifest.target_storage_binding_id
   +--> Manifest.target_path
   |
   v
ExecutionInputResolver
   |
   v
Target.Scope = UserSpace.native_dir_id
Target.Path  = UserSpace.canonical_prefix
   |
   v
现有 Worker / 115 DownloaderProvider
   |
   v
115 专属目录
~~~

### 7.2 Manifest 目标必须冻结

新 Manifest 必须绑定 UserSpaceID。

执行时必须验证：

- Manifest.UserID == UserSpace.owner_user_id。
- Manifest.UserSpaceID == 已持久化 UserSpace。
- Manifest.TargetStorageBindingID == UserSpace.binding_id。
- Manifest.TargetPath == UserSpace.canonical_prefix。
- UserSpace 状态仍然允许执行。
- Provider Connection 与已确认的账号身份一致。

新任务不能在每次重试时重新根据当前 login 计算目录。

历史 Manifest 没有 UserSpaceID，继续使用原冻结目标，不自动迁移、不重写、不重新启动。

### 7.3 Provider Scope 解析

当前 ExecutionInputResolver 直接读取 Binding.ProviderScope。

建议增加 provider-neutral 的 TargetScopeResolver / UserSpaceTargetReader，由执行边界根据 Manifest.UserSpaceID 获取已确认的原生目录 ID。

不要：

- 在 HTTP 层直接 import 115driver。
- 在浏览器提供 saveDirID。
- 把用户目录 ID 写进公开 DTO。
- 为每个用户复制一份 StorageBinding 或创建重复 IndexCore Root。
- 为每个用户创建独立 Worker。

仍然使用一个受控 115 Session 和现有串行 Worker。

### 7.4 同一 InfoHash 的跨用户冲突

115 的离线任务列表是共享账号级列表，原始任务包含 DirId。115 对相同 magnet/info_hash 是否会跨目录去重、返回旧任务或产生多个任务，需要真实受控验证。

在行为未确认前：

- 不能因为找到相同 InfoHash 就认定另一用户也已完成下载。
- 不能把用户 A 的 ProviderTask 当作用户 B 的任务结果。
- 不能允许用户 B 取消用户 A 的任务。
- 任务状态必须同时核验 InfoHash 和预期 DirId。
- DirId 与当前 UserSpace 不匹配时，进入 DESTINATION_MISMATCH / RECONCILE_REQUIRED，不得 READY。
- 不得用文件名、最新任务或“唯一匹配文件”猜测归属。

## 8. 用户页面 CRUD 定义

用户要求的是个人云下载页面完整的增删改查。必须将“下载任务操作”和“已下载文件操作”分开定义。

### 8.1 下载任务 CRUD

| 操作 | 页面功能 | 后端行为 |
|---|---|---|
| Create | 提交 HTTP / HTTPS / magnet / ed2k 下载 | 复用现有 AdmissionService，目标由 UserSpaceResolver 决定 |
| Read | 查看自己的任务列表、详情、进度、状态 | SQL 按 user_id + user_space_id 过滤，Provider 状态再核验 DirId |
| Update | 修改任务 | 115driver 没有离线任务编辑接口；不支持直接改源或保存目录 |
| Delete | 取消自己的未完成任务 | 校验 Manifest 所有权、ProviderTask 和 DirId，再 DeleteOfflineTasks(..., false) |

已完成任务不能通过取消接口删除文件。

如果用户需要修改下载源或保存位置，应取消原任务后创建新任务；不得伪装成 115 原生支持任务编辑。

### 8.2 文件和目录 CRUD

| 操作 | 页面功能 | 115driver 能力 | 约束 |
|---|---|---|---|
| Create | 新建子目录 | Mkdir | 父目录必须在自己的空间内 |
| Read | 浏览文件夹、文件详情 | ListPage / GetFile / Stat | 用户列表以 IndexCore Canonical 观察为准 |
| Update | 重命名文件或目录 | Rename | 不允许修改 UserSpace 根目录 |
| Update | 移动文件或子目录 | Move | 目标必须在同一 UserSpace 内，防止目录环和越权移动 |
| Update（可选） | 复制文件或子目录 | Copy | 可放到后续版本，不能作为首版上线阻塞项 |
| Delete | 删除已完成文件 | Delete | 单文件、二次确认、所有权证明、操作台账、IndexCore 结果核验 |
| Delete | 删除空子目录 | Delete | 首版仅允许空目录 |
| Delete（后续） | 删除非空目录 | Delete | 递归范围预览、单独授权和更严格恢复机制 |

首版禁止：

- 删除 UserSpace 根目录。
- 批量删除。
- 递归删除非空目录。
- 通过用户输入的 native file ID / directory ID 直接删除。
- 清空 115 回收站。
- 将取消任务等同于删除文件。

### 8.3 用户页面布局建议

个人云下载页面建议包含：

1. 顶部：当前登录用户名、个人空间状态、退出登录。
2. 云下载输入区：链接输入、文件名、提交前检查、提交下载。
3. 我的下载任务：任务名称、提交时间、状态、进度、取消操作。
4. 我的文件：目录导航、文件列表、文件大小、修改时间。
5. 文件操作：新建文件夹、重命名、移动、删除。
6. 操作结果：处理中、已完成、结果待核验、需要人工处理。
7. 空间状态：未开通、开通中、可用、异常待核验。

页面不能显示其他用户的任务数量、文件数量、目录名称、容量或错误详情。

## 9. 文件列表与 115driver 的职责划分

不能把 115driver 的 ListPage 直接变成第二套物理资源真相。

推荐边界：

- 115driver：创建/列出原生目录、读取原生文件信息、验证父目录、重命名、移动、复制、删除。
- OpenList：继续提供现有文件读取数据面。
- IndexCore：继续唯一负责 Canonical 物理资源观察、Q4/Q5/Q6/Q8 和删除确认。
- CloudSite：负责用户空间、权限、任务归属、操作台账和前端展示。

个人文件页面应通过 IndexCore 的现有读取链路展示已经被观察确认的文件。

115driver 的 ListPage / Stat 用于：

- 目录操作前核验。
- 目标 ID 解析。
- 原生父子关系验证。
- Provider 操作结果核验。

不能因为 115driver 返回一个文件，就直接把它标记为 CloudSite READY。

### 9.1 不向浏览器暴露原生 ID

浏览器只使用 CloudSite / IndexCore 资源标识。

服务器根据已授权的 Canonical ResourceID + CanonicalPath，在当前用户空间范围内解析原生文件 ID：

1. 通过 IndexCore 读取精确资源身份和 CanonicalPath。
2. 从服务器确定父目录路径。
3. 使用 115driver DirName2CID / ListPage 找到精确子项。
4. 使用 Stat 检查 Parents 链中包含当前用户 native_dir_id。
5. 名称、类型、大小或身份出现歧义时拒绝操作。
6. 原生 ID 只在服务端 Adapter 内部短暂使用。

客户端不能提交任意 115 FileID、DirID、PickCode 或 URL。

## 10. 后端私有资源 ACL：上线前置条件

这是本功能最重要的安全门禁。

当前 FileSurface 的注释明确说明它只是前端展示，不是授权。现有 LibraryService 使用统一 CloudPath 区分 library/downloads，尚未按用户目录过滤。

### 10.1 必须增加 PrivateResourceAuthorizer

建议建立统一的私有资源授权服务：

输入：

- 已认证 Principal。
- BindingID。
- IndexCore RootID。
- Canonical ResourceID。
- CanonicalPath。
- UserSpace 持久化映射。

授权规则：

- 当前 principal.UserID 必须等于 UserSpace.owner_user_id。
- UserSpace 必须处于 ACTIVE。
- Binding、Connection、Root 必须与空间映射一致。
- 资源必须位于当前用户 canonical_prefix 的真实子树。
- 路径比较必须使用标准化路径和目录边界，不能用不安全的裸字符串前缀。
- 资源 ID、路径、父目录和当前 Canonical 身份必须一致。
- 缺失、歧义、账号切换、目录移动、空间异常均 fail closed。

### 10.2 必须覆盖的接口

| 入口 | 必须增加的权限校验 |
|---|---|
| GET /api/v1/resources | 普通用户不能枚举其他用户的私有目录 |
| whole_root Q6 | 服务端过滤或拒绝包含其他用户私有空间的全根查询 |
| parent_resource_id 浏览 | 先验证父资源属于当前 UserSpace |
| GET /api/v1/resources/resolve | 不能通过猜路径解析其他用户资源 |
| GET /api/v1/files/content | 每次交付重新核验所有权 |
| GET /api/v1/files/preview | 每次预览和缓存命中前核验所有权 |
| HEAD / Range / media | 同样执行授权，不因请求方式绕过 |
| GET /api/v1/library?surface=downloads | 先按当前用户 prefix 过滤，再计算 Counts、Search、Cursor 和分页 |
| 分享创建 | 私有空间默认禁止创建分享 |
| 匿名 Redeem / Grant / Open | 旧 token、code 和分享链接不能绕过私有 ACL |
| 302 DirectURL | 私有文件默认不走无法撤销的外部直链，优先受控代理 |
| 登录切换 / 过期请求 / BFCache | 旧账号 DOM、异步响应和缓存不能残留到新账号 |

### 10.3 Library Snapshot 过滤

当前 libraryview Engine 的快照是整个 IndexCore Root 的派生读模型。

私有子目录可能存在于同一个 Root 快照中，因此不能仅在前端隐藏。

当前 libraryview.Page 将 Owner 用于 Cursor 查询指纹，但这不是资源所有权校验，也不会按 Owner 过滤快照中的文件。必须在服务端 LibraryService / libraryview 查询投影阶段按当前 UserSpace 过滤：

- 当前用户的 downloads 页面只能得到自己的 canonical_prefix。
- 普通 library surface 必须排除整个云下载 Canonical 根（/云下载）及其全部子树，包括 _users 与历史未归属文件；downloads surface 只返回当前 user space prefix。
- 搜索、分类统计、Unknown 数量、Filtered 数量、Cursor 和分页都必须在私有 ACL 过滤后计算。
- Cursor 必须绑定 user_id、space_id、revision 和查询条件。
- 不允许通过分页数量或错误信息推测其他用户的资源。

### 10.4 管理员权限

ADMIN 不是跨用户私有文件读取的万能权限。

管理员默认只能管理自己的个人空间。运营排障如确实需要访问某个用户文件，必须单独设计审计授权，不得因为 RoleAdmin 就跳过 UserSpace ACL。

### 10.5 302 直链

现有 fileaccess 可以使用 LinkSource.ResolveLink 返回 CDN DirectURL。

第三方 URL 一旦签发，可能在有效期内无法撤销。即使 CloudSite 删除文件或撤销用户权限，也不能保证外部 URL 立即失效。

私有文件在确认 URL 的有效期、签名范围、撤销能力前，不应启用直接 302；首版优先使用每次重新授权的受控代理交付。

## 11. 文件操作的一致性与幂等

目录、重命名、移动、删除都是外部 Provider 写操作，不能只依赖一次 HTTP 请求成功。

建议新增独立的 cloud_space_operations 操作台账，不直接复用只服务下载准入的 product_operations。

操作台账至少保存：

- operation_id
- owner_user_id
- space_id
- operation_key
- action
- canonical_resource_id
- old_parent / new_parent（如适用）
- request_digest
- status
- attempt_count
- provider_result_category
- created_at / updated_at
- 脱敏错误分类

状态建议：

~~~text
REQUESTED
   |
   v
AUTHORIZED
   |
   v
PROVIDER_PENDING
   |
   +--> PROVIDER_UNKNOWN --> RECONCILE_REQUIRED
   |
   v
PROVIDER_CONFIRMED
   |
   v
AWAITING_INDEXCORE
   |
   v
COMPLETED
~~~

其他终态/阻塞态：

- BLOCKED
- FAILED
- RECONCILE_REQUIRED
- OWNER_DELETED

关键规则：

1. 同一个用户、同一个 operation_key、相同 request_digest 才能重放。
2. 同一个 key 不允许修改目标资源或操作内容。
3. 外部写操作超时不能直接记失败。
4. 未知结果必须先只读核验。
5. 重命名/移动完成后发送旧目录和新目录范围的可信 MutationHint。
6. 删除操作必须等待 IndexCore 新鲜观察，不能仅凭 115 HTTP 成功返回 COMPLETED。
7. 不直接修改 IndexCore Canonical、Journal 或 Copy。
8. 不增加第二个扫描器、第二个 Canonical 写入者或第二个 Worker。

## 12. IndexCore 集成与 READY 判定

继续使用现有：

- Q4：资源浏览。
- Q5：精确路径解析。
- Q6：WholeRoot / Inventory。
- Q8：Journal / 观察变更。
- 可信 Loopback MutationHint。

目录/文件操作后：

- 创建目录：115 创建成功不代表 IndexCore 已观察到目录。
- 下载完成：115 状态 SUCCEEDED 不代表 CloudSite READY。
- 重命名/移动：必须确认新的 CanonicalPath 与父目录关系。
- 删除：必须通过 IndexCore 的新鲜观察确认资源消失。
- READY：必须保留现有 Q5/Q8/Projector/Copy 确认机制。

如果 IndexCore 暂时没有观察到结果，页面显示“等待收录/结果待核验”，不能猜测成功，也不能绕过 IndexCore 直接读取 OpenList 并写入 Canonical。

## 13. API 合同建议

所有接口都从认证 Principal 获取用户身份，不接受客户端指定 user_id、login、Binding、native ID 或 target_path。

**不得原地改变既有 v1 API 的请求语义。** 当前 main 的 `POST /api/v1/cloud-downloads` 接受既有的 BindingID/TargetPath 字段；新 UserSpace 模式的目标由服务端解析，两者不是兼容的同一请求合同。

### 13.1 v1 兼容合同

| 方法 | 路径 | 过渡规则 |
|---|---|---|
| POST | /api/v1/cloud-downloads | feature flag 关闭时保持当前 v1 语义；当前用户启用 UserSpace 后返回 409 `PRIVATE_SPACE_REQUIRED`，不得回退到共享旧目录 |
| GET | /api/v1/tasks | 保留 owner-only 历史任务读取；旧 Manifest 继续使用冻结的旧目标 |
| GET | /api/v1/usage | 保留现有字段，按 1500 次目标政策返回用量；该政策未发布前 main 仍按现状执行 |

### 13.2 v2 私有空间合同

| 方法 | 路径 | 作用 |
|---|---|---|
| GET | /api/v2/cloud-downloads/space | 查询当前用户空间状态 |
| POST | /api/v2/cloud-downloads/space/provision | 显式创建当前用户空间 |
| GET | /api/v2/cloud-downloads/space/operations/{key} | 查询空间创建/管理操作结果 |
| POST | /api/v2/cloud-downloads | 创建云下载任务；目标由服务端 UserSpaceResolver 决定 |
| GET | /api/v2/cloud-downloads/tasks | 查询当前用户当前空间任务 |
| GET | /api/v2/cloud-downloads/tasks/{manifest_id} | 查询当前用户任务详情 |
| POST | /api/v2/cloud-downloads/tasks/{manifest_id}/cancel | 取消当前用户未完成任务 |
| GET | /api/v2/cloud-downloads/files | 浏览当前用户文件 |
| POST | /api/v2/cloud-downloads/folders | 创建当前用户子目录 |
| PATCH | /api/v2/cloud-downloads/items/{resource_id} | 重命名或移动当前用户资源 |
| DELETE | /api/v2/cloud-downloads/items/{resource_id} | 删除单个已完成私有文件或空目录 |

v2 新下载提交仅接受 `operation_key`、`source`、可选 `expected_name`；Binding、Connection、native_dir_id、ProviderScope 和 Canonical prefix 均由服务端读取。v1 与 v2 对同一用户不得同时开放新写入，避免绕过 UserSpace ACL。

### 13.3 通用接口约束

- 不接收 user_id。
- 不接收 login。
- 不接收 native_dir_id / native_file_id。
- 不接收 ProviderTaskRef / InfoHash。
- 不接收 ProviderScope。
- 不接收任意 BindingID / TargetPath。
- 文件操作以服务端验证后的 IndexCore ResourceID 为入口。
- 写操作使用 operation_key / Idempotency-Key。
- 保留现有严格 JSON、重复字段拒绝、未知字段拒绝、Origin / Host / Session 校验。
- 其他用户资源对当前用户返回统一不可见结果，避免泄露存在性。

## 14. 数据库迁移建议

当前主分支 CurrentSchemaVersion = 21。

如果实施时 main 仍保持 schema21，下一条迁移预计为 22；如果主分支已有新迁移，必须重新确认编号。

建议的追加迁移内容：

1. 新建 cloud_user_spaces。
2. 新建 cloud_space_operations。
3. 为 acquisition_manifests 增加可空 user_space_id。
4. 对新 Manifest 要求 user_space_id 与 user_id、Binding、TargetPath 一致。
5. 保留旧 Manifest 的 NULL user_space_id，不做自动回填。
6. 为账号删除事务增加 UserSpace OWNER_DELETED 状态更新。
7. 为 UserSpace 创建/归属/状态建立唯一索引和并发保护。

严格要求：

- 只追加 migration，不修改历史 migration SQL/checksum。
- 不在进程启动时自动执行生产迁移；由显式运维命令执行。
- 不为历史任务补造 user_space_id。
- 不自动移动、重命名或删除历史 115 文件。
- 不删除历史 Manifest、Job、ProviderTask 和审计记录。
- 新表设计必须兼容账号物理删除后的 owner tombstone；不得对会被物理删除的 product_users 建立级联归属关系。
- migration 前先取得 PostgreSQL 一致性备份并完成隔离恢复演练。
- **必须先解决 schema 兼容发布问题：** 当前 v1.0.3 migrator 只认识 schema21，数据库出现 migration22 时旧二进制会把它视为 UNKNOWN；包含 migration22 的新二进制又会把 schema21 视为 PENDING。不能直接把“新增表/列是 additive”当作旧二进制可回滚。优先先发布明确支持 schema21/schema22 的兼容版本；若不做兼容版本，只能停机并采用整库备份恢复方案，且必须接受恢复点之后的数据损失。
- schema21 -> schema22 只做 expand，不做破坏性字段替换；feature flag 默认关闭，确认兼容版本可运行后再切流。
- 回滚时优先关闭新功能并运行 schema22 兼容版本；不做常规 down migration，不直接切回只认识 schema21 的 v1.0.3。

## 15. 云下载上限和容量策略

### 15.1 每用户累计云下载上限

**目标政策：每个 CloudSite 用户 UUID 累计最多提交 1500 次云下载操作。** 这是每用户累计次数上限，不是并发任务数、全站总数、单个任务文件数、单文件大小或存储容量。

当前 main 的 `docs/PRODUCT-ADMISSION.md` 与 v1.0.2 实现仍是不限累计次数、也不限每用户并发任务；本节记录的是项目所有者要求的目标政策，必须经独立代码 PR、测试和发布后才生效。不得把文档变更描述成线上已经限额。

计数规则：

1. 以不可变 `user_id` 为主体，不以 login 计数；同名新账号因 UUID 不同拥有独立账本。
2. 一个新的、已持久化的 `(user_id, operation_key)` 计 1 次；同一下载任务包含多个文件仍只计 1 次。
3. 相同 operation_key + 相同 intent digest 的幂等重放复用原 Manifest/Job，不重复计数；相同 key 不同意图返回冲突。
4. 状态查询、轮询、Worker claim、任务恢复和相同操作重放不增加次数。
5. 已提交的失败、取消或 UNKNOWN 操作仍计数一次，不自动返还；不得通过重试创建第二份操作意图。
6. 以 `product_operations` 为计数账本，不按所有 Manifest 或物理文件数推算。
7. 已超过 1500 的现有用户保留全部历史记录；禁止新的不同意图，但允许原操作安全重放和历史任务恢复。
8. 超限拒绝必须发生在 Provider 副作用之前，不得创建新 Manifest、Job 或 operation ledger 行。

实现约束：

- 先在用户 quota row 锁内查找幂等重放，再检查 `used >= 1500`，确保达到上限后原操作仍可安全查询/重放。
- 新操作计数与 Manifest、Job 预留和 operation ledger 在同一 PostgreSQL 事务中原子提交。
- 不恢复每用户并发上限；`RECOVERY_REQUIRED` 不能占用独立的并发名额并阻塞其他新提交。
- `operation_limit=1500`、`used`、`remaining=max(0,1500-used)`；`unlimited=false`。若 `concurrency_limit=0` 表示不限并发，DTO 校验必须明确接受该含义，必要时增加 `concurrency_unlimited=true`，不能让字段语义混淆。
- 现有 quota 行中的旧 limit 值不能继续作为有效策略来源；本次统一上限由明确的服务端策略实施。不要为了这个策略单独改写历史 migration 或增加不必要的 schema 迁移。
- 全局队列水位、账号/IP 速率、Provider 预算和公平调度独立于 1500 次累计业务上限。

### 15.2 不实现个人存储容量配额

本功能不实现：

- 每用户 520G。
- 每用户已用容量。
- 每用户剩余容量。
- 每用户存储字节配额。
- 按用户容量拒绝下载。
- 个人容量告警。

115 容量属于共享运营账号级信息，不等于每个 CloudSite 用户的个人容量。

## 16. 安全验收矩阵

必须至少覆盖：

| 场景 | 预期结果 |
|---|---|
| 用户 A 登录并创建空间 | 只创建 A 的目录 |
| 用户 B 登录并创建空间 | 只创建 B 的目录 |
| A 查看自己的任务 | 只显示 A 的 UserSpace 任务 |
| A 查询 B 的 ManifestID | 不可见 |
| A 查询 B 的 ResourceID | 不可见 |
| A 猜测 B 的 CanonicalPath | 不可见 |
| A 通过 whole_root 浏览 | 不能枚举 B 的资源 |
| A 通过 Content / Preview / Range 访问 B | 拒绝 |
| A 使用 B 的分享 token | 拒绝 |
| ADMIN 查询 A 的私有文件 | 默认拒绝 |
| A 修改 login | 保留原 UserSpace |
| A 删除账号 | UserSpace 变为 OWNER_DELETED，文件不自动删除 |
| 同名用户重新注册 | 获得新目录 |
| A 移动文件到 B 目录 | 拒绝 |
| A 移动自己的目录到空间外 | 拒绝 |
| A 删除自己的 UserSpace 根目录 | 拒绝 |
| A 删除非空目录 | 首版拒绝 |
| A 取消自己的下载任务 | 仅取消任务，不删除已完成文件 |
| A 取消 B 的下载任务 | 拒绝 |
| 相同 InfoHash 出现在不同目录 | 必须按 DirId 核验，不能跨用户认领 |
| 115 Mkdir 超时 | 进入 RECONCILE_REQUIRED，不盲目重建 |
| 115 Rename / Move 超时 | 先只读核验，不盲目重复 |
| 115 Delete 成功但 IndexCore 仍 PRESENT | 不能标记 COMPLETED |
| Provider 账号发生变化 | UserSpace 变为 UNAVAILABLE，禁止操作 |
| 登录切换、过期请求、BFCache | 不残留上一用户数据 |

## 17. 测试计划

### 17.1 Domain / Unit

- UserSpace 唯一归属。
- Login rename 不改变空间身份。
- 删除账号后 tombstone 保留。
- 相同 login 新账号不能继承空间。
- Provisioning 并发只允许一个创建意图。
- 未知 Mkdir 结果只读恢复。
- 路径边界和子目录环检测。
- Provider DirId 不匹配时失败关闭。
- operation_key 重放和内容冲突。

### 17.2 Provider Adapter

使用 fake 115driver / injected Backend：

- Mkdir / ListPage / DirName2CID。
- GetFile / Stat / Parents 链。
- Rename / Move / Copy / Delete。
- ListOfflineTask 的 DirId 投影。
- InfoHash + DirId 联合核验。
- DeleteOfflineTasks 必须 deleteFiles=false。
- 不调用 ClearOfflineTasks。
- 不暴露 URL、Cookie、FileID、DirID、PickCode。
- 超时、429、5xx、畸形响应、分页异常。
- SDK 调用串行化和 HTTP timeout。

### 17.3 PostgreSQL Integration

- schema21 到新 migration 的升级。
- 同一用户唯一空间。
- 两用户不能共用 native_dir_id。
- Manifest 与 UserSpace 的原子绑定。
- 用户账号删除与 UserSpace OWNER_DELETED 同事务。
- 历史 Manifest NULL user_space_id 保持不变。
- 操作台账并发、幂等、未知结果。
- 数据库提交失败后的安全恢复。

### 17.4 HTTP / Security

- A/B 两用户全接口交叉访问负向矩阵。
- ADMIN 与匿名用户。
- 伪造 user_id、Binding、Path、Native ID。
- whole_root、cursor、search、counts。
- Content / Preview / HEAD / Range / 302。
- Share Create / Redeem / Open。
- 登录切换和 BFCache。
- strict JSON、Origin、Host、CSRF 和操作频率。

### 17.5 IndexCore Integration

- 目录创建后的 Q8 / Q5。
- 文件移动后的新旧路径确认。
- 重命名后的 CanonicalPath 更新。
- 删除后的资源移除确认。
- READY 必须对应同一个 Manifest、Job 和 Copy。
- IndexCore 不可用时不伪造成功。

### 17.6 真实环境验收

真实 115 写操作必须放在单独授权阶段。

仅使用明确指定的测试用户和测试目录，先做双用户最小闭环：

- A 创建空间并下载一个测试文件。
- B 创建空间并下载另一个测试文件。
- A 只能看到 A。
- B 只能看到 B。
- A 不能读取、移动或删除 B。
- 任务状态、IndexCore Canonical、Copy 和页面结果一致。

本轮文档 PR 不执行上述真实操作，也不开展 20K 规模或长期压力验证。大规模和长期稳定性验证继续安排在真实部署阶段。

## 18. 分阶段开发路线

| 阶段 | 开发范围 | 验收门禁 |
|---|---|---|
| Phase 0：本次 | 当前 main 代码审计、115driver 源码核验、架构审查和文档修订 | 仅文档，不改业务代码 |
| Gate 0：生产基线 | 二进制/schema 证据、备份加密与 ACL、隔离恢复、秘密权限、队列/Provider 快照 | DEP-01/02/03/07 证据闭合；未通过不得迁移 |
| Phase A：1500 上限 | 独立小 PR 恢复每用户累计 1500 次限制；不增加并发上限、不改 schema | 幂等重放、1499/1500/1501、并发提交、超限零副作用、历史超限用户测试 |
| Phase B：兼容与 UserSpace 基础 | schema21/22 兼容发布、UserSpace Domain/Store、追加 migration、账号删除 tombstone、fake tests | feature flag 默认关闭；不调用真实 115；schema 升级/恢复通过 |
| Phase C：后端 ACL 与 v2 API | PrivateResourceAuthorizer、Resources、Library、Content、Preview、Share、v1/v2 兼容 | A/B/ADMIN/匿名负向矩阵通过；旧 v1 不可绕过 |
| Phase D：115 专属目录 | 显式 Provision、Mkdir、只读对账、UserSpace target resolver、Manifest 目标冻结、DirId 核验 | 先 mock；真实 115 探针另行授权 |
| Phase E：只读与普通操作 | 用户任务、只读文件浏览、Mkdir；随后分别开放 Rename、Move | 每项独立验收；IndexCore 新鲜观察通过 |
| Phase F：删除能力 | 单文件删除、空目录删除、操作台账、回收站核验、Q8/Q5 确认 | 独立破坏性操作评审和明确放行 |
| Phase G：发布 | 备份、迁移、受控双用户验收、回滚预案 | 独立部署批准，不自动上线 |

每个阶段必须单独 Issue、分支、PR、CI 和架构审查。Phase E 删除功能必须单独审查；不能因为 Phase D 页面已上线就自动开放删除。

## 19. 未决问题和实施前阻塞项

以下事项不能凭源码推断：

| 编号 | 未决事项 | 处理方式 |
|---|---|---|
| U1 | 当前生产 native parent ID、Binding、Connection、Root 和 canonical prefix 是否仍与历史记录一致 | 实施前只读核验受保护配置与目录身份 |
| U2 | 115 当前账号的稳定身份如何只读确认 | 设计单独身份探针；现有 GetAccountInfo 只读取容量，不足以证明账号身份 |
| U3 | 115 对相同 InfoHash 的跨目录去重行为 | 使用授权测试账号做最小受控验证 |
| U4 | 115 Delete 的真实回收站语义、恢复期限和恢复边界 | 受控测试后再决定是否开放删除/恢复 |
| U5 | 历史 /云下载 未归属文件的访问策略 | Issue #138 与 D-065 均要求整个旧 /云下载 命名空间默认私有；待核验的是历史 Alias/read Binding、旧分享及运营审计的具体封锁/处置路径；新个人页面绝不自动认领 |
| U6 | 用户空间开通触发方式 | 推荐显式点击开通，GET 不产生 115 写副作用 |
| U7 | 首版是否允许选择子目录作为下载目标 | 推荐首版统一保存到个人根目录，后续再开放目标选择 |
| U8 | 是否开放用户空间文件分享 | 推荐首版禁止私有文件分享，后续另行设计 |

### U5 的特别说明

Issue #138 与 D-065 的目标是一致的：整个旧 /云下载 命名空间默认私有，包括没有 owner 映射的历史文件；普通共享库（云下载根之外）保持既有策略。D-065 明确禁止自动认领旧文件。历史 acquisition acceptance 还记录旧 Alias / read Binding 不属于当前活动消费者目录，但仍需实施时核验所有遗留读取入口与分享链接。

因此在正式实施前，必须明确：

- 新的 _users 私有空间只显示已绑定 UserSpace 的文件。
- 没有 UserSpaceID 的历史文件保持 UNCLAIMED/私有，不自动归属任何新用户；历史 Manifest 和 ProviderTask 不自动迁移到新空间。
- 整个旧 /云下载 根及其历史文件必须对普通用户默认不可见；实施时核验并封锁旧 Alias/read Binding、通用资源 API、旧分享和已签发直链的遗留入口。
- 不能用文件名、InfoHash、最近任务、同名 login 或创建时间猜测历史文件归属。

## 20. 非本次范围

- 不为每个用户配置独立 115 Cookie。
- 不开放用户直接调用任意 115driver SDK。
- 不使用 115driver CLI 子进程。
- 不引入 AI Agent 管理网盘文件。
- 不创建第二个 IndexCore Scanner / Canonical Writer。
- 不绕过 OpenList 和 IndexCore 现有职责。
- 不为每个用户创建独立 StorageBinding / IndexCore Root / Worker。
- 不迁移、复制或批量移动历史网盘文件。
- 不自动清理历史下载任务。
- 目标策略恢复每用户累计 1500 次云下载上限；不恢复每用户并发任务上限。
- 不实现每用户容量配额。
- 不在共享公共资源库开放删除。
- 不开放递归/批量删除。
- 不在本轮执行 20K 规模压力测试。
- 不在本轮执行真实 115、生产数据库或生产服务器操作。

## 21. 本次代码审计证据索引

| 结论 | 代码/文档位置 |
|---|---|
| 当前版本与 Schema | VERSION；internal/store/postgres/migrator.go；migrations/0021_library_snapshots.sql |
| 用户 UUID 与 login | internal/auth/auth.go；migrations/0013_product_auth.sql |
| 用户改名与删除 | internal/store/postgres/admin_user_lifecycle.go；internal/store/postgres/admin_user_delete.go |
| 固定下载目标 | internal/producthttp/cloud_download.go |
| Manifest 现有字段 | internal/acquisition/manifest.go |
| Provider Scope 来源 | internal/acquisition/execution_input.go |
| 用户任务 SQL 过滤 | internal/product/reads.go；internal/store/postgres/product_read_repository.go |
| 通用资源浏览与解析 | internal/product/reads.go；internal/producthttp/reads.go |
| 全局下载目录过滤 | internal/product/library.go；internal/libraryview/page.go |
| 前端只是展示分区 | internal/producthttp/web/file-surface.js |
| 文件交付和预览 | internal/fileaccess/access.go；delivery.go；preview_service.go |
| 分享入口 | internal/fileshare/shares.go |
| 当前下载页面 | internal/producthttp/web/downloads.js；downloads.html |
| 当前 115 适配器 | internal/providers/115/adapter.go；driver.go；poll.go；status.go |
| 115 历史路径关系 | docs/CLOUD-ACQUISITION-ACCEPTANCE.md |
| 旧私有空间方案 | docs/PRODUCT-PRIVATE-CLOUD-SPACES.md |
| 当前不限下载次数政策 | docs/PRODUCT-ADMISSION.md；docs/releases/v1.0.2.md |
| 当前开发暂停边界 | AGENTS.md；Issue #139 |

---

## 最终实施原则

**先隔离身份，再隔离目录；先补齐后端 ACL，再开放真实目录操作；先完成可核验的增删改查，再单独开放删除。**

最终验收标准不是“每个用户看到不同的文件夹名称”，而是：

- 115 真实目录不同。
- Manifest 和 Provider 目标不同。
- 任务状态不能跨用户认领。
- 页面文件列表只能查询自己的目录。
- 后端所有资源入口均有用户空间 ACL。
- 文件操作只能在自己的目录树内执行。
- 删除不会影响其他用户和公共文件库。
- IndexCore 对物理变化有新鲜、可验证的观察。
- 任何未知外部结果都不会被伪装成成功或自动重复执行。

本文通过评审后，再单独创建 Phase A 开发任务。
