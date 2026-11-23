## Context

- ADR 0007 已确定 WebSocket 通过一次性短期 ticket 鉴权，不把长期 Bearer 放入 URL。
- Route Catalog 规范只允许 `Public`、`Authenticated`、`PermissionControlled`，并明确禁止新增第四种访问等级。
- 当前 `GET /api/user/messages/ws` 被声明为 `Authenticated`；`AuthMiddleware` 在 handler 前强制要求 `Authorization: Bearer`。浏览器 WebSocket API 不能设置该 header，导致有效 ticket 也在握手前 401。
- `docs/reviews/contract-readiness-a3-assembly-response.md` 已记录过该浏览器限制，属于已知契约缺口。
- 无效 ticket 当前返回 `500 MSG_INTERNAL_ERROR`；根因是 Redis Lua 未命中时 go-redis 返回 `redis.Nil`，adapter 将其作为普通错误向上传播。

## Goals / Non-Goals

**Goals:**

- 浏览器可以使用 ticket 完成 WebSocket 升级，不依赖自定义 Authorization header。
- ticket 继续是一次性、60 秒、哈希存储、用户绑定的唯一升级凭据。
- Origin 和用户启用状态在升级时校验。
- 无效、过期、已消费、跨用户 ticket 稳定返回 401。
- 不新增第四种 Route Catalog access level。

**Non-Goals:**

- 不新增消息前端页面或聊天 UI。
- 不修改消息正文、收件箱、RabbitMQ 或刷新 payload。
- 不把长期 Bearer 放入 URL。
- 不改变普通 HTTP 的 Bearer 认证。
- 不引入 `InsecureSkipVerify`、通配符 Origin 或匿名升级后门。

## Decisions

### 1. upgrade 路由使用 Public＋Handler ticket 认证

`GET /api/user/messages/ws` 改为 `routecatalog.Public`：

- 不挂统一 Bearer 中间件。
- Handler 仍原子消费 ticket，并从 ticket 取得 userID。
- `Public` 只表示不经过 Bearer 中间件，不代表匿名；API Metadata 按现有契约保存 `need_auth=0`，OpenAPI／文档明确“需要 ticket”。
- `POST /api/user/messages/ws-ticket` 保持 `Authenticated`，签发前必须有 Bearer。

不新增 `TicketAuthenticated`，因为 Route Catalog 规范禁止第四种等级。

### 2. Ticket 消费返回所有者

当前 `Consume(ctx, ticket, authenticatedUserID)` 依赖 `c.GetUint("userID")`。改为：

```go
userID, err := tickets.Consume(ctx, ticket)
```

- 空 ticket／未找到／已消费／过期／跨用户统一 `ErrWebSocketTicketInvalid`。
- Gateway `Connect(ctx, request, accept)` 不再接收外部 userID。
- Handler 不再调用 `c.GetUint("userID")`。
- ticket 只消费一次；升级失败后客户端重新申请 ticket。

### 3. Origin 精确校验

`websocket.Accept` 使用 `OriginPatterns: []string{config.BrowserSession.Origin}`：

- 只允许精确配置的可信 Origin；不使用 `*` 或 `InsecureSkipVerify`。
- 本地允许 `http://localhost:15173`；生产允许实际 HTTPS Origin。
- 允许请求 host 的默认行为保留；非浏览器客户端没有 Origin 的策略在实现时明确测试。

### 4. 升级时检查用户启用状态

在 messaging application 声明最小用户状态接口，由 App 组合根注入 identity 实现：

```go
type WebSocketUserGuard interface {
    IsActive(ctx context.Context, userID uint) (bool, error)
}
```

- 用户停用 → 拒绝升级。
- 查询失败 → 稳定依赖错误，不返回 500 内部细节。
- 保留 60 秒 ticket TTL；ticket 不代表永久授权。

### 5. `redis.Nil` 映射为未找到

`redisWebSocketTicketBackend.Consume` 中：

```go
value, err := backend.client.Eval(...).Result()
if errors.Is(err, goredis.Nil) {
    return 0, false, nil
}
```

随后 `WebSocketTicketService.Consume` 将 `found=false` 转成 `ErrWebSocketTicketInvalid`；`classifyMessageError` 已有 401 映射。补充空值、重复消费、过期 ticket 测试。

### 6. 契约和文档同步

- Route Catalog 测试断言 upgrade 路由为 `Public`，`ws-ticket` 为 `Authenticated`。
- OpenAPI 描述 query `ticket`／`cursor`、101 原生响应和 401 错误。
- API Metadata 的 `need_auth=0` 说明为“无 Bearer，但需要 ticket”，不把它误写成匿名。
- 更新 `contract-readiness-a3` 的 WebSocket 限制说明为已裁决／待修复状态。
- 不修改普通 API 权限或消息权限码。

## Risks / Trade-offs

- **Ticket 在 query 中**：可能进入反向代理访问日志；当前设计已使用 query，首轮保持兼容，后续可评估子协议。不得记录完整 ticket。
- **Public 元数据语义**：`need_auth=0` 容易被误读为匿名；必须在 OpenAPI、API Metadata 文案和文档中明确 ticket 要求。
- **Origin 配置错误**：过窄会阻断本地 Vite／生产域名，过宽会引入跨站风险；必须用精确配置并测试拒绝路径。
- **用户状态检查依赖**：查询失败时不能误判为 ticket 无效或放行；需区分依赖错误与 401。
- **升级失败消耗 ticket**：客户端需重新申请；这是安全的一次性语义。

## Test Plan

- 单元：`redis.Nil` → `found=false, nil`；ticket 未找到／重复／过期／跨用户 → `ErrWebSocketTicketInvalid`。
- Gateway：有效 ticket 绑定 userID；无效 ticket 不调用 accept；用户停用不调用 accept。
- HTTP：Public upgrade 路由不带 Bearer 时有效 ticket → 101；无／无效／过期 ticket → 401；不允许 Origin → 拒绝；允许 Origin → 101；停用用户 → 拒绝。
- 契约：Route Catalog access level、OpenAPI query／101／401、API Metadata `need_auth=0` 语义。
- 浏览器：真实 Chromium `new WebSocket(url?ticket=...)` 无 Authorization → 101；断线后新 ticket＋cursor 重连；无效 ticket 401。
- 回归：`go test ./...`、`npm test`、`npm run typecheck`，以及现有 messaging／route catalog 测试。
