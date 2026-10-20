# CloudSite 私有云下载空间蓝图（D-065 / Issue #138）

> 状态：**拟议设计，尚未实现、尚未架构验收**。蓝图写入 GitHub 并不代表已更改线上服务、网盘文件、数据库或 IndexCore。
>
> 设计基线：CloudSite `main=49355f6f3c7a7164e1e08481056b9ba8fb0a9298`（2026-10-20）；正式编码前重新核对最新 main 与 D-064。关联 [总控 Issue #138](https://github.com/nathanxiangang-web/cloudsite/issues/138)。

## 0. 产品需求和容量决策（用户已明确）

- 登录用户根据规范化登录名获得**真实的 115 网盘专属文件夹**，但不可变的归属标识是 CloudSite `user_id(UUID)`。用户只能列出、搜索、浏览、播放、预览、下载及删除其私有文件。不能查看别人文件；管理员不默认享有普通用户私有内容访问权限，必要维护要单独审计授权。
- 其他 CloudSite 的**普通共享文件库**维持既有策略；本次隔离只针对定义明确的**私有云下载命名空间**，不把普通库误判为私有。
- **忽略用户容量：不实现每用户 520G、用户剩余容量、用户空间统计、用户容量告警、按用户存储字节收费或用户容量前置拒绝。** 现有下载**操作次数及并发**控制（D-060）是独立功能，不应误删。
- 早前所说 **520G** 仅作**同一 115 账号全局容量的参考/展示基准**：若需要配置常量，明确为十进制 520 GB = 520,000,000,000 bytes；它不是已实测的网盘实际额度，也**不是 V1 的硬限制或下载准入门禁**。当 `GetInfo()` 返回不同的真实 `all_total` 时以真实数据标注“提供方实际容量”，520GB 标注“计划参考值”，绝不伪称一致。此项可以在管理员页面后续只读展示；前台不出现个人容量进度条。
- **已核实的 SDK 能力，不等于现网已读成功：** 项目锁定 `github.com/SheltonZhu/115driver v1.3.5`；对应 `Pan115Client.GetInfo()` → `InfoData.SpaceInfo.AllTotal/AllUse/AllRemain.Size`，数据是 int64 字节。该版本还实现 `Mkdir(parentID,name)`、`ListPage(dirID,offset,limit)`、`GetFile(fileID)`、`Stat(fileID)`、`Delete(fileIDs...)`。这些仅经过 SDK 源码确认，**本规划未用线上凭据调用**；错误/登录失效时返回 UNKNOWN，不应拿虚构容量代替。

## 1. 现有真实代码边界

| 现有代码/契约 | 已有能力 | 本项目必须补齐 |
| --- | --- | --- |
| `internal/auth/auth.go`、schema 0013 | 登录名、UUID、session、USER/ADMIN | 不允许由用户名直接决定文件权限；以经过验证的 session user_id 绑定 |
| `internal/producthttp/cloud_download.go` | 服务端固定 `CloudDownloadTarget`，用户无法选目标 | 新请求按 session 取得已持久化的用户专属目标；旧操作冻结原目标 |
| `internal/acquisition/execution_input.go` | `Download.Target.Scope=StorageBinding.ProviderScope`；`Target.Path=Manifest.TargetPath` | 前者是原生 115 离线 `saveDirID`，后者是 IndexCore/Hint 的 canonical 观察路径；不能只拼接用户名改变 `Target.Path` |
| `internal/providers/115/adapter.go`、`driver.go` | StartDownload、状态/取消任务；取消固定 `deleteFiles=false` | 新增隔离的目录管理与**真实文件删除** provider port，不能复用取消离线任务 |
| `internal/product/reads.go`、`fileaccess`、`fileshare` | owner-only 任务；已登录共享 Binding 的 Q4/Q5、预览、302、分享 | 私有目录及子树必须在**服务端**实现所有权权限，不能仅依赖任务 owner |
| `docs/FILE-SURFACES.md` / D-059 | `/downloads` 与主库的前端展示分区 | 明确原有 FileSurface 是 presentation-only；本蓝图新增后端 ACL |
| `index-core` | OpenList Collector→Canonical/Journal→Q4/Q5/Q6/Q8 | 仍唯一认定物理可见与删除；CloudSite 不重写扫描、不能直接写 Canonical/Journal |

已有线上账户、schema15、受保护 115 原生会话、已知 ProviderTask、历史 8 项 unknown reservations、旧共享文件和现有 302/文档预览必须保留；不得为升级而补造旧用户归属或重启历史任务。

## 2. 原生真实目录与身份模型

运营人员确认唯一上级目录（历史原生 `/CloudSite/云下载` 与 canonical `/云下载` 关系必须复核），再在其下创建 **`_users`** 作为专属目录容器。实际布局示意：

```text
115: /CloudSite/云下载/_users/
  zhangsan__<full-user-uuid>/
    A.mp4
    torrent-folder/...
  lisi__<full-user-uuid>/
    B.zip

IndexCore canonical: /云下载/_users/zhangsan__<full-user-uuid>/...
```

- 人可读目录名由当时已规范化的 ASCII `login` + **完整 UUID** 组成；不使用用户提供的任意路径片段，也不能拿昵称、展示名称、相同登录名或哈希前缀作为安全边界。用户名变更以后保留已有目录 ID 与原文件夹名，展示别名可变，不能创建第二个空间。
- 持久化唯一 `(user_id, binding_id, provider identity, native_dir_id, canonical_prefix)` 关联；同一已认证用户多端并发只能得一个已确认空间。原生 ID、provider credential/ref 不返回浏览器。
- `_users` 上级原生目录、对应 canonical 根及账号身份必须由受保护配置和**只读证据**确认。若 provider 把目录移动/重命名或账号切换，空间标记 `UNAVAILABLE/RECONCILE_REQUIRED`，禁止推断所有权和提交下载。
- 新空间生命周期：`PENDING_CREATE → CREATING → ACTIVE`，未知外部结果进入 `RECONCILE_REQUIRED`；只有同一父目录下唯一名称、预期 native ID、数据库映射一致后才能 ACTIVE。外部 `Mkdir` 返回超时不得盲目再次创建；并发过程需要数据库租约/CAS 与后台唯一目录匹配。多匹配→阻塞人工处理，不自动删除或领用。
- 用户被禁用、撤销 session、文件移动出目录时立即失去私有访问权限，不因先前拥有路径字符串保留访问权。

## 3. 模块边界与建议持久化模型

本节是提议字段，**不是可直接执行的 migration 或已存在 schema**。采用下一未占用编号的**追加迁移**；不修改旧 SQL/校验和，Windows/LF 打包门禁必须通过。

`cloud_user_spaces`（owner-space）：
- `user_id uuid` 外键 `product_users`，`binding_id uuid`、`provider_connection_id uuid`（或受保护身份快照）、`canonical_root_id`。
- `native_parent_id`、`native_dir_id`、`canonical_prefix`、`directory_name`、`status`、`version`、`operation_nonce`、`updated_at`、`last_verified_at`。
- 强唯一约束确保同用户唯一活动空间、同一 native directory 不能分给两个不同用户、相同 canonical prefix 不能重复领用。目录创建不可放在长期 SQL 事务内；使用持久化意图、幂等只读对账、短事务状态变更。

`cloud_private_deletions`（未来删除 gate）：
- `operation_id`、`actor_user_id`、`space_id`、`binding_id`、`canonical_resource_id`、`canonical_path`、`provider_file_id`（仅受保护存储）及原生父目录证明摘要、`idempotency_key`、`status`、`attempt_count`、`created_at/updated_at`、脱敏错误类别。
- 状态：`REQUESTED → AUTHORIZED → PROVIDER_UNKNOWN/PROVIDER_CONFIRMED → AWAITING_CANONICAL → COMPLETED`，也可能 `BLOCKED/FAILED/RECONCILE_REQUIRED`。不可把外部请求发出后超时直接记失败且无副作用，也不可把 HTTP200 记为删除完成。
- 详细实施时审查是否需要独立 append-only audit 表；绝不把 provider ID 暴露给客户端，也不让用户指定 DB 中的任意行进行批量物理删除。

建议 provider-neutral 端口：`AccountSpaceReader`（仅 account total/used/remain，只读）；`PrivateDirectoryPort`（受保护父目录下创建/查验）；`PrivateFilePort`（检查真实文件/原生父子关系、单文件删除）。115 特定调用与错误分类仍在 provider adapter 下；product 层只有身份、结果状态与 opaque 引用，不能直接 import 115driver。

## 4. 隐私威胁模型：所有入口统一后端鉴权

定义 `PrivateResourceAuthorizer`，输入是服务端认证 principal、Binding/Root、Canonical resource ID + 路径、空间 DB 映射；对目标私有范围及其任意深度子节点做**正向所有权证明**，不能用 `strings.HasPrefix` 裸前缀而跨越 `/user` / `/user2`。文件移动、Root/Binding 更换、identity 歧义、信息缺失一律 404/403/503 中适当的 fail-closed 错误，不能返回别人的路径、名称、大小、封面或计数。

必须包含：
1. `GET /api/v1/resources` 的 Q4、`whole_root=true` Q6、cursor、按 parent_resource_id 枚举；**在服务端鉴权后再生成响应**，分页不能通过被排除记录/cursor 泄露信息。必要时仅提供独立私有空间资源端点，彻底关闭普通用户对 `/云下载` 私有子树的原始 whole_root 访问。
2. `GET /api/v1/resources/resolve`、`/api/v1/files/content`、`/api/v1/files/preview`、GET/HEAD、Range、媒体播放和 302 重定向前，都重新确认当前所有权、Canonical `PRESENT` 与确切 ResourceID/Path/Binding；猜中别人的资源 ID 也不准访问。
3. 搜索、任务 READY 跳转、后台返回 Copy/Manifest 身份映射、静态页面初始化、登录过期/切换账号/BFCache 与过期请求回调。现有任务 owner-only 约束必须保留；READY 不能跨用户借用相同 provider info_hash。
4. 现有 `fileshare` 的生成、列表和**匿名 redeem/grant**：私有云下载默认禁止新分享，旧链路必须识别私有目标并拒绝兑换/下载；不能靠创建时禁用却让历史 code/token 继续直接绕过 ACL。
5. 对外直达 CDN 的 302：**已签发的外部 URL 可能在有效期内不可撤回**，无论 CloudSite 是否注销或删除。对私有云文件须先验证直链有效期/撤销能力，否则禁用该路径并使用受控代理或其他不破坏私有性的交付方式。切勿承诺能吊销已外发的第三方 URL。
6. 老共享 `/云下载` 资源是 **UNCLAIMED**，不因同名、最近任务、hash、曾经登录用户被自动认领。单独列运营待归属清单（受控）；不搬运、不删除、不让新用户直接浏览。

普通用户只能使用自己的私有空间；`ADMIN` 不是跨用户资源读取万能钥匙。必要例外必须是额外明确的审计化运营权限，不是用户可自选角色。

## 5. 下载提交、同哈希冲突与恢复

```text
CloudSite verified session (user_id)
  -> validate admission & existing operation limits
  -> UserSpace ACTIVE (native_dir_id + canonical_prefix) server-only
  -> durable frozen Manifest target/opaque provider destination identity
  -> existing single Worker + 115 StartDownload(saveDirID=native_dir_id)
  -> 115 task poll / untrusted result-name
  -> Hint(root_id, canonical user subdirectory scope)
  -> IndexCore OpenList scoped observation + Canonical/Q8
  -> exact Q5/Journal/Copy confirmation in this UserSpace
  -> owner-only READY/files
```

- 对新的 Manifest 以一次性、不可变绑定记录 native saveDirID 与 canonical target。当前 `ExecutionInputResolver` 只有统一 `binding.ProviderScope`；这是需要正式调整的**协议缺口**，不能在前端接一个用户名字符串就当作实现。
- 产品认证 user_id 必须和 operation/Manifest/Job ownership 一致；重试/恢复时读取原冻结目标而非重新计算当前 login 或配置。保持历史操作和 8 项 unknown reservations 不动。
- 一个 115 账号下相同 magnet/info_hash 可能是全局离线任务。跨用户相同源提交时，不准因为已经有该任务就认定第二人的目标也有文件，更不能让 A 删除 B 的实际 Copy。若原生 115 不支持等价隔离语义，则明确 `DESTINATION_UNCERTAIN/BLOCKED`，通过经过授权且可验证的 provider Copy/独立目录存在性流程另行设计，禁止猜测。
- `GetInfo` 只是账号级独立只读能力；网络超时或源字段为空显示 `UNKNOWN`。**不对新下载执行个人剩余字节检查或 520GB 拦截**；保留现有 D-060 操作次数/并发配额。

## 6. 删除（最后单独放行，绝不随首版目录变更自动开启）

首版只支持**单个已完成的私有文件**，暂不开放文件夹/批量/递归删除。若想优先回收站恢复，必须先验证 115 `Delete` 的真实语义和恢复接口，不能假定一定可恢复；真实删除/移回收站均需单独明确授权。

执行序列：
1. 用户双重确认、严格 Origin/session、幂等 key，限制请求体与频率；
2. 事务读取删除对象、`user_id`、空间、Exact IndexCore ResourceID/Binding/path，锁定同一删除操作防重复；
3. 调用窄 provider **只读**文件 `GetFile`/parent-proof，检查 exact native file ID 位于已确认用户 native 目录树内，非其他用户目录/运营目录；不允许随意 ID、只凭路径、只凭 hash 或客户端参数删除；
4. 通过专用 provider 受限文件删除接口执行 **单目标** 外部写；外部超时进入 UNKNOWN 并只读对账，不盲目再次删除；
5. 发可信的 scoped Hint 并等待 IndexCore 新鲜可靠的观测/对应 Q8 transition，再将操作变为 `COMPLETED`；既有 canonical 文件消失并非可直接猜测的 115 删除成功；
6. 审计谁、何时、哪个用户空间/Canonical 身份、外部结果状态、最终证据；无法确定时 `RECONCILE_REQUIRED`，保留运营处理入口。

正在下载/共享被引用/原生位置已经变化的项目须拒绝或者进入显式待处理；私有删除绝不能影响普通公共库、其他人的目录或旧 provider task history。

## 7. 交付门禁（每阶段单独 Issue → 分支 → PR → 架构审查）

| 阶段 | 可交付范围 | 必须证明 |
| --- | --- | --- |
| A. 身份空间基础（首先实施） | PostgreSQL 追加 schema、UserSpace 生命周期与端口、模拟目录/账号空间能力 | username rename、UUID ownership、native/canonical 唯一性、事务/并发、未知 mkdir、无用户容量字段、原 schema 15 兼容；**本阶段无生产实际 mkdir** |
| B. 服务端 ACL（先于新用户真正下载） | private namespace 后端全路由权限门禁；拒绝共享/guest 绕过 | 用户 A/B/匿名/ADMIN 跨账号负向矩阵、Q6/Q5、Content/Preview/Range/302、分享和登录切换不泄漏 |
| C. 原生文件夹及固定目标 | 受控 115 创建/对账、每新 Manifest 冻结 native 目录与 canonical path；原 Worker 复用 | 真实目录 ID、双用户并发、重复 hash、UNKNOWN 恢复和 Q5/Q8 READY；先 isolated mock，再单次受控 provider 探针 |
| D. 真正删除 | 单文件 durable deletion ledger、受控 provider 删除、IndexCore Q8 回执与操作状态 | A 不能删 B、双提交、外部超时、父目录更换、回滚/历史 share、真实数据删除前再次授权 |
| E. 发布验收 | 线上前预检、备份、逐阶段切换、回滚、跨用户 E2E | 旧库/现有 302/预览和旧任务不回归；新文件不跨用户，失败不伪装空目录或成功 |

**范围和安全禁止事项：** 不新建 IndexCore 扫描器/写入器，不读取或直接修改 IndexCore/PostgreSQL 私表，不改历史 provider reservation，不自动大规模搬迁 `/云下载`，不对现网 115 批量列取、创建目录、启动下载或删除，不在这个蓝图 PR 中执行 schema migration，也不启动 AI Gate5。

## 8. 首轮实施任务建议

本蓝图接受后仅授权 **A 阶段**的独立 Codex PR：产品 `UserSpace` 类型/Store/持久化表/抽象 provider 端口和纯测试 + 现有 API 兼容性测试，不增加可以访问/更改真实网盘的公共入口。GitHub Issue #138 是总控；A、B、C、D、E 应以单独有界任务逐阶段创建，未经架构审查的分支不得合并。

### 合规文档引用

- `docs/AI-ARCHITECTURE-MEMORY.md` / `PROJECT-STATE.md`（实际当前状态）
- `docs/PRODUCT-AUTH.md` / `docs/PRODUCT-ADMISSION.md` / `docs/PRODUCT-CLOUD-RUNTIME.md`
- `docs/FILE-SURFACES.md`、`docs/PRODUCT-DIRECT-DELIVERY.md`、`docs/PRODUCT-FILE-SHARING.md`
- `PROJECT-CONTEXT.md`（provider vs IndexCore 权威边界）
- `docs/DECISIONS.md` 的 D-058/D-059/D-060/D-064 与拟议 D-065
