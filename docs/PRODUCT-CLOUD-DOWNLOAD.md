# 云下载接入准备与磁力兼容契约

2026-10-16，Issue108 / D-053。本文区分现有能力、当前准备工作与后续接线，
不以文档、离线测试或旧 Gate3 验收冒充当前公网云下载已可用。

## 1. 蓝图与实际缺口

后续实施已启动：Issue83 / D-054在当前基线上实现准入/用量及原始、执行、策略
三元组持久化，详见[准入模块](PRODUCT-ADMISSION.md)。追加开发迁移15，尚未升级
公网14或启用UI/Worker。以下Issue108内容记录准备阶段，不代表仍禁止已授权实现。

依据 MVP-BLUEPRINT 第8/9/10/11节及 GATE4-IMPLEMENTATION-PLAN：现有
SubmissionService、Manifest、ACQUISITION Job、115 Downloader、START_RESERVED、
单 Worker、Hint、Q5/Q8、Projector、原子 READY 已有实现与历史验收，不另建队列。
当前公网产品只组合登录、只读任务/文件、预览、302下载和产品分享；其 IndexCore
为只读查询组合，不能把它当作已启用的 Hint/增量写入运行时。

缺口是登录用户准入/用量、合法可写 Connection/Binding、受保护115原生下载会话、
当前产品数据库的投影闭环，以及单 Worker 与 IndexCore 正式运行时的安全接线。
当前 OpenList 的115 Open OAuth/应用访问 Token 不等于115driver所需的原生会话，
不得把两者互换，也不能假设旧会话今天仍有效。

Issue83仍有隔离、未提交且未验收的代码。其 draft migration14 已与已合并的
分享 migration14 冲突，后续须先审计、移植到当前基线并采用下一可用版本，不能覆盖
旧工作、修改已执行迁移或直接拿旧草稿升级公网。当前 schema14不变。

## 2. 模块划分与依赖

```text
消费者UI：粘贴来源、预览精简结果、提交、查看自己的任务
  -> HTTP鉴权 / Origin / 严格请求契约
  -> product Admission + Usage / 幂等操作
       -> acquisition.PrepareSource（纯输入策略，无网络）
       -> 原有 SubmissionService / Manifest / Job
  -> 原有单 Worker / RunOnce / StageDispatcher
  -> DownloaderProvider / 独立115会话
  -> 115完成 -> Hint -> IndexCore增量观察 / Canonical / Journal
  -> 原有Projector -> PRESENT Copy -> READY + 同一Job SUCCEEDED
```

前台只呈现文件和任务，不开放目录管理、provider_scope、Cookie或Root配置。
后台单独管理连接、存储目标、运行状态和明确的恢复操作；不增加文件删除功能。
IndexCore仍决定物理事实，OpenList只在其观察链路中采集；现有302内容访问模块
继续独立，不变成云下载验证器。新 Provider 通过端口和策略映射扩展，不修改 Job Engine。

## 3. 磁力策略，不是字符串硬截断

新增显式策略 `magnet-btih-name-v1`，由可信的准入组合按Provider能力选择，不能
让HTTP客户端任意选择策略或让Worker在恢复时重新清洗。仅保留一个有效BTIH身份及
可选dn；删除tr、ws等附加参数。参数顺序不固定，不使用按长度截断、简单split后
取前两项或不验证哈希的正则。只用合成示例，用户真实来源不写入Git：

```text
magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=demo-video&tr=...
->
magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=demo-video
```

- 支持40位十六进制及32位Base32 BTIH，统一成40位小写十六进制。
- dn可缺省；只作显示/提供方提示，不是可信的实际结果名或目录，也不擅自变成
  Manifest.expected_name。对保留值做正确URL编码，中文、空格、加号、&不会混淆。
- 相同身份/相同dn的重复项可去重；冲突BTIH或冲突dn拒绝，不猜第一项。
- 纯v2 btmh和混合v1/v2在本策略暂不降级，明确报不支持；不声称115已实测支持。
- so是选择文件语义，不是tracker；出现即拒绝本精简策略，避免变成整包下载。
- 错误转义、片段、控制字符、非法哈希或超限拒绝，不悄悄修复。
- 原始输入最多64KiB/512个参数，准备后仍须符合现有4096字节SourceRef限制。
  去掉过多tracker可以缩短来源；过长dn不被随意截断。
- 后续准备/准入HTTP必须同步采用有界请求体（上限512KiB，解析后来源仍限制64KiB），
  不能沿用Issue83草稿的8192字节请求上限而宣称支持完整输入范围。本轮没有新增HTTP。
- HTTP/HTTPS/ed2k保持原样，尤其不能清理HTTPS签名查询参数。

依据：[BEP9磁力格式](https://www.bittorrent.org/beps/bep_0009.html)、
[BEP53选择文件扩展](https://www.bittorrent.org/beps/bep_0053.html)。协议支持不等于
某个115端点实际兼容；本轮不进行真实下载。

## 4. 原始输入、执行来源与幂等

`PrepareSource`返回私有Original、执行Reference、策略版本、Changed和移除参数数。
普通JSON/字符串摘要隐藏原始及执行来源；不要记录完整magnet、任务引用或凭据。
纯函数自身不入库、不提交Job、不修改ResolveSource、SubmissionService或115 Adapter。
后续D-054准入模块已经组合纯函数与事务存储，见PRODUCT-ADMISSION；还未公网启用。

准入必须在同一受保护操作记录中持久化：原始输入、准备后的执行值、固定策略版本
及身份/目标/幂等信息。Manifest.source_ref使用已接受的执行值，之后直到Provider
仍严格逐字节一致。原始值留在操作记录追溯；不能只保留短值然后声称原文已保存。
未完成这种持久化和事务测试前，不将本策略接入现有提交/Worker。

同一用户同一operation_key必须复用原有操作/Manifest/Job及固定策略结果；冲突原始
意图拒绝。策略升级不能改变在途操作，不重新计算旧Manifest，也不跨用户全局去重。
不确定提交只能查询原操作证据，不能换ID再提交。用量按操作计，不按轮询/claim计。

## 5. 分段实施与验收

1. **本次准备**：核对蓝图、冻结D-053、实现纯输入策略及合成测试。无数据库迁移、
   API/UI启用、运行部署或真实账户调用。
2. **准入/持久化**：审计Issue83 WIP，按当前schema重做必要事务接线；原始/执行/
   策略三元组、权限、用量、幂等、冲突/崩溃/未知提交都通过真实隔离PG测试。
3. **运行依赖组合**：核实真实Provider会话和四坐标，确定合法可写Binding与当前产品
   Copy投影，启用一个有管理器的Worker和一个IndexCore写入运行时。保留严格127.0.0.1
   Hint边界，不能为了容器互通改成公网监听。现有Root与Binding如何复用/新增须从实际
   约束和目录映射确定，不能猜ID或造成重复资源展示。
4. **前后端接入**：前台新增云下载/我的任务入口，提交前显示精简结果；不让用户管理
   后端目录。后台提供配置、预检和运行观察。准备/排队/下载中/等待索引/READY/需恢复
   必须区分，失败不能自动新建下载。
5. **受控实测**：使用确定的小型合法测试源和目标验证完整同一Job闭环；Provider成功
   不等于READY。每一操作最多一次StartDownload；不确定副作用停止，保留预约。现有
   八组未知任务和原KNOWN完成记录保持不变，不复用旧GO。

每段独立文档、窄PR、测试/CI和同一智能体分阶段审查后再推进。不把磁力精简当成
万能修复，不把解析成功当成真实下载成功。当前网站仍未启用新的云下载入口。
