# CloudSite 网站管理后台：UI 与功能蓝图（D-066 / Issue #141）

> **状态：DESIGN ONLY，提案待架构审查。** 此文是管理控制台的产品需求、信息架构、交互和未来接口方案，**不是已上线功能**；不会因文档创建而修改产品代码、执行数据库迁移、操作 115 或 IndexCore。
>
> 设计基线：CloudSite main `403374c63121492b694acef3dfb738339d75f4ac`、产品 schema15（2026-10-20）。在具体开发时须重新检查最新 main、`PROJECT-STATE.md`、`docs/DECISIONS.md` 和现有代码。
>
> **优先级决策：** 用户将 [Issue #139（私有文件夹 Stage A）](https://github.com/nathanxiangang-web/cloudsite/issues/139) 延后，已撤回此前执行授权；D-065 与 [#138](https://github.com/nathanxiangang-web/cloudsite/issues/138) 保留待办。本项目先做**网站管理 UI 和功能蓝图**，不实施用户空间，不做每用户容量限制。

## 一、管理端定位与真实起点

### 产品目标

CloudSite 管理端是**网站运营、用户与资源生命周期的受控管理界面**。管理员在一个地方回答五个问题：

1. 网站当前是否正常、哪些模块真正可用？
2. 哪些用户在使用网站、有哪些下载操作正在执行或卡住？
3. 资源是否已被 IndexCore 可信确认，为什么有资源不可见或不可播放？
4. 115、OpenList、IndexCore 和 CloudSite 各自状态是什么，故障属于哪一层？
5. 哪些受控运维动作可以执行，结果是否有持久审计，能否安全恢复？

管理端**不**重新实现 Catalog、IndexCore Canonical、OpenList 扫描、Job Engine、115 Provider，也不直接修改那些系统的数据库。

### 现状核实（2026-10-20）

| 模块 | 仓库已实现且可复用 | 目前并未实现、不能伪称已有 |
| --- | --- | --- |
| 入口和角色 | `/admin/`、`GET /api/v1/admin/workspace` 强制已登录 `ADMIN`；`/api/v1/auth/me`，退出/登录、session | 独立完整管理端 API、细粒度管理员授权与二次确认 |
| 现有后台 UI | `internal/producthttp/web/index.html`、`app.js`，资源目录浏览、Copy 信息、当前登录者任务、只读提示 | 系统总览、跨用户管理、全站工单/下载状态、真正管理动作 |
| 资源 | 经产品 Binding、IndexCore Q4/Q5/Q6 及 Copy 关联读取 | 把一个页面计数当全局总量、由浏览器决定物理真实或修改 Canonical |
| 云下载 | 单 Worker/Manifest/Job、owner-only `GET /api/v1/tasks`、新用户提交/用量、零价格操作配额 | 已有 `GET /api/v1/admin/all-downloads` 或直接重试所有 UNKNOWN 的权限 |
| 文件 | 消费者文件列表、媒体预览、302、产品分享/撤销 | 任意物理文件删除或管理员直接执行 115 原生删除 |
| 提供方 | 115 下载适配器和受保护连接；115driver v1.3.5 源码有全账号 `GetInfo()` | 当前产品管理 API 已经提供实时全盘容量/登录健康、通用 Storage CRUD |
| IndexCore | 规范物理资源 Query/Journal；可信状态以 IndexCore 自己的证据为准 | 管理端任意 Root scan、重建索引、手改 Q8/CANONICAL、通过单个状态灯判定全链路健康 |
| 审计 | 局部持久事实与操作事件/日志 | 完整、可分页、不可篡改的管理操作审计表或 Worker 长期心跳事件库 |

**注意：** 现有后台的“我的任务”只读取当前登录者归属，不是全站下载列表；`/admin/workspace` 返回的只是受保护的能力描述，不是 Admin Dashboard 数据。现有网页写着“只读预览”，因此新版管理端一律按“查看已实现 / 新能力待接”明确分层。

## 二、角色与权限模型

产品身份继续使用既有 `USER` / `ADMIN`，不重新建立一套登录密码或自动增加 `SUPER_ADMIN`。后台访问首先验证真实产品 session 和最新用户 `enabled` / RoleAdmin。今后如果需要操作权限细分，用**服务端固定、可审计的 action grants** 扩展，不能相信浏览器权限字段。

| 能力/操作 | USER | ADMIN（本轮设计） | 将来受额外授权的 Operator |
| --- | :---: | :---: | :---: |
| 自己的任务/下载/分享、前台文件 | 自己 | 自己 | 依角色原权限 |
| 后台状态概览/脱敏诊断 | 无 | 只读 | 已批准范围 |
| 全站用户与任务的**脱敏概览** | 无 | 未来受控只读 | 已批准范围 |
| 查看他人的原始下载源、Cookie、密钥、私人文件正文 | **拒绝** | **拒绝** | 默认拒绝，例外另行决策 |
| 注册/禁用用户、重置认证、调整操作次数 | 无 | 未来单独审核 + 额外 action grant | 明确操作授权+审计 |
| Job 取消/恢复、Provider Task、Hint | 无 | **默认只读** | 必须由原有领域服务明确开放的窄命令 |
| IndexCore 原始数据修补、手动 SQL、原生 115 删除 | 无 | **禁止** | 不由管理 UI 通用授权，需单独工程安全门禁 |
| 私有 UserSpace 管理 | 延后 | 延后 | D-065 Stage A/B/C/D 单独授权 |

HTTP 页面可隐藏不可用操作，但**权限判断必须在每一个服务端接口重新执行**；管理员不等于其他用户所有私有文件的主人。用户注销、角色降级、session 撤销后，管理请求和迟到回包不得继续显示数据。

## 三、信息架构与导航（目标结构）

```text
CloudSite Admin /admin/
├── 工作台总览      /admin/overview
│   ├── 服务状态 / 今日待处理 / 快速定位
│   └── 任务趋势 / 最新告警 / 操作动态
├── 用户管理        /admin/users
│   ├── 用户列表 / 账号详情 / Session 与操作策略
│   └── 用户禁用 / 状态更改（后续受审计）
├── 资源中心        /admin/resources
│   ├── 物理规范资源 Q4/Q5/Q6 / Copy / 分类
│   ├── Root / Binding / Canonical 身份证据
│   └── 资源详情与可见性排错（只读）
├── 云下载与任务     /admin/downloads
│   ├── Operation / Manifest / Job / Provider 状态
│   ├── REFERENCE_KNOWN / RECOVERY_REQUIRED / READY
│   └── 单任务详情 / 失败阶段与证据 / 运维待办
├── 存储与连接       /admin/storage
│   ├── StorageBinding / Connection / 115 能力
│   ├── 115 全账号容量（可选只读，需实时数据）
│   └── OpenList 与原生115身份/命名空间校验
├── IndexCore 观测   /admin/indexcore
│   ├── Root 健康 / Watch/DirtyWork / Canonical
│   └── Q8 Journal / 差异诊断 / UNKNOWN 提示
├── 分享管理        /admin/shares
│   ├── 数量与状态 / 到期 / 撤销审计
│   └── 不显示分享明文 token/保护码
├── 审计与安全       /admin/audit
│   ├── 操作意图与结果 / 不确定状态
│   └── 登录安全 / 授权记录（待建设）
└── 系统设置        /admin/settings
    ├── 系统版本 / 只读配置摘要 / 功能开关
    ├── 服务、发布与备份状态（仅可信来源）
    └── 未来：AI 集成（仅显示未启用）
```

**导航规则：** 以服务能力驱动显示：未接线模块可以是“规划中”，但不得有仿真的实时统计；用户、任务、资源和存储各在自身领域页面，避免把所有表塞到首页。界面可跨模块以稳定 ID 导航，但服务端返回最小安全 DTO。

## 四、视觉设计规范

目标：**深色侧边导航 + 明亮内容区，简洁、清晰、偏工程控制台的现代 SaaS 后台**，不是消费型视频网站，也不是“所有卡片霓虹特效”。

- 左侧固定宽约 232px：品牌、两级导航、当前环境、当前管理员与退出。
- 顶部高约 64px：面包屑、全局搜索（仅已实现对象）、工作环境提示、提醒入口。
- 主内容区：最大宽 1440px；12 列栅格，卡片间距 16–20px；页面标题/解释/最后更新时间。
- 色板参考：导航 `#111827`，主内容 `#F5F7FB`，卡片白 `#FFFFFF`，品牌蓝 `#315CF5`；成功 `#15803D`，警告 `#B45309`，危险 `#B91C1C`，非可用灰 `#6B7280`。颜色必须配状态文字/图标，不能只靠颜色。
- 中文优先（系统字体栈 `PingFang SC`、`Microsoft YaHei` 等），标题 24px、正文 14px、说明 12px，信息密度中高。表格有 44px 左右行高，超长 ID 截断+复制动作。
- 响应式：≥1200px 左侧完整菜单 + 多列统计；768–1199px 收窄导航，卡片两列；＜768px 菜单抽屉、单列卡片、筛选弹层、表格水平滚动或转为字段堆叠。
- WCAG AA 对比度/键盘焦点、Tab 顺序、语义表格、`aria-live` 只用于必要状态提示；高风险按钮单独二次确认。纯图标需文本标签。关闭浮层后焦点返回触发按钮。
- 操作视觉区分：只读**查看** vs 受控**申请/确认**。禁用按钮必须写明“后端未接入/需要权限/状态不可操作”，不是点击后假成功。

### 页面级线框

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ CLOUDSITE ADMIN            当前：生产环境（只读状态）     搜索 [对象 ID/用户] │
├─────────────────────┬────────────────────────────────────────────────────────┤
│ 总览 ●             │ 运营总览                         最后更新：--        │
│ 用户               │ [可用服务] [待关注任务] [可确认资源] [接口状态]         │
│ 资源               │                                                        │
│ 云下载与任务       │ ┌─────────────────────┐ ┌──────────────────────────┐ │
│ 存储与连接         │ │ 服务状态 / 数据来源  │ │ 任务状态趋势 / 数据缺口  │ │
│ IndexCore          │ └─────────────────────┘ └──────────────────────────┘ │
│ 分享               │ ┌────────────────────────────────────────────────────┐ │
│ 审计与安全         │ │ 待处理的异常任务                                   │ │
│ 系统设置           │ │ 状态 · 归属 · 阶段 · 发生时间 · 详情 →             │ │
│                     │ └────────────────────────────────────────────────────┘ │
│ 当前账号 / 退出     │                                                        │
└─────────────────────┴────────────────────────────────────────────────────────┘
```

**任务详情抽屉：** 右侧占宽约 440–520px；顶部状态及不确定性警示，下面依序显示 Operation→Manifest→Job→Provider Reference→IndexCore 观察→Copy READY，逐项标出“已证实 / 待证实 / 无数据”。敏感字段不能显示原值；动作位默认只读，没有审定命令时显示禁用原因。

## 五、九个功能页面的详细需求

### 5.1 总览 Overview（P0 首版只读）

**布局：** 最多四个可信 KPI（服务可用/待处理任务/已确认规范资源/异常警告），趋势图、最近异常和来源状态。所有卡片都有 `source`、`observed_at`、`coverage`、`status`。没有聚合 API 时显示“未接入”，不能以当前 Q4 页数或上次报告总量冒充实时全站数据。

筛选：时间范围（如今天、7 天）；刷新是**只读查询**。组件加载/降级/无数据/失去权限各有不同状态；服务拒绝时保留先前值并标“已过期”，不重置为 0。

### 5.2 用户管理 Users（P1）

字段：登录名、公开 UUID 的脱敏展示、USER/ADMIN、enabled、创建时间、最后一次网站登录（仅将来有可信审计才显示）、现有下载操作数量及并发限额（来自业务后端）；**没有 GB 配额**。详情：账户信息、当前登录/session（只可失效处理；不返回 token）、用户任务汇总、安全事件。新建/禁用/重置密码/角色调整是未来独立有审计的 API，不借用现有认证 Provision CLI 伪装网页功能。用户名修改默认无入口（现有语义未定义）。

### 5.3 资源中心 Resources（P0 已有能力复用）

管理员可以看指定 allowlist Binding 的 Q4/Q5/Q6、精确路径 resolve 和 Copy 可用性；与用户库共享 `ResourceDisplay` 组件，但管理端单独显示 Canonical/IndexCore 事实和数据源。可筛 Root/Binding/目录、规范存在性、Copy 关联和类型。**不要承诺跨所有文件的全文搜索**；当前查询支持游标/分页，分类只针对已加载集合。

详情展开稳定的 Root/ResourceID/Path/Copy/Variant/Release/Asset 链，根据信息来源显示缺失证据，不能从分类后缀猜测资产真实性；不在管理端直接删文件。

### 5.4 云下载与任务 Downloads（P1 先只读，P3 后操作）

列表按用户、Manifest/Job、状态、时间分页搜索；状态 `PENDING/ACTIVE/AWAITING_VISIBILITY/AWAITING_CANONICAL/READY/FAILED/RECOVERY_REQUIRED/CANCELED` 使用既有领域语义，不把 Provider SUCCEEDED 等同 READY，不把 `result_copy_id` 有值等同已证明可见。

详情时间线来自多个*明确来源*，不应假设完整历史时间线已经持久化。UNKNOWN provider task/reference、旧 8 项 reservations 必须隔离标记“恢复债务，需原有门禁”，无“一键重试全部/清除记录”入口。

后续安全动作：只在新的受控领域命令+幂等审计完成后开放“取消/安全恢复/复核”；下载 Worker 仍唯一所有者，管理员不能直接调用 SDK 伪造结果或命令 IndexCore 写 Canonical。

### 5.5 存储与连接 Storage（P1 只读）

页面分开：**存储 Binding 与 IndexCore Root**、**Provider Connection 与原生 115 会话**、**OpenList 观察映射**。显示脱敏别名、能力、来源、最近确认时间、可信的运行状态。原生 115 目录 ID、CredentialRef、Cookie、应用 token、Secret 内容永不作为前台 DTO。

`115driver v1.3.5.GetInfo()` 可读**共享网盘账号总/已用/剩余字节**，但现网尚未验证该接口；若未来开放，应是隔离只读管理 port、有独立超时/缓存/限频，不改变登录态，不影响任务。先前的 520GB 仅为过往**未核实全局参考值**，不是用户配额、也不必作为管理台核心 KPI。

### 5.6 IndexCore 观测（P1 只读）

所有状态以真实 IndexCore 暴露且可验证的 Query/诊断端口为准。IndexCore R3 提供的 `root diagnose` 是**运维 CLI**，不是 CloudSite 已存在的公网管理端接口。没有安全聚合端口时，健康模块标记 `NOT_CONNECTED`。若未来引入 operator-only 诊断代理，应要求服务身份、精确 Root 范围、限频与脱敏，不能暴露 PostgreSQL DSN、重新造一个扫描器。

界面需要区分 `OK`、`DEGRADED`、`UNKNOWN`、`NOT_TRACKED`、`STALE`，特别是 115 在线状态和 Q8 下游游标未被真实追踪时，不能呈现绿灯。修复/扫描不在首版授权范围内。

### 5.7 分享管理（P2）

只读列表：已创建、有效、到期、撤销（如果有业务可信数据）、关联 File Identity、归属 user_id、可见性类别。**token、明文验证码、签名 CDN URL、未授权的私有文件信息不返回**；管理员不能默认兑换别人的受保护链接。未来撤销入口需细粒度授权及审计。

### 5.8 审计与安全（P2 → P3）

目标记录：`event_id`、actor UUID/role、`action`、最小可定位目标、`intent_id`、时间、决定、结果 `SUCCESS/REJECTED/UNKNOWN`、相关事件标识；不存明文源 URL/credential/password/验证码。当前尚无完整持久审计库，**UI 首版显示功能规划中**，不能拿临时 Worker log 当完整操作审计。

### 5.9 系统设置（P2 只读；P3 审批动作）

只读：产品版本、schema 版本、显式配置能力开关、集成状态摘要、发布与回滚文档链接；不得显示原始 bootstrap 文件内容或执行任意 shell/SQL。后续功能开关变更需要配置 owner、验证、双人复核或等效制度及回滚。AI 状态只显示“尚未接入/独立计划”，不擅自启动 Gate5。

## 六、建议接口（拟议，当前**不存在**）

仅作未来产品 API 草案，非可以直接在当前服务器上调用的路由：

| 提议端点/能力 | 类型 | 返回数据/边界 | 初版优先级 |
| --- | --- | --- | --- |
| `GET /api/v1/admin/overview` | 只读聚合 | 由明确 owner 提供的数量、覆盖范围、更新时间、UNKNOWN | P0 |
| `GET /api/v1/admin/users` | 只读列表 | UUID、login、role、enabled、分页、无凭据 | P1 |
| `GET /api/v1/admin/downloads` | 只读列表 | admin 授权的跨用户脱敏任务摘要；不含原始 source/info_hash | P1 |
| `GET /api/v1/admin/downloads/{id}` | 只读详情 | 操作/Manifest/Job 链，证据/失败类别，时间线不可伪造 | P1 |
| `GET /api/v1/admin/storage` | 只读 | binding/connection/root 状态及证据；不含 SecretRef 原值 | P1 |
| `GET /api/v1/admin/indexcore` | 只读 | 单 Root 受控诊断、UNKNOWN/NOT_TRACKED 保留 | P1 |
| `GET /api/v1/admin/shares` | 只读 | 脱敏分享状态与归属，匿名 token 禁止 | P2 |
| `GET /api/v1/admin/audit` | 只读 | 真正持久操作审计及来源，不编造历史 | P2 |
| `POST /api/v1/admin/commands/{type}` | 审批写 | 固定允许的领域命令、幂等键、短租约/版本、独立审计 | P3，必须另审 |

所有返回严格 allowlist JSON、唯一稳定 ID、页游标与分页上限、无跨用户信息注入；服务端强校验 ADMIN、独立 action grant、Origin/Host、session、取消与并发。HTTP 未接线时 UI 显式显示 `NOT_IMPLEMENTED`，绝不退回用户接口取样造“全站统计”。

**禁止**做后台泛用 SQL 控制台、万能 `POST /provider/call`、任意 PID 重启、读取 115/OpenList 私钥、任意 Root Reset 或直接借浏览器身份操作 IndexCore。

## 七、运营场景与交互流程

### 场景 A：下载卡在“等待规范确认”

从总览“待关注任务”进入列表 → 单任务详情抽屉 → 显示 Manifest/Job/Provider 的**已确认数据** → 看 IndexCore 相应 Root/Scope 的状态 → 如果未观察或 UNKNOWN，提示只读运维手册链接和受控命令缺口，而**不**自动二次下发下载或重置重试计数。

### 场景 B：用户说“我的文件不见了”

在用户页定位唯一 UserID → 根据该用户自身任务和已明确授权的 Copy 查询 → 以 Q5/Q8 身份证据排除“仅过滤/未收录/物理已移除”，如果无可信覆盖则显示未确认，不猜删。D-065 后续私有目录功能未上线之前，**不声称已有私有文件所有权过滤**。

### 场景 C：115 登录不可用

存储与连接显示 native session 的最近证据与具体 `UNKNOWN/LOGIN_REQUIRED`；与 OpenList token、网站 session、IndexCore Query 在线分别展示，不能将三个凭证系统合并。所有救援流程由另一个受控作业门禁授权。

## 八、未来实现阶段和验收

| 阶段 | 未来实施范围 | 必须具备的证据 |
| --- | --- | --- |
| UI-0（当前） | 此蓝图与独立静态交互原型，不改产品功能 | 设计信息完整、角色/数据来源/空态/风险明确 |
| UI-1 | ADMIN-only 新壳、侧栏路由、已有 Q4/Tasks 只读，状态组件/空态 | 现有登录、资源、任务、响应式、越权拒绝、回归 CI |
| UI-2 | 用户/任务/存储/IndexCore **新的后端脱敏只读聚合** | 真实 PG/IndexCore 合约测试、全权限负面矩阵、UNKNOWN 不伪绿 |
| UI-3 | 分享概览、审计读模型及完整诊断 | 数据来源/审计持久性、关联准确性、敏感数据零泄漏 |
| UI-4 | 单项受控变更（账号管理、操作策略、审批命令） | 细粒度授权、持久审计、幂等、冲突、超时/断连/回滚 |
| D-065 后续 | 私有真实用户文件夹、用户内容 ACL、真实文件删除 | **仍暂停**，需要人类重新授权及独立 Issue 阶段验收 |

**UI-1 的原则：** 新版 `/admin/` 默认仍可只读工作，后端未提供的数据模块显示可识别的 `NOT_CONNECTED`，不因界面完整就开放危险操作。

### 验收设计 Checklist

- [ ] 一套可访问的静态原型至少展示总览、用户、任务、存储/IndexCore 诊断与设置模块（所有示例数值注明 DEMO）。
- [ ] 页面状态包括：加载、空数据、未授权、暂不可用、未知、过期数据、成功、失败、操作确认。
- [ ] 每个 KPI 都有实际来源与时间；缺口用文案而不是虚假 0/绿灯。
- [ ] USER/匿名无法调用 Admin API；ADMIN 不默认为他人私有内容授权。
- [ ] 登录、Host/Origin、CSRF 防护、固定工作流、审计和隐私边界完整记载。
- [ ] 旧 `/`、`/downloads/`、预览、分享、302、现有产品数据库行为不因设计而改变。
- [ ] 不引入每用户 GB 限制；现有下载操作次数/并发策略不改。
- [ ] 未合并和未实施的内容始终标注“规划”，不能用设计 PR CI 当 UI 上线验收。

## 九、关联与设计资产

- [Issue #141](https://github.com/nathanxiangang-web/cloudsite/issues/141) — 当前管理后台设计任务。
- `docs/design/CLOUDSITE-ADMIN-UI-PROTOTYPE.html` — **完全离线静态 Demo**，虚构数据，仅用于视觉与交互评审，不引用线上 API/外部 CDN/真实账户。
- `docs/CONTROL-PLANE-DOMAIN.md` — 既有控制层概念。
- `docs/PRODUCT-PRIVATE-CLOUD-SPACES.md` — 已合并 D-065、但 Stage A 开发暂停。
- `docs/PRODUCT-AUTH.md`、`docs/PRODUCT-ADMISSION.md`、`docs/PRODUCT-CLOUD-RUNTIME.md`、`docs/FILE-SURFACES.md`。
- D-064 是当前生产行为事实，D-066 仅提案；项目状态与设计链接必须保持这个时间顺序。
