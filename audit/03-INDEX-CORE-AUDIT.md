# 03 IndexCore 深度审计

基线：`9a33d0113ea8d6ddd1f66834ba7a781e5007fe3e`，main，schema 5，141 个 Go 测试文件。审计 SHA 的 GitHub CI 为 success。本文区分 main、历史隔离证据和未合并 Draft，未运行生产或数据库写入。

## 1. 当前能力基线

| 领域 | main 的事实 |
|---|---|
| Collector | rclone、AList、OpenList；普通观察默认 additive-safe |
| OpenList scoped | 直接子项、严格响应校验、强制 PARTIAL、可选可信 mirror coverage |
| Canonical | per-root FIFO admission、root 锁、Canonical+Journal+admission 原子提交 |
| Incremental | 默认关闭；普通每轮最多 5 项；读池 opt-in，读取并行、提交串行 |
| F1/F2 | 已合并但默认 OFF；不能默认视为生产 SLA |
| F3 | 容量调查明确 NO-GO，暴露 overdue/freshness debt |
| F4 | PR147/148/149 均 Draft/open/unmerged，不属于 main |
| Query | Q1—Q9 只读；generation cursor 与 per-root Journal cursor |
| Hint | 独立 loopback+Bearer listener，默认关闭；202 只表示 durable merge |
| Issue144 | PR145 已合并，窄范围处理精确 consumed stale-head 证据 |

## 2. 优秀且应保留的设计

1. **Completeness 保守**：`internal/kernel/completeness/evaluate.go:49-100`，UNKNOWN 不变成空 COMPLETE。
2. **Scoped 响应严格**：`internal/collector/alist/scoped.go:94-155`，验证 total/count、唯一名称和硬上限。
3. **Topology/coverage 多重 fence**：`collector/alist/topology.go:18-93`、`store/postgres/directory_coverage.go:25-105`。
4. **Canonical 事务清晰**：`store/postgres/coordinator.go:248-503`，避免 Canonical/Journal/admission 部分提交。
5. **确定性顺序**：Snapshot entry 排序与持久化 admission 序列不依赖 provider 到达顺序。
6. **claimed/pending 分桶**：`store/postgres/incremental_work.go:164-225,433-520,619-735`，IN_FLIGHT 新信号不被旧成功吞掉。
7. **读并行写串行 fencing**：`store/postgres/scoped_read_lineage.go:216-299`，绑定 roster、adapter、claim、dependency 和 frontier。
8. **Source Budget**：`internal/sourcebudget/budget.go:214-265`，login/identity/topology/list 经过同一 governor。
9. **故障证据诚实**：F3 明确记录未观察文件和 NO-GO，而非用绿色 CI 冒充 SLA。

## 3. P1 问题

### IC-01 — UNRESOLVED/CONFLICT 仍可能推进 MISSING 和删除

- 类型：FACT（代码分支）；实际影响 INFERENCE。
- 位置：`internal/kernel/reconcile/reconcile.go:21-67` `Reconcile`；`identity.go:115-118,165-211`。
- 数据/API：`index_canonical_resource`、Journal；controlled COMPLETE Snapshot。
- 触发：同路径重复、mtime 缺失/漂移等使 entry 不匹配，但 acceptance 为 COMPLETE；只有 `ResolutionMatched` 写入 observed。
- 影响：旧 PRESENT 可被视为未观察，连续独立 Snapshot 后进入 REMOVAL_CANDIDATE/REMOVED；与 `docs/architecture/GATE1B-SAFE-RECONCILE.md:288-300` 的冻结语义冲突。
- 建议：建立与冲突相关的保护集合，区分无关冲突与同源身份冲突，禁止后者推进删除。
- 难度：中高。
- 测试：无 hash+mtime 缺失、同路径重复 ID，连续两次 COMPLETE/grace=0，断言无 removal/Journal。

### IC-02 — R8 同路径替换存在不收敛或双方删除风险

- 类型：FACT（身份分支）；具体序列影响 INFERENCE/UNKNOWN，需 PG 重放。
- 位置：`identity.go:115-129`、`reconcile/directory.go:16-49`、`reconcile.go:40-67`；ADR-004。
- 数据/API：Q5 ambiguity、Canonical、Journal。
- 触发：A→PARTIAL(B) 产生同路径双 PRESENT；mirror 只看到 B 时路径冲突被保护，A/B 都可能保留；普通 COMPLETE 又可能把未匹配双方推进 MISSING。
- 影响：mirror 永久 ambiguous，或当前 B 在特定序列下误删。
- 建议：设计可审计替换 lineage/冲突解决，不按 newest/path 猜 winner。
- 难度：高。
- 测试：A→PARTIAL(B)→多次 mirror(B)，以及两次 COMPLETE(B)，验证当前对象不丢、旧对象可解释、Q5/Q8 收敛。

### IC-03 — 同一 Snapshot 重复稳定 provider ID 未判重

- 类型：FACT。
- 位置：`identity.go:94-111` 只检查 prior 匹配数；`reconcile.go:25-39` 逐项生成转换。
- 数据/API：`ProviderObjectIDScope+ProviderObjectID`；Canonical/Journal。
- 触发：同 Snapshot 两个路径声明相同稳定 ID。
- 影响：INFERENCE——同一资源重复 MOVE/RENAME/UPDATE，最终路径由 entry 排序决定；确定性错误仍是身份错误。
- 建议：observation index 增加 stable ID 计数，重复时相关 entry 全部冲突并保护 prior。
- 难度：中。
- 测试：同 ID 不同路径、同路径不同 local ID、重复 ID+属性变化，断言零 MOVE/UPDATE/Journal/MISSING。

### IC-04 — 正常 reconcile 的 counts/conflicts 未持久化

- 类型：FACT。
- 位置：`internal/store/postgres/tx.go:286-299` `insertResult` 硬编码 `{}`；`decision.go:18-53`；`diagnostic.go:352-358`。
- 数据/API：`index_reconcile_result.counts/conflicts`；root diagnose。
- 触发：正常 APPLIED/NOOP reconcile 产生真实冲突。
- 影响：冲突在内存发生却在 durable result 消失，诊断漏报，削弱 IC-01—03 排查。
- 建议：持久化真实 counts 和有界 conflicts；STALE_INPUT 保持 Issue144 所需精确空证据。
- 难度：小到中。
- 测试：制造冲突后检查表、latest diagnostic、历史 admission 和 stale negative matrix。

### IC-05 — Journal payload 只有 reason，不是自包含变更日志

- 类型：FACT。
- 位置：`internal/store/postgres/decision.go:20-31`；期望语义见 `GATE1C-POSTGRESQL-STORE.md:542-547`。
- 数据/API：Q8 `/v1/roots/{root_id}/journal`；Journal payload。
- 触发：消费者仅依据 Q8 重放 ADD/UPDATE/RENAME/MOVE/REMOVE。
- 影响：无法仅凭事件得到 old/new path、attributes、generation 等；Q8 只能作失效通知+回读。
- 建议：引入版本化 payload，保留 legacy reason-only 兼容；不得改写 append-only 旧事件。
- 难度：中高。
- 测试：仅用 Q8 重放并与 Q6 对比；覆盖 MOVE+UPDATE 顺序和 legacy 事件。

### IC-06 — 普通 full-scan 缺 adapter ABA/namespace fencing

- 类型：FACT。
- 位置：`internal/runtime/scan/scan.go:130-169,194-202`；`store/postgres/root_admin.go:192-202`；scoped fence 仅在 `scoped_fetch_fence.go`。
- 数据/API：Root adapter config、Snapshot admission。
- 触发：慢 full-scan 期间修改 collector/path/base_url 或 A→B→A。
- 影响：INFERENCE——旧命名空间 Snapshot 可进入新配置 Root；普通 additive 路径主要造成错误 add/update，升级为 COMPLETE 时风险更大。
- 建议：full-scan 捕获 adapter digest/revision，并在 Stage1/Stage2 重验；变化返回 STALE_INPUT。
- 难度：中。
- 测试：配置 ABA、删除配置、Root lifecycle 改变，断言旧 source 不能写入。

### IC-07 — Issue134 容量与 freshness debt 是生产 BLOCKER

- 类型：FACT（仓库内隔离测量）；生产实际吞吐 UNKNOWN。
- 位置：`docs/p0/P0-134-F3-CAPACITY-INVESTIGATION.md:72-90,120-188`；`runtime/incrementalorch/config.go:15-18`；`incrementalruntime/config.go:18-24`。
- 数据/API：Watch/Work/Q5/Q8；Source Budget。
- 触发：大量短周期 Watch、持续变化和每轮小上限。
- 影响：50K fixture 中 49,972 overdue、500 变化文件 492 未观察；PENDING 很小仍不代表无积压。Issue134/F2/F4 的 freshness/debt/capacity 证据仍为 NO-GO；这与历史 Gate 3 的 20K PostgreSQL full-runtime-ingestion 基线 PASS 是不同验收口径。完整 20K 源 traversal、长稳和生产部署后容量仍未验证。
- 建议：维持 NO-GO；显式 debt age/slope、生命周期、滚动预算和端到端验收，不能只加 Reader 或索引。
- 难度：高。
- 测试：授权部署后执行真实 20K+ traversal、多 Root churn、无 Hint 晚到文件、长稳、真实 HTTP/Q5/Q8 测量。

### IC-08 — item-local INTERNAL 可使整个 runtime/serve fail closed

- 类型：FACT（设计行为）；是否应改是架构评审项。
- 位置：`runtime/incrementalexec/cycle.go:165-180`、`internal/runtime/incrementalruntime/runtime.go:305-325`、`runtime/app/app.go:325-344`。
- 数据/API：incremental Work；serve 生命周期。
- 触发：单 scope 返回普通 INTERNAL。
- 影响：整个 runtime 退出，serve 失败；Issue144 只修精确 STALE_INPUT，不覆盖其他 INTERNAL。
- 建议：不要广泛吞错；先区分可证明 item-local provider/scope 故障与 DB/invariant/systemic 故障，再评审隔离策略。
- 难度：中高。
- 测试：多 scope 注入 typed provider、SQLSTATE、admission mismatch，只有明确 item-local 类可隔离，系统错误仍 fatal。

## 4. P2/P3 问题

### IC-09 — P2：scoped commit 加载全 Root prior，索引与排序不匹配

- 证据：`store/postgres/prior.go:21-35` LATERAL `ORDER BY observation_id DESC`；`internal/store/postgres/migrations/0001_init.sql:89-90` 索引不匹配；`coordinator.go:336-349,422-426` 全量 prior。
- 数据/API：Canonical/observation/Snapshot；串行 root 锁事务。
- 触发/影响：Root 和历史增长增加传输、decode、SQL 往返与锁时间；读池不能消除串行成本。
- 建议：独立评审 `(resource_id, observation_id DESC)`、批量 SQL 和 terminal-history 索引；不以有损 cache 替代真相。
- 难度：索引/批量中，安全 cache 高。
- 测试：EXPLAIN、20K/50K 长历史、锁等待、WAL/index build 成本；PR149 的 0008 不是 main 已有能力。

### IC-10 — P2：Snapshot/DRAFT/result/Watch 缺长期保留策略

- 证据：`snapshot_create_admit.go:27-48` 每扫描写 Snapshot；`coordinator.go:395-410` NOOP 仍写生命周期/result；生产无自动 retirement。
- 触发/影响：重复 NOOP、崩溃 DRAFT、Watch 增长导致存储、WAL、备份和 VACUUM 压力。
- 建议：区分不可变 Journal/必要身份历史与可退役 inert 数据，制定保留、备份与恢复策略。
- 难度：中高。
- 测试：长期 NOOP、崩溃 DRAFT、保留后不可执行证明、备份恢复、Watch COLD 与物理清理区别。

### IC-11 — P2：Query limit 无最大值，非法参数静默回退

- 证据：`transport/httpapi/server.go:250-263`；`store/postgres/query.go:180-220`；`internal/store/postgres/migrations/0001_init.sql:60-65`；server 仅 ReadHeaderTimeout。
- 数据/API：Q4/Q6/Q7/Q8 limit、after_seq、cursor。
- 触发/影响：超大 limit 内存/响应放大；非法 after_seq 回退 0 可全 Journal 重读；keyset 排序缺匹配索引。
- 建议：最大页大小、非法输入 400、表达式索引评估、HTTP/DB 超时。
- 难度：小到中。
- 测试：超大/负数/溢出 limit、非法 after_seq、50K 分页、慢客户端和取消。

### IC-12 — P2：普通 OpenList Collector 缺响应和分页硬上限

- 证据：`collector/alist/adapter.go:126-152` 无页数/条目/无进展上限；`:171-197` 无界 `io.ReadAll`；`:37-103` 全树内存累积。
- 数据/API：普通 listAll，与 scoped 16MiB/typed error 路径不同。
- 触发/影响：重复非空页、异常 total、超大 body 导致高内存或长扫描；通常 additive-safe，主要是可用性。
- 建议：普通/scoped 共用有界 transport、进度和重复页检测、typed error。
- 难度：中。
- 测试：重复页、total=-1/变化、超大 body、十万条目、timeout/cancel，不得升级为 COMPLETE。

### IC-13 — P2：plaintext-secret 配置校验可绕过

- 证据：`store/postgres/adapter_config_validate.go:54-65` 以 `env` 结尾即放行，`passwordenv` 可通过；base_url userinfo 未拒绝。
- 数据/API：`index_root_adapter_config`；受信任管理写入。
- 触发/影响：误配置可持久化真实秘密，与“永不持久化明文 secret”声明冲突。
- 建议：collector 严格 allowlist、精确 env 字段和变量名语法、拒绝 URL userinfo 与未知 secret 字段。
- 难度：小到中。
- 测试：`passwordenv`、嵌套 secret、URL userinfo、合法 env ref、未知字段。

### IC-14 — P2：writer lock 不是完整分布式租约

- 证据：`store/postgres/writer_lock.go:25-49` 专用 session 无健康监控/fencing generation；`runtime/scan/scan.go:142-202` 超时不覆盖 admission；`runtime/app/app.go:362-383` shutdown 等 actor。
- 触发/影响：锁连接失效、DB hang 或第二 daemon 竞争时，进程可能未及时发现；这不是 HA 实现。
- 建议：明确单写运行约束；如需更强保证，单独评审锁连接监控、fencing、statement/lock timeout。
- 难度：高。
- 测试：只 kill writer-lock connection、DB hang、lock wait、第二 writer 和关闭。

### IC-15 — P3：provider failure 指标分类过宽且不一致

- 证据：`incremental/state/model.go:122-126` 除 ROOT_INACTIVE 外都算 provider failure；`store/postgres/diagnostic.go:202-207` 只统计部分类。
- 影响：CONFIG_INVALID/INVALID_SCOPE/INTERNAL 计数与诊断语义漂移。
- 建议：区分 provider、永久 scope/config、本地 INTERNAL；检查 mirror INTERNAL 两次恢复依赖。
- 难度：低到中；测试逐 error class 校验 Work/Watch/诊断。

### IC-16 — P3：append-only 数据库防御深度不一致

- 证据：`internal/store/postgres/migrations/0001_init.sql:211-226` trigger 只保护 Journal 和 identity observation；`index_applied_snapshot` 主要靠代码约定。
- 影响：不是公开篡改路径，但最小权限/防御深度不一致。
- 建议：评审核对、最小权限或 trigger，不能只依赖“没有 UPDATE 方法”。
- 难度：低到中；测试非 owner/owner/只读角色的 UPDATE/DELETE 权限矩阵。

## 5. 身份与操作语义

| 操作 | main 的行为 |
|---|---|
| 新增 | 无匹配时新 UUID；Canonical+ADD Journal 原子提交 |
| 同内容复制到新路径 | 原对象仍 PRESENT 时新 resource_id；hash 不是对象身份 |
| 同路径替换 | 通常 R8 新对象；收敛受 IC-01/02 影响 |
| 稳定 ID 重命名/移动 | R1 保持 resource_id；RENAME/MOVE 先于 UPDATE |
| 无稳定 ID 重命名/移动 | 仅旧 MISSING、horizon 内、唯一候选/目的地才保持，否则新建/冲突 |
| OpenList/rclone 普通路径 | 当前不提供可用稳定 provider ID，不能依赖 R1 |
| 目录移动 | 后代独立匹配，没有批量保持所有后代 ID |
| 普通 PARTIAL 删除 | 不推进 absence |
| COMPLETE 删除 | 首次 MISSING，后续独立 Snapshot+阈值+grace 才 tombstone |
| 可信 mirror 删除 | 有效 coverage 可直接删除 direct child/缺失 subtree |
| tombstone 后重现 | 新 resource_id，不复活旧 ID |

因此 CloudSite 不能把 IndexCore resource_id 当成跨替换、删除重现和所有目录操作都永恒不变的 115 对象 ID。

## 6. OpenList 不确定响应处理

- transport/API/total/count/重复名称错误：不创建 Snapshot。
- `SCOPE_TOO_LARGE`：BLOCKED，不将大目录错误解释为空。
- weak/unknown assurance：PARTIAL/SUSPICIOUS，只允许正向观察。
- 有效 fresh identity + 完整直接子项 + topology before/after 一致 + generation/adapter/parent/claim/dependency 有效：才可能获得 mirror coverage。
- TRANSIENT/THROTTLED：durable RETRY_WAIT；AUTH/SCOPE/CONFIG：BLOCKED。
- 截至本基线已审计代码，topology absence authority 白名单为具体 `"115 Open"` covering storage；这不是对所有 115/OpenList API 能力的永久断言。

## 7. main 与 Draft 严格分界

| 项目 | 状态 | 可得结论 |
|---|---|---|
| PR147 | Draft/open/unmerged | replay/test/证据；F1 改善有限，F2 20K NO-GO |
| PR148 | Draft/open/unmerged | schema6、滚动预算和 Watch lifecycle 提案；20K 仍 NO-GO |
| PR149 | Draft/open/unmerged | stacked schema7/8、Root budget 注册和索引；基本部署回执不等于容量/长稳 |
| main PR145 | merged | 只修精确 consumed stale-head，不解决所有 INTERNAL/freshness |

## 8. CloudSite 可复用边界

- Query 可用于物理树、精确路径、当前清单、Journal 和 Root 状态；CloudSite 不得直接连接 IndexCore PostgreSQL。
- 当前 Q8 应采用“事件失效→按 resource_id 回读 Q3/必要 Q4/Q5→身份校验与投影提交→在同一事务原子推进 cursor”的模式，不可独立重放完整数据库。处理 removal 时 Q3 必须显式 `include_removed=true`；Root 进入 DEPRECATED/DELETED 时还需相应 lifecycle visibility。404 不能单独解释为删除。回读失败、gap 或身份冲突时不得推进 cursor。
- Hint 只能由 server-only 同宿主受信任组件发送；需 CloudSite 自己的 outbox/去重/重试，因为 Hint 无幂等键。
- 独立 TypeScript reference consumer 可使用 server-only typed IndexCore client；这属于实现选择，不得绕过 CloudSite 授权，也不得变成新架构冻结。