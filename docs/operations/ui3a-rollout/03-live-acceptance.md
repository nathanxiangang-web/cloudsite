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

> **Round 1 修正**：原文档把通过判定写成 `status = AUTHENTICATED` 属**字段错误**。实际 `adminops.Result` 的**顶层** `status ∈ {SUCCEEDED, REJECTED, UNKNOWN}`；`AUTHENTICATED` 位于**嵌套** `capacity.status`。以下为精确契约。

### 2.1 请求契约

```
POST /api/v1/admin/storage/<binding-id>/probe
Headers:
  Cookie: <ADMIN 产品会话>          # 必需
  Origin: <配置的 origin>            # 变更类请求必需；缺失/不匹配 → 403 ORIGIN_REJECTED
  Accept: application/json            # 缺失 → 406 NOT_ACCEPTABLE
  Content-Type: application/json
body: {"request_id":"<uuid4>"}          # 严格白名单字段；多余/缺失 → 400 INVALID_REQUEST
```
结果查询：
```
GET /api/v1/admin/actions/<request_id>
Headers: Cookie: <ADMIN 产品会话>; Accept: application/json
```
（仅 ADMIN 角色可访问，否则 403 `ADMIN_REQUIRED`。）

### 2.2 响应判定（精确字段）

`adminops.Result`：

- 顶层：`request_id, action, target_id, status, code, recorded_at, capacity`
  - 顶层 `status ∈ {SUCCEEDED, REJECTED, UNKNOWN}`（`UNKNOWN` 仅表示「已占位」，**不是**可重试的成功）。
  - probe 成功时：顶层 `status = SUCCEEDED` 且 `code = PROBE_RECORDED`。
- 嵌套 `capacity`：`{status, total_bytes, used_bytes, remaining_bytes, source, probed_at}`
  - 认证成功时：`capacity.status = AUTHENTICATED`，`capacity.source = 115_GET_INFO`，`probed_at` 非零。

**通过判定（三者须同时满足）**：
1. 顶层 `status = SUCCEEDED` **且** `code = PROBE_RECORDED`；
2. 嵌套 `capacity.status = AUTHENTICATED` **且** `capacity.source = 115_GET_INFO` **且** `probed_at` 非零；
3. `total_bytes / used_bytes / remaining_bytes` 为**十进制字符串**（避免 >2^53 的 JS 精度丢失），且 `used_bytes + remaining_bytes == total_bytes`、`total > 0`、`used >= 0`、`remaining >= 0`。

**不得判定为成功**：顶层 `status ∈ {UNKNOWN, REJECTED}`，或嵌套 `capacity.status ∈ {NOT_AUTHENTICATED, NOT_CONNECTED, UNKNOWN}`（此时字节字段为空）。UNKNOWN / NOT_AUTHENTICATED **一律不得**视为 canary 成功。

- 约束：同一 Connection 的 probe 有 **60s 冷却**；同一 `request_id` **不得重放**（去重 + 结果复用）。
- 证据记录：仅记录分类状态、计数与时间戳；**禁止**记录 cookie/token/账号明文、`Origin` 以外的任何 secret。

## 3. 用户管理（创建 / 停用 / 启用）验收步骤与判定

### 3.1 请求契约

```
POST  /api/v1/admin/users                 # 变更类：需 Origin 头
Headers: Cookie: <ADMIN 会话>; Origin: <配置的 origin>; Accept: application/json; Content-Type: application/json
body: {"request_id":"<uuid4>","login":"<disposable>","password":"<temp>"}

PATCH /api/v1/admin/users/<user_id>       # 变更类：需 Origin 头
body: {"request_id":"<uuid4>","enabled":false,"expected_version":<n>}
```
（仅 ADMIN 角色；缺失 Origin → 403 `ORIGIN_REJECTED`；缺失 `Accept: application/json` → 406 `NOT_ACCEPTABLE`；严格字段白名单，多余/缺失字段 → 400 `INVALID_REQUEST`。）

### 3.2 响应判定（精确字段）

- 创建成功：顶层 `status = SUCCEEDED` **且** `code = USER_CREATED`（仅普通 USER，不可提权）。
- 停用/启用成功：顶层 `status = SUCCEEDED` **且** `code = USER_STATE_CHANGED`。
- 版本冲突：`code = VERSION_CONFLICT`（`expected_version` 必需，并发写入不静默覆盖）。
- 目标不在管理范围：`code = TARGET_FORBIDDEN`。
- **停用后**：该用户**既有产品会话被拒（HTTP 401）**；ADMIN 与其他用户不受影响。
- 审计：`GET /api/v1/admin/actions/<request_id>` 返回 `request_id / action / status / code / recorded_at`（user 操作含 `user`），满足可审计与幂等（同 `request_id` 不重复执行）。
- 约束：仅使用**经批准的一次性测试账号**（可保留为受保护的 release test 账号，**不得**删除或改写他人账号）。

## 4. 未执行声明

| 项目 | 状态 |
| --- | --- |
| 真实 115 `GetInfo` 联机探针 | ⛔ NOT EXECUTED |
| 真实 USER 创建/停用/启用 | ⛔ NOT EXECUTED |
| 生产 schema16 | ⛔ 未达成（仍为 v15） |
| 生产新镜像 | ⛔ 未切换（仍为 `cloudsite:deleted-preview-bf89f5a`） |

上述项目依赖对生产主机的不可逆变更，本轮未获执行（见 `05-findings-and-blockers.md`）。