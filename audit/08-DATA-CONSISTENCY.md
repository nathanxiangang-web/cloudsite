# 08 115、OpenList、IndexCore、CloudSite 数据一致性

## 1. 真相与责任

| 层 | 负责 | 不负责 |
|---|---|---|
| 115 | 真实文件、目录、离线任务、回收站和 Provider 回执 | CloudSite 业务分类、IndexCore generation |
| OpenList | 现有文件访问与观察适配、内容交付 | 不拥有 CloudSite Copy，不证明业务 READY |
| IndexCore | 物理观察、身份判定、Canonical、Journal、Watch/Hint | 不执行 CloudSite 用户授权或 Provider 业务命令 |
| CloudSite | 业务 Catalog、用户、任务、Provider 命令、审计和派生读模型 | 不直接写 Canonical，不用 OpenList 内容成功替代 Canonical |

CloudSite 的 Projector 写 Copy、libraryview 写搜索/文件库快照，都属于业务/派生数据；不是第二物理真相写入者。

## 2. 当前离线下载一致性链

```text
POST /api/v1/cloud-downloads
  → product_operations + Manifest + ACQUISITION Job
  → START_RESERVED / REFERENCE_KNOWN fence
  → 115 AddOfflineTaskURI（一次发送，info_hash）
  → 轮询 Provider 成功
  → AWAITING_VISIBILITY
  → loopback trusted Hint(root_id, target_path, POSSIBLE_CHANGE)
  → IndexCore scoped OpenList 重新观察
  → Canonical + Q8 Journal
  → CloudSite Q5 精确 result_name + Q8 Projector
  → Copy PRESENT + result_copy_id
  → Manifest READY + Job SUCCEEDED
```

正面证据：`internal/acquisition/stage_dispatch.go:107-284`、`projector/projector.go:109-147`、`acquisition_canonical_repository.go:76-261`。

风险：CS-01 多结果 panic、CS-02 三页轮询、CS-04 Journal gap、CS-05 无总 TTL、CS-06 暂时错误恢复、IC-05 Journal 非自包含和 IC-07 freshness debt。

## 3. 通用写操作的七个必答问题

每个操作必须回答：谁发起、谁拥有物理状态、如何确认成功、UNKNOWN 如何处理、如何通知 IndexCore、如何更新 CloudSite、如何恢复。统一答案：

1. **发起者**：CloudSite 已认证用户/管理员经 FileOperation Job；当前只有离线下载实际接线。
2. **物理状态**：115 所有；OpenList 是访问/观察层；IndexCore 是观察后的 Canonical 真相。
3. **成功确认**：Provider 回执 + 必要 read-after-write；业务完成还需 IndexCore Canonical/Query 确认。
4. **UNKNOWN**：持久化发送证据并进入对账，不盲目重发、不直接标失败/成功。
5. **通知**：CloudSite transactional outbox 发送同宿主 trusted Hint；失败可重发信号，202 不是完成。
6. **CloudSite 更新**：业务命令回执与 Canonical 投影分开提交；只有 Query/Journal 证据才能更新物理可用性。
7. **恢复**：按 operation/item 保存 APPLIED/NOT_APPLIED/UNKNOWN/PARTIAL、预检和对账证据；支持重试安全子集或人工恢复。

## 4. 操作一致性矩阵

本节除现有离线下载外，均为目标设计，不表示 Upload/Move/Rename/Copy/Delete 等能力已在 CloudSite 接线或上线。

### 4.1 文件新增（上传）

- 发起/物理：CloudSite Upload Job → 115。
- 成功/UNKNOWN：checkpoint+最终 File ID/parent/name/size/SHA1 读回；Complete 断连先查 OSS/115，禁止重放 Complete。
- IndexCore/CloudSite：对目标父目录发 Hint，等待目标 resource 的 Q3/Q5 与 Projector；随后 Copy 关联 Variant。
- 恢复：同 UploadID+PartNumber+内容 SHA1 可续传；身份不匹配转人工；Canonical 超时保留 Provider 成功证据。

### 4.2 文件移动

- 发起/物理：CloudSite Move Job → 115。
- 成功/UNKNOWN：预存源 ID/parent，读回目标 parent 和源位置；批量逐项，不整体猜成功。
- IndexCore/CloudSite：对源父目录和目标父目录都发 Hint；稳定 ID 可保持 resource_id，弱身份需唯一移动证据；Copy 随 Canonical，业务 Asset 不动。
- 恢复：源/目标 ABA、部分移动、路径冲突分别对账；不能只向目标发 Hint 而遗留源 subtree。

### 4.3 文件重命名

- 发起/物理：CloudSite Rename Job → 115。
- 成功/UNKNOWN：按源 ID 读回新名称和父 ID；不能只按新名称搜索。
- IndexCore/CloudSite：对父目录 Hint；resource_id 是否保持由 IndexCore 身份证据决定；业务别名不被物理重命名覆盖。
- 恢复：名称冲突、源 ID 已变化、Provider ACK 后不可见均进入明确状态，不自动换绑 Copy。

### 4.4 文件复制

- 发起/物理：CloudSite Copy Job → 115。
- 成功/UNKNOWN：必须取得新 File ID；当前 driver 不返回时只能用可靠前后差异，无法唯一识别就保持 UNKNOWN。
- IndexCore/CloudSite：对目标父目录 Hint；新路径且源仍活跃通常得到新 resource_id；创建独立新 Copy。
- 恢复：禁止按 hash 把旧文件当新副本；同名同 hash 多候选时人工处理。

### 4.5 文件删除（入回收站）

- 发起/物理：CloudSite 高风险 Delete Job → 115。
- 成功/UNKNOWN：预检非根和影响，读回源不可见及回收站 rid；rid 与原 fid 的映射未由已审计 fixture 证明，对账前不得假定相同或可互换。
- IndexCore/CloudSite：对原父目录 Hint；只有 COMPLETE/可信 mirror coverage 才能产生 removal；Copy 变 REMOVED，业务历史保留。
- 恢复：Provider 已删但 Canonical 未删时显示等待，不用 SQL 强行 REMOVED；误删只能走 Restore，不重放 Delete。

### 4.6 文件夹移动

- 发起/物理：CloudSite → 115，需循环目标和 subtree 影响预检。
- 成功/UNKNOWN：Folder ID 读回 parent；后代真实状态由观察确认。
- IndexCore/CloudSite：源和目标父 scope 都 Hint，必要时受预算的 subtree 重新观察；IndexCore 当前不保证后代批量保持 resource_id。
- 恢复：允许部分后代冲突，不能只改一个目录 Copy 就声明整树完成；业务分类不随目录批量移动。

### 4.7 文件夹重命名

- 发起/物理：CloudSite → 115。
- 成功/UNKNOWN：Folder ID+parent+新名称读回；路径可能 ABA，不能仅靠路径。
- IndexCore/CloudSite：父目录和该目录的观察信号分别处理；后代路径由 IndexCore 独立收敛。
- 恢复：若目录本身确认而后代未收敛，显示 partial/freshness debt，不伪造整树成功。

### 4.8 文件夹删除

- 发起/物理：CloudSite 高风险递归 Delete → 115。
- 成功/UNKNOWN：新鲜影响预览、二次确认、递归授权；源和回收站读回，递归服务端语义仍需合同验证。
- IndexCore/CloudSite：对父目录和目录 coverage 发 Hint；只有有效 direct-child/subtree coverage 才授权 IndexCore 删除。
- 恢复：绝不能把虚拟/失败/空响应当空目录；部分 Canonical 未删保持恢复状态，历史不物理清除。

### 4.9 文件下载完成

- 发起/物理：用户经 CloudSite 内容授权；通常不改变 115 物理状态。
- 成功/UNKNOWN：HTTP status、长度、Range、可用 SHA1 校验；浏览器保存行为不一定可被服务端证明。
- IndexCore/CloudSite：无需 mutation Hint 或 Copy presence 更新；只记产品访问审计/热门聚合。
- 恢复：短 lease 刷新和 Range；已签发 302 URL 的撤销边界单独说明。

### 4.10 离线下载完成

- 发起/物理：CloudSite acquisition → 115 离线任务。
- 成功/UNKNOWN：info_hash 精确任务状态；页数耗尽不等于确认不存在，添加响应丢失按 URI/saveDir/hash 对账。
- IndexCore/CloudSite：现有 Hint→Q5/Q8→Copy→READY 链；不得只凭任务 status=成功就 READY。
- 恢复：Provider 成功但文件不可见进入 visibility TTL/恢复；历史 REFERENCE_KNOWN 不重新 Start。

## 5. Journal 与 Projector 契约

当前 Q8 payload 只有 `{"reason":...}`（IC-05），因此消费者必须：

1. 用 Q4/Q6 建立/校验基线；
2. 用 Q8 发现变化，但此时尚不能推进已应用游标；
3. 按 resource_id 回读 Q3，必要时 Q4/Q5；处理 removal 必须显式 `include_removed=true`，Root 为 DEPRECATED/DELETED 时还需相应 lifecycle visibility，404 不能单独解释为删除；
4. 以 Canonical 当前值为准，允许跳过中间状态，完成身份校验和投影提交；
5. 仅在回读与投影全部成功后，才在同一数据库事务中以 expected cursor CAS 原子推进 per-root event_seq；gap、回读失败或身份冲突时不得推进；
6. 跨 Root 保存 cursor vector，不按 event_id 全局排序。

CloudSite 旧 Projector 必须修复 CS-04；libraryview 已有连续校验，可抽取共同契约。未来版本化 Journal payload 可减少回读，但 legacy reason-only 仍需兼容。

## 6. Hint outbox 与事务边界

推荐 CloudSite 新表（设计）：`indexcore_hint_outbox(operation_id, root_id, scope_key, reason, state, attempts, next_attempt_at, accepted_signal_seq, created_at)`。

- 与 Provider outcome 同事务写 outbox intent，不在 DB 事务内调用 Hint。
- 同一 operation/scope 可合并重复信号，但不能合并掉不同 claimed/pending provenance。
- Hint 无幂等键，CloudSite 需自行去重；重复 POSSIBLE_CHANGE 应安全，但不能假定所有 reason 都可合并。
- 保存 accepted signal_seq 只作回执，完成仍以 Query 为准。
- Hint 不可用时有周期 Watch/基线兜底；不得为了“可靠”把 Hint 暴露公网。

## 7. 失败场景矩阵

| 场景 | 禁止行为 | 正确收敛 |
|---|---|---|
| Provider 写后响应丢失 | 立即重发删除/移动/复制 | UNKNOWN→预检信息驱动对账 |
| Provider 部分成功 | 整批成功或整批重放 | 逐项结果，未执行/失败/未知分离 |
| Provider 成功，Hint 失败 | 直接 READY 或重新 Provider 写 | 保留回执，重试 Hint/等待 Watch |
| Hint 202，Canonical 未变 | 把 202 当文件存在 | 有界等待+Q3/Q5/Q8 验证 |
| Q8 gap | 推进 cursor | 停止该 Root 投影并重建 |
| Q5 ambiguous | 前端选第一项 | 展示冲突并触发身份诊断 |
| OpenList virtual empty | 当 COMPLETE 空目录 | additive-only，不删除 |
| Provider tombstone 后重现 | 复活旧 resource_id | 新 Canonical ID，业务重连需审计 |
| 长时间 pending | 永久轮询无预算 | 阶段 TTL、backpressure、RECOVERY_REQUIRED |

## 8. 一致性验收

必须覆盖操作级矩阵：新增/移动/重命名/复制/删除、目录三类操作、离线下载，叠加响应丢失、部分成功、路径 ABA、稳定 ID 重复、进程崩溃、Hint 429、Q8 gap、Root 配置 ABA 和投影重启。

端到端不存在跨 115/PostgreSQL 的全局 ACID 事务，目标应是“可证明的 effectively-once + 可恢复最终一致”，而不是声称 exactly-once 分布式事务。任何性能优化都不能以绕过这些证据链为代价。