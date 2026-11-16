# 10 安全、权限与操作风险审计

## 1. 总体判断

CloudSite 当前产品层的认证、会话、Host/Origin、严格 JSON、Catalog Binding、内容访问实时 Q5 授权、管理操作审计和密钥文件加载具有较强安全基础。已审计代码中未发现匿名管理员越权、直接 SQL 注入或 CloudSite 绕过 IndexCore 修改 Canonical 的实现。

最高优先级风险位于：115driver 下载 Header/HTTP 凭证边界、共享会话与危险操作防护、CloudSite source 明文入库、生产秘密文件和备份、未来文件写操作的 UNKNOWN/批量/递归语义。

## 2. 认证与授权现状

| 控制 | 证据与评价 |
|---|---|
| Session | 256-bit 随机，DB 仅 SHA-256 digest；30 天固定期，撤销/禁用/删除每次重查 |
| Cookie | HttpOnly、HTTPS Secure、SameSite=Strict、host-only；不在 URL 中传递 |
| 注册/登录 | 开放注册可配置；登录/注册有进程级速率与并发 gate，密码最少 6 字符，需评估暴力破解和共享 DoS |
| Origin/CSRF | mutation 要求精确 Origin、Sec-Fetch-Site 非 cross-site，严格 JSON 和 body 限制 |
| 用户权限 | owner-only 任务；管理写事务内重查 actor/session/role；Binding 限定可见物理范围 |
| 文件内容 | 每次（含缓存）先 Q5 校验 Binding/resource/path/PRESENT/size，再 OpenList 内容 |
| 管理操作 | Reserve→Apply→Result、request digest、expected_version/CAS、审计历史 |
| 危险工具 | CloudSite 当前无原生物理删除 API；115driver MCP destructive tools 默认关闭 |

## 3. 安全发现映射

| ID | 级别 | 安全结论 | 详细证据 |
|---|---|---|---|
| DR-01 | P0 | 长期 Cookie 可能随下载 Header 到 CDN/其他源 | `04-115DRIVER-CAPABILITY.md` |
| DR-02 | P0（配置触发） | HTTP/任意 API URL 可发送 client Cookie | 同上 |
| DR-03/04/05 | P1 | debug 泄密、共享 client 竞态、多账号 Cookie 叠加 | 同上 |
| DR-13 | P1（潜在 P0） | 根删除/空清理缺本地防爆半径 | 同上 |
| CS-03 | P1 | URL/magnet 及签名参数明文入两张产品表 | `02-CLOUDSITE-AUDIT.md` |
| CS-08 | P2 | 开放注册+无限提交缺系统背压 | 同上；生产公开注册已确认 |
| CS-09 | P2 | 产品撤销不即时回收已签发 302 URL | 同上 |
| IC-13 | P2 | adapter config secret 校验可绕过 | `03-INDEX-CORE-AUDIT.md` |
| DEP-01/02 | P1 | `.env` 0644、备份包含秘密且远端保护未知 | `09-DEPLOYMENT-AUDIT.md` |
| DEP-05 | P1 | Portainer socket RW、OpenList root 扩大控制面 | 同上 |

这些条目的项目、文件、函数、触发、影响、建议、难度和测试方法已在对应报告逐项给出，本节聚焦统一安全策略。

## 4. 注入、路径和 SSRF

### SQL 注入

已审 CloudSite/IndexCore SQL 主要使用 pgx 参数绑定；动态片段来自固定模板/锁模式。未发现用户输入直接拼接为 SQL 的证据。后续搜索/筛选必须采用字段 allowlist 和参数值绑定，排序字段不能原样拼接。

### 路径穿越与对象越权

CloudSite target/mount 拒绝反斜杠、NUL、`.`、`..`，离线结果名只允许单一直接子段。未来文件管理必须：

- 物理对象以 account+Provider ID/Root+resource_id 授权，路径只作显示/解析；
- 服务端选择 Binding/credential_ref，浏览器不能指定任意 native scope；
- Move/Copy/Delete 同时验证源和目标，防止跨账号/跨 Root 混淆；
- Q5 ambiguous 不得自选 winner；
- 本地临时上传/下载使用受控 root、符号链接检查和原子 rename。

### SSRF/重定向

CloudSite OpenList ContentClient 禁默认代理和未知重定向，外部重定向在 dial 前验证目标 IP。115driver 任意 API URL/代理与下载 URL 则必须由新 Adapter 限制 scheme/host、DNS 解析和重定向，且不得继承账号 Cookie。

## 5. 凭证与日志安全

### 必须保留

- CloudSite 保护文件要求绝对规范路径、当前用户所有、0700/0600、禁符号链接和替换竞态：`internal/platform/bootstrap/bootstrap.go:25-76,178-223`。
- Worker 不从 CLI 明文接收 Cookie/token；上游错误在 CloudSite Adapter 中转封闭诊断。
- Session/share token 数据库存 digest，前端分享秘密放 fragment 而非 query 的设计方向正确。

### 必须整改

1. 115driver debug Header/Body 全面脱敏，真实测试不得默认 debug 输出 Cookie。
2. 下载 Header allowlist，长期 Cookie 不得到 CDN；API URL 强制 HTTPS 和 host allowlist。
3. 产品 source 改 opaque secret ref+digest；历史明文数据的保留/清除需专门迁移决策，不能盲目删除审计证据。
4. 生产 `.env` 最小权限并评估轮换；备份避免自包含备份凭证，确认远端加密和 ACL。
5. 日志只记录 operation_id、typed kind、provider code、status、attempts、account/scope 的受控标识；不得记录 URL query、magnet、Cookie、签名下载 URL、密码、回收站密码或分享提取码。

## 6. 上传、下载与分享安全

### 上传

- 当前 CloudSite 无原生 multipart 上传面，不存在已暴露的对应入口。
- 未来上传需限制大小、类型、目标 scope、磁盘/内存、chunk 数和并发；streaming，不整文件读内存。
- checkpoint 绑定账号/目标/内容，防旧 UploadID 跨内容复用。
- 文件名仅作显示，不直接拼本地路径；完成后校验 FileID/size/SHA1。

### 下载

- 上游 lease 必须短期、Header allowlist、HTTPS、有效期明确或标 unknown。
- `DownloadInfo.Get` 当前不查 HTTP status 且全量读内存，不可直接复用为大文件产品实现。
- Range/ETag、最大响应、错误页识别、长度/hash 校验和原子写入必需。
- 撤销产品授权与 CDN URL 失效是不同时间线，UI/API 不得承诺即时全线撤销。

### 分享

- CloudSite 当前是产品自管单文件分享，不是 115 原生分享。
- 未来创建 115 原生分享必须另行验证权限、有效期、提取码、撤销和访问日志；不得把“分享快照/下载存在”当作完整管理。
- 私有资源不得出现在其他用户的搜索计数、热门、最近浏览、ETag 或合集统计中。

## 7. 文件操作风险分级与强制闸门

| 风险 | 操作 | 强制控制 |
|---|---|---|
| 只读 | 树、列表、搜索、Stat、预览、下载 lease | 身份+scope、速率、响应上限、日志脱敏、下载保护 |
| 普通写入 | Mkdir、Rename、Move、Copy、Upload、Offline、Restore | 预检、幂等、UNKNOWN 对账、审计、结果核验、Canonical 等待 |
| 高风险写入 | 目录递归删除、批量删除、永久清理、连带删除文件、大规模移动 | 目标确认、影响预览、二次确认、爆炸半径限制、审计、读回和恢复方案 |

高风险确认必须绑定 `preview_id + request_digest + expected_version + expires_at`，不能只弹一个通用“确定吗”。根 ID、空 ID 集、未解析目标和跨 scope 必须在发送 HTTP 前拒绝；空 RID 清理绝不能隐式表示清空全部。

批量部分成功要展示每项结果；UNKNOWN 不得自动进入“重试全部”。永久删除不可承诺可恢复，入回收站与永久清理必须是不同 API/权限/确认文本。

## 8. 任务与多进程安全

CloudSite 的 owner+claim generation+lease+provider fence 是正确基础，未来扩展必须保持：

- 每操作 side-effect fence 和 request digest；
- HTTP 不持 DB 事务；
- ACQUISITION 与其他文件操作都不能被通用 Cancel 改成单边状态；
- 多 worker 通过数据库原子认领和 fencing，不依赖进程内锁；
- 暂时错误 typed 分类，写请求 UNKNOWN 不等同 transient 可重试；
- 权限在每次重试/恢复时重新校验，避免撤销后旧任务继续写；
- 会话 generation 防止重认证前请求覆盖新会话；
- 公开提交有全局水位和公平调度，不以串行 Worker 代替入队背压。

## 9. 安全验证计划

1. **静态**：secret/DSN/log 扫描、SQL 动态片段 allowlist、危险端点和 MCP 工具注册清单。
2. **Hermetic fault injection**：Cookie 跨源、429、提交后断连、根删除、空清理、批量部分成功、路径 ABA。
3. **Race**：mock 下多账号会话、上传/离线初始化、重认证和并发任务。
4. **权限矩阵**：匿名/USER/ADMIN/撤销 session/删除用户 × 每 API × 每 Binding/Root。
5. **浏览器**：CSP、Origin、CSRF、XSS、分享撤销、私有 ETag 和缓存隔离。
6. **部署**：秘密文件权限、容器 UID/socket、端口、备份 ACL/加密、隔离恢复。
7. **真实接口**：仅专用账号和隔离目录，逐项授权；禁止用生产真实文件做安全试验。

## 10. 尚需证据

当前数据库角色/最小权限、真实 115 Cookie 存储文件 metadata、日志实际脱敏、Portainer MFA/Cloudflare 规则、远端备份加密、容器只读 rootfs/capabilities/seccomp、DB RLS（若有）均为 UNKNOWN。不得因源码存在保护就推定生产配置全部一致。