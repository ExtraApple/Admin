## Why

授权运行时审计在真实隔离 MySQL 故障下观察到：携带有效 Access Token 的请求返回 `401 AUTHN_TOKEN_INVALID`，而不是区分凭据无效与基础设施不可用。该结果会让客户端误判 Token 已失效，并隐藏数据库／授权存储故障；当前需要在独立修复 change 中确认故障层并修正分类。

## What Changes

- 仅针对 Access 认证链路，区分明确的“用户不存在／授权版本行缺失”与真实数据库、查询、连接故障。
- 明确 Access 认证真实基础设施故障返回 HTTP `500`、稳定 `IDENTITY_INTERNAL_ERROR`、`data=null`，不回显底层错误。
- 保留明确授权版本缺失的 HTTP `401 AUTHN_TOKEN_INVALID` 语义，不补版本行、不读取退役旧存储、不签发 Token。
- 验证用户查询故障和 `user_access_versions` 查询故障两条路径；只修复当前基线中真实复现的错误分类。
- 保留 Refresh Token 既有统一拒绝契约：无效、缺版本、版本不一致或 Refresh 基础设施失败继续返回 HTTP `401 AUTHN_REFRESH_TOKEN_INVALID`，不签发新 Token。
- 增加服务层、HTTP 层及真实 MySQL 故障回归验证，覆盖故障恢复后的正常登录、Access 和 Refresh 链路。
- 本 change 不修改 Token 生命周期、授权策略、版本推进算法、Redis key、路由、权限码、模型、MinIO bucket 或第三方依赖。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `auth`：明确 Access 认证中授权版本／用户身份存储故障与不可验证会话的错误分类、HTTP 错误契约和无副作用边界；保留 Refresh 的统一 `401` 拒绝契约。

## Impact

- 可能修改 `internal/identity` 的认证错误传播、身份仓储或授权版本读取边界，以及对应服务／HTTP 测试。
- 修改 `openspec/specs/auth/spec.md` 的 Access 存储故障场景，明确 `500 IDENTITY_INTERNAL_ERROR`；不新增路由。
- 使用独占 MySQL 做表级授权版本读取故障和完整数据库故障验收；不写共享业务库。
- 不新增数据库表、Redis key、MinIO bucket、权限码或配置项。
- 当前基线缺陷证据来自 `audit-authorization-runtime`；该 change 只负责修复和回归验证，不替代原始审计结论。
