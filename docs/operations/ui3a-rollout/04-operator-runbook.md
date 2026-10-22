# 04 · Operator Runbook（剩余生产步骤，脱敏）

> 仅当获得生产变更授权后执行。命令中的 `<...>` 为占位符，需由操作者按现场填入；**任何口令/DSN 均从受保护文件读取，不得回显或记录**。

变量约定：
```
COMPOSE=/opt/my-servers/cloudsiteV1.0.0alpha.yml          # 项目 cloudsite-v100-alpha
ENVV=<受保护的 formal.env 路径>                            # 含 CLOUDSITE_TRAEFIK_ENABLED=true
ROOT=/opt/my-servers/cloudsiteV1.0.0alpha
NEWIMG=cloudsite:ui3a-c43408bb
OLDIMG=cloudsite:deleted-preview-bf89f5a
PANTA_DB=<panta-db 一次性工具路径>
```

## 0) 清理验证遗留容器（可选）
```
docker rm -f <隔离验证 PG 容器> <新镜像冒烟容器>
```

## 1) 冻结 writer（仅 product；catalog/catalog-cloud 保持）
```
cd /opt/my-servers
docker compose --env-file "$ENVV" -f "$COMPOSE" stop cloudsite
```

## 2) 权威备份（含历史 reservations）
```
TS=$(date -u +%Y%m%dT%H%M%SZ); BK=$ROOT/private/ui3a-rollout/backup-$TS; umask 077; mkdir -p "$BK"
docker exec <product-db> pg_dump -U <db-user> -d <db-name> -Fc > "$BK/product-db.dump"
cp <受保护 product config>       "$BK/product-config.json"
cp <受保护数据库 DSN 文件>        "$BK/product-database.url"
cp <受保护 acquisition bootstrap> "$BK/acquisition-bootstrap.json"
cp "$ENVV" "$BK/formal.env"; cp "$COMPOSE" "$BK/compose.orig.yml"
docker inspect <product-container> > "$BK/product-container-pre.json"
sha256sum "$BK"/* | tee "$BK/SHA256SUMS"
docker exec <product-db> psql -U <db-user> -d <db-name> -At -c "select max(version) from schema_migrations" | tee "$BK/head-before.txt"   # 期望 15
```

## 3) 隔离恢复验证
```
docker run -d --name ui3a-verify-pg -e POSTGRES_USER=<u> -e POSTGRES_PASSWORD=<临时> \
  -e POSTGRES_DB=panta_restore_check -p 127.0.0.1:55460:5432 postgres:16-alpine
until docker exec ui3a-verify-pg pg_isready -U <u> -d panta_restore_check; do sleep 1; done
docker exec -i ui3a-verify-pg pg_restore -U <u> -d panta_restore_check --no-owner --no-privileges --exit-on-error < "$BK/product-db.dump"
docker exec ui3a-verify-pg psql -U <u> -d panta_restore_check -At -c "select max(version) from schema_migrations"   # 期望 15
docker rm -f ui3a-verify-pg
```

## 4) 生产迁移 schema15→16（migrator 事务 + advisory lock）
```
docker run --rm --network <compose 数据网络> \
  -v "$PANTA_DB:/usr/local/bin/panta-db:ro" \
  -v "<受保护数据库 DSN 文件>:/run/dsn:ro" \
  postgres:16-alpine sh -c 'PANTA_DATABASE_URL="$(cat /run/dsn)" /usr/local/bin/panta-db migrate'
docker exec <product-db> psql -U <db-user> -d <db-name> -At \
  -c "select 'head='||max(version)||' rows='||count(*) from schema_migrations"     # 期望 head=16 rows=16
docker exec <product-db> psql -U <db-user> -d <db-name> -At \
  -c "select count(*) from pg_class where relname in ('product_admin_actions','product_admin_action_results','product_admin_probe_cooldowns')"  # 3
```

## 5) 切换镜像
```
cp "$COMPOSE" "$COMPOSE.bak-$(date -u +%Y%m%dT%H%M%SZ)"
# 将 cloudsite 服务 image 由 $OLDIMG 改为 $NEWIMG（编辑 compose 或使用 override 文件）
docker compose --env-file "$ENVV" -f "$COMPOSE" up -d --no-deps --force-recreate cloudsite
docker inspect <product-container> --format 'image={{.Config.Image}} rev={{index .Config.Labels "org.opencontainers.image.revision"}} running={{.State.Running}}'
```

## 6) 健康 + 路由
```
docker compose --env-file "$ENVV" -f "$COMPOSE" ps
curl -fsS -o /dev/null -w '%{http_code}\n' -H 'Host: cloud.netioi.com' https://cloud.netioi.com/
```

## 7) 真实联机验收
按 `03-live-acceptance.md` 执行 115 探针与 USER 启停；证据仅记录分类状态与计数。

## 8) 回滚（任一步失败）
```
docker compose --env-file "$ENVV" -f "$COMPOSE" stop cloudsite
# 恢复 schema15 备份（回滚会丢失迁移后的写入）
docker exec -i <product-db> pg_restore -U <db-user> -d <db-name> --clean --if-exists --no-owner --no-privileges --exit-on-error < "$BK/product-db.dump"
# compose 还原 image: $OLDIMG
docker compose --env-file "$ENVV" -f "$COMPOSE" up -d --no-deps --force-recreate cloudsite
```
禁止：down-migrator；让旧镜像运行在 schema16 上。