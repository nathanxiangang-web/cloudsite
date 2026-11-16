# 14 关键结论证据索引

## 1. 证据规则

- `FACT`：指定 SHA 的代码、migration、GitHub 状态或本次只读运行观测直接证明。
- `INFERENCE`：由代码链推导的条件性影响，不表示线上事故已发生。
- `UNKNOWN`：真实服务响应、生产配置或未获权限材料不足。
- `BLOCKER`：在补证前不能作对应生产/容量结论。
- 行号以基线 SHA 为准，路径均相对对应仓库。为控制篇幅，带目录前缀使用固定缩写：CloudSite `providers/115/*`、`acquisition/*`、`projector/*`、`store/postgres/*`、`product/*`、`producthttp/*`、`fileaccess/*`、`libraryview/*`、`integrations/*` 等均省略首个 `internal/`；IndexCore `kernel/*`、`collector/*`、`store/postgres/*`、`runtime/*`、`transport/*`、`incremental/*`、`sourcebudget/*` 等同样省略首个 `internal/`，`incrementalruntime/*` 省略 `internal/runtime/`。同一证据条目内，后续裸文件名继承同条首个目录，但以下跨目录专用映射优先：CloudSite 裸 `*repository.go`、`*store.go`、`*lifecycle.go`、`admin_pending_cancellation.go`、`migrator.go`→`internal/store/postgres/`，`poll.go`→`internal/providers/115/`，`canonical.go`、`stage_dispatch.go`、`provider_outcome.go`→`internal/acquisition/`，`hint_client.go`→`internal/integrations/indexcore/`；IndexCore `identity.go`、`reconcile.go`、`reconcile/*`→`internal/kernel/reconcile/`，`scope_identity.go`→`internal/collector/alist/`，裸 `tx.go`、`decision.go`、`diagnostic.go`、`prior.go`、`coordinator.go`、`snapshot_create_admit.go`、`scoped_fetch_fence.go`、`query.go`、`writer_lock.go`、`incremental_work.go`、`adapter_config_validate.go`、`root_admin.go`→`internal/store/postgres/`，`migrations/0001_init.sql`→`internal/store/postgres/migrations/0001_init.sql`；115driver 裸驱动文件名→`pkg/driver/`，`cmd/*`→`cli/cmd/`，`server/server.go`→`mcp/server/server.go`。

## 2. 基线与 GitHub 证据

| 项目 | 基线/状态 | 证据 |
|---|---|---|
| CloudSite | main `1b5b560bb8006ea69f05c61926040c3d9ad7679f`，工作树干净 | 本地 `git status/rev-parse`；`go.mod:1-28`；`VERSION:1`；`migrator.go:19-21` |
| CloudSite CI | run 43830975446 success；release 43831989416 success | GitHub Actions 对精确 HEAD 的只读 `gh run list --commit` |
| IndexCore | main `9a33d0113ea8d6ddd1f66834ba7a781e5007fe3e`，工作树干净 | 本地 git；`go.mod:1-13` |
| IndexCore CI | run 40265606961 success | GitHub Actions 精确 HEAD |
| IndexCore 历史 20K 基线 | Gate 3 `SCALE_20K: PASS`，仅指历史 PostgreSQL full-runtime-ingestion；不等于 Issue134/F4 freshness 或生产容量 | `docs/gate3/GATE3-ALPHA-VERIFICATION-REPORT.md:12-25,50,66-91`；F3/完整 traversal/长稳另见 IC-07 |
| 115driver HEAD | `1cfb0d6e43e6c6487d53a7977bff58f914983514` | 本地 git 与 GitHub main |
| CloudSite pinned | v1.3.5 `d89999540a6d35838582ac06e31f81312d1f66fd` | `cloudsite/go.mod:6`、`go.sum`；`git show v1.3.5` |
| 上游差异 | v1.3.5..HEAD 8 个提交、9 文件 diff | 本地 `git log/diff --stat v1.3.5..HEAD`；最新 release 仍 v1.3.5 |
| IndexCore 开放事项 | Issue134/138/132/129/125/120 等；PR147/148/149 Draft/open | `gh issue/pr list`；PR149 不属 main |
| CloudSite 开放事项 | Issue189/187/176/155/153 等；截至 2026-11-16T11:45:01Z，PR190 为 open/non-draft、PR191 为 open/draft，均未合并且不属于审计 main | `gh issue/pr list` 时间化查询；PR190 涉及云下载会话前置检查，PR191 涉及 per-user 云下载 CRUD 设计，实施 R1-1/R1-2/R2-1/R2-3 前需先评审重叠 |
| 115driver 开放事项 | Issue96 仍开放，#100 只解决其中 >5GiB/部分重试，不含 resume | GitHub issue/PR 与上游提交正文 |
| 静态测试规模 | CloudSite 182、IndexCore 141、115driver 21 个 Go `_test.go` | `rg --files -g '*_test.go'` 计数；不是覆盖率 |
| 115driver 测试入口 | `make test` 排除 `pkg/driver`；`make test-integration` 独立要求 COOKIE；直接 `go test ./...`/`go test ./pkg/driver` 无 build-tag 防护 | `Makefile:36-46`；`pkg/driver/driver_test.go:18-20,76-111,231-310`；本审计未运行任何 115driver 测试 |

本审计未重新运行完整测试。115driver `pkg/driver/driver_test.go:18-20,76-111,251-310` 含真实网络/写操作，故未执行。

## 3. CloudSite 证据

| ID | 类型 | 直接证据 |
|---|---|---|
| CS-01 | FACT/INFERENCE | `internal/providers/115/adapter.go:141-159`；pinned `pkg/driver/offline.go:148-151` |
| CS-02 | FACT/UNKNOWN | `internal/runtime/runtime.go:293-298`；`providers/115/adapter.go:215-256`；`poll.go:39-60`；`stage_dispatch.go:287-299` |
| CS-03 | FACT | `migrations/0015_product_operations.sql:7-18`；`0005_acquisition_manifests.sql:4-9`；`acquisition/source.go:31-41`；`product_admission_repository.go:120` |
| CS-04 | FACT/INFERENCE | `projector/projector.go:150-183`；`store/postgres/projection_store.go:109-119`；对照 `libraryview/engine.go:318-343` |
| CS-05 | FACT | `acquisition/provider_outcome.go:72-79`；`canonical.go:275-306`；`acquisition_canonical_repository.go:170-203` |
| CS-06 | FACT | `acquisition/stage_dispatch.go:247-282`；`canonical.go:266-299`；`hint_client.go:344-366`；`acquisition_lease_recovery_repository.go:126-158` |
| CS-07 | FACT | `store/postgres/job_repository.go:221-242`；`admin_pending_cancellation.go:97-136` |
| CS-08 | FACT | `product/admission.go:26-32`；`producthttp/cloud_download.go:52-89`；`registration.go:16-58` |
| CS-09 | FACT/UNKNOWN | `fileaccess/delivery.go:60-95`；`producthttp/content.go:166-173`；`admin_share_lifecycle.go:14-40` |
| CS-10 | FACT | `cmd/panta-db/main.go:38-40,58-63`；`store/postgres/database.go:19-26` |
| CS-11 | FACT | `migrations/0001_catalog.sql:1-47`；`store/postgres/catalog_repository.go:145-179,246-253,310-325` |
| CS-12 | FACT | `README.md:3,14`；`VERSION:1`；`store/postgres/migrator.go:19-21` |

### CloudSite 正面边界证据

- Job claim/fence：`job_repository.go:116-152`、`acquisition_provider_task_repository.go:67-131`。
- 原子 READY：`acquisition_canonical_repository.go:76-261`。
- 不直写 Canonical：`acquisition/stage_dispatch.go:107-284` + `projector/projector.go:109-147` + `projection_store.go:53-124`。
- Hint：`integrations/indexcore/hint_client.go:85-127,139-197,210-269`。
- Session：`auth/auth.go:161-245`、`store/postgres/auth_repository.go:63-125`。
- Library 权限后 ETag：`producthttp/library.go:18-80`。
- Migration 防篡改：`migrations/migrations.go:15-66`、`store/postgres/migrator.go:17-125,133-223`。

## 4. IndexCore 证据

| ID | 类型 | 直接证据 |
|---|---|---|
| IC-01 | FACT/INFERENCE | `kernel/reconcile/reconcile.go:21-67`；`identity.go:115-118,165-211`；`docs/architecture/GATE1B-SAFE-RECONCILE.md:288-300` |
| IC-02 | FACT/INFERENCE | `identity.go:115-129`；`reconcile/directory.go:16-49`；`reconcile.go:40-67`；ADR-004 |
| IC-03 | FACT | `identity.go:94-111`；`reconcile.go:25-39` |
| IC-04 | FACT | `store/postgres/tx.go:286-299`；`decision.go:18-53`；`diagnostic.go:352-358` |
| IC-05 | FACT | `store/postgres/decision.go:20-31`；`docs/architecture/GATE1C-POSTGRESQL-STORE.md:542-547` |
| IC-06 | FACT/INFERENCE | `runtime/scan/scan.go:130-169,194-202`；`store/postgres/root_admin.go:192-202`；`scoped_fetch_fence.go:19-21` |
| IC-07 | FACT/BLOCKER | `docs/p0/P0-134-F3-CAPACITY-INVESTIGATION.md:72-90,120-188`；PR147/148/149 Draft |
| IC-08 | FACT | `runtime/incrementalexec/cycle.go:165-180`；`incrementalruntime/runtime.go:305-325`；`runtime/app/app.go:325-344` |
| IC-09 | FACT | `store/postgres/prior.go:21-35`；`migrations/0001_init.sql:89-90`；`coordinator.go:336-349,422-426` |
| IC-10 | FACT | `snapshot_create_admit.go:27-48`；`coordinator.go:395-410`；`runtime/app/read_pool_crash_test.go:153` |
| IC-11 | FACT | `transport/httpapi/server.go:250-263`；`store/postgres/query.go:180-220`；`migrations/0001_init.sql:60-65` |
| IC-12 | FACT | `collector/alist/adapter.go:126-152,171-197,37-103`；对照 `scope_identity.go:54-88` |
| IC-13 | FACT | `store/postgres/adapter_config_validate.go:54-65`；`runtime/scan/scan.go:75-88` |
| IC-14 | FACT/INFERENCE | `store/postgres/writer_lock.go:25-49`；`runtime/scan/scan.go:142-202`；`runtime/app/app.go:362-383` |
| IC-15 | FACT | `incremental/state/model.go:122-126`；`store/postgres/incremental_work.go:714-720`；`diagnostic.go:202-207` |
| IC-16 | FACT | `store/postgres/migrations/0001_init.sql:211-226` |

### IndexCore 正面边界证据

- Completeness：`kernel/completeness/evaluate.go:49-100`。
- Scoped 严格校验：`collector/alist/scoped.go:94-155`。
- Topology：`collector/alist/topology.go:18-93`。
- 原子 Canonical：`store/postgres/coordinator.go:248-503`。
- claimed/pending：`store/postgres/incremental_work.go:164-225,433-520,619-735`。
- Source Budget：`sourcebudget/budget.go:214-265`。
- Query 路由：`transport/httpapi/server.go:29-47`。
- Hint loopback/Bearer/独立能力：`transport/hintapi/server.go:1-85`。
- F3 数值：50K 时 overdue=49,972、变化未观察=492；这是隔离 fixture，不是生产实测。

## 5. 115driver 证据

| ID | 类型 | 直接证据 |
|---|---|---|
| DR-01 | FACT/UNKNOWN | `pkg/driver/download.go:48-63,100-105,149,161-165,173-195` |
| DR-02 | FACT | `pkg/driver/api.go:21-25`；`option.go:86-90,223-229`；Resty `middleware.go:246-256` |
| DR-03 | FACT | `client.go:163-165`；`option.go:41-45`；`driver_test.go:40-44`；Resty debug middleware |
| DR-04 | FACT | `client.go:19-28,178-187`；`upload.go:43-66`；`offline.go:98-104`；MCP `server/server.go:89-126` |
| DR-05 | FACT | `client.go:158-160`；`login.go:42-84`；Resty `client.go:269-287` |
| DR-06 | FACT | `error.go:139-185`；`login.go:13-24`；CLI `cmd/root.go:61-63` |
| DR-07 | FACT | `option.go:31-38`；`op.go`、`offline.go`、`recylcle.go` 写方法无 Outcome/reconciliation |
| DR-08 | FACT | `upload.go:362-469` |
| DR-09 | FACT | `upload.go:471-491` |
| DR-10 | FACT | `offline.go:108-152,195-202` |
| DR-11 | FACT | `dir.go:38-76,109-143` |
| DR-12 | FACT | `response.go:283-285`；`op.go:166-183` |
| DR-13 | FACT/UNKNOWN | `pkg/driver/op.go:9-26`；`cli/internal/resolver/resolver.go:85-88`、`cli/cmd/rm.go:24-45`；`pkg/driver/recylcle.go:9-23` |
| DR-14 | FACT | `option.go:119-125`；`upload.go:154-172` |
| DR-15 | FACT | `upload.go:154-172,456-469` |
| DR-16 | FACT | `download.go:39-63,68-152` |
| DR-17 | FACT | `search.go:161-170`；`share.go`；`op.go:9-95` |
| DR-18 | FACT | `app_version.go:28-36`；`cli/cmd/mkdir.go:74-83`；`login.go:93-118` |

能力矩阵依据实际 `pkg/driver` 文件，而非 README。分享创建/撤销“未发现”仅限截至 HEAD 已审计公开方法表面，不扩大为 115 服务永久不支持。

## 6. 生产只读证据

观测方式：SSH 只读命令与公开 HTTPS GET/HEAD，未使用 sudo。

| 结论 | 类型 | 证据摘要 |
|---|---|---|
| SSH/OS/资源 | FACT | 2026-11-16 `id/hostname/lsb_release/uname/nproc/free/df/vmstat/PSI` |
| 公网端口 | FACT | `ss -lntup` 仅见 22/80/443 |
| 运行进程 | FACT | `ps`：CloudSite acquisition、IndexCore、OpenList、Traefik、Portainer、双 PG |
| 静态资源版本 | FACT | 线上/本地 `sha256sum`：library-page.js=b1c39e3c…，library.js=b34b7429…，分别完全一致 |
| 公开注册 | FACT | `GET https://cloud.netioi.com/api/v1/auth/capabilities` → `{"registration_enabled":true}` |
| 浏览器安全头 | FACT | 公开 HEAD：CSP、DENY、nosniff、no-referrer、no-store |
| Compose | FACT（已脱敏） | `/opt/my-servers/docker-compose.yml`：Traefik、Portainer、OpenList、rclone 配置 |
| `.env` 权限 | FACT | `stat` → `0644 root:root`；只读键名，未读值 |
| 备份行为 | FACT | `rclone/docker_backup.sh:4-29`；Compose 每日 04:30、整目录只读挂载后 tar+move |
| 资源压力 | FACT/UNKNOWN | 2 vCPU、swap 488544KiB、I/O PSI some avg300=43.48；短窗口不能证明泄漏 |
| 二进制/schema/日志 | BLOCKER | Docker socket、root alpha 配置、`/proc/<pid>/exe`、系统 journal 均 permission denied |

生产观测中的个人邮箱、数据库名、容器 ID 和秘密值不纳入证据索引；秘密值从未读取。

### 生产问题逐 ID 映射

| ID | 类型 | 直接证据与来源 |
|---|---|---|
| DEP-01 | FACT | 2026-11-16 只读 `stat /opt/my-servers/.env` 为 `0644 root:root`；仅列键名，三类 CloudSite 秘密键存在 |
| DEP-02 | FACT/UNKNOWN | 脱敏 Compose 的整目录 `/backup_source` 挂载；`rclone/docker_backup.sh:4-20` tar 后上传；远端加密/ACL 未读 |
| DEP-03 | FACT/UNKNOWN | `rclone/docker_backup.sh:11-29` 仅 live tar/rclone，未见 pg_dump/快照/恢复验证；是否有另行的受控 PG 备份未获权限 |
| DEP-04 | FACT | 脱敏 Compose `portainer/portainer-ce:latest`、`rclone/rclone:latest`；OpenList 则固定 digest |
| DEP-05 | FACT/INFERENCE | 脱敏 Compose Portainer Docker socket RW 与 HTTPS 路由；OpenList `user: 0:0`、PUID/PGID=0 |
| DEP-06 | FACT/UNKNOWN | 同窗口 `nproc`、`free`、`vmstat`、`/proc/pressure/*`：2 vCPU、swap 488544KiB、I/O some avg300=43.48；长期原因未测 |
| DEP-07 | BLOCKER | Docker socket、root-only alpha Compose、`/proc/<pid>/exe`、系统 journal 均 permission denied；未使用 sudo |
| DEP-08 | FACT/UNKNOWN | 全部关键进程位于同一主机；备份脚本无 retention/告警验证；监控告警覆盖未取得证据 |

PR149 另有 2026-11-15 自述部署回执（`3a82f542…`、schema8、基础检查 PASS、真实20K/长稳 NOT_RUN），已登记于 09；它属于间接材料，本次未独立核验，不能替代上述 BLOCKER 的解除证据。

## 7. 架构与产品结论证据

| 结论 | 证据 |
|---|---|
| CloudSite 当前只有离线下载 Provider，不是完整文件管理 | `internal/providers/115/driver.go:50-110`；`adapter.go:127-159,162-212`；无原生文件管理路由 |
| CloudSite 不直写 IndexCore Canonical | acquisition→Hint/Q5/Q8；Projector 只写 CloudSite Copy |
| Copy 当前一对一物理资源 | `migrations/0001_catalog.sql:37-47` |
| 业务发现能力缺口 | `product_copy_presentation` 仅 overlay；`libraryview/page.go:176-206` 四类；无合集/收藏/专题表和路由 |
| IndexCore Q8 只能失效+回读 | IC-05 payload 证据 |
| F4 不是生产稳定能力 | GitHub Draft 状态+F3/PR 证据 |
| Next.js/TS 是选择非现状 | CloudSite 当前静态资源由 `producthttp/workspace.go:197-245` 嵌入服务 |

## 8. 交付物核对

| 文件 | 内容 |
|---|---|
| `00-EXECUTIVE-SUMMARY.md` | 总体结论、关键风险、改造顺序、验证边界 |
| `01-SYSTEM-ARCHITECTURE.md` | 架构、模块依赖、数据/控制车道、API 与边界 |
| `02-CLOUDSITE-AUDIT.md` | 领域、Job、前端、安全和 CS-01—12 |
| `03-INDEX-CORE-AUDIT.md` | Kernel/Collector/Store/Runtime/容量和 IC-01—16 |
| `04-115DRIVER-CAPABILITY.md` | 能力矩阵、DR-01—18、Adapter 和测试边界 |
| `05-115-STORAGE-MANAGEMENT.md` | 文件管理中心、操作状态机、上传下载、批量与风险 |
| `06-RESOURCE-MANAGEMENT.md` | 身份映射、元数据、分类、标签、去重 |
| `07-SEARCH-AND-DISCOVERY.md` | 三种发现路径、搜索投影、合集专题、性能验收 |
| `08-DATA-CONSISTENCY.md` | 十类操作七问、Journal/Hint/outbox/恢复 |
| `09-DEPLOYMENT-AUDIT.md` | 生产事实、DEP-01—08、缺失材料 |
| `10-SECURITY-AUDIT.md` | 认证、注入、凭证、风险等级、安全验证 |
| `11-TECHNICAL-DEBT.md` | 54 条记录、优先级、根因和伪优化 |
| `12-OPTIMIZATION-ROADMAP.md` | P0—P3 处理包，九要素与回滚验收 |
| `13-OPEN-QUESTIONS.md` | Q00—Q22 共 23 项决策/授权与 UNKNOWN |
| `14-EVIDENCE-INDEX.md` | 本索引与验证声明 |

## 9. 最终验证声明

- 未修改三个项目工作树，未创建 commit/PR，未执行生产 mutation。
- 未运行真实 115 操作、数据库 migration、服务重启、容器变更或可能写入的测试。
- CI 状态来自 GitHub 只读查询，不等同本审计重新执行。
- 报告中的“验收标准/测试方法”是后续要求，不应被读成已通过。
- 对生产二进制、schema、日志、真实容量和上游当前协议未取得的证据全部保留 UNKNOWN/BLOCKER。