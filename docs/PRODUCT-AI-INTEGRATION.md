# AI 功能接入规划：真实文件的整理助手

2026-10-18，Issue129 / D-062。**规划与只读现状核查，不是 AI 已实现或已上线。**
依据 [MVP 蓝图第16/19节](MVP-BLUEPRINT.md)、[产品域边界](CONTROL-PLANE-DOMAIN.md)
和当前 main `67d3135`。用户指定 AI 执行主机为 `192.168.178.50`，与
IndexCore 开发环境同机；软件来自本地 [CodeartsBridge](https://github.com/nathanxiangang-web/CodeartsBridge)
`c190fc8`，凭据来源是用户指定的本地 Bridge 私有账号 CSV 目录。
公网 CloudSite 继续部署于 `113.20.6.57`；本地源码/账号来源与服务器运行态分开核验。

## 1. 产品目标与首个可用版本

不是先做通用聊天窗口，而是让已有真实文件更容易理解和查找：

- 软件：识别名称、版本、平台/架构候选，给出依据；不能仅凭名字认定发布版本。
- 视频：建议作品名、年份、分辨率等语义信息，不改文件、不宣称已识别全部内容。
- 文档：先提供标题、主题和标签建议；正文摘要单独启用受控文本输入后再做。
- 其他文件：给出资源类型/用途建议，无法判断则明确“证据不足”。

第一版只做 **用户选中文件 -> 元数据整理建议 -> 结构化校验 -> 展示依据**。
支持真实文件，不新设一份“允许哪些文件”的名单；复用当前用户/Binding/资源权限。
先限单个资源，后续批量上限建议20项/次、串行处理，均为待实施配置值。
不自动扫描全盘、不自动上传正文、不移动/重命名/删除网盘文件、不自动发起下载。
现有软件/视频/文档/其他的确定性格式分类保持独立；AI 语义标签不是格式真值。

随后按蓝图加入 Release 提取、来源候选和结果复核，仍是建议，不是自主网盘代理。

## 2. 已验证基线与缺口

| 项目 | 核查结果与接入意义 |
| --- | --- |
| 当前公网产品 | `f1bceee`，schema15；账户/30天登录、文件列表、预览、下载、产品分享、云下载已组合；不是全蓝图终验 |
| 云下载文件展示 | IndexCore `e03d14a9` 的有界子目录发现；产品静默读取 Canonical；已验证2个云下载文件/主库283，不能当成永久总量 |
| 逻辑 Catalog | Asset/Release/Variant/Copy 和分类端口存在；实际 UI 仍主要是物理文件视图，不冒称已经实现 AI 资源整理 |
| AI 实现 | 当前仓库没有 `internal/agent`、AI 路由、Agent 适配器或 AI 专属持久化 |
| 软件来源 | 用户指定本地 CodeartsBridge，HEAD `c190fc8`；工作树有用户改动，未修改；不是 IndexCore 内置 AI |
| AI 凭据来源 | 用户指定本地 Bridge 账号目录：3份CSV，各含1条非空AK/SK记录；只验证结构，未输出秘密、未导入服务器、未验证模型授权 |
| `.50` 映射 | Bridge `workers.json` 的 `bus-w02-dev` 指向 `.50`，配置CLI路径和默认 `huaweicloud-maas/GLM-5.2`；具体账号CSV与该Worker的绑定尚未确认，不按文件名猜测 |
| `.50` 现场 | `.codeartsdoer/CodeArts_Agent` 中存在 CodeArts 组件；不等于已运行的模型服务，也不证明独立 API 凭据可用 |
| `.50` 旧 Bridge | systemd `bridge.service` 指向不存在的 `/home/nathan/bridge-python`，自动重启、退出状态200；既有8080/8765健康地址不可达 |
| `.50` CLI/配置 | 本次非交互会话 PATH 未找到 `codearts`；检查的常见 CLI auth/config 文件未找到；不能断言其他位置不存在凭据 |
| 公网现场 | 检查的容器环境、两处 dotenv、常见 CLI 配置位置无 AI 接入项；网站签名/主密钥不是模型凭据 |
| 网络拓扑 | 未验证公网主机到私网 `.50` 的可达性；不能直接填私网 URL 就声称接通 |
| 现有限制 | 原始 ISO 测试任务另有5/5耗尽预算，保留原引用；原生115 ShareProvider 与完整蓝图收口不因本规划被宣布完成 |

核查只读配置元数据和健康状态；**没有打印/复制密钥，没有模型请求，没有迁移、
服务重启或下载调用**。旧环境版本/模型记录只作线索，不是今天可用性的证明。

### 2.1 已核实的 Bridge 协议，不冒充模型网关

本地源码 `src/bridge/api/server.py` 提供 `/api/health`、`/api/workers`、
`/api/tasks`、任务取消/重试/人工审查和 `/api/events` SSE；这是开发任务控制面，
不是 OpenAI Chat Completions，也不是已经实现的 CloudSite AgentPort。
`src/bridge/codearts.py` 通过 CLI `run ... --format json --thinking` 执行，
解析 JSONL 中的 session/用量，并对常见AK/SK/Bearer等错误信息做脱敏。
CLI和模型值是源码/登记配置事实，**不是当前可执行或可计费调用成功的证据**。

可以复用会话/结果解析与脱敏思路，但不能将公网用户输入直接代理到开发 `/api/tasks`，
更不能允许代码执行、自动审查通过或Git集成。产品接入必须是独立受限任务适配器，
有自己的鉴权、严格DTO、能力限制和运行身份；若桥缺少强制隔离能力，在桥项目另开
窄范围变更与测试，不在本规划中修改它的脏工作树，也不靠Prompt保证安全。

## 3. 两台服务器与凭据接入方式

```text
浏览器：文件库 / 云下载 / 后台
  -> 113.20.6.57 CloudSite：登录、资源授权、AI Task/Usage、结果校验
     -> Agent Port -> CodeArts 适配器
        -> 经验证的私有连接 -> 192.168.178.50 受限 AI Gateway
           -> .50 本机受保护模型配置/凭据 -> 原有模型服务

独立不变：网盘 -> OpenList -> IndexCore -> Canonical/Journal -> CloudSite
```

“在 IndexCore 那台机器上”是**部署位置**，不是把模型接进索引 Kernel。
IndexCore 仍只提供物理事实，不增加 LLM 依赖、用户会话或 AI 结果写入端点。
AI Gateway 必须与 IndexCore 服务/数据库/数据目录分离，不复用其写入进程。

### 3.1 优先接入既有环境，但不能假装它已可用

1. 软件和账号种子已从用户指定的本地 Bridge 项目定位，不再要求用户另找密钥。
   后续核实哪一个现有账号绑定 `.50`、CLI兼容版本、provider/模型路由和实际授权；
   不凭CSV文件名选账号，不把非空AK/SK当成已认证。部署阶段只将选定的一份凭据
   一次性导入 `.50` 的专用受保护profile，并记录不含秘密的引用/权限/验证时间；
   本轮未导入。不能复制全部账号目录、整个开发home，或把凭据复制到公网主机。
   CodeArts IDE 登录态不自动等于独立 API Key；不从历史日志或浏览器中挖掘密钥。
2. 旧 Bridge 路径缺失，不盲目重启、覆盖旧服务或升级 CLI。独立修复任务先核实
   本地权威源码、现有配置和兼容版本，再启动**专用受限**服务；不能复活带任意 shell/项目写入权限的
   开发 Worker 来处理公网用户输入。历史 CLI 版本仅作兼容线索，不自动采用最新版。
3. 优先同机解析模型凭据：`.50` 的模型 Key/AK/SK 留在 `.50`；公网仅持有专用
   Gateway 客户端凭据，不能把 SSH root 密码当应用凭据，也不复制开发账户配置。
4. 先核实两端现有私有网络。若没有，后续部署首选固定目标的反向 SSH Unix-socket
   通道：`.50` 主动连接公网主机，在受保护目录生成远端 socket，转发到 `.50` 本机
   Gateway；公网产品只读挂载**socket目录**并通过专用 HTTP transport Dial 访问。
   这避免把宿主 `127.0.0.1` 当成容器 loopback，也不需要公网监听8080/8765。
   使用专用受限 SSH 身份和网关鉴权，固定目标/禁用通用shell，不让网站持有root密码。
   实施时验证两端OpenSSH能力、转发限制、socket属主/权限、重连inode、目录挂载、
   Docker实际访问和断线恢复；若能力不符，先提交经验证的私有网络替代，不扩大绑定。
   语法能力参考 [OpenSSH ssh -R](https://man.openbsd.org/ssh#R) 与
   [StreamLocalBindMask](https://man.openbsd.org/sshd_config#StreamLocalBindMask)，
   官方支持不代表本机已配置可用，本轮没有建立通道。
5. 不把模型流量放入 IndexCore loopback Hint 端口，不改变 Hint 安全边界。

`.50` 是可能关机的本地虚拟机，不把它默认视为全天可用的公网基础设施。
AI健康/可用性与115登录、IndexCore可见性分别呈现。新任务准入前做短超时的能力检查，
AI离线时拒绝**新**调用、不扣新AI次数；同operation_key的已存在任务仍先返回原记录。
已预约后断线按真实调用是否确定处理，不用重建任务掩盖不确定性。前台不等长模型请求，
播放/下载/文件列表不等待AI健康检查或模型响应；恢复在线也不自动补发未知调用。

### 3.2 配置与密钥规范（拟定，不是当前已支持字段）

- 复用现有 [受保护文件读取](../internal/platform/bootstrap) 的权限/路径校验能力；
  使用专用 AI SecretResolver/credential_ref，不把115 CredentialRef 的语义硬套到模型。
- `.50` 专用服务使用自己的运行目录/用户和受保护凭据引用；只解析已核实的模型来源，
  不挂载 IndexCore 数据库配置、OpenList/115 会话或完整开发 home。
- 公网未来 AI 客户端配置放在本部署受保护 `private/ai/` 下，只读挂载；600文件、
  700目录、正确的运行 UID。桥接密钥与模型密钥分开；不写 compose 明文/Git/DB/日志。
- 拟定配置概念：`enabled`、`adapter`、`gateway_endpoint_ref`、`credential_ref`、
  `model_profile_ref`、超时/额度。**当前 product 配置严格拒绝未知字段**，不能把这些
  规划字段直接粘入现有 config.json；须先实现解析与兼容性测试。
- 模型选择来自已验证服务器配置；蓝图的 CodeArts/GLM 初始设想不是当前服务承诺。
  若现场只有另一协议网关，新增独立适配器并更新 ADR，不静默更换执行后端。
- 凭据不得出现在前端、Prompt、结构化结果、来源地址、错误、CLI 参数或任务 payload。
  后台只显示已配置/不可用/验证时间/配置标签，不提供明文回显。

## 4. 模块职责、依赖与扩展规范

以下包和端口均为**拟新增**，当前不存在；不因写入规划而宣称可调用。

| 模块 | 单一职责 | 明确禁止 |
| --- | --- | --- |
| `internal/agent` | 任务/结果/建议/证据模型，AgentPort、TaskStore、UsageStore，结构化校验规则 | HTTP/pgx/CodeArts SDK/115/OpenList 依赖 |
| `internal/product` AI Facade | 复用身份/Binding权限，组装最小输入，校验资源版本和使用额度 | 直调模型SDK或改 Canonical/Job |
| `internal/integrations/agent/codearts` | 将固定任务协议映射到经验证的 CodeArts Gateway，超时、取消、状态分类；先定义任务级capabilities | 通用 shell、任意项目运行、网盘/数据库工具 |
| 未来其他 Agent Adapter | 实现相同 AgentPort，接受同一合约测试 | 让模型类型渗入 Catalog/Acquisition |
| `internal/store/postgres` AI repository | AI Task/Result/Usage 的原子持久化、owner隔离、DB-time CAS | 重写既有迁移或绕过现有 Catalog 分类规则 |
| 独立 AI runner 组合 | 显式按类型领取既有 Job、构造模型客户端并管理生命周期 | 新建 Job Engine、抢领 ACQUISITION、阻塞下载 Tick |
| `internal/producthttp` | 严格 DTO、现有 cookie/Host/Origin、owner GET、建议展示 API | 接收用户传入的模型 endpoint/Key/root/owner/tools |
| 可选 EvidenceFetcher/TextExtractor | 后续获取受授权证据/限量文本，校验来源与输入范围 | 模型自己联网抓任意 URL、读取整盘或直连 OpenList 判断存在 |

调用方向：`UI -> product Facade -> agent ports -> injected adapters/store`。
`Catalog`、`Acquisition`、`IndexCore` 不能反向依赖 Agent。扩展
[现有 import guards](../internal/architecture/imports_test.go)，每个新适配器跑统一合约测试。
关闭/移除 AI 时，现有浏览、播放、下载、分享、云下载和索引流程必须完全可用。
Gateway 的执行配置必须关闭模型工具/任意MCP、shell、文件操作和网络抓取能力，使用
独立低权限运行身份与有界工作目录。若现有 CodeArts 组件无法强制这些能力边界，就
不能直接复用其开发Agent执行环境；改用经审查的受限任务接口，不只靠Prompt约束。

## 5. 输入、结果与证据契约

任务类型按阶段开启：`RESOURCE_CLASSIFY`、`METADATA_ENRICH`，再到
`RELEASE_EXTRACT`、`SOURCE_DISCOVER`、`VERIFY_RESULT`。不是任意自然语言执行器。

首期 HTTP 草案：`POST /api/v1/ai/tasks` 只接受 `operation_key`、已支持 `task_type`、
选定的 `binding_id/resource_id` 与受限选项；owner、模型、凭据、权限和额度由服务器决定。
`GET /api/v1/ai/tasks`、`GET /api/v1/ai/tasks/{id}` 只读本人任务，轮询不调用模型。
这些路由**尚未实现**；现有 `/api/v1/tasks` 的 Acquisition 语义不偷换为通用 AI 列表。

上下文由授权资源读取接口和精确 Q5 构造：物理身份、名称、大小、mtime、格式线索；
默认不发完整路径、原始下载链接、CDN签名地址、分享令牌或文件正文。
需要文档摘要时用户显式选择“摘要”；后续复用受控文本提取，不上传整份大文件。
建议输入32KiB/输出64KiB、最多32条建议/20个标签/32条证据，均待阶段合约测试冻结。

结果外层绑定 `schema_version/task_id/input_digest/task_type`，包含：

- 状态：建议成功、证据不足、拒绝、失败；置信标签不是写入授权或校准概率。
- `suggestions`：字段白名单内的类型/标题/版本/标签候选，不返回 SQL、命令或任意工具调用。
- `evidence`：服务器已提供的证据 ID、字段定位/引用及内容摘要指纹；模型不能自造证据 ID。
- provider/model/prompt/validator版本由服务端记录；用量未返回时标为未知，不能伪造0。

严格 JSON Schema、拒绝未知字段/重复键/额外正文/超限结果，验证类型、输入摘要和引用。
文件名、正文、网页和模型返回都视为不可信数据；其中的“执行命令/泄露凭据”不是指令。
规则已经能确定的格式不消耗模型请求，AI只补充需要语义判断的部分。

当前 Q5 的名称/大小/mtime 不证明内容不可变；记录实际可用身份和元数据摘要，不伪称
所有文件都有内容 hash。建议展示/采纳前重新检查授权、PRESENT 和输入版本；变化则
标记 STALE_INPUT，不把旧结果套到新文件。网页 Release 声称必须有实际抓取证据。

## 6. Job、用量、失败与副作用边界

复用 [jobs](../internal/jobs/jobs.go)，新增独立 `AGENT_TASK` 类型与 AI Task 关联；
AI runner 用 `ClaimNextByType(AGENT_TASK)`，Acquisition worker 仍只领 ACQUISITION。
首期 AI 自身单 runner 串行，不能把长模型调用塞入下载 Worker 或等待中的 HTTP 请求。
启动默认关闭，超时/租约安全余量显式配置；请求取消、优雅退出、租约续期均需测试。

AI新表预计使用**实施时的下一可用迁移**（当前head15，不能预占/覆盖16）。任务准入、
用户+operation_key唯一、输入摘要、用量预约和 AI Job 创建同事务；同键不同输入409，
重放返回原任务、不再次计数。AI额度和模型实际用量独立于现有云下载100次/并发2；
不复用或修改那张下载配额表。限额可配置，provider计费未知时不承诺免费模型调用。

在模型请求前持久化 request_id/调用预约。网关若支持按同一run_ref读状态，重用该引用；
若不支持幂等/结果查询，连接中断或提交不确定不得自动当成“未调用”重试计费。
未知结果进入明确恢复状态，不能无限重跑或声称外部计费 exactly-once。
仅证明请求未发出的短暂故障可按限次退避重试；拒绝、无凭据、格式不合法与预算耗尽
分别分类；不默认用另一模型自动修补坏结果。模型不应得到 DB-time lease 或所有权凭据。

结果、AI Task终态、相同 Job终态和实际可知用量在一个 DB-time lease/generation-fenced
事务提交；失败整体回滚。人工采纳建议是另一个明确的产品命令，不在模型调用事务中执行。
不改写旧8组预约、原ISO测试状态/失败预算、Manifest/Copy或 IndexCore Journal。

## 7. 双 UI 与采纳边界

前台：在文件详情增加“AI整理建议”，展示待处理/完成/证据不足/不可用、建议与依据。
建议面板独立更新，不清空文件卡、不重置播放、不闪登录提示。云下载页与文件库共享
同一 AI 组件；只处理真实已授权文件，不因 AI 不可用阻塞下载按钮。
后台：模型服务配置标签、连通状态、任务数量/错误类别/用量和显式开关；不回显秘密，
不把普通ADMIN默认变成可读取所有用户 Prompt/正文的跨用户超级账户。

首期只有建议，不写资源。后续“采纳”默认只写 CloudSite 的语义注释层：标签/摘要/
展示标题，不改变网盘文件、Canonical 名称和现有格式分类。逻辑 Asset/Release/Variant
绑定必须通过既有 Catalog 分类服务及单调约束，再次校验资源/权限，带 expected_version
和审计；没有已投影 Copy 时不得为通过测试直接造 Copy。副作用命令不授予 Agent。
AI来源候选只有用户选择“提交云下载”后才进入现有纯链接校验与准入/幂等/配额流程；
模型不能自动提交、修改磁力、复用旧操作或绕过 START_RESERVED。

## 8. 分阶段任务与验收（每段一个有界 PR）

首个开发任务已拆为 [Issue131：5.1合约与纯结果校验](https://github.com/nathanxiangang-web/cloudsite/issues/131)。
它在本规划验收后接续，不代表当前已实现或已启动真实模型调用。

| 阶段 | 范围 | 放行标准 |
| --- | --- | --- |
| 本次准备 | 现状、模块/协议/凭据/网络规划，更新项目记忆 | 文档与实机元数据一致，0模型调用/0运行变更；不宣布 Gate5 实现完成 |
| 5.1 合约与纯校验 | AgentPort、task/result/evidence值对象、严格校验、mock、import guards | 禁止工具调用/坏引用/超限输出测试；无凭据时确定性核心回归全过；不依赖现场服务 |
| 5.2 `.50` 接入恢复与适配 | 验证实际配置/兼容版本、受限Gateway、专用网络/凭据、取消/超时 | 先0模型只读preflight，再单次非敏感合成任务；真实协议、断线与脱敏证据；缺可用模型凭据则明确阻塞，不猜/不复制IDE令牌 |
| 5.3 Job/持久化/用量/API | 独立AGENT_TASK、owner隔离、DB-time完成事务、独立AI额度 | 真PG并发准入/重放/崩溃/过期lease/预算/跨用户隔离；无下载/索引状态修改；迁移与兼容回滚验收 |
| 5.4 真实文件整理 UI | 首期软件/视频/文档/其他的元数据建议与依据面板 | 实际前台/后台、AI关闭/离线/登录过期、播放不打断；先非敏感样本，再显式选择的真实文件；不自动采纳 |
| 5.5 Release/Source/摘要 | 受限证据获取和文本提取、明确候选与不足，不自动下载 | SSRF/重定向/大小/来源引用/Prompt注入/伪证据测试；每个能力单独开启；正文不默许整盘发送 |
| 5.6 可选采纳与Gate5收口 | 语义注释/逻辑分类的显式采纳、可替换Adapter合约回归 | 新旧资源版本冲突、单调绑定、审计/撤销策略、服务故障；AI完全禁用仍不影响现有确定性流程 |

先写5.1合约，不以 `.50` 当前故障阻塞纯接口开发；但阶段验收前不得冒称已连通或越过
5.2进入真实用户调用。步骤5.5只规划蓝图要求，不把网页抓取、搜索引擎/向量库/DAG/
自主Agent平台顺带加入第一版。每段实施前重查当前代码/配置，以新验收证据更新状态。

## 9. 测试、部署与回滚

- 合约：严格JSON、重复键、错误任务ID、过期上下文、无依据结果、拒绝/格式/传输错误，
  普通文件名中的指令注入；CodeArts假HTTP/受控进程测试不得当成实际模型可用证据。
- 持久化：仅隔离PG测试库，原子准入/完成、幂等用量、并发、取消、进程崩溃、未知调用、
  DB-time过期/旧claim、未知usage；Acquisition保持原实现与故障预算。
- UI：owner隔离、迟到响应、禁用/断线/缺凭据、无自动提交、无播放中断、无登录闪烁。
- 接线：`.50`服务与模型权限最小化、网络命名空间、鉴权/非授权请求、断线恢复、单次
  非敏感合成任务、凭据不进Prompt/日志；不以 `/health` 成功替代任务协议验证。
- 发布：代码/文档审查、exact-head CI，受控候选环境，再仅更新 owned AI/产品服务。
  保留当前产品镜像、配置/数据库备份；不动 IndexCore/OpenList/115登录和已有 Worker。
- 有新迁移时准备**兼容新schema的 AI-disabled fallback**；旧strict-schema15二进制
  不能直接拿来回滚新schema。不能为回滚清除运行数据或修改已执行迁移。
- 关闭AI/不可用不能取消已有下载或影响文件内容访问；不额外暴露公网管理端口。

## 10. 当前可交付与明确待办

本次可交付：可执行的有界分工、代码接点、任务协议/证据/用量/权限/双UI设计与测试门槛。
软件及本地私有凭据来源已找到。真正接入前仍须确认账号与 `.50` 的绑定、导入选定
受保护profile、验证实际模型权限，基于权威Bridge源码建立受限专用服务并验证两机私有连接。
模型凭据导入后留在 `.50`，不要求用户把秘密发到聊天，也不把开发Worker当成产品接口。
不能将本规划、文档CI或 CodeArts安装残留标记为 AI 已上线，也不强行关闭 Gate4/5/6。
