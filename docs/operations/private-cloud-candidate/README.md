# 公网候选包 — CloudSite 原地升级与回滚运维手册

> 本手册描述**操作者在生产主机上手动执行**的分阶段流程。当前阶段（候选包构建）**不远程执行**任何备份、迁移、容器切换或回滚；本文件只是可执行前的操作说明。
>
> 所有命令中的 `<...>` 均为占位符，由操作者在现场填写。**禁止**把真实 DSN、数据库口令、Cookie、会话令牌、主机私钥写入本文件、提交记录、工单或聊天记录。受保护文件只以只读挂载/路径引用方式使用，不回显内容。

---

## 0. 范围与边界

### 0.1 本手册负责什么

- 从**已发布的候选包**（`cloudsite_<VERSION>_private_candidate_<SHORT_SHA>_linux_amd64`）到**仅替换 CloudSite 产品服务**的原地升级；
- 升级前的**权威备份**、**隔离 PG16 恢复验证**与 **schema 21→22 迁移演练**；
- 升级失败后的**完整恢复回滚**。

### 0.2 本手册不负责什么

| 对象 | 归属 | 本手册的动作 |
| --- | --- | --- |
| `catalog` / `catalog-cloud`（IndexCore 内核与 query 服务） | IndexCore | **不重建、不重启、不替换镜像** |
| `catalog-db`（PostgreSQL 18，IndexCore catalog 库） | IndexCore | **不备份、不迁移、不触碰** |
| OpenList、Traefik、Portainer、备份服务 | 现有基础设施 | **不重启、不改配置** |
| IndexCore Canonical / Journal / 扫描 | IndexCore 物理真相 | **不写入、不修补、不触发扫描** |
| 115 / provider 账号、Cookie | 受保护秘密源 | **不读取、不注入、不记录** |

CloudSite 只读 IndexCore 的 Canonical 视图；本次升级**不改变** IndexCore 的 root、binding、mount 或扫描行为。若任何一步需要改动上表对象，**停止并回到授权方**，不要在本流程内自行扩展。

### 0.3 三个必须遵守的门槛

1. **候选包 ≠ 生产准入。** 构建成功、校验和正确、镜像可加载，只证明“产物可复现且完整”，不证明其已通过生产验收。生产切换必须由独立的授权确认（见 §2.1）单独触发。
2. **备份必须在 writer 冻结之后取。** 停机前取的 `pg_dump` 是非一致快照，只能用于预演，不能作为权威回滚依据。
3. **旧镜像不得直接运行在 schema22 上。** schema 升级后如需回滚，必须“恢复匹配备份 + 切回旧镜像”成对执行，禁止 down-migration，禁止让旧 `cloudsite` 镜像连到 21→22 已迁移的库。

---

## 1. 变量约定（现场填写，勿提交）

```bash
ROOT=/opt/my-servers/cloudsiteV1.0.0alpha          # 生产项目目录（现有 compose 所在目录）
COMPOSE=$ROOT/cloudsiteV1.0.0alpha.yml             # 现有主 compose 文件（不改写）
ENVV=<受保护的 env 文件路径>                        # 含 CLOUDSITE_TRAEFIK_ENABLED 等既有配置
OVERRIDE=$ROOT/product-upgrade.override.yml        # 由候选包提供的 product-only override
DBUSER=<产品库用户>; DBNAME=<产品库名>              # 例如 cloudsite / panta_mvp_preview
PDB=cloudsite-alpha-product-db                     # 产品库容器（PostgreSQL 16）
PRODUCT=cloudsite-alpha-product                    # 产品容器
PRODUCT_SVC=cloudsite                              # compose 服务名（仅此服务被替换）
PKG=<解包后的候选包目录，如 /home/nathan/桌面/cloudsite-release-candidate/cloudsite_1.0.3_private_candidate_<SHORT_SHA>_linux_amd64>
NEWIMG=cloudsite:private-candidate-<SHORT_SHA>
OLDIMG=<切换前镜像>                                 # 由 §3.1 的 inspect 记录取得，不要凭记忆填写
BK=$ROOT/private/private-candidate/backup-$(date -u +%Y%m%dT%H%M%SZ)
```

候选包目录可以放在仓库之外的任意位置；**不要**把候选包解包进 Git 工作树。

---

## 2. 阶段 P0 · 准备与准入

### 2.1 确认授权

- [ ] 已获得**独立的生产变更确认**，其中写明：源 commit、`VERSION`、计划窗口、回滚负责人。
- [ ] 已确认本次只涉及 `cloudsite` 产品服务；catalog/IndexCore/OpenList/Traefik 不在变更范围。
- [ ] 已确认当前生产 schema 版本为 **21**（见 §2.3），目标为 **22**。

缺少任一项则**不进入 P1**。

### 2.2 校验候选包完整性

在**构建机或运维机**（非生产主机）上：

```bash
cd <候选包父目录>
sha256sum -c SHA256SUMS                       # 顶层：cloudsite_<VERSION>_private_candidate_<SHORT_SHA>_linux_amd64.tar.gz 与 SHA256SUMS
tar -tzf cloudsite_<VERSION>_private_candidate_<SHORT_SHA>_linux_amd64.tar.gz | head   # 顶层只有该包目录（即 $PKG），无额外路径
tar -xzf cloudsite_<VERSION>_private_candidate_<SHORT_SHA>_linux_amd64.tar.gz
PKG=./cloudsite_<VERSION>_private_candidate_<SHORT_SHA>_linux_amd64
cd "$PKG"
sha256sum -c BINARY-SHA256SUMS                # 必须逐项 OK
cat VERSION; cat BUILD-INFO.txt               # 记录 VERSION / commit / 分支 / 生成时间
```

必需文件清单（缺一即判**不合格**，退回构建方重建）：

| 文件 | 用途 |
| --- | --- |
| `cloudsite` | 产品二进制（Linux amd64） |
| `panta-db` | 迁移工具（只在隔离库与迁移窗口使用） |
| `VERSION` | 版本号，须与授权记录一致 |
| `SOURCE-MANIFEST.json` | 记录仓库、分支、精确 commit、VERSION |
| `BUILD-INFO.txt` | 构建时间与环境 |
| `BINARY-SHA256SUMS` | 二进制校验和 |
| `cloudsite-private-candidate-image.tar.gz` | 产品镜像（显式 `SKIP_DOCKER=1` 时缺席） |
| `product-upgrade.override.yml` | 仅覆盖 `services.cloudsite.image` 的 product-only compose override（C-PRODUCT-OVERRIDE@1）；缺席时**不得**进入 P4 切换 |

同时核对：

- [ ] `SOURCE-MANIFEST.json` 中的 commit 等于授权记录中的 commit，且 `git status --porcelain --untracked-files=no` 在构建时为空（构建脚本已强制）。
- [ ] 包内**不含** IndexCore 源码/二进制（`cloudsite-index-reader`、`catalog-entrypoint.py` 等）、私有配置、凭据、日志。
- [ ] 若镜像 tar 缺失（`SKIP_DOCKER=1`），本次**不得**用于生产切换，只能用于二进制与手册验证。
- [ ] `product-upgrade.override.yml` 存在，且 `grep -c 'image' $PKG/product-upgrade.override.yml` 只命中产品服务镜像；该文件是 P4 切换的唯一镜像入口。

导入镜像（仅在需要镜像的阶段）：

```bash
docker load -i "$PKG/cloudsite-private-candidate-image.tar.gz"
docker image inspect "$NEWIMG" --format '{{.Config.Image}} {{index .Config.Labels "org.opencontainers.image.revision"}}'
docker image inspect "$NEWIMG" --format '{{range .Config.Cmd}}{{.}} {{end}}'   # 确认只含产品入口
```

### 2.3 记录生产当前状态（只读）

```bash
docker inspect "$PRODUCT" > /tmp/product-pre.json
docker inspect "$PRODUCT" --format 'image={{.Config.Image}} running={{.State.Running}}'
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select 'head='||max(version)||' rows='||count(*) from schema_migrations"   # 期望 head=21
docker compose --env-file "$ENVV" -f "$COMPOSE" ps
```

把 `image=` 的结果记入 `OLDIMG`。**不要**在这里执行任何迁移、重启或 `up`。

---

## 3. 阶段 P1 · 冻结 writer + 权威备份

### 3.1 冻结产品写入

只停产品服务（含其内的 acquisition worker），**catalog/catalog-cloud 保持运行**：

```bash
docker compose --env-file "$ENVV" -f "$COMPOSE" stop "$PRODUCT_SVC"
docker inspect "$PRODUCT" --format 'running={{.State.Running}}'    # 期望 false
```

确认没有其他 writer 连接产品库（若存在独立的一次性管理进程，先让其结束）。IndexCore 向 CloudSite 的读取不受影响。

### 3.2 生成权威备份（一次性目录，权限 0700）

```bash
umask 077; mkdir -p "$BK"
docker exec "$PDB" pg_dump -U "$DBUSER" -d "$DBNAME" -Fc > "$BK/product-db.dump"
cp <受保护的 product config>        "$BK/product-config.json"
cp <受保护的数据库 DSN 文件>         "$BK/product-database.url"
cp <受保护的 acquisition bootstrap> "$BK/acquisition-bootstrap.json"
cp "$ENVV" "$BK/formal.env"
cp "$COMPOSE" "$BK/compose.orig.yml"
docker inspect "$PRODUCT" > "$BK/product-container-pre.json"
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select max(version) from schema_migrations" | tee "$BK/head-before.txt"     # 期望 21
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select relname||'='||n_live_tup from pg_stat_user_tables order by relname" > "$BK/rowcounts-before.txt"
sha256sum "$BK"/* | tee "$BK/SHA256SUMS"
```

**验收**：`head-before.txt` = 21；`product-db.dump` 非空且 `sha256sum` 已记录；`product-container-pre.json` 中的镜像即 `OLDIMG`。

> `$BK` 中的 DSN/env 文件是**秘密副本**：仅留在受保护路径，禁止复制到仓库、聊天或工单；流程结束后按组织策略处置。

---

## 4. 阶段 P2 · 隔离 PG16 恢复与 schema 21→22 迁移演练

**任何生产迁移之前，本节必须在隔离环境全部通过。** 隔离环境不得与生产数据网络互通。

### 4.1 起隔离 PG16 并恢复备份

```bash
docker run -d --name pc-verify-pg --network none \
  -e POSTGRES_USER=<临时用户> -e POSTGRES_PASSWORD=<临时口令> \
  -e POSTGRES_DB=panta_restore_check \
  postgres:16-alpine
until docker exec pc-verify-pg pg_isready -U <临时用户> -d panta_restore_check; do sleep 1; done
docker exec -i pc-verify-pg pg_restore -U <临时用户> -d panta_restore_check \
  --no-owner --no-privileges --exit-on-error < "$BK/product-db.dump"
```

> 只恢复到**全新空库**。不要对生产库执行 in-place `pg_restore --clean`：它不会删除 schema22 之后才可能存在的对象，且可能因依赖顺序中途失败，把唯一的生产库留在不可用状态。

### 4.2 校验恢复副本

```bash
docker exec pc-verify-pg psql -U <临时用户> -d panta_restore_check -At \
  -c "select 'head='||max(version)||' rows='||count(*) from schema_migrations"       # 期望 head=21
docker exec pc-verify-pg psql -U <临时用户> -d panta_restore_check -At \
  -c "select relname||'='||n_live_tup from pg_stat_user_tables order by relname" > /tmp/rowcounts-restored.txt
diff "$BK/rowcounts-before.txt" /tmp/rowcounts-restored.txt && echo ROWCOUNTS_IDENTICAL
```

`n_live_tup` 是估算值；出现差异时对相关表补 `select count(*)` 精确比对，并记录差异原因。**行数或关键历史数据不一致即判恢复失败**，回到 P1 重取备份。

### 4.3 在隔离库执行 21→22 迁移

把 `panta-db` 与**临时 DSN 文件**放进隔离容器后调用。DSN 只含本次临时口令（非生产秘密），以文件方式注入，**不经过命令行、shell 历史或日志回显**：

```bash
docker cp "$PKG/panta-db" pc-verify-pg:/usr/local/bin/panta-db
docker exec pc-verify-pg chmod 0755 /usr/local/bin/panta-db

# 用编辑器在受保护目录写入临时 DSN 文件（umask 077 后创建），内容形如：
#   postgres://<临时用户>:<临时口令>@127.0.0.1:5432/panta_restore_check?sslmode=disable
umask 077; mkdir -p /tmp/pc-verify; VERIFYDSN=/tmp/pc-verify/verify-dsn
# （此处由操作者填入文件内容；不要用 sed/echo 把口令写进命令历史）
chmod 0600 "$VERIFYDSN"
docker cp "$VERIFYDSN" pc-verify-pg:/run/verify-dsn
docker exec pc-verify-pg chmod 0600 /run/verify-dsn

docker exec pc-verify-pg sh -c 'PANTA_DATABASE_URL="$(cat /run/verify-dsn)" /usr/local/bin/panta-db status'
# 期望：compatible=false current_version=21 latest_version=22 pending=[22]
docker exec pc-verify-pg sh -c 'PANTA_DATABASE_URL="$(cat /run/verify-dsn)" /usr/local/bin/panta-db migrate'
docker exec pc-verify-pg sh -c 'PANTA_DATABASE_URL="$(cat /run/verify-dsn)" /usr/local/bin/panta-db status'
# 期望：compatible=true current_version=22 latest_version=22 pending=[]
```

> `PANTA_DATABASE_URL` 由容器内 `cat` 读取文件赋值；**不要**用 `-e PANTA_DATABASE_URL=<明文>` 传参，也不要让 `panta-db` 输出连接串。

迁移后校验：

```bash
docker exec pc-verify-pg psql -U <临时用户> -d panta_restore_check -At \
  -c "select 'head='||max(version)||' rows='||count(*) from schema_migrations"       # 期望 head=22 rows=22
docker exec pc-verify-pg psql -U <临时用户> -d panta_restore_check -At \
  -c "select count(*) from pg_class where relname in ('cloud_user_spaces','acquisition_result_object_evidence','acquisition_copy_operations')"   # 期望 3
docker exec pc-verify-pg psql -U <临时用户> -d panta_restore_check -At \
  -c "select count(*) from information_schema.columns where table_name='acquisition_manifests'"  # 与迁移前对比，仅新增列
```

迁移后**再次**比对关键表行数：既有表（`product_users`、`product_sessions`、`copies`、`product_file_shares`、`jobs`、`acquisition_manifests`、`acquisition_provider_tasks` 等）行数必须与 `rowcounts-before.txt` 一致；历史 reservations 与历史任务记录不得减少。

### 4.4 新镜像在 head22 上的隔离启动（需单独授权的保护配置步骤）

> 产品容器启动依赖受保护产品配置（`PANTA_PRODUCT_CONFIG_FILE=/run/cloudsite/config.json`，只读挂载）与指向**隔离库**的 DSN。本步骤**不得**复用生产配置直连生产库，也**不得**把秘密写进命令行。
>
> 因此本步骤**需要单独的启动授权**。未获授权时记 `NOT_RUN`，**禁止**据此宣称新镜像可启动或可升级。

获授权后，在受保护目录准备隔离启动配置（从 `$BK` 的产品配置派生，仅改数据库指向）：

```bash
umask 077; VERIFYCFG=/tmp/pc-verify/product-run; mkdir -p "$VERIFYCFG/cache"
cp "$BK/product-config.json" "$VERIFYCFG/config.json"
# 用编辑器修改 $VERIFYCFG/config.json：
#   - 数据库连接指向隔离库（或改用 $VERIFYCFG/product-database.url 中的临时 DSN）
#   - catalog/IndexCore 查询地址替换为隔离占位值；隔离环境不提供 catalog，相关依赖必须关闭
chmod 0600 "$VERIFYCFG/config.json"
```

启动（只读根文件系统、无网络，与生产安全约束一致）：

```bash
docker run -d --name pc-verify-product --network none \
  --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --tmpfs /tmp:size=64m,mode=1777 \
  -e PANTA_PRODUCT_CONFIG_FILE=/run/cloudsite/config.json \
  -e PANTA_ACQUISITION_WORKER_ENABLED=false \
  -v "$VERIFYCFG:/run/cloudsite:ro" \
  -v "$VERIFYCFG/cache:/var/cache/cloudsite" \
  "$NEWIMG"
docker inspect pc-verify-product --format 'running={{.State.Running}} status={{.State.Status}} exit={{.State.ExitCode}}'
docker logs pc-verify-product 2>&1 | tail -20    # 判定：无配置/schema/DSN 错误，容器未陷入重启循环
docker rm -f pc-verify-product
```

判定标准：容器保持 `running`（或按产品设计正常退出且退出码 0），日志不含配置解析错误、schema 不兼容或 DSN 连接错误。若配置要求 catalog 可达而隔离环境无法满足，必须显式记录该依赖缺口，**不得**用“镜像能拉到本地”替代启动验证。

不要求访问外部 provider；**不要**在隔离启动中启用 acquisition worker 或任何写库路径。

### 4.5 结论与清理

- [ ] 恢复副本 `head=21` 且行数一致；
- [ ] `panta-db status` 从 `pending=[22]` 变为 `current_version=22`；
- [ ] 迁移后既有表行数不变、0022 新增对象存在；
- [ ] §4.4 的隔离启动在**获得单独授权后**完成并留证（未授权时记 `NOT_RUN`，不得视为通过）。

四项全通过才可进入生产迁移；任一项失败则**停止升级**，清理隔离资源并保留证据。

```bash
docker rm -f pc-verify-pg pc-verify-product 2>/dev/null
rm -f /tmp/pc-verify/verify-dsn       # 临时 DSN
```

---

## 5. 阶段 P3 · 生产 schema 21→22

> 仅在 §4 全部通过、且 P1 的 `$BK` 已通过恢复验证（§4.2）后执行。生产库迁移是**单向门**：从这一刻起，旧 schema21 镜像不能再直连该库。

`cloudsite` 保持停止（P1 已冻结），用包内 `panta-db` 对生产库执行迁移：

```bash
docker run --rm --network <compose 数据网络> \
  -v "$PKG/panta-db:/usr/local/bin/panta-db:ro" \
  -v "<受保护数据库 DSN 文件>:/run/dsn:ro" \
  postgres:16-alpine sh -c 'PANTA_DATABASE_URL="$(cat /run/dsn)" /usr/local/bin/panta-db migrate'
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select 'head='||max(version)||' rows='||count(*) from schema_migrations"     # 期望 head=22 rows=22
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select count(*) from pg_class where relname in ('cloud_user_spaces','acquisition_result_object_evidence','acquisition_copy_operations')"  # 期望 3
```

迁移失败或校验不符：**不要**尝试去重跑或手改 `schema_migrations`，直接进入 §7 回滚。

---

## 6. 阶段 P4 · 产品-only 切换与验证

### 6.1 部署 product-only override

override 只覆盖 `services.cloudsite.image`，不影响其他服务：

```bash
cp "$PKG/product-upgrade.override.yml" "$OVERRIDE"
grep -n 'image' "$OVERRIDE"      # 期望只出现 cloudsite.image: ${CLOUDSITE_CANDIDATE_IMAGE:?}
```

### 6.2 只替换 cloudsite

```bash
export CLOUDSITE_CANDIDATE_IMAGE="$NEWIMG"
docker compose --env-file "$ENVV" -f "$COMPOSE" -f "$OVERRIDE" \
  up -d --no-deps --force-recreate "$PRODUCT_SVC"
```

约束：

- 必须带 `--no-deps`，且只列 `cloudsite` 服务 —— **不得**重建、重启 catalog/catalog-cloud/catalog-db/edge 或 IndexCore 相关容器；
- `--force-recreate` 只作用于产品服务；
- 不使用 `down`、`--remove-orphans` 或全量 `up -d`。

### 6.3 切换后验证

```bash
docker inspect "$PRODUCT" --format 'image={{.Config.Image}} running={{.State.Running}}'
docker compose --env-file "$ENVV" -f "$COMPOSE" -f "$OVERRIDE" ps
curl -fsS -o /dev/null -w '%{http_code}\n' -H 'Host: <生产域名>' https://<生产域名>/
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select max(version) from schema_migrations"          # 22
docker exec "$PDB" psql -U "$DBUSER" -d "$DBNAME" -At \
  -c "select count(*) from pg_stat_user_tables" | tee /tmp/tables-after.txt
```

- [ ] `image` 为 `$NEWIMG`，容器 `running=true`，无重启循环（`docker inspect ... {{.RestartCount}}`）；
- [ ] `ps` 中 catalog/catalog-cloud/catalog-db/edge 的 uptime 未被重置；
- [ ] 首页与登录/资源读取路径返回预期状态码；
- [ ] 既有表行数与 `rowcounts-before.txt` 相比只增不减（除本窗口内的正常业务写入）；
- [ ] 日志不含 schema 兼容性错误、拒绝迁移或启动失败。

任何一项不满足：停止继续操作，进入 §7。

---

## 7. 阶段 P6 · 回滚（完整恢复）

### 7.1 触发条件

迁移失败、切换后无法启动、schema/数据校验不符、健康检查持续失败、或关键业务路径回归。

### 7.2 原则

- **成对回滚**：恢复**匹配的 schema21 备份** + 切回 `$OLDIMG`。禁止 down-migration，禁止旧镜像直连 schema22 库。
- **丢数据声明**：恢复到 schema21 备份会丢失该备份之后的所有写入。因此回滚前必须**先为当前（可能已损坏/已迁移）状态再取一份“故障现场”备份**，并保留 `$BK`；是否回滚到哪个时间点属于**单独的数据恢复决策**，不得盲目在线恢复。
- **不碰非产品服务**：catalog、catalog-cloud、catalog-db、OpenList、Traefik 全程不动。

### 7.3 步骤

1. **再次冻结 writer**（若服务仍在运行）：
   ```bash
   docker compose --env-file "$ENVV" -f "$COMPOSE" -f "$OVERRIDE" stop "$PRODUCT_SVC"
   ```
2. **保存故障现场**（不可跳过）：
   ```bash
   umask 077; mkdir -p "$BK-failed"
   docker exec "$PDB" pg_dump -U "$DBUSER" -d "$DBNAME" -Fc > "$BK-failed/product-db.failed.dump"
   docker logs "$PRODUCT" > "$BK-failed/product.log" 2>&1
   docker inspect "$PRODUCT" > "$BK-failed/product-container-failed.json"
   sha256sum "$BK-failed"/* | tee "$BK-failed/SHA256SUMS"
   ```
3. **把匹配备份恢复到全新库**（不覆盖现有库，保留 owner/grants）：
   ```bash
   docker exec "$PDB" psql -U "$DBUSER" -d postgres -c 'CREATE DATABASE <恢复库名> OWNER <DBUSER>'
   docker exec -i "$PDB" pg_restore -U "$DBUSER" -d <恢复库名> \
     --no-owner --no-privileges --exit-on-error < "$BK/product-db.dump"
   ```
4. **验证恢复库**：`head=21`、`schema_migrations` 历史与 checksum 精确匹配、0022 新增对象（`cloud_user_spaces`、`acquisition_result_object_evidence`、`acquisition_copy_operations`）与 0022 新增列**均不存在**、关键表行数与 `rowcounts-before.txt` 一致。
5. **切回连接与旧镜像**：把产品库 DSN 指向恢复库（经已测试、经批准的连接切换方式），移除 `$OVERRIDE` 的效果并恢复到 `$OLDIMG`：
   ```bash
   unset CLOUDSITE_CANDIDATE_IMAGE
   docker compose --env-file "$ENVV" -f "$COMPOSE" \
     up -d --no-deps --force-recreate "$PRODUCT_SVC"
   docker inspect "$PRODUCT" --format 'image={{.Config.Image}} running={{.State.Running}}'   # 期望 $OLDIMG
   ```
   配置文件/Compose 如已改动，以 `$BK/formal.env`、`$BK/compose.orig.yml` 恢复原值后再启动。
6. **验证回滚**：首页与登录路径返回预期状态码；`schema_migrations` = 21；既有业务数据可读；日志无 schema 不兼容错误。
7. **留档**：记录 `$BK`、`$BK-failed`、迁移与切换输出、验证结果，恢复库在确认前**不要删除**。

---

## 8. 证据与记录

每个阶段完成后记录（脱敏）：

| 项 | 内容 |
| --- | --- |
| 候选包 | 包名、`SHA256SUMS` 校验结果、`SOURCE-MANIFEST.json` 的 commit、`VERSION` |
| 备份 | `$BK` 路径、`SHA256SUMS` 摘要、`head-before.txt`、行数文件摘要 |
| 隔离演练 | 恢复后 `head`/行数一致性、`panta-db status` 前后值、迁移后新增对象计数、新镜像启动结果 |
| 生产迁移 | `panta-db migrate` 输出摘要、迁移后 `head`/行数 |
| 切换 | `$OLDIMG` / `$NEWIMG`、切换时间、`ps` 与健康检查结果 |
| 回滚（如发生） | 触发原因、`$BK-failed` 摘要、恢复库名、切回镜像、验证结果 |

证据中**不得**包含 DSN、口令、Cookie、令牌或主机密钥。

---

## 9. 禁止事项（硬约束）

- ❌ 让 schema21 旧镜像运行在 schema22 库上；或对生产库做 down-migration、手改 `schema_migrations`。
- ❌ 对现有生产库执行 in-place `pg_restore --clean`。
- ❌ 用生产产品配置或生产库 DSN 做新镜像的隔离启动；或在未获单独授权时把 §4.4 记为通过。
- ❌ 用 `-e PANTA_DATABASE_URL=...` 明文传参、`sed`/`echo` 写口令，或在日志/终端回显任何 DSN、口令、Cookie。
- ❌ 重建、重启或替换 `catalog`、`catalog-cloud`、`catalog-db`、OpenList、Traefik、IndexCore 内核。
- ❌ 在 writer 未冻结时把备份当作权威回滚依据。
- ❌ 把候选包或构建成功当作生产准入；或在未获得独立变更确认时切换。
- ❌ 以任何形式触碰 IndexCore Canonical/Journal、触发扫描或修补 catalog 数据。
- ❌ 把秘密写入仓库、日志、工单或聊天；或从候选包以外的来源替换 `cloudsite` 二进制/镜像。
- ❌ 使用全量 `docker compose up -d`、`down`、`--remove-orphans` 执行本次变更。