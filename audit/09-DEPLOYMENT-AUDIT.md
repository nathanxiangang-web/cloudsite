# 09 生产服务器部署只读审计

## 1. 访问和证据边界

目标：`nathan@113.20.6.57`，仅执行只读 SSH 命令和公开 HTTPS GET/HEAD。未使用 sudo，未读取 115 Cookie、数据库密码、私钥内容或秘密值，未重启/写配置/执行 migration/网盘操作。

初次 SSH 失败原因是本地私钥权限 `0664` 被 OpenSSH 拒绝；仅将本地私钥权限收紧为 `0600` 后连接成功，未改密钥内容或服务器。

`nathan` 属 sudo 组，但无 Docker socket、root-only 配置、系统 journal 和 `/proc/<pid>/exe` 读取权限。本审计遵守只读边界，没有提权。因此部分必需材料为 BLOCKER/UNKNOWN，而不是推测为正常。

## 2. 直接观测到的运行环境

观测时间：2026-11-16 09:50—10:40 UTC。

| 项目 | 事实 |
|---|---|
| OS/Kernel | Ubuntu 24.04 LTS，Linux 6.8.0-31-generic x86_64 |
| CPU/内存 | 2 vCPU，1.9 GiB RAM，1 GiB swap |
| 磁盘 | `/dev/sda1` 109G，已用 23G（23%），inode 4% |
| 运行时长 | 首次采样 uptime 19 天 19 小时，load 6.75/6.08/5.66 |
| 公网监听 | 仅见 22、80、443；未见 PostgreSQL/IndexCore 直接公网监听 |
| 容器运行时 | docker.service、containerd.service active；无 Docker API 权限 |
| Web 入口 | Traefik（不是 Nginx），Cloudflare HTTPS 响应 |
| 进程 | CloudSite acquisition、IndexCore serve、cloudsite-index-reader、OpenList、Traefik、Portainer、两个 PostgreSQL 集群 |
| Systemd | host 上 nginx/postgresql inactive，符合数据库容器化运行；存在 CloudSite storage-token timer |
| 项目目录 | `/opt/my-servers`；另有 root-only 历史目录和 tar 包，未读取 |

进程启动时间：CloudSite acquisition 为 2026-11-16 09:17:26 UTC，IndexCore 为 2026-11-15 04:17:07 UTC，OpenList/PostgreSQL/Traefik 等多数为 2026-10-27。进程存在不证明其源码 SHA 或健康。

## 3. 源码与运行版本交叉核验

### 3.1 CloudSite 静态资源直接匹配

| 资源 | 本地审计 HEAD 与生产共同 SHA-256 |
|---|---|
| `library-page.js` | `b1c39e3cc331c3c660f6d2e3f1dffb4095ebf056e0572f6c834df3ff23cded1f` |
| `library.js` | `b34b7429e5b477027eb5a3f41c8fc299c59f37dc34681292cad4150198573fba` |

生产公开首页引用新版静态资源，`GET /api/v1/auth/capabilities` 返回 `{"registration_enabled":true}`。安全响应含 CSP、`X-Frame-Options: DENY`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer`，静态资源 `Cache-Control: no-store`。

**结论仅为 FACT：生产所服务的这两个静态文件与本地 1.0.3 HEAD 字节一致。** 由于 `/proc/<pid>/exe`、Docker inspect 和 DB 均不可读，CloudSite 二进制 Git SHA、schema21 migration、library_snapshots 实际配置仍为 UNKNOWN。

### 3.2 IndexCore/OpenList

- IndexCore `indexcore serve` 进程存在，启动时间晚于本地 main 提交；不能由此推断运行 main 或 Draft PR149。
- OpenList Compose 固定 image digest `sha256:c555c6e1...`，但实际容器 image ID/配置不可读。
- IndexCore/OpenList 二进制 Git SHA、实际启动 flags、schema、Root/Watch 状态和日志均为 UNKNOWN/BLOCKER。

**间接材料（不是本审计独立运行证据）**：PR149 正文包含 2026-11-15 的自述部署回执，称运行 `3a82f542bf7ab56814f47b8523255afc72a83a5d`、schema 5→8、F1/F2/F4 默认 OFF、基础检查通过，并明确真实 20K、长稳和 HTTP 吞吐为 NOT_RUN_POST_DEPLOYMENT。本次账号无法独立复现其 `/proc` SHA、schema 或日志，因此仍保留上述 UNKNOWN/BLOCKER；该回执不能升级为生产容量、长稳或当前状态证明。

## 4. 脱敏 Compose 与网络关系

可读 `/opt/my-servers/docker-compose.yml` 显示：

- Traefik v3.7 发布 80/443，挂载 Docker socket 只读，连接 `app-net` 和外部 `cloudsite-proxy`。
- Portainer `portainer/portainer-ce:latest`，挂载 Docker socket **读写**，经 HTTPS host 路由暴露。
- OpenList 使用固定 digest，挂载 `/opt/my-servers/openlist/data`，`user: 0:0`，PUID/PGID=0，UMASK=077。
- rclone-backup 使用 `rclone/rclone:latest`，整个 `/opt/my-servers` 以只读挂载进容器，每日 04:30 执行脚本。
- 根 Compose 中未见 Nginx；TLS/host 路由由 Traefik 负责。

`cloudsiteV1.0.0alpha.yml` 为 root `0600`，无法读取；不能推断其中的实际环境变量、资源限制、healthcheck、DB 连接池和卷。

## 5. 部署问题清单

### DEP-01 — P1 — 含秘密键的 `.env` 对所有本地用户可读

- 类型：FACT。
- 位置：`/opt/my-servers/.env` metadata `-rw-r--r-- root:root`，大小 138；仅读取键名。
- 数据/API：`CLOUDSITE_SECRET_KEY`、`CLOUDSITE_MASTER_KEY`、`CLOUDSITE_SETUP_TOKEN`；未读取值。
- 触发：任一可登录本地用户或低权限进程读取该文件。
- 影响：产品秘密泄露，具体用途和轮换状态 UNKNOWN。
- 建议： owner 授权的后续窗口调整为最小权限、确认 Docker Compose 引用来历、轮换可能暴露的秘密并审查历史备份；本阶段不修改。
- 难度：小（权限）/中（轮换与发布）。
- 测试：以非 owner 账号读取应拒绝，服务启动仍成功，日志/配置脱敏，旧秘密失效。

### DEP-02 — P1 — 每日归档包含秘密和状态，远端加密/访问未知

- 类型：FACT（挂载和脚本）；远端保护 UNKNOWN。
- 位置：Compose rclone-backup；`/opt/my-servers/rclone/docker_backup.sh:4-20`。
- 数据/API：源 `/backup_source` 包含 `.env`、OpenList data、`rclone/config/rclone.conf` 等；上传 `mybackup:/`。
- 触发：每日 tar + rclone move。
- 影响：秘密和凭证进入归档及远端；若远端非加密 crypt/权限过宽，扩大暴露面。
- 建议：提供脱敏远端类型/加密/ACL/保留证据；秘密与数据备份分级，避免把备份凭证自包含进同一归档。
- 难度：中。
- 测试：在隔离环境列归档内容、验证不含不应备份的秘密，检查远端最小权限和加密恢复。

### DEP-03 — P1 — live tar 不能证明 PostgreSQL 一致备份或可恢复

- 类型：FACT（脚本行为）；是否存在另行受控 PG 备份 UNKNOWN。
- 位置：`docker_backup.sh:11-29`；无 pg_dump/快照/恢复校验。
- 数据/API：整个运行目录，可能包括数据库卷和任务状态；实际卷清单 UNKNOWN。
- 触发：备份期间数据库/文件持续写入，脚本还将 tar exit 1 当成功。
- 影响：归档可能存在跨文件/页不一致；仅有 tar 成功和 rclone 成功不代表可恢复。
- 建议：使用 PostgreSQL 一致的逻辑/物理备份、WAL/恢复策略、隔离 restore 和表/身份/任务不变量验证；保留现有发布备份。
- 难度：中到大。
- 测试：网络隔离 restore、schema/data digest、CloudSite/IndexCore 启动和只读查询、RPO/RTO 演练。

### DEP-04 — P2 — `latest` 镜像增加配置漂移和回滚不确定性

- 类型：FACT。
- 位置：Compose `portainer/portainer-ce:latest`、`rclone/rclone:latest`。
- 数据/API：镜像供应链和重启部署。
- 触发：重新 pull/重建或节点迁移。
- 影响：同 Compose 可运行不同代码，审计和回滚不可复现。
- 建议：固定审核后的 digest，记录镜像 ID/SBOM/升级回滚。
- 难度：小。
- 测试：两次隔离部署镜像 digest 一致，固定版本回滚可用。

### DEP-05 — P1 — 高权限管理面与 root 容器扩大单点影响

- 类型：FACT（配置）；是否已被利用 UNKNOWN。
- 位置：Portainer HTTPS host 路由 + Docker socket RW；OpenList `user: 0:0`。
- 数据/API：Docker 控制面、OpenList 数据卷和 Provider 配置。
- 触发：Portainer/OpenList 漏洞或账号失陷。
- 影响：INFERENCE——Portainer 可控制全部容器；root OpenList 增加容器逃逸后主机文件风险。
- 建议：核验 MFA/访问策略/管理面暴露，限制 Docker socket，评估非 root OpenList 与只读挂载；变更须单独窗口。
- 难度：中。
- 测试：未授权/低权限访问矩阵、socket 权限、容器 UID、卷写范围和备份恢复。

### DEP-06 — P2 — 小规格主机存在 CPU/I/O/swap 压力信号

- 类型：FACT（短窗口指标）；长期瓶颈/泄漏 INFERENCE/UNKNOWN。
- 位置：主机资源，不是某一代码函数。
- 数据/API：PostgreSQL、IndexCore、OpenList 所有负载。
- 触发：采样时 2 vCPU、swap 488544 KiB；vmstat 首样本 wa=39；CPU pressure some avg300=45.88，I/O some=43.48/full=13.70。
- 影响：扫描、查询、内容预览和双 PG 集群可能相互放大延迟；不能据单点证明资源泄漏。
- 建议：先读容器/DB 指标定位 I/O、查询、内存和交换来源，再决定预算/资源调整；不盲目加并发。
- 难度：中。
- 测试：授权后长时间容器 stats、PG pg_stat_statements/锁/临时文件、磁盘延迟、PSI 与端到端 Q5/Q8 关联。

### DEP-07 — BLOCKER — 实际二进制、schema、日志和 DB 状态无法核验

- 类型：BLOCKER/UNKNOWN。
- 位置：Docker socket、root-only alpha Compose、`/proc/<pid>/exe`、系统 journal、数据库凭证均不可访问。
- 数据/API：CloudSite/IndexCore Git SHA、schema_migrations、连接池、Watch/Work/Journal/Job 积压、备份回执。
- 触发：当前账号权限边界。
- 影响：不能回答源码与生产是否完全一致，也不能宣称生产稳定或无积压。
- 建议：由所有者提供脱敏只读回执或专门只读账号；不要求真实 Cookie/密码。
- 难度：取决于运维授权。
- 测试：只读 `docker inspect/ps/logs`、二进制 `go version -m`、SELECT-only schema/统计查询与运行配置交叉比对。

### DEP-08 — P2 — 单点、保留与监控闭环证据不足

- 类型：FACT（单机/脚本无 retention）；监控覆盖 UNKNOWN。
- 位置：所有关键服务集中一台主机；rclone 脚本按日期上传但无保留/清理/告警验证。
- 触发：主机/磁盘/单 PG/单 OpenList/单 IndexCore 故障，备份失败或远端归档无限增长。
- 影响：服务整体不可用、备份成本和恢复不确定性；sysstat 存在不代表业务告警完整。
- 建议：明确单点接受度、离线/异机备份、保留策略、恢复演练和业务 freshness/queue/backup 告警。
- 难度：中到大。
- 测试：模拟备份失败、远端不可用、磁盘满、主机丢失后的重建；验证告警实际到达。

## 6. 当前可确认的安全正面项

- 数据库和 IndexCore 未直接监听公网；公网仅见 SSH/HTTP/HTTPS。
- OpenList image 使用 digest，而非 latest。
- Traefik Docker socket 为只读，且 `no-new-privileges`。
- 产品 HTTPS 有较完整浏览器安全头。
- rclone 配置本身为 root `0600`，本审计未读取。
- CloudSite 静态资源版本可精确匹配，未依赖文档回执猜测。

## 7. 关闭 UNKNOWN 所需脱敏材料

1. `docker ps --no-trunc`、各服务 `docker inspect` 中 image/labels/flags/mounts/resources/healthcheck（环境值脱敏）。
2. CloudSite/IndexCore 二进制 `go version -m` 或 build manifest + SHA256。
3. alpha Compose、systemd unit/timer、启动参数和环境变量**键名/非敏感值**。
4. 两集群 `schema_migrations`、migration 版本、DB 尺寸/连接/锁/长事务/临时文件和关键队列表聚合计数。
5. CloudSite/IndexCore/OpenList 最近脱敏日志、重启计数和 OOM 事件。
6. 备份调度、最近成功记录、远端加密/ACL/保留、隔离恢复回执。
7. Traefik/Portainer 访问控制、MFA、Cloudflare 安全策略的脱敏摘要。

未取得这些材料前，本报告不会推断实际 schema、数据库健康、真实吞吐或生产长稳。