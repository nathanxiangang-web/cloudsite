# 06 资源身份、元数据、分类与标签体系

## 1. 当前模型结论

CloudSite 已有正确的逻辑分层起点：Asset 是用户认知的资源，Release 是版本/修订，Variant 是平台/架构/质量等形式，Copy 是物理存储实例。`Copy.variant_id` 可空使未分类物理副本能够存在，这是应保留的设计。

当前实际数据库中，一个 Copy 唯一对应一个 `(indexcore_root_id, indexcore_resource_id)`，不是文档目标中的“一对多物理资源”。建议默认保持一对一，用 Variant→多个 Copy 表达安装包、补丁、字幕等多个文件；资源合集/专题另建业务聚合，不污染 Copy 身份。

主要缺口：业务分类仍主要依赖物理路径和扩展名，Copy presentation 只有 alias/category/hidden，未形成统一 Asset 元数据、标签、别名、合集和检索投影。

## 2. 身份所有权与稳定性

| 身份 | 所有者 | 用途 | 稳定性与禁止事项 |
|---|---|---|---|
| 115 File ID | 115 | 文件对象寻址、写操作预检 | 由 Provider 响应和读回证明；不得与 QuickID/info_hash/rid 混用 |
| 115 Folder ID | 115 | 目录 scope、目标预检 | 文件常来自 fid，目录常来自 cid，父字段也不同 |
| Provider Account ID | 115/CloudSite Connection | 防止跨账号同 ID 冲突 | 所有 Provider 身份必须带账号命名空间 |
| 文件路径 | IndexCore/Provider 观察 | 浏览、Q5 解析、显示 | 可移动/重命名/复用，绝不能作永久主键 |
| 文件名 | Provider | 显示、搜索、同名冲突预检 | 非唯一、可修改 |
| 大小/mtime | Provider/IndexCore | 弱身份证据、变化检测 | 可重复/缺失/漂移，不能独立确认身份 |
| 内容 hash | Provider/IndexCore（实际获得时） | 内容判重、移动候选、校验 | 同内容不等于同对象；没有 hash 不得伪造 |
| Offline info_hash | 115 离线任务 | 任务轮询 | 不是最终 File ID |
| 回收站 rid | 115 回收站 | 恢复/永久清理 | rid 与原 File ID 的映射尚未由已审计 fixture 证明；对账前不得假定相同或可互换，恢复后的 ID 需读回 |
| Upload QuickID/SHA1 | 上传协议 | 闪传/内容证明 | 不是最终云端 File ID |
| IndexCore resource_id | IndexCore | Canonical 物理资源身份 | 稳定 ID 匹配可跨移动保持；tombstone 后重现会新建；不是 115 ID 的替代 |
| CloudSite Asset ID | CloudSite | 逻辑资源 | 不因文件移动/复制改变 |
| Release ID | CloudSite | 版本/渠道 | 与 Asset 关联，不绑定路径 |
| Variant ID | CloudSite | 平台/架构/封装形式 | 一个 Variant 可有多个 Copy |
| Copy ID | CloudSite | 业务中的物理副本投影 | 绑定 root+resource；物理 tombstone 后保留业务历史，不擅自改绑 |

所有跨系统键必须带命名空间：`provider_account_id + provider_object_id`、`indexcore_root_id + indexcore_resource_id`。单独存一个字符串 ID 不足以避免 Root/账号碰撞。

## 3. 操作后的身份变化

| 操作 | 115 身份 | IndexCore resource_id | CloudSite 业务关系 |
|---|---|---|---|
| 新上传/离线下载完成 | 产生新 File/Folder ID；info_hash 只作任务 ref | 观察后新增；若与现存对象冲突可能 ambiguous | 新建或关联 Copy，READY 必须等 Canonical |
| Rename | 通常预期 ID 保持，但必须读回确认 | 稳定 Provider ID 可保持；OpenList 弱身份可能靠 hash/路径证据，无法保证 | Copy 继续指 Canonical ID；业务 Asset 不变 |
| Move | 以读回 parent 为准，不盲假定 | 稳定 ID 或唯一移动证据可保持；目录后代无批量保持保证 | 物理位置更新，业务分类不变 |
| Copy | 应产生新 File ID；当前 driver 不返回 | 新路径且原对象仍 PRESENT 时新 resource_id | 新 Copy；不得把两个物理副本合并成同一 Copy |
| 同路径内容替换 | 可能新对象或同 ID 内容变化，取决于 Provider | 不同 hash 通常 R8 新对象；当前有收敛风险 | 不自动复用旧 Copy；需要冲突处理 |
| Delete→Recycle | 原对象从活跃树消失，生成/关联 rid | 经 COMPLETE/可信 mirror 后 tombstone | Copy availability→REMOVED，保留审计，不物理删业务历史 |
| Restore | 恢复原位置或指定位置；ID/rid 关系读回 | tombstone 后重现按当前规则是新 resource_id | 新建 Copy 或经明确审核重连，不能静默复活旧 Canonical |
| 文件夹 Rename/Move | Folder ID 读回；后代各自有 ID | 目录与后代分别匹配，可能部分冲突 | 业务分类不随物理目录批量改变 |
| 文件夹 Delete | 递归范围由 Provider 决定 | coverage 足够时 subtree 删除，否则只 additive | 各 Copy 独立收敛，不能仅凭目录 ACK 全标 REMOVED |
| 下载文件 | 不产生物理变更 | 不变 | 只记产品访问/下载审计，不改 Copy presence |

## 4. 推荐持久化边界

### 4.1 保留现有核心表

- `assets/releases/variants/copies` 继续承担逻辑身份。
- `storage_connections/storage_bindings` 继续隔离 Provider 连接和观察 Root。
- `indexcore_projection_cursors` 只表示消费进度，不表示 fresh SLA。
- `product_copy_presentation` 可保留轻量 overlay，但新统一分类不能永远停留在 Copy 单表别名。

### 4.2 建议新增的业务表（设计，未实施）

```text
categories(category_id, parent_id, slug, display_name, status, version)
asset_category_links(asset_id, category_id, assigned_by, assigned_at)
tags(tag_id, slug, display_name, tag_type, status)
asset_tag_links(asset_id, tag_id, source, confidence, assigned_by, assigned_at)
asset_aliases(alias_id, asset_id, alias, alias_type, normalized_alias, locale)
asset_synonyms(synonym_id, canonical_term, synonym, locale, status)
asset_summaries(asset_id, summary, source, revision, updated_at)
collections(collection_id, title, description, curator_id, status, revision)
collection_items(collection_id, asset_id, position, note)
topics(topic_id, title, description, status, revision)
topic_items(topic_id, collection_id or asset_id, position)
```

原则：

- `category/tag/collection/topic` 都链接 Asset，不链接物理路径；Copy 变化不会要求重新分类。
- 可筛选字段使用类型化列/关联表，不把所有内容塞入 EAV/JSONB。
- `Variant.attributes` 可继续存平台/架构等扩展，但必须有按 provider/resource type 的 JSON Schema 和允许键集合。
- 分类树需防循环、最大深度和版本化发布；标签与分类不互相替代。
- `source/confidence/assigned_by` 用于区分手工、规则、导入、可选 AI 建议；AI 建议必须经确定性校验/人工接受。

### 4.3 类型化元数据

| 层级 | 字段示例 |
|---|---|
| Asset | vendor、resource_type、summary、homepage_ref、license、language、tags、aliases |
| Release | version_raw、version_normalized、version_scheme、channel、release_date、upstream_ref |
| Variant | platform、architecture、package_type、codec、resolution、quality、quantization、file_type |
| Copy | Provider/Root/resource、availability、size、mtime、hash、location（观察值） |
| Collection/Topic | 策展说明、排序、适用人群、用途、发布时间、修订版本 |

## 5. 物理分类与业务分类

```text
115 物理目录：/软件/开发工具/某厂商/...
                  │ 观察
                  ▼
           IndexCore Canonical
                  │ 投影
                  ▼
                Copy
                  │ 绑定 Variant
                  ▼
Asset：某 IDE
  ├─ 分类：开发工具 / IDE
  ├─ 标签：Windows、macOS、编辑器、离线安装
  ├─ 别名：产品旧名、中英文别名、常见缩写
  ├─ Release：2026.1 / 2025.4
  ├─ Variant：Windows x64 / macOS arm64
  ├─ 合集：开发者新机器装机
  └─ 专题：离线开发环境
```

给一个 Asset 增加五个检索入口，不需要在 115 中创建五份文件，也不需要移动原目录。

## 6. 分类与绑定工作流

1. IndexCore 发现物理资源，CloudSite 建立 PHYSICAL_ONLY/UNRESOLVED_COPY。
2. 确定性规则生成候选：路径段、文件名 token、扩展名、版本/平台/架构解析、已有厂商词典。
3. 候选必须带 `rule_id/rule_version/confidence/evidence`，不能只保存结论。
4. 高置信且唯一匹配可自动建议；影响业务绑定时按风险选择自动或人工确认。
5. 冲突时保留 UNRESOLVED，不根据文件名强行覆盖人工绑定。
6. 用户修改分类只写 CloudSite，不发 115 移动操作。
7. 重新绑定/取消绑定有版本和审计；搜索投影按 revision 更新。
8. 可选 AI 只能在后续生成别名/简介建议，不直接写 Canonical、不操作文件、不成为搜索依赖。

## 7. 去重与副本策略

- `content_hash + size + hash_algorithm` 只生成 duplicate candidate。
- 同 hash 不同 Provider ID/路径的活跃文件保留不同 Copy，不自动合并。
- 重复检测展示“相同内容副本”及存储账号、位置、业务绑定和最后观察时间。
- 清理副本是高风险独立命令，需要用户明确选择保留项，不得由去重任务自动删除。
- 无 hash 时不能用文件名+大小作确定去重，只能作为弱候选。
- 逻辑“同一资源”由 Asset/Release/Variant 绑定表达，不由内容 hash 决定。

## 8. 状态模型

IndexCore 使用两个独立维度：`resource_presence` 为 `PRESENT / REMOVED`；`removal_evidence_state` 可经历 `MISSING_CONFIRMED_BY_COMPLETE_SNAPSHOT / REMOVAL_CANDIDATE` 等证据状态。MISSING 或 CANDIDATE 时资源仍可保持 PRESENT，只有确认删除后才改为 REMOVED；可信 mirror coverage 也可能直接确认删除，不能把它描述为所有资源必经的同一线性 presence 链。两个维度均由 IndexCore 所有。

业务 Copy：`UNRESOLVED_COPY / CLASSIFIED / HIDDEN / REMOVED` 等应明确状态机和 DB CHECK，不能用单个 availability 同时表达分类、隐藏、删除和权限。

操作建议：

- `copy.availability` 只表达物理副本在业务投影中的可用状态。
- `presentation.hidden` 只表达展示，不删除、不撤销既有分享权限，权限另判。
- `Asset/Release/Variant.status` 用受控枚举表达草稿、发布、归档，不与 Copy presence 联动覆盖。
- 所有状态迁移需要 expected_version/CAS，防止管理员并发覆盖。

## 9. 大规模元数据与分类性能

- 分类/标签筛选走索引关联表和 keyset 分页，避免每页 N+1。
- 搜索投影异步更新但带 revision/checkpoint，失败保留上一个完整发布版本。
- 规则重跑按 `rule_version + input_fingerprint` 增量，不整库重复解析。
- 厂商/别名/同义词词典版本化，可回放、可审计。
- 物理移动只更新 Copy 观察位置，不重算 Asset 分类。
- 统计和热门使用独立聚合任务，不在每次首页请求扫描完整操作历史。

## 10. 需要项目所有者决策

1. 是否正式确认 Copy 保持一对一物理资源，合集由 Asset/Collection 表达（推荐）。
2. 哪些高置信规则允许自动绑定 Variant，哪些必须人工确认。
3. 业务分类是否全站共享，未来私有空间如何隔离分类和标签。
4. 资源简介/别名是否允许公开，是否含版权、供应商或敏感内部名称。
5. 删除物理副本后，业务 Asset 何时自动归档；建议默认不自动删除逻辑资源。