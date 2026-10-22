# 01 · 源码版本、镜像构建与校验

## 1. 源码版本（权威来源）

- 仓库：`https://github.com/nathanxiangang-web/cloudsite`
- 分支：`main`
- 提交：`c43408bb61722fb3d575063b8da79c04d736abdc`
- 标题：`feat(admin): implement real audited user actions and protected 115 account probe (#150)`
- 提交时间：2026-10-22 11:02:12 +0800
- 说明：该提交为 PR #150 的合并提交，已在 `main` 上；本次**未**修改任何业务代码或迁移文件。

## 2. 构建工具链与命令

- Go：`go1.27.1`（`go.mod` 声明 `go 1.27.1`）
- 目标：`linux/amd64`，`CGO_ENABLED=0`（静态链接）

```
git clone --depth 1 https://github.com/nathanxiangang-web/cloudsite
cd cloudsite && git rev-parse HEAD   # => c43408bb…

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o cloudsite ./cmd/panta
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o panta-db  ./cmd/panta-db
```

说明：
- `cmd/panta` → 产品二进制 `cloudsite`（含 `go:embed` 的前端资源 `internal/producthttp/web/*`）。
- `cmd/panta-db` → 迁移工具 `panta-db`（**不在** `Dockerfile.release` 中，故单独构建为一次性工具）。

## 3. 二进制校验（sha256）

| 产物 | sha256 |
| --- | --- |
| `cloudsite` | `6686cd9184543cc33feeb8792d3ab200953e5364a7fa62fe9cef7ec64866648b` |
| `panta-db` | `db43dc06031b8ea6119bc2caa173f96bbaff2ff061dd1d12ef51423dff048bea` |

本地构建与上传到生产后的 sha256 **双端一致**。

## 4. 镜像标识

- 镜像 tag：`cloudsite:ui3a-c43408bb`
- 镜像 ID：`sha256:caa56df2e44fdc65387f0980d931541c9af29e1ae52c0632599b699d80547cf0`
- OCI labels：
  - `org.opencontainers.image.version=cloudsite-ui3a`
  - `org.opencontainers.image.revision=c43408bb61722fb3d575063b8da79c04d736abdc`
  - `org.opencontainers.image.source=https://github.com/nathanxiangang-web/cloudsite`
- 镜像内 `/usr/local/bin/cloudsite` 的 sha256 = `6686cd91…`（与权威构建一致）。

## 5. 镜像构建方式（生产主机上）

复用现有 `Dockerfile.release` 与既有 catalog 运行文件（**未提交任何私有配置或二进制**）：

- build context 组成：`cloudsite`（本次新构建）、`cloudsite-index-reader`（既有）、`catalog-entrypoint.py`（既有）。
- `Dockerfile.release` 行为：`FROM debian:bookworm-slim`；安装 `ca-certificates python3 poppler-utils util-linux curl`；`COPY cloudsite cloudsite-index-reader /usr/local/bin/`；`COPY catalog-entrypoint.py /usr/local/lib/cloudsite/`；`RUN chmod 755`；`USER 10001:10001`；`ENTRYPOINT ["/usr/local/bin/cloudsite"]`；`CMD ["serve-product-readonly"]`。
- 构建命令：
```
docker build \
  --label org.opencontainers.image.version=cloudsite-ui3a \
  --label org.opencontainers.image.source=https://github.com/nathanxiangang-web/cloudsite \
  --label org.opencontainers.image.revision=c43408bb61722fb3d575063b8da79c04d736abdc \
  -t cloudsite:ui3a-c43408bb -f Dockerfile.release <build-context>
```

## 6. 运行模式说明（与 admin 功能的关系）

- 生产 `cloudsite` 服务当前以 `serve-product-acquisition`（代码常量 `CloudAction`）运行，启用 acquisition worker。
- 代码核对：`productserver.Run` 对 `serve-product-readonly` / `serve-product-acquisition` 两种 action **均装配** `producthttp.WithAdminOperations`，因此 UI-3A 的真实管理功能在该模式下可用。
- **启动约束**：`productserver.openDatabase` 在启动时要求 `schema_migrations` 已达 `CurrentSchemaVersion(=16)`，否则返回 `ErrStartup`。故部署顺序必须为「**先迁移、后切换新镜像**」。