# 工作台前后端集成验收证据

## 环境

- 日期：2026-11-17
- 隔离栈：MySQL 8.0 `13306`、Redis 7 `16379`、MinIO `19000/19001`、Mailpit `11025/18025`、RabbitMQ `15672/15673`、后端 `18080`、Vite `15173`。
- 后端从临时目录运行构建产物；未修改仓库默认 `config.yaml`、`.env` 或业务代码。
- 测试账号：admin、manager、viewer、disabled、user-a、user-b；组织 A/B。

## 自动化与页面读取

| 项 | 实际结果 |
|---|---|
| `go test ./...` | 通过 |
| `npm test` | 14 个文件、184 个测试通过 |
| `npm run typecheck` | 通过 |
| 登录、注册、404、no-management | 真实页面渲染；未授权入口不显示 |
| 授权总览、风险清单 | 数据、筛选、分页正常 |
| 角色、用户、组织列表／详情 | 正常；用户详情含角色、组织、操作 |
| 菜单、接口、审计日志 | 正常；菜单树、RouteState、审计筛选可读 |
| `/docs`（前端） | SPA 路由返回“找不到这个地址” |
| `/docs`（后端） | 200，Admin API Docs |
| `/docs/openapi.json` | 200，OpenAPI 3.0.3，96 paths |
| 头像默认资源 | 200 image/png |

## 写链路与权限

- 个人资料：昵称更新、头像上传／读取／恢复默认、邮箱换绑＋Mailpit 确认、改密后旧 token 401。
- 角色：创建 `connectivity-manager`，分配 11 个权限、菜单和自定义组织 A 数据范围。
- 用户：admin 分配用户角色／组织、启停、踢下线；manager 在 A 内可读，B 用户详情 403。
- 组织：创建／移动到 A／移回根／删除成功；manager 可更新 A，B 目标返回 `422 ORG_VALIDATION_INVALID`（不是 403，但 B 不可见）。
- 菜单：创建、更新、删除预览＋fingerprint、API 绑定／清空、路由同步全部成功。
- 接口：元数据更新、创建／删除测试元数据、路由同步、权限同步、按钮菜单生成成功。
- 权限错位：manager 导航只显示角色／用户／组织；直接访问 `/menus`、`/apis`、`/audit-logs` 被页面守卫拦截，对应 API 均 `403 API_META_PERMISSION_DENIED`。
- viewer 管理路由被守卫拦截；disabled 登录 403 `IDENTITY_ACCOUNT_DISABLED`，旧 token 403。
- 409：使用过期 API fingerprint 重复提交返回 `409 API_META_CONFLICT`。
- 422：错误密码字段、错误验证码、manager 访问 B 组织均返回稳定错误码。

## 失败与弱网

- Offline 首屏：导航到 `/login` 返回 `net::ERR_INTERNET_DISCONNECTED`；恢复网络后可重新打开登录页，未显示受保护内容。
- Offline 保存昵称：页面显示“保存结果尚未确认。草稿仍保留”，没有伪报成功；草稿仍在。
- Slow 3G 登录：真实登录成功；网络记录只有 1 次 `POST /api/login`，未产生重复登录。
- Slow 3G 保存＋重复点击：只产生 1 次 `PUT /api/user/info`；第二次点击因按钮进入忙碌态被阻止。
- 后端断连：停止后端后保存昵称，页面显示“保存结果尚未确认”，草稿保留；重启后恢复。
- Redis 停止：重新读取资料失败，页面显示“资料或头像显示读取未完成”“用户操作未完成，请稍后重试”，草稿保留；Redis 恢复后可重新读取。
- Access／Refresh 失效：刷新后 `/api/refresh` 401，页面进入登录，不显示受保护内容。
- 登录锁定：连续 5 次错误密码后第 6 次及正确密码均返回 `429 AUTHN_LOGIN_LOCKED`；前 4 次返回 `AUTHN_CREDENTIALS_INVALID`，`remaining_attempts` 从 4 递减到 1。
- Web Storage 抽查：localStorage 只有 `admin-workbench.auth.refresh-attempt`，sessionStorage 为空；`document.cookie` 为空，Refresh 为 HttpOnly Cookie。

## 截图

- [`login.png`](workbench-connectivity-2026-11-17/login.png)
- [`admin-home.png`](workbench-connectivity-2026-11-17/admin-home.png)
- [`manager-roles.png`](workbench-connectivity-2026-11-17/manager-roles.png)
- [`mobile-users.png`](workbench-connectivity-2026-11-17/mobile-users.png)
- [`failure-profile.png`](workbench-connectivity-2026-11-17/failure-profile.png)

## 数据库与 Redis 抽查

- 用户：6 个；`disabled` 用户 status=0，其余 status=1。
- 角色权限：admin 角色 107 条，`connectivity-manager` 11 条。
- 组织关系：manager/ua 属于组织 A，ub 属于组织 B。
- 访问版本：admin 10，manager 8，viewer 8，disabled 8，ua 9，ub 8。
- Redis：`auth:session:*` 28 个，`captcha:*` 1 个，WebSocket ticket 键 0 个；未发现 token 泄露。
- 敏感信息扫描：未命中测试 Access token、JWT、数据库／MinIO／RabbitMQ／admin 密码。

## WebSocket（阻塞）

| 步骤 | 实际 |
|---|---|
| `POST /api/user/messages/ws-ticket` | 200，ticket 签发成功 |
| 浏览器 `new WebSocket('ws://127.0.0.1:18080/api/user/messages/ws?ticket=...')` | 连接前失败，控制台：`HTTP Authentication failed; no valid credentials available` |
| 无效 ticket／无 ticket | 同样在握手前失败 |

代码核对：`internal/messaging/adapters/http/routes.go` 将 `/api/user/messages/ws` 声明为 `routecatalog.Authenticated`；`internal/identity/adapters/http/middleware.go` 在进入 handler 前强制要求 `Authorization: Bearer`。浏览器 WebSocket API 不能设置 Authorization header，因此 query ticket 无法到达 handler。该阻塞影响 W-02～W-05；W-01 ticket 已通过。

### curl 替代验证

curl 8.13 支持 `ws://`，可以设置 Authorization header，但不能模拟浏览器“不能设置 header”的限制：

| 请求 | 实际 |
|---|---|
| 有效 Bearer＋有效 ticket | `101 Switching Protocols`，`Upgrade: websocket` |
| 无 Authorization＋ticket | `401 Unauthorized` |
| 有效 Bearer＋无效 ticket | `500 MSG_INTERNAL_ERROR`（预期 401，另发现错误映射问题） |
| 有效 Bearer＋无 ticket | `401 Unauthorized` |

结论：后端 WebSocket 在“能设置 Authorization header 的客户端”下可以升级；浏览器集成仍被认证中间件阻断。无效 ticket 的 500 是额外缺陷，不能标记 W-03 通过。

## 可访问性与性能

- Lighthouse Accessibility：登录、首页、用户列表、用户详情、个人资料均为 `100`。
- 性能基线：登录 LCP 284 ms、CLS 0.05；首页 LCP 439 ms、CLS 0.00。原始 trace 已随 11.6 清理，基线数值保留在本文件。
- Lighthouse 报告已随 11.6 清理；本文件保留各页可访问性结果与基线数值。
- 移动端 390×844、暗色、减少动态：无横向溢出，菜单按钮和账号菜单可用。
- 控制台：未见未处理 Promise；角色／用户／接口／审计页有 Chrome “form field element should have an id or name attribute” issue，登录页 Lighthouse best-practices 96 由初始 401 console error 引起。

## 待修复问题

以下问题已记录，未修复；环境按用户要求保留。

1. **浏览器 WebSocket ticket 升级被认证中间件阻断**
   - 现象：浏览器 `new WebSocket(.../api/user/messages/ws?ticket=...)` 在握手前失败，控制台 `HTTP Authentication failed; no valid credentials available`。
   - 证据：ticket 签发 200；curl 带有效 Bearer＋ticket 返回 `101 Switching Protocols`；无 Authorization 返回 401。
   - 代码位置：`internal/messaging/adapters/http/routes.go` 将 upgrade 路由声明为 `routecatalog.Authenticated`；`internal/identity/adapters/http/middleware.go` 在 handler 前强制要求 Authorization。
   - 影响：W-02 的浏览器链路、W-05 重连、W-06 消息刷新。
   - 待定方向：让 upgrade 路由按 ticket 认证，或设计浏览器可携带的凭据；修复时需同时保留 Origin／ticket 校验。

2. **无效 WebSocket ticket 返回 500**
   - 现象：有效 Bearer＋`ticket=invalid-ticket` 返回 `500 MSG_INTERNAL_ERROR`，预期 `401 MSG_WS_TICKET_INVALID`。
   - 证据：curl 响应与后端日志 `error_code=MSG_INTERNAL_ERROR`。
   - 代码位置：`internal/messaging/adapters/redis/websocket_ticket.go` 的 Lua consume；`internal/messaging/adapters/http/errors.go` 的错误映射。
   - 影响：W-03；replay／expired ticket 的错误契约尚未验证。
   - 待定方向：确认 Redis `false` 解码为空值还是 0，并让未找到／已消费／过期统一映射 401。

## 矩阵状态

| 矩阵 | 状态 |
|---|---|
| P-01～P-13 | 通过；页面读取、权限渲染和守卫符合预期 |
| P-14 | 后端 `/docs`、OpenAPI、头像通过；前端 `/docs` 落入 SPA 404（Vite 只代理 `/docs/`，属路由代理差异） |
| F-01～F-12 | 通过；离线、慢网、401／Refresh、403、422、409、后端断连、Redis 故障、停用用户 |
| W-01 | 通过：ticket 签发 |
| W-02 | 修复后通过：真实 Chromium 无 `Authorization`、有效 ticket 升级返回 `101`（后端访问日志确认）；原阻塞保留在本节前文 |
| W-03／W-04 | 修复后通过：无 ticket、无效 ticket、过期 ticket 均 `401 MSG_WS_TICKET_INVALID`；不可信 Origin、停用用户 `403`；依赖失败聚焦测试为 `503` |
| W-05 | 修复后通过：新 ticket＋`cursor=0-0` 在 Chromium 重连返回 `101` |
| W-06 | 修复后通过：管理员创建组织广播后，WebSocket 收到 `inbox.refresh`，Outbox／Consumer ACK 日志一致 |
| N-01～N-08 | 通过；a11y 100、性能基线已记录、移动端无横向溢出 |

## 当前状态

已通过：环境、门禁、页面读取、代表写链路、角色／数据范围、403／422／409、离线／慢网、后端断连、Redis 故障、登录锁定、a11y／perf、证据和敏感信息扫描。
WebSocket：原 W-02～W-06 阻塞已由 `fix-websocket-ticket-auth` 修复并复验，详见下一节。
已清理：后端、Vite、浏览器页面、5 个 `workbench-connectivity-*` 容器、2 个命名卷、2 个匿名卷及临时配置／凭据目录均已移除；仓库内脱敏证据保留。

## WebSocket ticket 认证修复复验（2026-11-23）

Change：`fix-websocket-ticket-auth`。

### 实现摘要

- `GET /api/user/messages/ws` 从 `Authenticated` 改为 `Public`；该等级只表示不经过 Bearer 中间件，Handler 仍以一次性 ticket 为唯一升级凭据。
- `WebSocketTicketService.Consume` 返回 ticket 绑定 userID；Gateway 不再接收外部 userID，并在 accept 前重新查询用户启用状态。
- Redis `redis.Nil` 与 nil Lua 结果映射为 `found=false`；无效／过期／重复 ticket 统一 `401 MSG_WS_TICKET_INVALID`。
- `websocket.Accept` 使用精确 `browser_session.origin` 的 `OriginPatterns`；Handler 在消费 ticket 前拒绝不匹配的 Origin，不使用通配符或 `InsecureSkipVerify`。
- 用户停用返回 `403`，用户状态查询失败返回 `503 MSG_WEBSOCKET_UNAVAILABLE`；错误响应不包含 Redis／Identity 内部细节。
- `POST /api/user/messages/ws-ticket` 保持 `Authenticated`；未新增 Route Catalog access level、消息权限码或 RabbitMQ 拓扑。

### curl 冒烟（隔离后端 `127.0.0.1:18080`，Origin `http://localhost:15173`）

| 请求 | 实际 |
|---|---|
| 无 ticket，无 Authorization | `401 MSG_WS_TICKET_INVALID` |
| 不可信 Origin＋有效 ticket | `403 MSG_PERMISSION_DENIED`，且同一 ticket 随后仍可升级（Origin 拒绝未消耗 ticket） |
| 同一 ticket，可信 Origin，无 Authorization | `101 Switching Protocols` |
| 无效 ticket | `401 MSG_WS_TICKET_INVALID`（不是 500） |
| 通过 Redis 删除 ticket 摘要模拟过期 | `401 MSG_WS_TICKET_INVALID` |
| 有效 Bearer＋有效 ticket | `101 Switching Protocols` |
| ticket 用户被管理员停用后升级 | `403 MSG_PERMISSION_DENIED`；随后恢复用户启用 |

### 真实 Chromium

在 `http://localhost:15173` 同源页面（干净页面，无应用启动请求）执行：

- `fetch('/api/user/messages/ws-ticket')` 使用 Bearer 签发 ticket，状态 `200`。
- `new WebSocket('ws://127.0.0.1:18080/api/user/messages/ws?ticket=...')` 不设置 `Authorization`，收到 `open`；后端访问日志记录 `GET /api/user/messages/ws status=101`。
- 关闭后重新签发 ticket，并携带 `cursor=0-0` 重连，再次收到 `open`；后端日志第二条 `status=101`。
- 游标补发：向空的 `admin:messaging:refresh:1` 临时写入两条合成事件，携带第一条 Stream ID 重连，收到第二条 `inbox.refresh`（`event_id=fix-replay-2`、`message_copy_id=9002`、`aggregate_version=2`）；测试后删除临时 Stream key。
- 消息刷新：管理员通过真实 HTTP 创建组织广播后，manager WebSocket 收到 `inbox.refresh`（`message_copy_id=3`、`aggregate_version=1`）；后端日志确认 Outbox 发布、Consumer 完成和 ACK，广播随后撤销，manager 收件箱副本删除。
- 修复后干净页面控制台无消息；随后对无效 ticket 单独测试，Chromium 关闭连接，后端日志记录 `status=401`。
- 修复前控制台错误为握手前认证失败；修复后有效连接不再出现该失败。

### 自动化回归

- `go test ./...`：通过。
- `npm test -- --run`：14 个文件／184 个用例通过。
- `npm run typecheck`：通过。
- 聚焦覆盖：Redis nil、重复／过期／无效 ticket、Gateway 用户绑定与停用、Origin 允许／拒绝／Vite 回环、Public／Authenticated access level、OpenAPI query／101／401／503。

### 环境

本轮已于 2026-11-23 执行 11.6：停止后端与 Vite，关闭浏览器页面，移除 5 个 `workbench-connectivity-*` 容器，删除 2 个命名卷和 2 个匿名卷，并删除临时目录中的配置、凭据、token、日志、trace 与 Lighthouse 报告。仓库内脱敏证据、截图和基线数值保留。

