# CloudSite 私人空间、公共资源库、302-only 交付与一键云下载整改方案

> **Issue：** #199  
> **基线：** main @ 87812596926e73d7ed6184739ef64d56e46f7b4f（2026-11-19 核查）  
> **设计状态：** OWNER-LOCKED / IMPLEMENTATION NOT STARTED  
> **本文件用途：** 架构约束、产品整改方案、分阶段开发指导和验收合同  
> **本次提交范围：** 仅提交设计/指导文档，不改应用代码、不执行 migration、不调用真实 115 文件写操作、不部署生产。

## 0. 产品所有者要求与优先级

本次整改包含四项必须同时考虑的产品要求：

1. **私人空间允许分享。** 私人空间是个人文件管理中心，支持单文件分享、访客访问、下载、有效期、提取码、分享管理和撤销。
2. **下载、视频播放、分享下载永远使用 HTTP 302 直连。** CloudSite 公网服务器不得传输这些流程的文件字节。后续任何页面/API/播放器/分享/重构不得引入代理回退。
3. **公共资源库按类型分区。** 视频、软件、文档等不得再默认混在同一个文件列表中。
4. **云下载改为一次点击。** 用户输入链接后点击“开始下载”，后端自动检查、规范化和截断符合规则的磁力链接；不符合时明确提示，不再要求用户先点“检查链接”再点“提交下载”。

本方案还保留上一轮已提出的私人文件单文件删除要求：二次确认、服务端所有权证明、幂等操作、UNKNOWN 对账和 IndexCore 确认。不开放文件夹/递归/批量删除。

**最高优先级是 302-only 数据交付约束。** UI 体验、分享功能和下载便利性都不能以重新启用公网服务器中转文件字节为代价。

## 1. 当前代码基线与已确认事实

当前 main 基线为 87812596926e73d7ed6184739ef64d56e46f7b4f。已核查以下代码与历史设计：

| 模块 | 当前实现/缺口 | 本次要求 |
|---|---|---|
| internal/fileaccess/delivery.go | 私人路径在某些分支会调用 Open；direct hosts 为空时保留 proxy 交付 | 所有下载/播放/分享必须直连，禁止 fallback |
| internal/producthttp/content.go | writeFile 仍保留 Body/io.CopyN 文件流输出分支 | 受保护文件交付只允许空 body 的 302 |
| internal/fileshare/shares.go | Create、Redeem、Open 对 private path 返回拒绝 | 允许 owner-owned 私人单文件分享，并在每次访问时重新授权 |
| internal/producthttp/share_http.go | 分享访客通过 grant cookie 访问单个分享文件 | 保留 token/grant 模型，交付结果统一为 302 |
| internal/producthttp/web/library.html / file-browser.html | 公共库默认 all 列表，分类作为同页筛选 | 默认分类首页，分类独立路由/列表 |
| internal/producthttp/web/resource-display.js | software/video/document 为独立分类；image/audio/archive 被归入 other | 扩展为七类一致分类 |
| internal/producthttp/web/downloads.html / downloads.js | 用户必须先“检查链接”，再“提交下载” | 只保留一次“开始下载”操作 |
| internal/acquisition/source_prepare.go | 已有 MagnetBTIHNameV1 规范化和严格拒绝规则 | 复用，不另写 JS 解析器 |
| internal/store/postgres/product_admission_repository.go | 新操作的 reservation 过程中已调用 PrepareSource，并持久化 original_source、execution_source、source_policy | 保留此持久化与幂等语义，移除多余的用户手动检查步骤 |
| internal/producthttp/cloud_space.go / product/private_files.go | 已有 UserSpace、provision、owner private file list 基础 | 在此基础上完成独立私人空间页面与访问授权 |

### 1.1 与历史设计的关系

本方案通过 D-069 明确 supersede 以下旧规则，不静默改写历史记录：

- D-052 / docs/PRODUCT-DIRECT-DELIVERY.md 中“direct_link_hosts 为空时允许 proxy 回退”的旧配置说明；
- D-065 / docs/PRODUCT-PRIVATE-CLOUD-SPACES.md 中“私人云下载默认禁止分享”的旧产品限制；
- docs/PRODUCT-FILE-SHARING.md 中仅面向非私人资源的历史分享范围；
- docs/PRODUCT-CLOUDSITE-LIBRARY.md 中图片、音频、压缩包统一归 Other 的旧分类约定。

这些文档仍保留历史事实；当前实现必须以 D-069 和本文件为准。

## 2. D-069：永久 302-only 文件交付架构

### 2.1 不可变约束

**CloudSite 对下载、视频/音频播放、分享下载只做授权和重定向，不传输文件字节。**

该约束覆盖所有用户类型、资源类型和交付入口：

| 场景 | 交付方式 |
|---|---|
| 公共资源普通下载 | HTTP 302 |
| 私人空间普通下载 | HTTP 302 |
| 公共视频/音频播放 | HTTP 302 |
| 私人视频/音频播放 | HTTP 302 |
| 公共文件分享下载 | HTTP 302 |
| 私人文件分享下载 | HTTP 302 |
| 浏览器 GET / HEAD | CloudSite 授权后 HTTP 302 |
| 视频 Range / seek / 续播 | CloudSite 对每次请求授权后 HTTP 302；浏览器随后向 CDN 请求 Range/206 |
| 直链不可用/配置错误 | fail closed，返回安全错误；不允许代理回退 |

成功的 CloudSite 文件交付响应必须满足：

- HTTP 状态为 302 Found；
- Location 仅包含经过校验的临时 HTTPS 上游 URL；
- CloudSite 响应 body 长度为 0；
- Cache-Control: no-store；
- Referrer-Policy: no-referrer；
- 不在 JSON、HTML、DOM、前端日志、服务端日志或审计记录中暴露完整上游 URL；
- 不把 CloudSite session cookie、分享 grant、115/OpenList Authorization 或其他凭据转发到 CDN。

**禁止的行为：**

- 下载/播放/分享路径调用 Source.Open 或 fileaccess.Open 读取整个文件或 Range 字节；
- producthttp 将文件 Body 以 200/206 返回；
- 使用 io.Copy、io.CopyN、反向代理、应用层 relay、Blob 缓冲等方式替代 302；
- direct host allowlist 为空时回退 proxy；
- 视频直连失败时回退普通下载代理；
- 私人路径因为旧 ACL 限制而回退 Open；
- 分享直链失败时回退 CloudSite 文件流；
- 为了“兼容某些浏览器”而增加服务端媒体转码/转发路径。

### 2.2 唯一允许的数据链路

1. 浏览器向 CloudSite 发起受保护的文件请求。
2. CloudSite 验证网站 session，或验证访客分享 grant。
3. 服务端从可信 principal/分享记录中确定 owner、UserSpace、Binding 和目标资源。
4. 服务端正向证明目标是当前可见的精确文件：ResourceID、Canonical Path、Binding/Root、PRESENT、文件大小/类型及私人空间归属一致。
5. LinkSource 只解析该已授权文件的临时上游链接，不发起文件字节 GET。
6. 服务端验证上游 URL：HTTPS、精确 host allowlist、443 端口、无 userinfo、无 fragment、无 IP literal、无 localhost/private/internal host、无控制字符，长度有界。
7. CloudSite 返回空 body 的 302。
8. 浏览器直接向 115/CDN 请求文件字节。视频 Range/seek 和下载续传发生在浏览器与 CDN 之间。

IndexCore 继续负责 Canonical/PRESENT/资源身份真相；CloudSite 负责产品身份、owner ACL、share/grant 和 302；OpenList/115 提供已知文件的直链解析。不得新增扫描器、Canonical writer 或直接读取 IndexCore 私有数据库。

### 2.3 统一配置与 fail-closed

建议统一为一个必需的 direct delivery allowlist，覆盖普通下载、视频/音频、私人资源及所有分享下载。旧的 direct_link_hosts、video_direct_link_hosts、shared_direct_link_hosts 不得继续代表不同的回退策略。

兼容迁移原则：

- 旧配置字段可在迁移期只读解析，但最终必须归一为同一份精确 host 集合；
- 缺少、空值、格式错误或来源能力不满足时，文件交付不可用；
- 不允许“配置没写就先 proxy”；
- 启动时可以保留网站登录、任务查询和文件元数据浏览，但下载/播放/分享交付必须明确返回不可用；
- 不允许在错误提示中泄露上游 URL、provider response、Cookie 或 token。

允许的 host 是精确小写 DNS host，不允许通配符。继续复用当前 host 校验的安全方向：限制数量、拒绝 IP literal、localhost/.local、异常 label 和重复值。上游 URL 只能使用 HTTPS，默认仅 443。

### 2.4 LinkSource 与字节读取接口隔离

建议将两个能力在接口/类型层明确分离：

- LinkSource / ResolveLink：只返回已知文件的临时 URL 与必要的有效期元数据；不得读取文件 body。
- PreviewSource / OpenPreview：仅供明确的服务端文档预览/转换使用。

fileaccess.Deliver 应只走 LinkSource，且无论 public/private/shared/video 都不能调用 Open。可以保留 OpenPreview 给 PDF 页面渲染、文本提取等明确预览路径，但必须确保下载、播放和分享 handler 没有调用路径。

producthttp 的文件交付 writer 应拒绝 DirectURL 为空的成功文件对象；不能再把 Content.Body 当作隐式备用方案。若返回对象没有 DirectURL，应返回安全错误，而不是流式输出。

### 2.5 302 与上游直链撤销边界

CloudSite 撤销分享、删除文件或用户退出，无法自动撤销已经发给浏览器的第三方 bearer URL。已签发 URL 可能在其上游有效期内继续可用。

因此：
- 撤销操作立即阻止后续分享兑换和后续 302；
- 上游临时 URL 必须有可验证的 ExpiresAt；
- 私人分享设计目标：单次签发 URL 的有效窗口不超过 60 秒；
- 60 秒是本方案的目标上限，不是当前 115/OpenList 已验证能力；必须通过真实 provider metadata/隔离验收确认；
- 如果 LinkSource 当前不能给出可信有效期，必须先扩展窄接口或确认上游能力；在证明前私人分享交付 fail closed；
- 不得为了“撤销立即生效”改用 CloudSite 代理，因为那违反 D-069；
- 用户界面和分享说明必须披露已签发 URL 的剩余有效期窗口。

### 2.6 302 架构门禁与测试

以下测试必须成为长期 CI 保护：

| 测试 | 必须证明 |
|---|---|
| 普通文件下载 GET/HEAD | 302、空 body、Location host 合法 |
| 私人文件下载 GET/HEAD | 302、空 body、owner ACL 通过 |
| 视频 GET/Range/seek | 302、零源字节读取，CDN 端返回 206 |
| 公共分享 GET/HEAD/Range | grant 有效后 302 |
| 私人分享 GET/HEAD/Range | owner/UserSpace/share grant 全部有效后 302 |
| 匿名/错误 owner/跨用户 | 不返回 Location，不解析上游 URL |
| share revoked/expired | 不签发新 URL |
| 空 allowlist/host mismatch/非 HTTPS | fail closed，Source.Open 调用次数为 0 |
| provider LinkSource 失败/超时 | fail closed，不流式回退 |
| CSP | media-src 只包含精确允许的 CDN HTTPS hosts |
| URL 泄露 | 响应 JSON、DOM、日志和审计不包含完整签名 URL |
| Range | 302 响应不包含文件 body；浏览器对 CDN 的 Range/206 单独验证 |

必须有架构级保护，防止未来代码绕过 fileaccess.Deliver 直接调用 Open 或在 producthttp 新增文件代理。测试不能只检查当前 UI 按钮；必须覆盖 HTTP handler 和字节 source spy。

## 3. 私人空间产品与分享设计

### 3.1 信息架构

桌面和移动端顶部导航统一为：

- 公共资源
- 私人空间
- 云下载
- 当前账户 / 退出

建议页面职责：

| 页面 | 建议路径 | 责任 |
|---|---|---|
| 公共资源分类首页 | / | 分类入口，不默认展示全量混合列表 |
| 公共分类列表 | /library/{category}/ | 单分类搜索、统计、分页 |
| 私人空间 | /space/ | 当前用户独立 UserSpace 的文件管理 |
| 视频播放 | /player/ | 独立视频/音频播放页面 |
| 云下载 | /downloads/ | 新建下载、任务状态与原操作核实 |
| 分享访问 | /s/{share-token} | 单文件访客分享兑换与访问 |

前端页面路由不是授权边界。无论从哪个 URL、旧页面、直接 API、书签、Range 请求进入，后端都必须重新验证 owner/share。

### 3.2 私人空间文件管理

私人空间只呈现当前用户通过云下载收录并归属其 UserSpace 的文件与目录；它不是整个 115 账号的通用文件浏览器，也不混入公共资源库。\n\n私人空间页面包括：

- 标题、空间状态、面包屑；
- 文件夹浏览；
- 全部/视频/软件/文档/图片/音频/压缩包/其他分类；
- 服务端搜索和分页；
- 卡片/列表视图；
- 文件名、格式、大小、收录状态；
- 视频播放、文件下载、创建分享、删除；
- 我的分享入口及分享状态。

私人空间只展示当前登录用户 UserSpace。不能从公共库整库结果中用前端 path filter 模拟隔离。列表、搜索、详情、内容、预览、分享和删除均由服务端授权。

文件卡片操作建议：

| 文件类别 | 操作 |
|---|---|
| 视频/音频 | 播放、下载、分享、删除 |
| 文档 | 预览、下载、分享、删除 |
| 软件/压缩包/其他 | 下载、分享、删除 |
| 文件夹 | 进入、返回上级；不开放递归删除 |

分享按钮不应被隐藏；分享功能在私人空间是正式能力。

### 3.3 私人单文件分享生命周期

创建：
1. 用户必须已登录，principal.UserID 是唯一 owner 身份。
2. 后端从服务端 UserSpace 映射确认 ACTIVE 空间。
3. 精确解析 Binding、Root、ResourceID、Canonical Path；验证目标为 PRESENT 的单个文件且确实位于当前用户目录树。
4. 不接受用户提供的 native provider file ID、directory ID、任意路径前缀或 owner 字段。
5. 复用现有分享有效期和提取码规则：5m/1h/6h/24h/7d、可选 4 位大小写字母/数字提取码。
6. 持久化 owner UUID、UserSpace ID、Binding、ResourceID、Canonical Path、name/size 快照、token hash、code hash、expiry、revoked 状态。
7. token 只返回一次；token/hash 不出现在 owner 分享列表 DTO、日志和审计正文。

兑换：
1. 访客只能访问一个分享记录，不获得文件库、私人目录或搜索权限。
2. 提取码验证成功后签发独立 grant；grant 使用 HttpOnly、Secure、SameSite=Strict、限定分享路径和到期时间的 cookie。
3. 分享过期、撤销、owner 禁用、UserSpace 非 ACTIVE、目标资源不再 PRESENT 或 identity mismatch 时拒绝。
4. 访客不能通过修改 URL 参数更换 Binding/ResourceID/Path。

下载：
1. 每次 guest GET/HEAD/Range 都重新读取当前 share/grant 状态。
2. 以分享记录的 owner principal 进行私人空间正向授权。
3. 重新验证精确资源身份和文件 size/name 快照。
4. 获取上游临时 URL，验证 allowlist/HTTPS/ExpiresAt。
5. 返回 302 空 body；不经 CloudSite proxy。

撤销：
- Owner 只能列出和撤销自己的分享；
- 撤销立即阻止新的 redeem 和新的 302；
- 已签发 CDN URL 在上游到期前可能仍可用，目标上限 60 秒，必须在 UI 说明；
- 分享撤销不删除原文件。

### 3.4 私人分享的 ACL 规则

必须正向证明 owner 和资源关系，不能只用 path prefix 字符串比较。

每次访问必须确认：
- 经过验证的 website principal / share grant；
- share.owner_user_id 与 UserSpace.owner_user_id 一致；
- UserSpace.status 为 ACTIVE 且 owner 仍有效；
- Binding、Root、UserSpace native/canonical 映射一致；
- ResourceID、Canonical Path、PRESENT、文件类型/大小一致；
- 文件在该用户目录树内，而不是相邻用户目录或公共目录；
- share 仍 active、未过期、未撤销；
- 直链 host 与有效期符合 D-069。

ADMIN 不自动成为所有私人空间的读取者。匿名用户只获得单条分享记录授予的单文件能力。分享记录中的 owner、资源 ID 和路径不可由访客覆盖。

### 3.5 私人文件删除（沿用上一轮需求）

首期只允许单个已完成文件删除；不开放目录、批量、递归删除。

建议状态流：
REQUESTED → AUTHORIZED → PROVIDER_UNKNOWN / PROVIDER_CONFIRMED → AWAITING_CANONICAL → COMPLETED
并保留 BLOCKED / RECONCILE_REQUIRED 等安全终态。

执行顺序：
1. 双重确认、严格 Origin/session、幂等 key；
2. 事务锁定当前用户的精确删除目标；
3. 只读核验 native file ID 和 parent proof 确属当前用户目录树；
4. 撤销该文件所有活动分享，阻止新 302；
5. 通过窄 provider port 执行单文件删除；
6. 外部超时进入 UNKNOWN，只读核验，不盲目重复删除；
7. 发可信 scoped Hint，等待 IndexCore 新鲜观察/Q8 证据；
8. 只有真实确认后才显示删除完成。

已签发的上游直链仍受 D-069 的有效期限制。删除不允许影响其他用户、公共资源、旧任务或历史 share 记录。

## 4. 公共资源库分类整改

### 4.1 页面布局

公共资源首页改为分类入口，不默认渲染全量文件。建议优先突出视频、软件、文档，其他分类作为第二行或次级入口。

每个分类页面有：
- 分类名称、简介和真实分类总数；
- 分类内搜索；
- 卡片/列表切换；
- 服务端分页；
- 文件格式、大小；
- 对应的预览、播放、下载和分享操作（仍受 D-069 约束）。

“全部资源”保留为明确的次级入口，不再是默认首页。分类之间不重复计数。

### 4.2 七类 taxonomy

以当前 ResourceDisplay 的扩展名规则为起点：

| 分类 | 初始扩展名 |
|---|---|
| 软件 | exe、msi、msix、msixbundle、appx、appxbundle、apk、aab、dmg、pkg、deb、rpm、appimage |
| 视频 | mp4、mkv、webm、mov、avi、m4v、mpeg、mpg、wmv、ts、m2ts、flv、3gp、ogv |
| 文档 | pdf、doc、docx、odt、rtf、md、markdown、txt、ppt、pptx、odp、xls、xlsx、ods、csv、epub |
| 图片 | jpg、jpeg、png、gif、webp、avif、bmp、tif、tiff、svg、heic |
| 音频 | mp3、m4a、aac、flac、wav、ogg、opus、wma |
| 压缩包 | zip、7z、rar、tar、gz、bz2、xz、zst、iso |
| 其他 | 未知后缀或未匹配格式 |

分类必须在后端和前端保持同一版本。扩展名只代表展示分类，不代表文件安全性、内容真实性或编码兼容性。压缩包不自动当作软件，未知类型归 Other。

### 4.3 服务端读取规则

- 复用现有 Library Read/Snapshot 读模型和 IndexCore 权威边界。
- category、search、counts、cursor 必须作用于同一完整已发布数据集，而不是仅作用于浏览器已加载的 50 条。
- 每个资源只归属一个 category；category count 之和等于 all count。
- 搜索默认只在当前分类内执行；“全部资源”页才进行跨分类搜索。
- 分类切换保留账户 session，但重置分类 cursor。
- 读取失败保留最近成功结果，不把未知统计显示为 0。
- 公共资源 API 不返回私人 UserSpace 文件；私人文件访问不能依赖 FileSurface 的前端分区。

## 5. 云下载：一次点击、后端自动检查与规范化

### 5.1 新交互

下载页面只保留：
- 下载链接输入框；
- 可选预期文件名；
- 一个主按钮：“开始下载”；
- 账号状态提示；
- 当前任务和历史任务状态；
- 原操作核实入口（仅在 UNKNOWN/待确认时出现）。

删除用户可见的“检查链接”按钮、二次提交按钮和必须确认规范化后链接的流程。允许在提交状态中显示“正在检查链接格式”“已自动清理附加参数”“正在提交”等状态文案，但这些是同一次后台操作的进度，不是第二次用户确认。

### 5.2 复用现有后端规范化

当前已存在：
- internal/acquisition/source_prepare.go 的 PrepareSource；
- MagnetBTIHNameV1 策略；
- internal/store/postgres/product_admission_repository.go 在新 reservation 中调用 PrepareSource；
- product_operations 已持久化 original_source、execution_source、source_policy；
- admission digest 以原始用户意图进行幂等核对。

因此不应另写一套前端解析器或在浏览器做“猜测式截断”。改造重点是把用户交互改为单次提交，并完善后端错误分类和提示。

### 5.3 磁力链接规范化规则

对于 magnet：
- 仅接受当前已支持的 BTIH 语义；
- BTIH 支持当前实现的 40 位 hex 或 32 位 base32；
- 保留唯一且一致的 xt；
- 保留可确认的唯一 dn；
- 清理 tracker 和不影响下载内容的附加参数；
- 对 literal &amp; 尾部，先检查被截断部分是否包含 so、冲突 xt 等会改变下载含义的参数，再按现有规则安全截断；
- 相同 xt/dn 重复项可按现有策略去重；
- 不同 BTIH 或冲突 dn 返回歧义错误；
- so 选择性下载参数明确拒绝，不得删除 so 后变成整包下载；
- BTMH/其他不支持的 topic 明确拒绝；
- 无有效 BTIH、坏百分号编码、fragment、控制字符、异常长度、非 UTF-8 输入明确拒绝；
- 不得将 tracker、任意 URL 或未经确认的 query 拼回执行链接。

对于 HTTP/HTTPS/ed2k：
- 沿用现有 exact policy；
- 不擅自截断 query、token、路径或 fragment；
- 不改变其原始语义；
- 不将 HTTP URL 的 query token 写入日志或错误提示。

### 5.4 用户提示

建议将通用 INVALID_REQUEST 拆为稳定、无敏感数据的错误类别：

| 错误码建议 | 用户提示 |
|---|---|
| SOURCE_INVALID | 链接格式无法识别，请检查是否为完整有效的下载链接 |
| MAGNET_BTIH_MISSING | 磁力链接缺少有效的 BTIH 资源标识 |
| MAGNET_AMBIGUOUS | 磁力链接包含冲突的资源标识或文件名，无法安全判断 |
| MAGNET_UNSUPPORTED | 当前不支持该磁力链接类型或选择性下载参数，请使用标准 BTIH 链接 |
| MAGNET_MALFORMED | 磁力链接参数编码或格式有误 |
| PROVIDER_LOGIN_REQUIRED | 115 登录已失效，本次没有启动新下载 |
| PROVIDER_SESSION_UNKNOWN | 暂时无法确认 115 登录状态，本次没有启动新下载 |
| OPERATION_CONFLICT | 同一操作标识对应的输入不同，请核实原操作 |
| ADMISSION_UNCERTAIN | 提交结果尚未确认，请查询原操作，不要重复新建 |

错误响应不得回显原始 magnet、tracker URL、HTTP signed query 或 provider response。

### 5.5 幂等和副作用顺序

必须保留现有安全顺序：

1. 前端为一次用户意图创建 operation_key，并保存到当前用户会话存储；
2. 发送原始 source 和可选 expected_name；
3. 后端校验请求大小、UTF-8、控制字符和字段；
4. 查询相同 operation_key 的既有操作，精确区分 replay 与 conflict；
5. 仅对新操作调用 PrepareSource；
6. 规范化成功后，原子保存原始输入、执行输入、策略和 durable intent；
7. 通过既有 UserSpace/Binding 固定目标；
8. 115 session health 通过后进入现有 StartDownload 生命周期；
9. 后续 READY 仍必须等待 IndexCore Canonical/Q8/Copy 证据。

无效链接不能创建新 Manifest/Job、不能触发 StartDownload、不能增加新 operation 使用量。已接受操作的重试继续使用原 operation_key 和原始输入；不确定的 provider POST 只允许 GET 核实，不允许浏览器自动重提。

## 6. 建议 API 与页面边界

优先复用现有端点，不为 UI 重排无意义地复制业务逻辑：

| 现有/建议端点 | 责任 |
|---|---|
| GET /api/v2/cloud-downloads/space | 当前用户私人空间状态 |
| POST /api/v2/cloud-downloads/space/provision | 用户主动开通私人空间 |
| GET /api/v2/cloud-downloads/files | 私人空间当前目录分页 |
| POST /api/v2/cloud-downloads | 一键下载的私有 admission |
| GET /api/v1/cloud-downloads/operations/{operation_key} | 原操作 GET-only 核实 |
| GET /api/v1/files/content | 受保护文件内容入口；下载/播放必须 302 |
| POST /api/v1/shares | 当前 owner 创建单文件分享；扩展支持私人资源 |
| GET /api/v1/shares | 当前 owner 的分享列表 |
| POST /api/v1/shares/{id}/revoke | 撤销当前 owner 分享 |
| GET /api/v1/shared/{id}/content | 访客 grant 校验后 302 下载 |

如为私人资源增加专用 content route，必须复用同一 D-069 交付组件和相同 ACL，不能产生第二套允许 proxy 的通道。

## 7. 实施拆分与顺序

避免把所有变更堆成一个不可审查的大 PR。Issue #199 为总控，以下阶段各自独立 PR：

| 阶段 | 范围 | 阻断条件 |
|---|---|---|
| A. D-069 302-only 核心 | 统一 allowlist、取消 proxy fallback、隔离 LinkSource/OpenPreview、空 body writer、架构测试 | 任何文件字节经 CloudSite 返回即阻断 |
| B. 私人分享 | owner/UserSpace/share/grant、访客单文件访问、撤销、302 | 未证明上游 URL TTL 前不开放生产分享 |
| C. 私人删除 | 单文件 durable deletion、native parent proof、UNKNOWN、Q8 | 不允许真实数据破坏性测试或盲重试 |
| D. 公共资源分类 | 七类 taxonomy、分类首页、服务端分类/搜索/统计/分页 | 分类 counts 不一致或私人资源混入即阻断 |
| E. 私人空间与播放器 UI | /space、/player、文件操作、分享列表、移动端 | 不允许仅做静态按钮，必须连真实受保护 API |
| F. 一键云下载 | 删除 UI prepare step，复用 admission PrepareSource，错误码与幂等 | 不允许弱化原始意图 digest、自动 POST 重放或绕过 session health |
| G. 综合验收 | 全链路、桌面/移动端、隔离数据库、真实 302/CDN Range 证据 | 所有硬性约束全部通过 |

推荐先完成 A，再开放 B。D/E/F 可在不改变 302 安全合同的前提下并行开发。

## 8. 验收矩阵

### 8.1 网络流量与 302

- 所有普通下载、私人下载、公共/私人视频播放、公共/私人分享下载均得到 302；
- CloudSite 302 body 为 0 bytes；
- file byte source Open 调用次数为 0；
- 只有 LinkSource metadata resolve；
- CDN 端独立返回 200/206，Range/seek/续传正确；
- 下载完整 SHA256 与源文件一致；
- 空 allowlist、错误 host、过期 URL、provider 失败均返回安全错误；
- 不存在任何 proxy fallback；
- 浏览器 Network 面板显示文件 body 来自 CDN/115 host，而非 CloudSite host。

### 8.2 权限与分享

覆盖 A 用户、B 用户、ADMIN、匿名访客、正确分享访客、错误提取码、过期分享、已撤销分享：
- A 不得列取、预览、播放、下载、分享或删除 B 文件；
- ADMIN 默认不能读取 A/B 私人文件；
- 匿名只能访问分享授予的单文件；
- 不能目录浏览、任意路径、任意 ResourceID 或跨 Binding；
- 分享撤销后新兑换/新 302 被拒绝；
- 已签发 CDN URL 的有效期窗口符合 D-069；
- owner 禁用、UserSpace 关闭、Canonical 不再 PRESENT 后新访问被拒绝。

### 8.3 私人删除

- 只允许单个已完成文件；
- 必须证明 native parent/file identity；
- A 不能删 B；
- 双击/并发提交只有一个 durable operation；
- provider timeout 进入 UNKNOWN，不盲重试；
- 删除完成必须有 IndexCore Q8/Canonical 证据；
- 活动分享被撤销；原始任务和历史审计保留。

### 8.4 公共资源分类

- 每个文件唯一分类；
- 七类 counts 合计等于 all；
- 图片/音频/压缩包不再误入 Other；
- 分类和搜索覆盖完整数据集，不只覆盖已加载页面；
- 分类分页不重复、不漏项；
- 默认首页不显示混合全量列表；
- 公共列表不泄露 UserSpace 私人资源。

### 8.5 一键云下载

- 用户只有一个开始下载按钮；
- 后端自动规范化 magnet；
- 正常 BTIH/dn/tracker 和 literal &amp; 样例符合现有测试规则；
- conflicting xt/dn、so、BTMH、invalid percent、fragment、control chars、超长输入均拒绝并给出明确中文提示；
- HTTP/HTTPS/ed2k 原样保留；
- 无效输入无 Manifest/Job/新 operation/StartDownload；
- 同 operation_key 精确 replay/conflict；
- 网络超时只 GET 核实，不自动重发 POST；
- 115 登录失效仍被现有 session health gate 拦截。

## 9. 不在本次文档授权范围内的事项

- 不修改 IndexCore Kernel/Canonical/Journal 语义；
- 不增加第二个 IndexCore writer、扫描器或独立 OpenList catalog crawler；
- 不读取/写入 IndexCore 私有数据库；
- 不自动认领、搬迁或删除历史 /云下载 文件；
- 不改变历史 Manifest、ProviderTask、UNKNOWN reservation 或旧分享目标；
- 不引入个人 GB/字节容量配额；
- 不修改现有 operation idempotency、115 session health、单 Worker 和 provider uncertain side-effect fence；
- 不在本次文档提交中执行 migration、真实 115 StartDownload、真实 115 Delete、生产配置变更或生产部署。

## 10. 完成定义

本次设计文档合并不代表代码实现、CI 验收或生产上线。只有各阶段实现 PR 通过独立审查、exact-head CI、权限负向测试、302 零字节测试、CDN Range/hash 验证和必要的隔离 PostgreSQL 测试后，才可标记相应阶段完成。

**永久底线：下载、视频播放、分享下载的文件字节永远不能经过 CloudSite 公网服务器。直连不可用就失败，不存在代理兜底。**
