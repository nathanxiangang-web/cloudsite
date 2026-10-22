# 03 · 真实联机验收条件（115 账号检测 与 用户管理）

> **状态：NOT EXECUTED。** 本节仅定义**联机**验收的前置条件与判定标准，供获批后执行。
> 所有步骤均不得记录或提交任何 secret、Cookie、Token、账号身份或受保护配置内容。

## 1. 前置条件（同时满足才可执行）

1. 生产库已显式迁移至 schema16（`schema_migrations` head=16/rows=16；三张新表存在）。
2. 生产 `cloudsite` 服务已运行 `cloudsite:ui3a-c43408bb`（revision `c43408bb…`），健康检查通过。
3. 具备一个**经批准的 ADMIN 产品账号**（login/口令由操作者持有，不落盘），可登录获得产品会话。
4. 存在一个 **ACTIVE 且 provider_type=115** 的 Binding + 受保护 session binding：
   - 产品配置 `cloud_download.bootstrap_file` 指向受保护 bootstrap（`<credential-ref>` 形式的会话绑定）；
   - probe 复用它（`admin_probe_bootstrap_file` 缺省时代理该文件）。
5. 监管路由开放：`POST /api/v1/admin/storage/{binding_id}/probe`、`GET /api/v1/admin/actions/{request_id}`。

## 2. 115 账号检测（GetInfo）验收步骤与判定

- 以 ADMIN 会话调用：
```
POST /api/v1/admin/storage/<binding-id>/probe
body: {"request_id":"<uuid4>"}
```
- 结果查询：
```
GET /api/v1/admin/actions/<request_id>
```
- **通过判定**：
  - `status = AUTHENTICATED`，来源为 115 `GetInfo`；
  - `total_bytes / used_bytes / remaining_bytes` 为**十进制字符串**（避免 >2^53 的 JS 精度丢失），且 `used_bytes + remaining_bytes == total_bytes`；
  - 审计记录可查，且**不含** secret/账号细节。
- 约束：同一 Connection 的 probe 有 **60s 冷却**；同一 `request_id` **不得重放**（去重 + 结果复用）。
- 证据记录：仅记录分类状态、计数与时间戳；**禁止**记录 cookie/token/账号明文。

## 3. 用户管理（创建 / 停用 / 启用）验收步骤与判定

```
# 创建普通 USER
POST /api/v1/admin/users
body: {"request_id":"<uuid4>","login":"<disposable>","password":"<temp>"}

# 停用（撤销现有会话）
PATCH /api/v1/admin/users/<user_id>
body: {"request_id":"<uuid4>","enabled":false,"expected_version":<n>}
```
- **通过判定**：
  - 创建返回 `USER_CREATED`（仅普通 USER，不可提权）；
  - 停用后该用户**既有产品会话被拒（HTTP 401）**；ADMIN 与其他用户不受影响；
  - 启用/停用具备版本校验（`admin_version`），并发写入不会静默覆盖；
  - `GET /api/v1/admin/actions/<request_id>` 可查 `request_id / intent / result`，满足可审计与幂等（同 request_id 不重复执行）。
- 约束：仅使用**经批准的一次性测试账号**（可保留为受保护的 release test 账号，**不得**删除或改写他人账号）。

## 4. 未执行声明

| 项目 | 状态 |
| --- | --- |
| 真实 115 `GetInfo` 联机探针 | ⛔ NOT EXECUTED |
| 真实 USER 创建/停用/启用 | ⛔ NOT EXECUTED |
| 生产 schema16 | ⛔ 未达成（仍为 v15） |
| 生产新镜像 | ⛔ 未切换（仍为 `cloudsite:deleted-preview-bf89f5a`） |

上述项目依赖对生产主机的不可逆变更，本轮未获执行（见 `05-findings-and-blockers.md`）。