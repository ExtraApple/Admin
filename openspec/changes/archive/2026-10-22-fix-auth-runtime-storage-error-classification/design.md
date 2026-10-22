## Context

授权运行时审计曾记录完整 MySQL 故障返回 `401 AUTHN_TOKEN_INVALID`，但该现象在当前干净基线中无法复现。当前独占环境中，授权版本表不可用和完整 MySQL 不可用均返回 `500 IDENTITY_INTERNAL_ERROR`，连接时序探针仅得到传输失败。

认证链路按 `token parse -> Redis blacklist -> users.FindByID -> user status -> authorization.Snapshot` 执行。源码和现有服务测试已经保留“记录不存在”“版本缺失”“账号禁用”和“内部错误”的类型边界；本 change 以证据闭合和回归保护为目标，不凭不可复现现象修改生产逻辑。

## Goals / Non-Goals

**Goals:**

- 保存表级授权版本故障、完整 MySQL 故障、连接时序和恢复后的去敏真实结果。
- 核对当前实现已经满足 Access `401`／`500` 与 Refresh `401` 的稳定错误契约。
- 记录旧 `401` 观察在当前基线中不可复现的事实，分类为证据缺口而不是当前产品缺陷。
- 核对现有服务层、HTTP 层和真实 MySQL 回归覆盖；只有发现具体覆盖缺口时才增加最小保护。
- 保留可复核的错误身份、无 Token、副作用和恢复事实。

**Non-Goals:**

- 不修改认证生产逻辑、Token 生命周期、Refresh 语义或版本推进算法。
- 不修改路由、权限码、数据模型、Redis key、MinIO bucket、配置项或第三方依赖。
- 不在本 change 内完成 `audit-authorization-runtime` 的剩余授权变更、范围或并发审计。
- 不把不可复现的历史 `401` 观察重新表述为已确认产品缺陷。
- 不新增永久故障注入服务；Runner 仅作为隔离验收的临时工具并在结束后删除。

## Decisions

### 1. 以当前基线真实结果确定范围

已执行两类真实 MySQL 故障和连接时序探针：

1. 保持 `users` 可读，临时使 `user_access_versions` 查询失败；
2. 停止完整 MySQL，验证用户查询失败路径；
3. 请求开始后立即停止 MySQL，重复 20 次观察连接池／时序行为。

前两类均返回 HTTP `500 IDENTITY_INTERNAL_ERROR`，恢复后返回 `200`；时序探针均为传输失败，没有返回 `401`。因此当前没有可确认的生产修复文件集合，历史 `401` 只记录为证据缺口。

### 2. 保留类型化哨兵错误，不按文本或空结果分类

仓储边界只把明确的 GORM `record not found` 映射为已有业务哨兵错误；连接、查询、表不可用和其他驱动错误必须原样包裹并保持可用的 `errors.Is`／类型身份。应用层按错误类型区分 `TokenInvalid`、`AccountDisabled` 与 `InternalError`，不得在 HTTP 中间件中猜测数据库错误。

### 3. Access 与 Refresh 使用不同的公开错误契约

Access 认证故障对客户端公开为稳定 `IDENTITY_INTERNAL_ERROR`、HTTP 500、`data=null`，不暴露 MySQL、GORM、Redis 或连接详情。明确缺行仍为 `AUTHN_TOKEN_INVALID`。

Refresh 继续使用其既有安全拒绝契约；无效、缺行、版本不一致、用户不可用和 Refresh 侧基础设施失败均返回 `AUTHN_REFRESH_TOKEN_INVALID`、HTTP 401、`data=null`，不签发新 Token。这样不会把 Refresh 的拒绝原因暴露给客户端，也不改变既有消费者契约。

### 4. 回归保护覆盖服务、HTTP 和真实 MySQL

- 运行现有服务层错误分类测试，只有真实覆盖缺口才增加测试；
- 验证 HTTP 状态、错误码、消息和 `data=null`；
- 保存真实 MySQL 表级故障、完整故障、连接时序和恢复结果；
- 不把 mock-only 或 SQLite 结果当作 MySQL 故障的唯一证据。

## Risks / Trade-offs

- 历史 `401` 无法在当前干净环境重现，不能据此证明当前代码存在缺陷；后续新复现材料需要重新建立基线。
- 完整 MySQL 停止可能同时影响审计日志写入；旁观日志失败不改变认证响应判定。
- Refresh 继续以 401 隐藏存储故障，运维可观测性依赖服务端日志和健康检查。
- 本 change 不声称修复未观察到的其他身份、Redis 或消息故障。
- 所有临时容器、schema、Token、验证码和 Runner 在证据写入后清理。
