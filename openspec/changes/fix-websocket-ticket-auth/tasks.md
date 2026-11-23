## 1. Ticket 消费与错误映射

- [x] 1.1 修改 `internal/messaging/adapters/redis/websocket_ticket.go`：Lua 未命中导致 `redis.Nil` 时返回 `(0, false, nil)`，不把空值当内部错误。
- [x] 1.2 将 `WebSocketTicketService.Consume` 改为返回 ticket 绑定 userID 和错误；空／未找到／已消费／过期／无 owner 统一 `ErrWebSocketTicketInvalid`。
- [x] 1.3 为 Redis 空值、重复消费、过期 ticket 增加测试，断言 `found=false` 或 `ErrWebSocketTicketInvalid`。
- [x] 1.4 确认无效 ticket 经 `classifyMessageError` 返回 `401 MSG_WS_TICKET_INVALID`，不返回 500。

## 2. Upgrade 路由与 Gateway

- [x] 2.1 将 `GET /api/user/messages/ws` 的 Route Descriptor 从 `Authenticated` 改为 `Public`；保留 `POST /api/user/messages/ws-ticket` 为 `Authenticated`。
- [x] 2.2 调整 `webSocketHandler.upgrade`：不再读取 `c.GetUint("userID")`，只传 ticket／cursor 给 Gateway。
- [x] 2.3 调整 `WebSocketGateway.Connect`：从 ticket 消费结果取得 userID，再注册连接；不得接受外部伪造 userID。
- [x] 2.4 更新 Gateway／handler 测试：有效 ticket 调用 accept 并绑定 ticket owner；无效 ticket 不调用 accept；外部 userID 不再存在。

## 3. Origin 与用户状态

- [x] 3.1 将 `browser_session.origin` 传入 `websocket.Accept` 的 `OriginPatterns`；仅允许精确可信 Origin，不使用 `InsecureSkipVerify` 或通配符。
- [x] 3.2 增加允许 Origin、拒绝 Origin、Vite 回环代理场景测试。
- [x] 3.3 在 messaging application 声明最小用户状态接口，由组合根注入 identity 实现；升级时检查 ticket 用户仍启用。
- [x] 3.4 区分用户停用／ticket 无效／依赖查询失败：分别返回稳定 401／403／503，不泄漏内部错误。

## 4. 契约、文档与元数据

- [x] 4.1 更新 Route Catalog 测试：upgrade 为 Public，ticket 签发为 Authenticated，且不新增第四种 access level。
- [x] 4.2 更新 OpenAPI：描述 query `ticket`／`cursor`、101 原生响应、401 `MSG_WS_TICKET_INVALID`、503 `MSG_WEBSOCKET_UNAVAILABLE`。
- [x] 4.3 明确 API Metadata `need_auth=0` 表示“无 Bearer 中间件，但 upgrade 需要 ticket”；不得写成匿名。
- [x] 4.4 更新 `docs/reviews/contract-readiness-a3-assembly-response.md` 的 WebSocket 已知限制说明。
- [x] 4.5 不在普通 HTTP 路由、消息权限码或 RabbitMQ 拓扑中引入无关变更。

## 5. 验证

- [x] 5.1 单元测试：Redis `redis.Nil`、ticket 过期／重复／无 owner、Gateway 用户绑定与停用。
- [x] 5.2 HTTP 测试：无 Bearer＋有效 ticket → 101；无／无效／过期 ticket → 401；不允许 Origin → 拒绝；停用用户 → 拒绝。
- [x] 5.3 浏览器测试：真实 Chromium `new WebSocket(...?ticket=...)` 返回 101；断线后新 ticket＋cursor 重连；控制台无认证错误。
- [x] 5.4 curl 冒烟：有效 Bearer＋ticket → 101；无 ticket → 401；无 Authorization＋有效 ticket → 101；无效／过期 ticket → 401（不是 500）；不可信 Origin → 403。
- [x] 5.5 回归：`go test ./...`、`npm test`、`npm run typecheck`；相关 route catalog／messaging 测试通过。
- [x] 5.6 记录修复前后证据，更新 `verify-workbench-connectivity` 的 W-02／W-03／W-05／W-06 状态。

## 6. 收尾

- [x] 6.1 更新 TODO 和验收 evidence，确认两个 WebSocket 问题均已关闭。
- [x] 6.2 证据已落盘；隔离后端、Vite、容器与临时配置按既有验收要求保留，未执行删除。
