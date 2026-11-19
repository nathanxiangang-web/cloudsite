# 07 搜索、分类导航与资源发现优化

## 1. 当前产品事实

当前 CloudSite 已提供文件优先首页、层级浏览、Q5 精确路径解析、详情、预览、下载、分享和云下载。schema21 的 libraryview 提供 50 行服务端分页、名称搜索、分类统计、revision、stale/ready 和私有条件 304，解决了每次打开页面整库重读的一类性能问题。

证据：`internal/producthttp/library.go:18-80`、`internal/libraryview/page.go:176-206`、`internal/producthttp/web/library-page.js`。

但分类仍主要是 `software/video/document/other` 的物理名称/扩展名归并，其他大量进入 `other`。搜索主要匹配文件名；没有统一 Asset 搜索、结构化筛选、别名/同义词、简介、合集、收藏、最近浏览、热门和专题。Copy presentation 的 alias/category 尚未成为消费者统一检索体系。

## 2. 三种独立使用路径

### 路径 A：知道名称

```text
搜索框 → 名称/别名/缩写/文件名匹配 → 结构化结果 → Asset 或物理文件详情
```

要求：精确匹配优先、支持文件名和逻辑资源名、明确区分 Asset/Release/Variant/Copy、结果显示为何命中。

### 路径 B：知道用途，不知道名称

```text
分类树 → 用途/厂商/平台/架构/格式/版本筛选 → 排序 → 对比候选
```

要求：分类和筛选来自业务元数据，不依赖记住目录；每个筛选显示数量和空结果原因。

### 路径 C：不知道具体资源

```text
首页专题 / 合集 / 编辑精选 / 热门 / 最近更新 → 策展内容 → 相关资源
```

要求：专题和合集是一等数据，不是临时搜索关键词；有策展说明、适用场景、版本和更新日期。

## 3. 目标页面结构

### 3.1 首页

1. 搜索框：支持自然关键词，但不要求 AI。
2. 顶部主导航：文件、资源、合集、专题、任务。
3. 常用分类：开发工具、设计、视频、文档等，数量来自已发布业务投影。
4. 策展区域：专题、合集、最近更新、热门（有数据后开启）。
5. 个人区域：收藏、最近浏览、自己的任务；无数据时给引导，不伪造推荐。
6. 数据状态：快照 revision/更新延迟、部分索引不可见时明确提示。

### 3.2 搜索结果

- 分组：逻辑资源、物理文件、合集/专题；默认优先逻辑资源，避免同 Asset 多副本刷屏。
- 展开组后可比较 Release/Variant/Copy，显示平台、架构、大小、更新时间和可用副本。
- 筛选：分类、标签、厂商、资源类型、平台、架构、文件类型、版本渠道、大小、日期。
- 排序：相关度、名称、更新时间、大小、热门（有真实统计后）。
- 服务端 keyset 分页；排序和筛选条件进入游标签名，禁止客户端改变游标语义。
- 无结果时提供可移除筛选、近似别名和业务分类入口，不擅自修改物理文件。

### 3.3 详情页

- Asset：简介、厂商、别名、标签、分类、合集。
- Release：版本、渠道、发布日期。
- Variant：平台、架构、格式、大小、hash（可得时）。
- Copy：物理位置、Provider、可用状态、最后观察时间；普通用户不必看到敏感 native scope。
- 操作：下载、收藏、分享、管理入口按权限展示。
- “相关资源”由同标签/同合集/同厂商的确定性规则生成，AI 非必需。

## 4. 统一检索文档

建议 CloudSite 自有搜索投影，不让 IndexCore 承担业务搜索：

```text
SearchDocument
  document_id / document_type / revision / visibility
  asset_id / release_id / variant_id / copy_id（按类型可空）
  canonical_name / file_names[] / aliases[] / synonyms[]
  summary / vendor / resource_type
  category_ids[] / tag_ids[] / collection_ids[]
  version_normalized / channel
  platforms[] / architectures[] / package_types[] / file_types[]
  size / release_date / updated_at / availability
  popularity_score / editorial_boost
```

规则：

- 物理文件变更只更新对应 Copy 文档，不复制 Asset 的业务历史。
- 发布按 revision 原子切换；失败保留上一份完整可用投影，沿用 libraryview 的优点。
- 权限过滤必须在查询时或服务端发布分支执行，私有 ETag 不跨用户共享。
- 删除/隐藏是不同语义；隐藏不应意外撤销已有权限，权限必须实时检查。

## 5. 确定性搜索实现建议

第一阶段优先 PostgreSQL，避免过早引入新搜索服务：

1. 名称、别名、文件名建立规范化列和 trigram 索引。
2. 简介使用 PostgreSQL FTS；中文分词配置需单独评估，不假定默认分词适合中文。
3. 分类、标签、平台、架构等使用关联表/B-tree/GIN 复合查询。
4. `normalized_name` 统一大小写、全半角、空白和有限的标点规则；保留原文显示。
5. 版本排序使用确定性版本解析，无法解析时回退字符串/date，不猜 semver。
6. 同义词和别名词典版本化，查询扩展有最大数量，防止词表爆炸。
7. 仅当真实查询规模和延迟证明 PostgreSQL 不足时，再评审独立搜索引擎；同步仍以 CloudSite 发布投影为来源。

### 推荐排序信号

```text
精确 canonical_name > 精确别名 > 精确文件名 > 前缀 > 分词匹配 > 简介匹配
+ 当前可用副本 + 编辑精选 + 合理热门
- 归档/不可用（默认过滤而非永久惩罚）
```

热门不能使用伪造固定分数；初期无行为数据时不显示热门排序。

## 6. 元数据生产与质量控制

优先级：

1. 管理员/策展人明确录入；
2. 官方元数据导入或结构化 sidecar；
3. 确定性文件名/路径解析规则；
4. 用户纠错和社区反馈（有审核）；
5. 可选 AI 生成建议，最后且非强制。

规则解析输出 `rule_id/version/evidence/confidence`。低置信、冲突或多候选保持 UNRESOLVED，不自动覆盖人工数据。AI 不得成为搜索、分类或权限的强制依赖，不得自主移动、删除或重命名真实文件。

## 7. 合集、专题、收藏、最近浏览、热门

| 能力 | 数据与更新 | 注意事项 |
|---|---|---|
| 合集 | 策展人创建，Asset 有序列表、说明、revision | 不复制物理文件，合集可跨分类 |
| 专题 | 页面级策展，可组合合集/Asset/说明 | 发布/草稿/归档，保留历史版本 |
| 收藏 | user_id+asset_id、created_at | 私有，唯一约束，删除账号按保留策略处理 |
| 最近浏览 | user_id+asset_id+last_viewed_at | 有上限、可清空、可关闭，避免敏感浏览泄露 |
| 热门 | 真实下载/详情行为的周期聚合 | 防刷、时间窗、隐私、管理员操作不应无限加权 |
| 最近更新 | Asset/Release 发布事件 | 不用物理 mtime 冒充业务新发布 |

## 8. API 建议

```text
GET /api/v2/search?q=&type=&category=&tag=&platform=&sort=&cursor=
GET /api/v2/assets/{asset_id}
GET /api/v2/categories/tree
GET /api/v2/collections /api/v2/collections/{id}
GET /api/v2/topics /api/v2/topics/{slug}
GET /api/v2/me/favorites /api/v2/me/recent
POST /api/v2/me/favorites
DELETE /api/v2/me/favorites/{asset_id}
DELETE /api/v2/me/recent
PUT /api/v2/me/privacy-settings
```

搜索 API 只暴露业务 DTO，不返回 credential_ref、Provider Cookie、内部 native scope 或 SQL cursor 原文。管理端编辑使用 expected_version 和审计回执。`DELETE /me/recent` 用于清空，`PUT /me/privacy-settings` 控制是否记录；需明确生效时间、最大保留数/期限、账号删除处理、管理员操作是否记录，以及关闭记录的用户是否也从热门聚合排除。

## 9. 前端技术选择

当前原生 JS 可通过模块化继续交付第一阶段，不必为搜索重写整个产品。

若建设独立 reference web consumer，推荐 Next.js + TypeScript + App Router，并使用 server-only typed client。它是消费端实施选择，不是新冻结架构：

- 浏览器只访问 CloudSite BFF/API，不直连 PostgreSQL。
- 独立 IndexCore reference consumer 可用 typed TS IndexCore client；产品用户链路不得绕过 CloudSite 授权。
- 迁移采用只读 strangler：先搜索/合集，再详情，再管理； parity、权限、安全头和性能未达标前保留现有页面。
- 不把 Go 内部类型或数据库模型暴露给 TypeScript。

## 10. 性能与验收指标

指标：查询 P50/P95/P99、每页 SQL 数、投影 lag、空结果率、搜索后详情点击率、分类路径点击率、合集发现转化率、各排序使用量、权限拒绝数。

验收场景：

1. 用户只记得缩写，命中别名并显示命中原因。
2. 用户不知道名称，能通过“用途→平台→架构”找到唯一 Variant。
3. 同 Asset 十个 Copy 默认聚为一组，可展开比较。
4. 文件移动后，别名、标签、合集和收藏不变。
5. 某搜索投影发布失败，上一完整 revision 仍可读。
6. 私有资源不出现在其他用户结果、计数、ETag、热门和最近浏览中。
7. 50K 业务文档的授权部署验证再放行性能承诺；本阶段不虚构结果。