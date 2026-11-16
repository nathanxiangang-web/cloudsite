# 11 技术债与风险清单

## 1. 统计口径

共 54 个唯一问题 ID；按互斥主分类统计为 P0 2、P1 26、P2 19、P3 6、BLOCKER-only 1。IC-07 主分类为 P1，同时兼具 BLOCKER 属性，因此带有 BLOCKER 语义的 ID 实际有 2 个，第 6 节会重复引用 IC-07，但不增加唯一问题数。CS-01/DR-10 等为跨项目同一调用链的关联记录，因此数量不等于独立修复 PR 数。`FACT` 描述代码/运行证据，影响若为条件性则明确标 `INFERENCE`；实际线上触发情况未知时不宣称事故已发生。

| 级别 | 含义 |
|---|---|
| P0 | 凭证/数据安全、身份或核心功能存在严重边界问题 |
| P1 | 重要架构缺陷、生产正确性/性能阻断、敏感信息风险 |
| P2 | 恢复、维护性、体验、可扩展性和长期运行问题 |
| P3 | 局部一致性、文档和一般优化 |
| BLOCKER | 缺少必需证据或真实容量/部署验收，不能作生产结论 |

## 2. P0

| ID | 项目 | 问题 | 主要影响 | 难度 | 处理包 |
|---|---|---|---|---|---|
| DR-01 | 115driver | 下载 Header 可能携带长期 Cookie 到 CDN/其他源 | 跨源账号凭证暴露；CloudSite 当前未调用该方法 | 中 | R0-1 |
| DR-02 | 115driver | 可选 HTTP/任意 API URL 与 client Cookie 组合 | 配置触发明文或跨源凭证暴露 | 小 | R0-1 |

## 3. P1

| ID | 项目 | 问题 | 主要影响 | 难度 | 处理包 |
|---|---|---|---|---|---|
| CS-01 | CloudSite/115driver | 离线多结果在本地校验前可能 panic | 产品进程终止、副作用 UNKNOWN | 中 | R1-1 |
| CS-02 | CloudSite | 离线任务只查三页且错误语义混合 | 页外任务误进人工恢复 | 中到大 | R1-1 |
| CS-03 | CloudSite | original/execution source 明文入库 | 签名 URL/私密 magnet 进入 DB 和备份 | 大 | R1-2 |
| IC-01 | IndexCore | UNRESOLVED/CONFLICT 可推进 MISSING | 特定 COMPLETE 序列误删 Canonical | 中高 | R1-3 |
| IC-02 | IndexCore | R8 同路径替换不收敛/双方删除风险 | 永久 ambiguous 或当前对象误删 | 高 | R1-3 |
| IC-03 | IndexCore | 同 Snapshot 重复稳定 ID 未判重 | 重复 MOVE/UPDATE、身份错误 | 中 | R1-3 |
| IC-04 | IndexCore | counts/conflicts 未真实持久化 | 故障不可诊断 | 小到中 | R1-3 |
| IC-05 | IndexCore | Journal payload 只有 reason | Q8 不能自包含重放 | 中高 | R1-4 |
| IC-06 | IndexCore | full-scan 无 adapter ABA fence | 旧命名空间写入新配置 Root | 中 | R1-6 |
| IC-07 | IndexCore | Issue134 freshness/debt/capacity NO-GO（区别于历史 Gate 3 20K ingestion PASS） | 大规模可见性无界延迟；完整源 traversal/长稳仍未验证 | 高 | R1-6 |
| IC-08 | IndexCore | item-local INTERNAL 可使 serve fatal | 单 scope 故障扩大为服务退出 | 中高 | R1-6 |
| DR-03 | 115driver | debug 记录 Cookie/password/登录响应 | 日志凭证泄露 | 小到中 | R0-1 |
| DR-04 | 115driver | 共享 client 可变状态竞争 | 多账号/并发身份混乱 | 大 | R1-2 |
| DR-05 | 115driver | Cookie 重导入叠加 | 账号不确定 | 中 | R1-2 |
| DR-06 | 115driver | 429/5xx/网络错误无统一分类 | 暂时故障误报认证、无安全重试 | 大 | R1-2 |
| DR-07 | 115driver | 写 UNKNOWN 无对账，可注入盲重试 | 重复移动/复制/离线等副作用 | 大 | R1-2 |
| DR-08 | 115driver | 分片上传无跨进程恢复/Complete 对账 | 上传遗留、状态未知 | 大 | R1-2 |
| DR-09 | 115driver | 非闪传恰好 9GiB 可 panic | 上传进程终止 | 小 | R1-1 |
| DR-10 | 115driver | 离线内部状态未查、多结果越界 | 部分成功不可见/panic | 中 | R1-1 |
| DR-11 | 115driver | 目录分页可跳项/死循环/假空 | 漏文件、无效 scope 误判 | 中 | R1-1 |
| DR-12 | 115driver | Stat/GetFile 可假成功 | 零值对象进入业务判断 | 小到中 | R1-1 |
| DR-13 | 115driver | 根删除/空 RID 清理无本地保护 | 潜在大规模不可逆删除 | 小 | R1-2 |
| DEP-01 | 生产 | `.env` 0644 且含秘密键 | 本地低权限读取秘密 | 小到中 | R0-2 |
| DEP-02 | 生产 | 每日归档包含秘密和状态 | 备份/远端扩大凭证暴露面 | 中 | R0-2 |
| DEP-03 | 生产 | live tar 不证明 PG 一致和可恢复 | 备份存在但灾难恢复失败 | 中到大 | R1-5 |
| DEP-05 | 生产 | Portainer socket RW、OpenList root | 单点失陷影响全容器/数据 | 中 | R1-5 |

## 4. P2

| ID | 项目 | 问题 | 主要影响 | 难度 | 处理包 |
|---|---|---|---|---|---|
| CS-04 | CloudSite | Projector 接受 Journal gap 并推进 | 永久漏投影/错误 PRESENT | 小 | R1-4 |
| CS-05 | CloudSite | acquisition 无总/阶段 TTL | 永久轮询和状态积压 | 中 | R2-1 |
| CS-06 | CloudSite | Hint/Q5/Q8 暂时错误未安全重试 | 可恢复故障转人工 | 中 | R2-1 |
| CS-07 | CloudSite | 通用 Cancel 未隔离 ACQUISITION | Manifest/Job 状态分裂 | 中 | R2-1 |
| CS-08 | CloudSite | 无限提交缺系统背压 | 队列/DB/Provider 滥用风险 | 中 | R2-5 |
| CS-09 | CloudSite | 302 URL 撤销边界 | 撤销后短窗口仍可访问 | 中到大 | R2-2 |
| IC-09 | IndexCore | scoped commit 全 Root prior+索引不匹配 | 串行事务/解码/锁成本增长 | 中到高 | R2-5 |
| IC-10 | IndexCore | Snapshot/DRAFT/result/Watch 无保留策略 | 存储/WAL/备份积压 | 中高 | R2-5 |
| IC-11 | IndexCore | Query limit 无上限/非法参数回退 | 内存放大、全 Journal 重读 | 小到中 | R2-5 |
| IC-12 | IndexCore | 普通 OpenList listAll 无界 | 高内存、长扫描 | 中 | R2-5 |
| IC-13 | IndexCore | adapter secret 校验可绕过 | 明文秘密误持久化 | 小到中 | R2-6 |
| IC-14 | IndexCore | writer lock 非完整租约 | DB 故障下第二 writer/关闭风险 | 高 | R2-6 |
| DR-14 | 115driver | WithAsc 写错字段 | 排序和上传核验错误 | 小 | R3-1 |
| DR-15 | 115driver | 上传成功核验弱 | 同内容旧文件误当新文件 | 中到大 | R2-1 |
| DR-16 | 115driver | 下载无 lease/Range，Get 全量读内存 | 错误页、OOM、无法恢复 | 中到大 | R2-2 |
| DR-17 | 115driver | 搜索/分享/批量语义不完整 | 文件/目录分类计数缺失、完整分页未验证、部分成功和新 ID 不可表达 | 中到大 | R2-2 |
| DEP-04 | 生产 | Portainer/rclone 使用 latest | 配置漂移、回滚不可复现 | 小 | R2-6 |
| DEP-06 | 生产 | 2 vCPU 小规格+I/O/swap 压力 | 服务间性能放大，长期原因未知 | 中 | R2-6 |
| DEP-08 | 生产 | 单点、保留和监控闭环不足 | 整体不可用/备份成本/告警缺口 | 中到大 | R2-6 |

## 5. P3

| ID | 项目 | 问题 | 处理包 |
|---|---|---|---|
| CS-10 | CloudSite | `panta-db` 底层错误输出脱敏较弱 | R3-1 |
| CS-11 | CloudSite | Catalog 状态 CHECK/唯一约束不足 | R2-3 |
| CS-12 | CloudSite | README 1.0.2/schema20 与实际 1.0.3/schema21 漂移 | R3-1 |
| IC-15 | IndexCore | provider failure 指标分类不一致 | R3-1 |
| IC-16 | IndexCore | append-only 数据库防御深度不一致 | R3-1 |
| DR-18 | 115driver | app_version/mkdir -p/Cookie 解析等局部缺陷 | R3-1 |

## 6. BLOCKER

| ID | 问题 | 阻断的结论 | 解除方式 |
|---|---|---|---|
| DEP-07 | 无 Docker/root/system journal/DB 只读证据 | 实际 Git SHA、schema、日志、连接池、积压、生产长稳 | 脱敏只读回执或专用只读账号 |
| IC-07 | Issue134/F2/F4 freshness/capacity NO-GO；完整源 traversal/长稳未验证（同时为 P1） | 大规模生产 freshness/capacity 承诺；不否定历史 Gate 3 20K ingestion PASS | 授权部署后的真实容量与长稳验收 |

## 7. 根因主题

1. **把协议表面当可靠性**：115driver 有函数不等于有分页、部分成功、UNKNOWN、幂等和恢复。
2. **身份证据不完整时的收敛**：IndexCore 强安全设计仍有重复 ID/R8/冲突删除边界，必须先用对抗测试封闭。
3. **多个派生消费者契约漂移**：CloudSite 两套 Q8 消费逻辑连续性不同，Journal payload 又不足以独立重放。
4. **队列深度不等于工作量**：IndexCore lazy materialization 和 CloudSite 无限 pending 都会让“队列很小”掩盖债务。
5. **秘密生命周期跨出进程**：URL、Cookie、备份和日志形成二次存储，单点加密/脱敏不足。
6. **发布证据链断裂**：源码、静态资源、二进制、schema、运行配置和备份回执必须分别证明。

## 8. 不建议的“伪优化”

- 通过加 Reader/goroutine 解决所有 Issue134，而不处理全 Root prior、Watch 生命周期和预算。
- 把 Q8 当完整 CDC 或在 gap 后继续 cursor。
- 用文件路径/hash 作为永久对象身份，或自动合并同内容文件。
- 写请求超时后直接重试删除/移动/复制/Complete。
- 为提高搜索体验让 AI 自主分类并移动真实文件。
- 以静态资源 hash、绿色 CI、Draft 部署回执或进程存在替代二进制/schema/生产容量证明。