# CloudSite 全系统深度技术审计：执行摘要

- 审计日期：2026-11-16
- 阶段：只读研究；未修改项目源码、Git 分支、生产配置、数据库或真实网盘
- CloudSite：`1b5b560bb8006ea69f05c61926040c3d9ad7679f`，VERSION 1.0.3，schema 21
- IndexCore：`9a33d0113ea8d6ddd1f66834ba7a781e5007fe3e`，main，schema 5
- 115driver 上游：`1cfb0d6e43e6c6487d53a7977bff58f914983514`
- CloudSite 实际固定依赖：115driver `v1.3.5`，tag 提交 `d89999540a6d35838582ac06e31f81312d1f66fd`
- 生产只读观测窗口：2026-11-16 09:50—10:40 UTC

## 1. 总体结论

现有分层方向合理，尤其应保留以下责任边界：

```text
115：真实文件与离线任务
OpenList：现有访问和观察适配层
IndexCore：物理资源观察、Canonical Inventory 与 Journal 的唯一真相所有者
CloudSite：业务资源、用户、任务、分类、检索、展示与受审计的 Provider 命令
```

CloudSite 当前的 acquisition 链路没有绕过 IndexCore 直写 Canonical；`libraryview` 也只是可重建派生读模型，不是第二个物理真相写入者。Job 的 side-effect fence、claim/failure 双计数器、数据库时间 fencing、Manifest/Job 原子状态提交，以及 IndexCore 的 completeness、topology、coverage 和串行 admission 设计，都是后续扩展应复用而非推翻的基础。

但当前产品仍主要是“文件库展示、搜索、预览、分享和离线下载”，尚不是完整的 115 文件管理平台。原生上传、目录创建、移动、复制、重命名、删除、回收站管理、操作历史闭环均未形成产品能力；合集、收藏、最近浏览、热门、专题、统一标签与别名体系也不存在。当前 Copy 展示 overlay 不等于物理文件管理，也不等于完整业务分类。

## 2. 最重要的审计发现

### 2.1 需要立即处理的凭证边界

1. **DR-01（P0，上游路径）**：115driver 取下载链接后复制实际请求 Header，其中可能含长期 `UID/CID/SEID/KID` Cookie，随后 `DownloadInfo.Get` 将这些 Header 用于下载 URL。若目标为 CDN/其他源，会形成跨源凭证暴露。CloudSite 当前未调用此下载方法，因此不能据此声称 CloudSite 已发生泄露。
2. **DR-02（P0，配置触发）**：可选明文 HTTP 列表端点和可注入 API URL 与 client 级 Cookie 组合，可能让凭证经明文或非预期主机发送。默认 HTTPS 配置不受同一条件影响。
3. **DEP-01（P1）**：生产 `/opt/my-servers/.env` 权限为 `0644`，键名显示含 CloudSite secret/master/setup 三类秘密；本审计未读取其值。
4. **DEP-02/DEP-03（P1）**：每日备份脚本把包含 `.env`、OpenList 数据和 rclone 配置的整个目录上传远端；是否加密、远端保留和访问控制未取得证据。脚本也未提供数据库一致性快照或恢复校验证明。

### 2.2 影响核心正确性的缺陷

1. **CS-01 / DR-10（P1）**：固定版 115driver 在单 URI 返回多个离线任务结果时可能切片越界；CloudSite 的“多 hash 拒绝”发生在 SDK 返回之后，无法拦截该 panic。
2. **CS-02（P1）**：CloudSite 生产工厂只轮询离线任务前三页；页数耗尽与“确认不存在”均可能进入不可重试的人工恢复。无限提交政策使页外任务风险更现实。
3. **IC-01—IC-03（P1）**：IndexCore 的冲突/UNRESOLVED、R8 同路径替换和同 Snapshot 重复稳定 ID 存在身份与删除收敛缺口。代码路径确定，实际删除影响属于有条件推断，需要对抗性 PG 重放确认。
4. **IC-04 / IC-05（P1）**：正常 reconcile 的 counts/conflicts 未按真实结果持久化；Journal payload 只有 reason，不能作为自包含 CDC。
5. **CS-04（P2）**：CloudSite 旧 Journal Projector 接受序号缺口并推进游标；若上游发生缺口，可能永久漏投影。
6. **DR-04—DR-12（P1）**：115driver 存在共享可变 client、多账号 Cookie 叠加、无统一 429/超时分类、写结果 UNKNOWN 无对账、上传无跨进程恢复、目录分页缺陷和详情假成功等问题，不能直接承担确定性大规模文件管理。

### 2.3 当前最明确的规模阻断

**IC-07（P1 / BLOCKER）**：Issue134/F3 的已有隔离证据显示，50K fixture 中有 49,972 个 overdue Watch，500 个变化文件中 492 个未观察；较小 PENDING 队列会掩盖 freshness debt。F1/F2/读池已合并但默认受限，F4 的 PR147/148/149 仍是 Draft 且未合并，Issue134/F2/F4 的 freshness/debt/capacity 证据仍为 NO-GO。历史 Gate 3 的 20K PostgreSQL full-runtime-ingestion 基线曾 PASS，但这不证明完整源 traversal、长稳或生产 freshness；不得把这些能力描述为生产稳定。

生产主机本次快照还显示：2 vCPU、1.9 GiB RAM、约 477 MiB swap 已用，I/O pressure `some avg300=43.48`、`full avg300=13.70`。这是容量排查信号，不足以单独证明资源泄漏或长期瓶颈。

## 3. 架构保留与改造判断

| 模块 | 判断 |
|---|---|
| CloudSite Catalog 的 Asset→Release→Variant→Copy | 保留；需明确 Copy 当前一对一物理映射，并补业务元数据与约束 |
| CloudSite acquisition Job/fence/原子恢复 | 保留并抽象为通用文件操作任务框架 |
| CloudSite libraryview | 保留为可重建派生读模型；不能与旧 Projector 长期维持不同 Journal 连续性规则 |
| IndexCore Canonical/Journal/串行 admission | 保留；先修身份冲突和 Journal 契约，再扩容量 |
| IndexCore Hint | 只作同宿主受信任的“重新验证”信号；202 不是操作完成 |
| OpenList 观察与内容访问 | 保留现有边界；不新增 CloudSite 私有物理扫描器 |
| 115driver | 复用协议与加密实现，但必须增加窄 Provider Adapter、不可变会话、typed error、结果核验和幂等状态机 |
| 原生 JS 前端 | 当前可维护功能可继续服务；Next.js/TypeScript 是候选消费端实施选择，不是必须推倒重来的架构门槛 |

## 4. 推荐改造顺序

前置决策：IndexCore 项目当前把 CloudSite 1.0 标为 Legacy/Frozen Product，而本任务研究目标是升级 CloudSite。R2 产品施工前，所有者/Architect 需在 Q00 明确“扩展 CloudSite 1.0”或“授权 successor product”，并处理旧边界冲突；本审计不替代该批准。

1. **安全止血**：下载 Header allowlist、移除 HTTP 端点、debug 脱敏、危险删除本地防爆半径、生产秘密文件权限与备份访问审查。
2. **正确性优先**：修复离线多结果 panic、三页轮询错误分类、Journal gap、IndexCore 身份冲突及结果持久化。
3. **建立通用写操作内核**：确定性 Provider Adapter、`APPLIED/NOT_APPLIED/UNKNOWN/PARTIAL`、预检、回执、对账、审计、Hint outbox、Canonical 等待。
4. **先只读文件管理中心**：全局树、列表、详情、搜索、操作进度先基于 IndexCore；再逐项开放 mkdir/rename/move/copy/upload/delete。
5. **业务分类与发现**：物理目录不承载唯一分类；建立 Asset 中心、标签、别名、厂商/版本/平台/架构元数据、合集和专题。
6. **最后做真实规模验收**：在授权部署后执行 20K+、长稳、真实限流、PG 负载、任务积压和增量索引验证；本阶段不伪造这些结论。

详细任务的数据库/API/兼容性/回滚和验收见 `12-OPTIMIZATION-ROADMAP.md`。

## 5. 生产核验边界

已直接核验：

- SSH 可用，Ubuntu 24.04，Docker/containerd 运行；公网监听仅见 22/80/443。
- CloudSite、IndexCore、OpenList、两个 PostgreSQL 集群、Traefik、Portainer 等进程存在。
- 生产 `library-page.js`、`library.js` 与本地 1.0.3 HEAD 的 SHA-256 完全一致。
- 公开注册当前为 enabled；安全响应头包含 CSP、frame deny、nosniff、no-referrer。

仍为 `UNKNOWN/BLOCKER`：

- CloudSite/IndexCore 实际二进制 Git SHA、完整镜像清单、数据库 schema/migration 内容、数据库连接池和积压、容器日志、实际 backup/restore 回执。
- 原因：`nathan` 无 Docker socket、root 配置、系统 journal 和 `/proc/<pid>/exe` 读取权限。本审计没有使用 sudo 提权，也没有索取数据库或 115 凭证。

因此，“静态资源匹配 1.0.3”不能扩大为“生产全部二进制和 schema 均已证明为 1.0.3/schema21”。

## 6. 验证声明

- CloudSite 审计 SHA 的 GitHub CI 与 release workflow 均为 success；IndexCore 审计 SHA 的 CI 为 success。
- 本审计没有重新运行完整测试套件。115driver 的 `make test` 会排除 `pkg/driver`，`make test-integration` 则独立要求真实 `COOKIE`；直接 `go test ./...` 或 `go test ./pkg/driver` 没有 Go build-tag 防护，配置真实 Cookie 时可执行创建目录、上传、离线下载、删除和清空任务等危险行为，因此本审计未运行任何 115driver 测试。
- 报告中的测试方法均为后续验证要求，不是已执行结果。
- 所有未能直接证明的项目均标为 `INFERENCE`、`UNKNOWN` 或 `BLOCKER`。