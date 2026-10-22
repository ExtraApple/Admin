## Why

授权运行时审计曾记录真实 MySQL 故障返回 `401 AUTHN_TOKEN_INVALID`，但在当前干净基线和独占环境中未能复现；表级授权版本故障、完整 MySQL 故障及连接时序探针均返回契约要求的 `500 IDENTITY_INTERNAL_ERROR`。当前优先闭合证据缺口并建立回归保护，不凭不可复现现象修改产品代码。

## What Changes
- 保存两类真实 MySQL 故障和连接时序探针的去敏结果，明确区分通过、证据缺口和不可复现历史观察。
- 保留 Access 缺版本／明确不存在用户的 `401 AUTHN_TOKEN_INVALID`、Access 基础设施故障的 `500 IDENTITY_INTERNAL_ERROR` 以及 Refresh 的统一 `401 AUTHN_REFRESH_TOKEN_INVALID` 契约。
- 核对现有服务层与 HTTP 层回归覆盖；只有发现具体覆盖缺口时才增加最小回归保护，不修改生产认证逻辑。
- 记录先前 `401` 观察在当前基线中不可复现的事实，不把它写成当前产品缺陷修复完成。
- 写入去敏证据和修复说明，保留后续出现新复现材料时可继续定位的边界。
- 本 change 不修改 Token 生命周期、授权策略、版本推进算法、Redis key、路由、权限码、模型、MinIO bucket 或第三方依赖。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `auth`：将已观察和当前重现的 Access／Refresh 错误契约作为回归保护边界；记录历史不可复现观察，不宣称新增产品行为。

## Impact

- 不预期修改 `internal/identity` 生产逻辑；仅在发现具体回归覆盖缺口时调整最小服务／HTTP 测试。
- 保留 `openspec/changes/.../specs/auth/spec.md` 作为待同步的契约保护 delta，不新增路由。
- 使用独占 MySQL、Redis 和真实 JWT 保存表级故障、完整故障、连接时序和恢复后的去敏证据；不写共享业务库。
- 不新增数据库表、Redis key、MinIO bucket、权限码或配置项。
- 当前基线缺陷证据来自 `audit-authorization-runtime`；本 change 记录证据缺口和回归保护，不把不可复现历史观察写成已修复产品缺陷。
