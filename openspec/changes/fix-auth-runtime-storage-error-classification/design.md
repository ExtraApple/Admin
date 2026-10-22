## Context

授权运行时审计在当前基线中通过真实 HTTP、真实 JWT 和独占 MySQL 观察到：MySQL 完全不可用时，带有效 Access Token 的 `/api/user/info` 请求返回 `401 AUTHN_TOKEN_INVALID`。该现象已经恢复运行资源，但尚未证明故障发生在用户查询、授权版本查询或其他认证阶段。

认证链路按 `token parse -> Redis blacklist -> users.FindByID -> user status -> authorization.Snapshot` 执行。当前应用层已经有“记录不存在”“账号禁用”和“内部错误”的类型边界；本 change 先用隔离故障定位实际错误传播，再做最小修复。缺少授权版本行与真实存储故障必须保持不同语义：前者是不可验证会话，后者是服务故障。

## Goals / Non-Goals

**Goals:**

- 以表级授权版本查询故障和完整 MySQL 故障分别定位 Access 错误分类。
- 只将明确的用户不存在或授权版本行缺失映射为 Access `401 AUTHN_TOKEN_INVALID`。
- 将已确认的用户查询／授权版本查询基础设施故障映射为 HTTP `500 IDENTITY_INTERNAL_ERROR`、`data=null`，不泄露底层错误。
- 保持 Refresh 的既有统一拒绝契约：业务上无法验证的 Refresh 请求，包括其存储故障，继续返回 HTTP `401 AUTHN_REFRESH_TOKEN_INVALID`，不签发 Token。
- 保证故障路径不补版本、不回退旧存储、不产生 Token；恢复依赖后正常登录、Access 和 Refresh 链路恢复。
- 用服务层、HTTP 层和真实 MySQL 回归验证固定错误身份与无副作用边界。

**Non-Goals:**

- 不重新设计认证、Refresh、Token 版本推进或授权策略。
- 不修改路由、权限码、数据模型、Redis key、MinIO bucket、配置项或第三方依赖。
- 不把完整 MySQL 故障的根因预设为授权版本仓储；若表级故障通过而整体故障失败，只修复实际确认的身份查询错误分类。
- 不在本 change 内完成 `audit-authorization-runtime` 的剩余授权变更、范围或并发审计。
- 不新增永久故障注入服务；Runner 仅作为隔离验收的临时工具并在结束后删除。

## Decisions

### 1. 先隔离故障阶段，再确定修改层

执行两类真实 MySQL 故障：

1. 保持 `users` 可读，临时使 `user_access_versions` 查询失败；
2. 停止完整 MySQL，验证用户查询失败路径。

每次均记录 HTTP 状态、错误码、`data`、Token 是否签发和版本行变化，恢复依赖后再执行正常链路。只修改返回 `401` 且不符合明确缺失／不存在语义的实际路径。

备选方案是只修授权版本仓储，证据不足；或直接重构认证错误分类，范围过大，均不采用。

### 2. 保留类型化哨兵错误，不按文本或空结果分类

仓储边界只把明确的 GORM `record not found` 映射为已有业务哨兵错误；连接、查询、表不可用和其他驱动错误必须原样包裹并保持可用的 `errors.Is`／类型身份。应用层按错误类型区分 `TokenInvalid`、`AccountDisabled` 与 `InternalError`，不得在 HTTP 中间件中猜测数据库错误。

### 3. Access 与 Refresh 使用不同的公开错误契约

Access 认证故障对客户端公开为稳定 `IDENTITY_INTERNAL_ERROR`、HTTP 500、`data=null`，不暴露 MySQL、GORM、Redis 或连接详情。明确缺行仍为 `AUTHN_TOKEN_INVALID`。

Refresh 继续使用其既有安全拒绝契约；无效、缺行、版本不一致、用户不可用和 Refresh 侧基础设施失败均返回 `AUTHN_REFRESH_TOKEN_INVALID`、HTTP 401、`data=null`，不签发新 Token。这样不会把 Refresh 的拒绝原因暴露给客户端，也不改变既有消费者契约。

### 4. 回归验证覆盖服务、HTTP 和真实 MySQL

- 服务层验证精确哨兵错误与基础设施错误的分类；
- HTTP 层验证稳定状态码、错误码、消息和 `data=null`；
- 真实 MySQL 验证授权版本表故障、完整数据库故障、无补行／无回退／无 Token，以及恢复后的正常链路；
- 现有 Refresh 测试继续验证基础设施错误不签发新 Token并保持既有 401 契约。

不把 mock-only 或 SQLite 结果当作 MySQL 事务和连接故障的唯一证据。

## Risks / Trade-offs

- 完整 MySQL 停止可能同时影响审计日志写入，审计日志失败只作为旁观证据记录，不改变认证响应判定。
- 表级故障如果无法稳定注入，相关路径标为证据缺口而不是猜测性通过；仍保留完整数据库故障结果。
- Refresh 继续以 401 隐藏存储故障，运维可观测性依赖服务端日志和健康检查，不通过公开错误码泄露内部细节。
- 修复只覆盖真实复现的认证分类路径；未覆盖的其他身份、Redis 或消息故障不在本 change 中推断为已修复。
- 本 change 依赖独占 MySQL 验收，不涉及生产 schema 迁移；所有临时资源由验收脚本清理。
