# CloudSite 安全过渡与架构审查意见（2026-11-16）

> **审查状态：文档级架构修正；不构成代码实施、数据库迁移、真实 115 写操作或生产部署授权。**
>
> 审查基线：CloudSite `main` = `1b5b560bb8006ea69f05c61926040c3d9ad7679f`，VERSION 1.0.3，CurrentSchemaVersion 21。PR #191 和 PR #192 均为未合并的文档提交，不代表 main 已具备相应能力。

关联提交：
- [PR #191：多用户独立云下载空间与 CRUD](https://github.com/nathanxiangang-web/cloudsite/pull/191)
- [PR #192：全系统深度技术审计与优化路线图](https://github.com/nathanxiangang-web/cloudsite/pull/192)
- [PR #190：云下载 Session 健康前置检查](https://github.com/nathanxiangang-web/cloudsite/pull/190)
- [PR #186：取消下载次数和任务名额限制](https://github.com/nathanxiangang-web/cloudsite/pull/186)

---

## 1. 架构结论

### PR #191

**方向有条件认可，原文不应直接作为实施合同。**

正确且必须保留的设计：
- 以不可变 `user_id` 作为空间所有权，不以 login、目录名或客户端输入作为身份。
- 继续使用一个受控运营方 115 账号、现有 Provider Session、现有 Worker；不为每个用户复制 Binding、Root、Worker 或下载引擎。
- 新任务在服务端解析 UserSpace，Manifest 冻结 UserSpace、Binding、Provider 目标和 Canonical 路径。
- 历史 Manifest 的 `user_space_id=NULL` 保持原样，不自动回填、不重新启动、不批量移动历史文件。
- 所有私有资源访问必须在服务端 ACL 层完成，不能依赖前端隐藏。
- 115 写操作结果必须区分 APPLIED / NOT_APPLIED / UNKNOWN / PARTIAL；UNKNOWN 先只读对账，禁止盲目重试。
- 取消离线任务与删除已下载文件分离；取消必须保持 `deleteFiles=false`。

必须修正的合同缺口：
1. 新 API 不得改变既有 v1 请求含义。当前 `POST /api/v1/cloud-downloads` 接受 BindingID/TargetPath；新空间模式不接受客户端指定目标。应新增 v2 提交接口，v1 在过渡期保持原契约，启用私有空间后对 v1 新写入 fail closed（返回明确 409），v1 历史读取保留。
2. 当前 main 仍是全站不限累计次数/不限每用户并发；新上限是目标策略，不是 main 现状。代码发布前不得在 UI、API 或文档中宣称已生效。
3. `UserSpace.owner_user_id` 必须保留删除账号后的历史归属，不得 FK 级联到会被物理删除的 `product_users`。账号删除事务需原子标记 OWNER_DELETED；同名新账号因 UUID 不同不能继承旧空间。
4. `InfoHash` 单独不足以识别共享 115 账号中的任务。任务归属至少联合核验 InfoHash、预期 DirId，并保留足够的任务时间/结果证据；不唯一时进入 UNKNOWN/RECONCILE_REQUIRED。
5. 115 的 Mkdir/Rename/Move/Delete 是外部副作用，不存在跨 PostgreSQL 与 115 的 ACID。每个写操作必须先持久化 intent/fence，写后只读核验；超时不得直接重发。
6. 文件读取 ACL 必须先于缓存、ETag、分页、搜索和计数。需覆盖 List/Resolve/Content/Preview/HEAD/Range/Share/Redeem/302；普通库必须排除整个私有下载根，downloads surface 只投影当前 user_id 的 prefix。
7. 首期不得把完整文件管理 CRUD 一次性上线。先完成空间身份、任务归属、读取隔离和只读浏览；Mkdir/ Rename/Move 分别验收，Delete 最后单独批准。

### PR #192

审计发现和 FACT / INFERENCE / UNKNOWN / BLOCKER 区分值得保留。路线图需避免将 54 项技术债同时变成施工计划。

- 先处理凭证、备份/恢复、生产证据与关键正确性。
- 再建立用户空间最小安全闭环。
- 通用 FileOperation Engine 仅在多个真实操作证明重复需求后再抽象，不先造大框架。
- Next.js、独立搜索集群、HA、多 writer、AI、自主文件操作、20K/长稳压测均不属于本次安全过渡必需项。
- 115driver 上游问题与 CloudSite 当前实际调用面分开陈述；上游存在风险不等于 CloudSite 已发生相同事故。

---

## 2. 云下载 1500 上限：目标合同

> 本次按“**每个 CloudSite 用户 UUID 的累计云下载提交次数上限为 1500 次**”定义。它不是并发数、文件数、单文件大小、存储容量或全站总量。

| 项目 | 规则 |
|---|---|
| 计数主体 | 不可变 `user_id`；不使用 login 作为计数键 |
| 上限 | 每用户累计 1500 次已持久化的新云下载操作 |
| 计数单位 | 一个新的 `(user_id, operation_key)` 操作意图计 1 次；一个任务包含多个文件仍计 1 次 |
| 重放 | 相同 operation_key + 相同 intent digest 重放原 Manifest/Job，不重复计数 |
| 冲突 | 相同 key、不同 intent 返回冲突，不创建新任务 |
| 状态查询/轮询/Worker claim | 不增加次数 |
| 失败/取消/UNKNOWN | 已提交的操作意图计数一次，不自动返还；不得通过重试创建第二份意图 |
| 历史数据 | 保留所有历史 Manifest/Job/ProviderTask；按现有 `product_operations` 用户账本计数，不按文件数或 Manifest 猜测 |
| 已超过 1500 的用户 | 历史记录不删除、不重置；禁止新的不同意图，允许原 operation_key 的安全重放和历史任务恢复 |
| 并发限制 | 不恢复每用户并发任务上限；避免重现 PR #186 的 RECOVERY_REQUIRED 占满并发名额问题 |
| 系统保护 | 账号/IP 速率、全局队列水位、Provider 预算和公平调度独立于 1500 次业务上限 |
| 存储容量 | 不定义每用户字节/GiB 配额；不将 1500 解释为容量或文件大小 |

**明确区分：** UserSpace 只隔离用户身份、目录和访问权限，不做个人存储容量分配或容量限制；不按个人已用字节、剩余字节或文件数量拒绝下载。1500 只限制云下载提交次数，不限制用户空间可占用容量，也不代表单文件大小限制。

实现约束：
- 新操作计数必须在同一 PostgreSQL 事务中，与 Manifest 和 operation ledger 原子提交。
- 使用现有 per-user quota row 锁串行化新提交；先验证重放，再判断 1500 上限，避免上限达到后历史重放被误拒。
- 超限拒绝不得创建 Manifest、Job、operation ledger 行或触发 Provider 请求。
- 计数以 `product_operations` 为准；不以所有历史 Manifest 总数代替，因为其中包含未归属/旧路径数据。
- 保持 API 字段兼容：`operation_limit=1500`、`used`、`remaining=max(0,1500-used)`；`unlimited=false`。明确 `concurrency_limit=0` 表示无每用户并发上限（必要时增加 `concurrency_unlimited=true`），不能继续把“无限”同时用于累计次数和并发两个维度。
- 当前 main 仍未执行此政策。该规则必须经过独立实现、测试和发布后才生效；文档修改本身不会改变线上限制。

---

## 3. 安全过渡设计

### Gate 0 — 生产基线和恢复能力（未通过不得迁移）

必须取得脱敏证据：
- CloudSite / IndexCore 实际二进制 SHA、镜像 digest、启动参数和服务健康状态。
- CloudSite 实际 `schema_migrations` 版本与校验和；不能以静态资源 SHA 推断整个二进制/schema 一致。
- PostgreSQL 一致性备份（逻辑或物理备份/WAL 策略）、远端加密与 ACL、保留期。
- 在隔离环境完成恢复演练，并核对账号、Manifest、Job、ProviderTask、操作账本和关键身份关系。
- 修复生产秘密文件过宽权限（审计记录为 `.env 0644 root:root`）并确认秘密轮换/备份策略；不得把秘密值写入审计材料。
- 取得当前队列积压、RECOVERY_REQUIRED、连接/锁/磁盘和 Provider 健康的只读快照。

当前 PR #192 将 DEP-01/02/03/07 标记为 P1 或 BLOCKER；这些证据缺失时，只能做文档和隔离测试，不能宣称生产迁移可回滚。

### Gate 1 — 兼容性准备

当前 migrator 对 migration history 的校验是严格的：
- v1.0.3 只认识到 schema 21；若数据库出现未知 migration 22，旧二进制会判为不兼容。
- 含 migration 22 的新二进制也会把 schema 21 视为存在 pending migration，不会自动当作兼容状态。
- 因此不能简单声称“新增列/表是 additive，所以可以直接回滚 v1.0.3”。

实施前必须二选一并验证：
1. 提供一个显式支持 schema21/schema22 的兼容发布版本（需调整兼容性判定并测试，不自动执行 migration）；或
2. 明确采用停机、完整备份、迁移后不兼容回滚只能恢复整套数据库与对应旧制品的方案，并接受恢复点之后的数据损失。

优先选择兼容发布版本。migration 由显式运维命令执行，不得在服务启动时自动执行。

### Gate 2 — Expand（只扩展，不切流）

- 追加 migration，不修改历史 SQL/checksum。
- 新增 `cloud_user_spaces`、`cloud_space_operations`；给 Manifest 增加 nullable `user_space_id`。
- 旧 Manifest 保持 NULL；不回填、不移动、不重命名、不删除任何旧 115 文件。
- owner UUID 不依赖可被物理删除的 product_users 外键；删除账号时同事务写 OWNER_DELETED。
- 新表/列在 feature flag 默认关闭时不改变旧产品行为。
- migration 前后核对行数、主键、外键、唯一性和关键状态分布。

### Gate 3 — 新 API 与 ACL（无 Provider 写入）

- 新建 v2 API，目标 Binding、native directory ID、Canonical prefix 全由服务端解析。
- v1 API 保持原语义；私有空间对该用户启用后，v1 新写入返回明确 409，不得回退到共享旧目录。
- v1 历史任务读取保持 owner-only；旧任务继续使用冻结的旧目标。
- A/B/ADMIN/匿名权限矩阵覆盖列表、详情、搜索、统计、cursor、ETag、Content、Preview、HEAD、Range、Share、Redeem 和 302。
- 私有 ACL 在缓存/计数/分页之前执行；不得用前端过滤替代。
- 此 Gate 不调用真实 115，不创建目录。

### Gate 4 — Provider 最小闭环（独立授权）

- 仅在 fake provider、PostgreSQL integration、race、fault-injection 和 IndexCore 一致性测试全部通过后申请真实接口授权。
- 先只读核验运营方父目录、Binding、Connection、115 账号身份和 IndexCore Root。
- 显式 provision；先写操作意图，再 Mkdir，随后通过父目录 ListPage + Stat/Parents 链唯一核验。
- 超时/响应丢失进入 RECONCILE_REQUIRED；不得自动重复 Mkdir。
- 双用户测试：相同 InfoHash、不同 DirId、交叉读取/取消/移动/删除均必须拒绝。
- 首期只开放下载任务和只读文件列表；删除不属于首期。

### Gate 5 — 逐项开放写能力

顺序：Mkdir → Rename → Move →（Copy/Upload 单独评审）→ 单文件 Delete → 空目录 Delete。非空递归/批量删除和永久清理默认不开放。

每项均需独立操作台账、request_digest、expected_version/CAS、side-effect fence、APPLIED/NOT_APPLIED/UNKNOWN/PARTIAL、逐项回执、IndexCore Hint/Q5/Q8 核验和恢复方案。

---

## 4. 回滚与停止规则

1. Feature flag 关闭后停止新 UserSpace 写入；不得删除 UserSpace 数据或历史账本。
2. 保持 schema22 的情况下，只能回滚到明确支持 schema22 的兼容版本；不得直接运行只认识 schema21 的 v1.0.3。
3. 不做常规 down migration，不删除新表，不重写历史 migration。
4. 115 已发生的目录/文件副作用不能靠数据库回滚撤销；必须通过只读核验和受控补偿操作处理。
5. 不自动重试 UNKNOWN，不自动清理重复目录，不自动认领不明文件。
6. 发现跨用户可见、错误 owner、错误 DirId、凭证泄露、备份不可恢复或 migration history 异常，立即停止该 Gate。
7. 两次同根因验证失败后停止扩大改动，定位根因后再申请继续。
8. PR 合并、生产部署、真实 115 写入均需独立明确授权；本审查不自动合并、不部署。

---

## 5. 必须保留与暂缓范围

### 必须保留
- 现有稳定 API 和 DTO 的兼容路径。
- Manifest / Job 稳定 ID、operation_key 幂等和数据库时间 fencing。
- IndexCore 作为 Canonical 物理观察真相；CloudSite 不直写 Canonical。
- 现有单 Worker、Provider Session/凭证隔离、下载/预览授权。
- 历史任务、历史迁移校验和只读恢复能力。
- 115 物理事实、CloudSite 语义身份、IndexCore 观察事实三者边界。

### 本轮不做
- 不批量迁移历史文件和任务。
- 不新建第二 Worker、第二扫描器、第二 Canonical writer。
- 不按用户复制 StorageBinding 或 IndexCore Root。
- 不做 Next.js 全量重写、独立搜索集群、HA/multi-writer、AI 自主文件操作。
- 不把 54 条技术债全部列为近期实施；按真实风险分批关闭。
- 不开展未授权的生产 20K、长稳、吞吐或真实网盘压力测试。

---

## 6. 架构验收结论

- **PR #191：REQUEST_CHANGES（修订后再评审）**。重点补齐 v1/v2 API 兼容、1500 计数口径、migration 回滚兼容、最小首发范围。
- **PR #192：审计内容可保留，需加入本审查并修正 Q06 与路线图优先级**。当前生产 UNKNOWN/BLOCKER 不得降级。
- **本轮只修改文档分支**；main、生产数据库、115 网盘、CloudSite/IndexCore 源码均不变。
