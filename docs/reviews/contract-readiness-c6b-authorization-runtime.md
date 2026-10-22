# C6b Authorization Runtime Contract Readiness

## 结论

本审计在当前代码基线 `7f63e6a` 上完成。使用真实 HTTP、真实 JWT 会话、独占临时 MySQL schema、独占 Redis database 和真实编译服务执行 W1-W4。未修改产品代码、主规格或永久测试。

| 风险域 | 结论 | 依据 |
| --- | --- | --- |
| W1 Token／授权版本 | 通过 | `c6b-auth-runtime-token.json`；旧会话、缺失身份、禁用／软删、版本存储故障和首次登录初始化均有 HTTP 与存储事实 |
| W2 授权变化／no-op／CAS／回滚 | 通过 | `c6b-auth-runtime-mutations.json`；逐人、角色级、保护对象、过期 no-op 和受控故障均核对关系与版本 |
| W3 数据范围／投影 | 通过 | `c6b-auth-runtime-scope.json`；all、self、org、org_and_children、custom、不可管理祖先、隐藏组织关系和菜单投影均分别观察 |
| W4 确定性并发 | 通过 | `c6b-auth-runtime-scope.json` 的 C01-C04；锁等待、首次版本初始化、跨维度竞争和重试结果已记录 |

未发现需要在本 change 内修复的明确产品偏差；未创建后续修复 change。审计结论不等同于全端点安全认证，也不覆盖本 change 明确排除的候选设计。

## 已验证边界

### W1

- 当前版本允许正常 Access／Refresh 使用；旧 Access 返回 `401 AUTHN_TOKEN_INVALID`，旧 Refresh 返回 `401 AUTHN_REFRESH_TOKEN_INVALID`。
- 禁用用户的 Access 返回 `403 IDENTITY_ACCOUNT_DISABLED`；禁用用户的 Refresh 保持 `401 AUTHN_REFRESH_TOKEN_INVALID`。
- 软删和不存在用户分别按稳定 Access／Refresh 拒绝契约处理。
- 版本表读取故障时 Access 返回 `500 IDENTITY_INTERNAL_ERROR`，Refresh 返回 `401 AUTHN_REFRESH_TOKEN_INVALID`，响应不携带数据，故障期间不签发新会话。
- 首次成功登录将缺失版本行初始化为版本 `1`；初始化事实来自隔离 MySQL 与真实登录入口。

### W2

- 密码、状态、Kick、逐人角色、逐人组织和软删真实变化只使受影响用户的版本推进一次；旧会话按对应认证契约失效。
- 纯资料变化保持版本和旧会话。
- 角色权限、数据范围、成员差量变化只推进实际受影响成员；等价 no-op 不推进版本。
- `admin` 保护角色的权限、数据范围和成员入口均拒绝，关系和版本保持不变。
- 当前版本合法 no-op 返回当前版本；过期 no-op 分别返回 `AUTHZ_CONFLICT` 和 `ORG_CONFLICT`。
- 本人、受保护 admin、范围外目标和无权限调用方均拒绝，拒绝后关系和版本不变。
- 关系写入失败、版本提升失败和版本初始化失败均返回受控 `5xx`，关系、组织和版本整体回滚。

### W3

- 真实用户列表、用户详情、组织列表／树、组织成员和角色成员读取均使用真实服务入口。
- all、self、org、org_and_children、custom 的用户集合和组织集合与夹具范围一致；有效范围内的等价写入返回成功且不推进版本。
- 范围外目标、`manageable=false` 定位祖先、本人和保护对象的读取／写入均拒绝。
- 临时加入范围外组织关系后，用户详情只返回可见组织并设置 `has_unmanaged_organizations=true`；不泄露隐藏组织或隐藏版本事实。
- 菜单投影单独记录。受限管理员菜单为空时，受保护用户列表 API 仍按真实权限返回 `200`；菜单可见性未被当作 API 安全真值。
- 角色成员查询按 RBAC 规格返回该角色的脱敏成员列表；它不是替代用户详情、组织成员和写入范围校验的安全判定。

### W4

- C01 在数据库版本行锁等待下发出两条携同一旧版本的真实 HTTP 角色请求，最多一条返回 `200` 并推进一次，另一条返回 `409 AUTHZ_CONFLICT`；重复旧请求再次冲突。
- C02 从缺失版本行开始并发执行详情读取与角色变化：一条得到版本 `1`，另一条提交后得到版本 `2`，最终仅保留一条版本行且无版本倒退。
- C03 角色／组织跨维度竞争共享同一版本时一项成功、另一项返回 `409 ORG_CONFLICT`；清理写入后最终关系恢复基线。
- C04 已完成请求的旧版本重试返回冲突；以新版本提交等价 no-op 返回成功且不重复推进或重复失效。

## 证据与保密

机器证据：

- [`c6b-auth-runtime-token.json`](evidence/c6b-auth-runtime-token.json)
- [`c6b-auth-runtime-mutations.json`](evidence/c6b-auth-runtime-mutations.json)
- [`c6b-auth-runtime-scope.json`](evidence/c6b-auth-runtime-scope.json)

证据只保留去敏 HTTP 状态、稳定错误码、关系摘要、版本摘要、等待点和投影结果；不保存 Access／Refresh 内容、验证码答案、密码、Authorization header、数据库连接字符串或其他凭据。

## 覆盖边界

本审计不声称覆盖：

- 全部 `/api/admin` 端点、文件、消息、日志、Route Catalog、MinIO 或第三方通知系统；
- 全局最小失效集合、I3 复杂依赖图、S3／S4／S5 候选语义；
- 生产共享数据库、生产 Token、真实部署拓扑、压力测试或随机并发；
- 菜单投影之外的前端安全保证。

所有本 change 创建的临时服务、容器、schema、Redis 数据、Runner、二进制、配置和凭据在最终验证后清理；共享业务库未写入。
