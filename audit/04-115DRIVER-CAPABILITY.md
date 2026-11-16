# 04 115driver 接口能力与风险矩阵

## 1. 基线与依赖差异

- 上游审计 HEAD：`1cfb0d6e43e6c6487d53a7977bff58f914983514`（2026-11-04）。
- 最新 release/tag 仍为 `v1.3.5`，提交 `d89999540a6d35838582ac06e31f81312d1f66fd`（2026-08-01）。
- CloudSite `go.mod:5-8` 和 `go.sum` 固定 `v1.3.5`，没有 replace。
- v1.3.5..HEAD 包含：FileParentInfo string file_id 兼容、字符串 error 表面化、ShareFile.Duration、加密 app 分享下载、>5GiB CLI 分片修复和分片重试/worker 退出修复。
- 这些上游变化**没有修复**离线添加多结果越界、统一 429/UNKNOWN、多账号竞争或断点恢复；仅升级 HEAD 不足以获得安全文件管理。

未运行任何 115driver 测试。`Makefile:36-46` 已区分入口：`make test` 使用 `UNIT_PKGS` 并排除 `pkg/driver`，`make test-integration` 独立测试 `pkg/driver` 且强制要求真实 `COOKIE`，`make test-all` 组合两者。直接 `go test ./...` 或 `go test ./pkg/driver` 没有 Go build-tag 防护；`driver_test.go` 中多个测试直接访问真实 115，并包含创建目录、上传、离线下载、删除和清空任务，配置真实 Cookie 时可产生危险写行为。下文“已实现”仅指源码调用面存在，所有线上协议均“需要真实接口验证”。

## 2. 能力矩阵

| 能力 | 状态 | 实际实现与主要限制 |
|---|---|---|
| Cookie 登录/检查 | 部分/有限制 | CookieCheck/LoginCheck/GetUser、QR 登录存在；无过期模型、主动刷新、会话 generation；LoginCheck 可能有登出其他设备副作用 |
| 多账号 | 部分 | CLI profile/独立 client 可隔离；同 client 重导入 Cookie 会叠加，无账号请求作用域 |
| 并发请求 | 有限制 | 共享 `Request/UserID/Userkey/UploadMetaInfo`，不应声明共享 client 安全 |
| 目录列表 | 部分 | `/files` offset/limit、单目录遍历和排序表面；offset 按请求 limit 推进，可能跳项/死循环，CID mismatch 可假空成功 |
| 文件搜索 | 部分 | `/files/search` 单页、多筛选；总 `Count` 有传递，但 `FileCount/FolderCount` 固定为 0；无 SearchAll，offset 无稳定快照，完整分页与计数一致性未验证 |
| 新建目录 | 已实现表面/有限制 | `/files/add` 返回 CategoryID；无父类型/空 ID 校验和 UNKNOWN 后按 parent+name 对账 |
| 上传文件 | 部分 | 闪传、OSS、分片、每片最多 5 次和 token 刷新；无跨进程 checkpoint/resume/Abort/Complete 对账，9GiB 边界 panic |
| 下载文件 | 部分/高风险 | 取链存在；无统一有效期/刷新/Range/内容校验；Header 可能带长期 Cookie 到 CDN，Get 全量读内存且不查状态 |
| 移动文件 | 已实现表面/有限制 | `/files/move` 批量 BasicResp；无源/目标预检、逐项结果、读回或 UNKNOWN 对账 |
| 复制文件 | 已实现表面/有限制 | `/files/copy`；不返回新 ID，无法可靠建立 source→destination 映射 |
| 重命名 | 已实现表面/有限制 | `/files/batch_rename`；单 ID，无最终名称读回和冲突部分结果 |
| 删除文件 | 已实现表面/有限制 | `/rb/delete` 入回收站；整体 ACK，无逐项/读回，空参数语义不一致 |
| 删除目录 | 有限制/高风险 | 递归由服务端决定；客户端无影响统计、根保护、递归显式授权 |
| 回收站/恢复/清理 | 部分 | `/rb`、`/rb/revert`、`/rb/clean`；列表单页，rid 与原 fid 的映射未由已审计 fixture 证明、不得互换假定，清理需密码且空 RID 无 driver 防护 |
| 文件详情 | 部分 | category/get_info/index_info；Stat Err 永远 nil，GetFile 空 data 可返回零值对象，GetPath 固定空 |
| 批量操作 | 部分 | Move/Copy/Delete/Restore/Clean/Offline 接受多项；无固定分批、逐项结果和 notAttempted |
| 离线下载 | 部分 | add/list/delete/clear、状态映射；内部 BasicResp 未查，部分成功不可表达，多结果 panic，删除可连带文件 |
| 分享功能 | 部分 | 分享快照和分享文件下载；截至 HEAD 公开方法表面未发现创建、撤销、有效期/提取码管理、分享列表管理 |

## 3. 风险清单

### DR-01 — P0 — 下载 Header 可能把长期 Cookie 发到 CDN/其他源

- 类型：FACT（Header 复制链）；实际下载 host 和线上凭证接收情况 UNKNOWN。
- 位置：`pkg/driver/download.go:100-105,149,161-165,173-195`、`DownloadInfo.Get:48-63`；CLI/MCP 也复用。
- 数据/API：`proapi.115.com/...downurl` → 返回下载 URL；无 SQL。
- 触发：取链 RawRequest 含账号 Cookie，`buildDownloadHeaders` 克隆后用于不同 host。
- 影响：账号凭证跨源暴露。CloudSite 当前未调用此方法，不能声称其产品已泄露。
- 建议：仅允许 UA 和明确必要的短时下载令牌，禁止继承账号 Cookie/Authorization/表单 Header。
- 难度：中。
- 测试：mock 取链带 UID/CID/SEID/KID，CDN 为不同 host，断言只收到 allowlist Header。

### DR-02 — P0（配置触发）— 可选 HTTP/任意 API URL 与 client Cookie 组合

- 类型：FACT（配置与 Resty 行为）；被非默认配置利用条件。
- 位置：`pkg/driver/api.go:21-25`、`option.go:86-90,223-229`；Resty `middleware.go:246-256`。
- 数据/API：`WithMultiUrls()` 启用 `http://web.api.115.com/files`；自定义 URL。
- 触发：启用 HTTP 或指向非预期 host，client 级 Cookie 被加入请求。
- 影响：凭证明文传输或跨源泄露。
- 建议：删除 HTTP 端点、限制 scheme/host、按请求和 host 注入凭证。
- 难度：小。
- 测试：捕获 HTTP/跨 host 请求，断言无 Cookie；默认 HTTPS 回归。

### DR-03 — P1 — debug 明文记录 Cookie、密码和登录响应

- 类型：FACT（开启 debug 时）。
- 位置：`client.go:163-165`、`option.go:41-45`、`driver_test.go:40-44`；Resty middleware 记录 Header/Body。
- 数据/API：QR Credential、回收站 password、分享提取码 Referer。
- 触发：SetDebug(true)，真实测试默认开启。
- 影响：凭证进入日志。
- 建议：默认禁 Header/Body，注册脱敏 logger，测试禁止真实凭证 debug。
- 难度：小到中。
- 测试：用金丝雀 Cookie/password/code 扫描全部日志。

### DR-04 — P1 — 共享 client 存在数据竞争和账号语义混乱

- 类型：FACT。
- 位置：`client.go:19-28,178-187`；`upload.go:43-66`、`offline.go:98-104`；MCP `server/server.go:89-126` 共享实例。
- 数据/API：所有请求，上传/离线账号字段。
- 触发：并发列表、上传初始化、离线添加，或外部 GetRequest。
- 影响：Request 竞争确定；账号字段可能用于错误身份，实际串请求程度不作夸大。
- 建议：实例不可变、每账号一个 session、移除 Request 字段、方法接收 context。
- 难度：大。
- 测试：全 mock `go test -race`，两个账号交错检查请求身份。

### DR-05 — P1 — 重复导入凭据不会替换旧 Cookie

- 类型：FACT。
- 位置：`client.go:158-160`、`login.go:42-84`；Resty SetCookies append。
- 触发：同 client A→B→A 切换。
- 影响：新旧同名 Cookie 同时发送，服务端采用哪一个 UNKNOWN，账号不确定。
- 建议：凭据不可变；重认证原子替换 session 和 Jar。
- 难度：中。
- 测试：A/B/A 捕获 Cookie，断言仅当前账号。

### DR-06 — P1 — 429/5xx/网络错误无统一分类，认证检查可误报

- 类型：FACT。
- 位置：`error.go:139-185`、`login.go:13-24`、CLI `cmd/root.go:61-63`。
- 数据/API：HTTP status、provider errno/string error。
- 触发：DNS、超时、429、非 2xx；顶层 string error 可覆盖结构化 errno。
- 影响：暂时故障被当 bad cookie/重新登录；缺少 Retry-After 和结果语义。
- 建议：typed error + HTTP/provider code 双层映射，禁止按消息文本分类。
- 难度：大。
- 测试：401/403/408/429/500/503、HTML/JSON、Retry-After、DNS/超时矩阵。

### DR-07 — P1 — 写请求 UNKNOWN 无对账，注入 Resty retry 可盲重放

- 类型：FACT。
- 位置：Mkdir/Move/Copy/Rename/Delete/Offline/Recycle 写方法；`option.go:31-38` 接受外部 Resty。
- 数据/API：所有 Provider mutation。
- 触发：服务端提交后断连；调用方注入开启 retry 的 Resty client。
- 影响：重复创建/复制/离线任务，或对移动/删除盲目重试。
- 建议：默认 RetryCount=0，Adapter 统一拥有重试；已发出写请求 UNKNOWN→reconcile→retry/no-op。
- 难度：大。
- 测试：服务端提交后断连，验证零盲目第二次写。

### DR-08 — P1 — 分片上传无跨失败恢复，Complete 无对账/Abort

- 类型：FACT。
- 位置：`upload.go:362-469`。
- 数据/API：OSS UploadID/parts/token、115 callback。
- 触发：进程失败、token 过期、Complete 后断连。
- 影响：checkpoint 丢失、未完成分片遗留、最终状态不可知。
- 建议：持久化 UploadID/object/SHA1/chunk planner/ETag，ListUploadedParts、Abort、Complete UNKNOWN 对账。
- 难度：大。
- 测试：第 N 片失败、token 过期、Complete 断连、重启恢复。

### DR-09 — P1 — 恰好 9 GiB 的非闪传输入可 panic

- 类型：FACT（边界代码）；需先进入非闪传分支。
- 位置：`upload.go:471-491` chunk planner。
- 触发：`fileSize == 9*GB`，循环不生成 chunks，后续访问 chunks[0]。
- 影响：进程 panic。
- 建议：抽纯函数覆盖所有 GiB 边界并修正规划。
- 难度：小。
- 测试：无需真实 9GiB 文件，测试 5/9GiB、9GiB±1、>9GiB。

### DR-10 — P1 — 离线添加不能表达部分成功且可多结果越界

- 类型：FACT；对应 CloudSite CS-01。
- 位置：`offline.go:108-152`。
- 数据/API：`OfflineAddUrlResponse.BasicResp/Result`。
- 触发：内部 state=false；返回 hash 数少/多于 URI；重复/空 hash。
- 影响：空 hash 或 panic，部分成功不可见。
- 建议：检查内部状态、精确数量和 URI 映射，返回逐项 Outcome，未知时查任务列表对账。
- 难度：中。
- 测试：0/少/多/重复/空结果、单项失败、提交后超时。

### DR-11 — P1 — 列表分页可跳项、死循环或把错误 scope 当空目录

- 类型：FACT。
- 位置：`dir.go:38-76,109-143`。
- 触发：服务端降页大小仍按请求 limit 推进；limit<=0；CID mismatch 返回空响应+nil error。
- 影响：漏文件、无限请求、无效 scope 被误当空。
- 建议：校验 limit，按实际进度推进，检测无进展/重复页，CID mismatch 返回 INVALID_SCOPE。
- 难度：中。
- 测试：页大小缩减、空页、重复页、Count 变化、0/-1、CID mismatch。

### DR-12 — P1 — 文件详情可假成功

- 类型：FACT。
- 位置：`response.go:283-285` `FileStatResponse.Err` 永远 nil；`op.go:166-183` GetFile。
- 触发：`state:false,errno` 或空 data。
- 影响：零值详情、空 ID 且 IsDirectory=true 的假对象。
- 建议：检查状态/HTTP/必要字段，空数据 INVALID_SCOPE。
- 难度：小到中。
- 测试：state false、errno=99、429、data=[]、缺 fid/cid。

### DR-13 — P1（潜在 P0 影响）— 根删除和空 RID 清理缺客户端防爆半径

- 类型：FACT（无本地保护）；服务端是否允许根删/空清理 UNKNOWN。
- 位置：`pkg/driver/op.go:9-26`、`cli/internal/resolver/resolver.go:85-88`、`cli/cmd/rm.go:24-45`、`pkg/driver/recylcle.go:9-23`。
- 触发：`Delete("0")` 或 `CleanRecycleBin(password)` 空 RID。
- 影响：若服务端解释为递归根删/清空，影响不可逆；不得声称已复现。
- 建议：拒绝空/根/未解析 ID；目录删除需 Recursive、scope 预检、影响预览和二次确认；清空独立方法。
- 难度：小。
- 测试：危险输入必须零 HTTP；覆盖 driver/CLI/MCP 和部分成功 fixture。

### DR-14 — P2 — WithAsc 写错字段

- 位置：`option.go:119-125` 修改 showDir 而非 asc；上传核验也受影响。
- 测试：直接检查 query `asc/show_dir` 和上传核验页顺序；难度小。

### DR-15 — P2 — 上传成功核验过弱

- 位置：`upload.go:154-172,456-469`。
- 触发/影响：普通 OSS 只在前 500 项按 SHA1 匹配，可能选中同内容旧文件；分片只查 callback BasicResp。
- 建议：核验 FileID/parent/name/size/SHA1，不能仅按内容猜身份；难度中到大。
- 测试：同 SHA1 不同名、超过 500 项、重复文件、callback 后不可见。

### DR-16 — P2 — 下载无有效期/恢复模型且 Get 全量读内存

- 位置：`download.go:39-63,68-152`。
- 触发/影响：403 错误页可当内容、大文件 OOM、URL 过期无刷新，无 Range/ETag/SHA1 核验。
- 建议：DownloadLease、有界 streaming、过期刷新一次、Range+ETag、长度/SHA1 校验。
- 难度：中到大；测试 403/429/HTML、大 body、Range 恢复和内容不匹配。

### DR-17 — P2 — 搜索、分享、批量语义不完整

- 位置：`search.go:161-170` 传递总 `Count`，但 `FileCount/FolderCount` 固定为 0；`share.go` 仅快照；`op.go:9-95` 整体 BasicResp。
- 影响：文件/目录分类计数未实现，完整分页与计数一致性未验证；无法创建/撤销分享，不能表达逐项失败和新 ID 映射。
- 建议：Page/BatchResult/Share capability 分离，不把多个 ID 参数当可靠批量；难度中到大。
- 测试：分页及总数/分类计数一致性、逐项失败、复制新 ID、分享能力矩阵。

### DR-18 — P3 — 其他确定性缺陷

- 位置：`app_version.go:28-36` 预分配后 append/类型断言；`cli/cmd/mkdir.go:74-83` 用 `err == ErrExist`；`login.go:93-118` Cookie Split 和验证前改写接收者。
- 影响：空版本项、竞争创建失败、含 `=` Cookie 解析失败或 Credential 部分修改。
- 建议：修正 append、使用 `errors.Is`、Cookie 解析不可变且用 SplitN；难度小。
- 测试：版本 fixture、包装 ErrExist、Cookie fuzz。

## 4. 认证与凭证结论

- QR 登录表面完整，但 QRCodeLoginWithApp 不自动 Import，也不设置当前 UserID。
- Cookie expires/issued-at/refreshed-at、主动刷新、logout、session generation 均未实现。
- CLI profile 文件权限 0700/0600 是良好起点；MCP `--cookie` 命令行不是 CloudSite 应复用的凭证注入方式。
- `isCalledByAlistV3` 通过调用栈和函数名禁用功能（`util.go:34-42`），不是稳定授权边界。

## 5. 可靠 Provider Adapter 的最低契约

使用大写下划线 typed error：`AUTH_OR_PERMISSION`、`THROTTLED`、`TRANSIENT_PROVIDER`、`SCOPE_TOO_LARGE`、`INVALID_SCOPE`、`CONFIG_INVALID`、`ROOT_INACTIVE`、`INTERNAL`。不得用 `strings.Contains(err.Error(), "403")` 或中文消息判类。

```go
type Outcome string
const (
    OutcomeApplied    Outcome = "APPLIED"
    OutcomeNotApplied Outcome = "NOT_APPLIED"
    OutcomeUnknown    Outcome = "UNKNOWN"
    OutcomePartial    Outcome = "PARTIAL"
)

type ProviderError struct {
    Kind         ErrorKind
    Op           string
    AccountID    string
    ScopeID      string
    ProviderCode string
    HTTPStatus   int
    RetryAfter   time.Duration
    Outcome      Outcome
    Cause        error
}
```

强制规则：

1. 默认禁用 Resty 自动重试，Adapter 统一拥有重试、预算、context 和时钟。
2. 读请求可有界退避；已发出的写请求 UNKNOWN 后先对账。
3. 所有批量返回 `succeeded/failed/notAttempted` 和每项 verified/result_id，整体 ACK 不等于全成功。
4. Mkdir 按 parent+name 对账；Move/Rename 验证目标和源；Copy 无唯一新 ID 证据时 UNKNOWN，不猜。
5. Upload checkpoint 绑定账号、目录、文件名、SHA1、大小、UploadID、parts 和 token expiry。
6. Download 使用 Header allowlist、短期 lease、流式写入、Range/ETag 和最终长度/SHA1。
7. Delete/Restore/Clean 必须考虑回收站与最终一致性，永久清理和目录递归为独立高风险能力。

## 6. 可保留实现

- 空 UA 的实际 RoundTripper 处理和测试较严谨：`client.go:10-134`。
- 空间大小高精度解析和溢出测试：`info.go:81-124`。
- MCP 危险工具默认关闭；本地传输临时文件+rename；本地路径/SSRF/大小限制有多层防御。
- SHA1/prehash、OSS MD5/CRC、Sequential、每片重试和 token 刷新意识可作为上传基础。
- API 端点集中在 `api.go`，便于后续 host allowlist 和 per-endpoint 错误码合同。

## 7. 测试真实程度与缺口

现有 `driver_test.go` 多数是真实接口集成测试，不是 hermetic mock，不能默认 CI 运行。已有 httptest 主要覆盖空 UA、部分分享请求、Header 合并、空间解析和 MCP 本地安全辅助函数。

缺少：429/Retry-After/UNKNOWN、Cookie 重导入、多账号 race、写请求注入 retry、分片恢复、9GiB 边界、Range、批量部分成功、根删除零请求和成功解密合同。建议分为 hermetic unit、脱敏录制 contract、fault injection、mock race、显式手动集成五层；真实写测试必须独立 build tag、专用账号和额外危险许可。