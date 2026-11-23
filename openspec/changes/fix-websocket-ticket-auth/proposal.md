## Why

浏览器 WebSocket 使用一次性 ticket 升级时，当前 `GET /api/user/messages/ws` 声明为 `Authenticated`，Gin 中间件要求 `Authorization: Bearer`。浏览器 WebSocket API 不能设置该 header，因此有效 ticket 也在握手前返回 401。ADR 0007 已明确 WebSocket 通过一次性短期 ticket 鉴权，不把长期 Bearer 放入 URL；现有实现与该决策不一致。另一个独立问题是无效 ticket 返回 500，而不是稳定的 401。

## What Changes

- 将 `GET /api/user/messages/ws` 从 `Authenticated` 调整为 `Public`：不经过统一 Bearer 中间件，但 handler 仍以 ticket 为唯一升级凭据；Public 不代表匿名。
- `WebSocketTicketService.Consume` 返回 ticket 绑定的 userID；Gateway 使用该 userID 建立连接，不再依赖 `c.GetUint("userID")`。
- 在 WebSocket Accept 上按 `browser_session.origin` 做精确 Origin 校验，不使用通配符或 `InsecureSkipVerify`。
- 升级时重新检查用户启用状态，拒绝签发后被停用的用户。
- 修复 Redis ticket consume 对 `redis.Nil` 的处理：未找到／已消费／过期统一映射 `401 MSG_WS_TICKET_INVALID`，不返回 500。
- 更新 Route Catalog／OpenAPI／API Metadata 说明和 WebSocket 测试；不新增消息前端页面。
- 非目标：不新增第四种 Route Catalog access level；不改变普通 HTTP 认证；不把长期 Bearer 放入 URL；不修改消息正文、RabbitMQ 或收件箱事实。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `internal-messaging`: WebSocket ticket 握手改为 handler ticket 认证，补充 Origin、用户启用状态和无效 ticket 错误契约。

## Impact

- 代码：`internal/messaging/adapters/http`、`internal/messaging/application`、`internal/messaging/adapters/redis`、`internal/app` 的 WebSocket 路由装配。
- 契约：Route Catalog 中该路由仍使用现有 Public 等级；OpenAPI 说明和 API Metadata `need_auth=0` 需明确“无 Bearer，但需要 ticket”。
- 测试：ticket 消费、Gateway、HTTP 握手、Origin、停用用户、浏览器／curl 冒烟；不修改业务消息模型。
- 文档：更新 `docs/reviews/contract-readiness-a3-assembly-response.md` 的已知限制说明和本次 evidence／TODO 状态。
