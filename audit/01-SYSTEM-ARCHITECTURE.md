# 01 系统架构、服务关系与数据流

## 1. 审计基线与结论类型

本文架构图以指定源码 SHA 的实际组合根、路由、存储和 Provider 调用为依据；生产进程另标“运行观测”。文档中的目标描述不当作实现事实。

## 2. 当前系统上下文

```text
用户浏览器
   │ HTTPS / Cloudflare / Traefik
   ▼
CloudSite Go HTTP + 原生 JS
   ├─ CloudSite PostgreSQL：用户、业务 Catalog、任务、分享、审计、派生快照
   ├─ IndexCore HTTP Q4/Q5/Q6/Q7/Q8/Q9：物理资源只读与 Journal
   ├─ OpenList 内容 API：经实时 Q5 授权后的文件内容/直接链接
   └─ 115driver v1.3.5：当前仅离线下载 Start/Status/Cancel 和 GetInfo

115 网盘
   ▼
OpenList 观察适配
   ▼
IndexCore Collector / Incremental Runtime
   ▼
Canonical Inventory + append-only Journal（IndexCore PostgreSQL）
   ▼
CloudSite Projector / libraryview / acquisition canonical stage
   ▼
Copy、产品文件库与 READY 业务状态
```

核心不变量：

1. OpenList 不是 Canonical 真相所有者。
2. IndexCore 是唯一物理资源 Canonical 写入者。
3. CloudSite 可写自己的 Copy/业务状态，但不能直接修改 IndexCore 表或以 SQL 修复物理身份。
4. 115 Provider 写操作与 IndexCore 观察是两条独立车道，通过受审计任务、Hint 和 Query 收敛。
5. Hint 的 202 只表示 dirty signal 持久化，不表示扫描、Canonical 或 CloudSite READY 完成。

## 3. CloudSite 实际模块依赖

### 3.1 进程组合根

证据：`cmd/panta/main.go:31-80`、`internal/app/app.go:21-50`、`internal/platform/productserver/server.go:68-388`。

| Action | 实际职责 |
|---|---|
| 默认 `app.New` | 无参数不隐式启动完整 Worker；避免误启副作用 |
| `serve-product-readonly` | 认证、文件库、内容、分享、管理读取；不构造 acquisition Worker |
| `serve-product-acquisition` | 同一 HTTP 产品服务 + 独立 acquisition Runtime/第二 DB pool |
| `preflight-product-acquisition` | 启动前配置、schema、身份和权限检查 |
| `submit-acquisition` / operator actions | 受控运营入口，不等同公开路由 |

生产进程观测到 `/usr/local/bin/cloudsite serve-product-acquisition`，所以当前进程兼具产品 HTTP 与 Worker；未看到独立 readonly 主产品进程。

### 3.2 代码模块图

```text
producthttp
  ├─ auth / registration / WorkspaceHandler
  ├─ product.ReadService ───────────────┐
  ├─ product.LibraryService             │
  │    └─ libraryview.Engine            │
  ├─ fileaccess/fileshare               │
  ├─ adminread/adminobserve/adminops    │
  └─ acquisition AdmissionService       │
                                       ▼
                    integrations/indexcore typed HTTP client
                                       │
                    OpenList ContentClient（仅内容车道）

acquisition Worker
  ├─ jobs + PostgreSQL JobRepository
  ├─ Manifest/ProviderTask/Activation/Outcome repositories
  ├─ providers/contracts
  │    └─ providers/115 → 115driver
  ├─ integrations/indexcore.HintClient
  ├─ projector.Service → Copy + projection cursor
  └─ CanonicalStageRepository → result_copy_id + Manifest/Job 终态
```

### 3.3 数据库所有权

| 数据 | 所有者 | CloudSite 权限边界 |
|---|---|---|
| `index_canonical_resource`、Snapshot、Journal、Watch | IndexCore | 只经 HTTP Query/Hint，不直连写 |
| `assets/releases/variants/copies` | CloudSite | Copy 由受控 Projector 投影；逻辑分类由业务层拥有 |
| `jobs/acquisition_manifests/acquisition_provider_tasks` | CloudSite | 任务与 Provider 副作用 fence |
| `product_users/sessions/shares/operations/admin_actions` | CloudSite | 认证、授权、审计与幂等 |
| `product_library_snapshots` | CloudSite libraryview | 可删除重建的派生读模型，不是物理真相 |
| OpenList 配置及其 DB | OpenList | 不读取其数据库，不复用其凭证 |

## 4. CloudSite 资源模型

实际 migration `0001_catalog.sql:1-47`：

```text
Asset 1 ─ N Release 1 ─ N Variant 1 ─ N Copy
                                      │
                                      ├─ variant_id 可 NULL：UNRESOLVED_COPY
                                      └─ UNIQUE(indexcore_root_id, indexcore_resource_id)
```

`Copy` 当前是一条记录对应一个 `(root_id, resource_id)`；`PROJECT-CONTEXT.md` 的“one or more physical resources”是目标描述，不是当前表形状。建议保持“Variant 下多个 Copy 表达多文件/多副本”，不要为了合集概念仓促把 Copy 改成多对多。

`ResourceView` 明确区分：

- `PHYSICAL_ONLY`：IndexCore 存在但尚无业务 Copy；
- `UNRESOLVED_COPY`：有 Copy 但未绑定 Variant；
- `CLASSIFIED`：已进入逻辑资源层级；
- IndexCore presence 与 Copy availability 是不同状态，不能混为一个布尔值。

证据：`internal/resourceview/physical.go:12-20,68-145`、`internal/resourceview/resourceview.go:55-137`。

## 5. IndexCore 内核结构

```text
Collector（rclone / AList / OpenList）
   ▼
Snapshot + Completeness / scoped coverage evidence
   ▼
Postgres admission FIFO + root lock
   ▼
Kernel identity resolution + reconcile
   ▼
Canonical + identity observations + Journal + admission receipt（同一事务）
   ▼
Q1—Q9 read APIs

Mutation Hint → durable dirty work / Watch → scoped re-observation
                     │
                     └─ 只提供重新验证机会，不直接写 Canonical
```

关键设计：

- `internal/kernel/completeness/evaluate.go:49-100`：UNKNOWN 不会变成空 COMPLETE。
- `internal/collector/alist/scoped.go:94-155`：scoped total/count、名称唯一和大小上限严格校验。
- `internal/collector/alist/topology.go:18-93`：虚拟挂载、topology 变化和异常 driver 不取得删除授权。
- `internal/store/postgres/coordinator.go:248-503`：root 锁、reconcile、Canonical/Journal/admission 原子提交。
- `internal/store/postgres/incremental_work.go:164-225,433-520,619-735`：`claimed_*` 与 `pending_*` provenance 分离，避免 IN_FLIGHT 新 Hint 被成功确认吞掉。

## 6. API 契约面

### 6.1 CloudSite 对外产品 API

主要包括：

- 认证：`/api/v1/auth/login`、`logout`、`register`、`me`、`capabilities`；
- 工作区和资源：`/api/v1/workspace`、`resources`、`resources/resolve`、`library`；
- 云下载：`/api/v1/cloud-downloads`、owner-only tasks；
- 内容：`/api/v1/files/content`、`preview`、分享与 guest grant；
- 管理：用户、存储探测、分享撤销、PENDING 取消、Copy presentation、审计和 IndexCore Q9 观察。

路由证据：`internal/producthttp/workspace.go:90-285`、`auth.go`、`reads.go`、`library.go`、`cloud_download.go`、`admin_operations.go:19-153`。

未发现的公开产品面：完整 Asset/Release/Variant CRUD、物理上传/移动/复制/重命名/删除、115 原生分享、标签、合集、收藏、最近浏览、热门和专题。

### 6.2 IndexCore Query/Hint

| 契约 | 用途与限制 |
|---|---|
| Q1/Q2/Q9 | Root 与 Root Status（lifecycle、generation、last applied admission seq）；ready 不代表 source fresh；更丰富的 root diagnose 是独立 CLI，freshness debt 不是当前 Q9 字段 |
| Q3/Q4/Q5/Q6/Q7 | 资源、层级、精确路径、当前/历史清单 |
| Q8 | per-root `event_seq` 变化流；当前 payload 只有 reason，需回读 Canonical |
| Hint | 独立 loopback + Bearer；仅 same-host 受信任调用，不是公网写 API |

Q5 返回 ambiguous 时调用方不得自选 winner；Q4/Q6/Q7 遇 stale generation cursor 必须重建分页链。多 Root 不能拿全局 event_id 拼成一个顺序。

## 7. 两条数据车道不得合并

### 7.1 观察车道

```text
115 → OpenList → IndexCore → Canonical/Journal → CloudSite 派生读模型
```

负责回答“物理世界当前被证明是什么”。

### 7.2 控制车道

```text
CloudSite 用户命令
  → 权限/预算/预检/影响预览
  → Job + operation intent + side-effect fence
  → 115 Provider
  → APPLIED / NOT_APPLIED / UNKNOWN / PARTIAL
  → 结果核验 + 审计
  → trusted Hint（可重建 outbox）
  → IndexCore 重新观察
  → CloudSite Query/Journal 确认
```

负责回答“用户要求做了什么、Provider 是否接受、结果是否被证实”。HTTP 调用期间不能持有数据库事务；状态通过持久化阶段推进。

## 8. 重复实现与职责判断

| 现象 | 判断 |
|---|---|
| CloudSite 旧 Projector 与 libraryview 都消费 Q8 | 属于历史并存的两套派生读模型；连续性规则已漂移，应抽取共同 Journal 校验/游标语义 |
| CloudSite 产品分享与 115 原生分享 | 当前只有前者；不是重复实现，未来需明确产品授权与 Provider 分享的职责 |
| OpenList 内容访问与 IndexCore 观察 | 目的不同，可保留；不得用内容 API 成功推断 Canonical presence |
| CloudSite 115 Adapter 与 IndexCore OpenList Collector | 控制与观察分离，合理；不应合并成第二个物理扫描器 |
| Asset/Copy 分类与物理目录 | 当前业务分类很弱；目标应让业务分类独立于目录，而非再复制一棵物理树 |

## 9. 目标架构（分阶段，不是现状声明）

以下目标需先通过 `13-OPEN-QUESTIONS.md` 的 Q00：继续扩展 CloudSite 1.0，还是在正式授权的 successor product 中实现。IndexCore 当前项目文档把 CloudSite 1.0 标为 Legacy/Frozen Product，因此本图只表达候选责任分层，不构成继承 CloudSite 架构或开工授权。

```text
CloudSite 文件管理与发现平台
  ├─ Read Model：IndexCore 物理树 + CloudSite 业务 Catalog + 搜索投影
  ├─ Command Model：统一 FileOperation Job + Provider Adapter + Audit
  ├─ Taxonomy：Asset/Release/Variant、分类树、标签、别名、合集、专题
  ├─ Discovery：名称搜索 / 用途筛选 / 专题发现三条独立路径
  └─ Operations：backpressure、freshness debt、Provider 预算、恢复与对账
```

不建议引入：第二 Canonical writer、浏览器直连 PostgreSQL、公网 Hint、AI 自主文件操作、为搜索而复制物理文件、未经证明的分布式缓存。