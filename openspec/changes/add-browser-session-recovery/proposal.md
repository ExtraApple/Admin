## Why

工作台把全部 Token 仅保存在页面内存，导致每次完整刷新都需要重新输入密码与验证码。直接持久化现有 Refresh Token 又会保留旧凭据可重复续期、退出只撤销 Access Token 的风险，因此需要一起建立可恢复、可撤销且不削弱实时授权的浏览器登录会话。

## What Changes

- 刷新后先恢复会话并重新获取真实用户上下文，验证完成前不展示受保护内容；不把前端解析 JWT `exp` 当作登录凭据有效的证明。
- **BREAKING**：Access Token 只驻留内存，Refresh Token 改用服务端设置的 HttpOnly 会话 Cookie；登录及续期 JSON 不再返回 `refresh_token`，续期不再接受 JSON Token 通道。
- 为每次登录建立独立 Redis 登录会话，Access／Refresh Token 都绑定会话和现有授权版本。普通退出撤销当前会话全部凭据；其他独立登录不受影响，强制下线及授权变化仍由 Authorization 的版本事实约束。
- Refresh Token 一次性轮换；同一续期尝试可在 30 秒内幂等取得同一代签发结果，不延长到期时间。浏览器协调认证操作；异常旧凭据重用撤销该会话，不把合法并发或晚到结果当作新授权。
- 同一浏览器配置、同一工作台站点共享当前用户会话；新标签可恢复，退出和换用户使其他标签清除旧上下文，不把旧编辑静默带入新身份。
- **BREAKING**：续期凭据无效保持 `401 / AUTHN_REFRESH_TOKEN_INVALID`；数据库、Redis 及签发真实故障改为安全的 `500 / IDENTITY_INTERNAL_ERROR`。临时故障允许手动恢复，不自动重放业务请求。
- 退出请求失败时保存不含 Token 的退出待确认标记，阻止 Cookie 自动恢复，并提供手动重试；浏览器必要存储不可用时显式拒绝恢复，不伪称退出成功。
- 生产限定同源 HTTPS、Secure Cookie 和可信 Origin／CSRF 校验；HTTP 开发例外只允许显式配置的 `localhost`／`127.0.0.1` 回环地址。
- **BREAKING**：前后端同批切换，旧 Token 统一拒绝并要求重新登录；移除旧 Token 用途兼容与旧退出黑名单路径，不保留双通道或自动回退。现有历史 Redis key 自然到期，不批量清库。

### 非目标

- 不增加长期“记住我”、跨域 Cookie 登录、第三方续期协议兼容或不同标签同时使用不同用户的能力。
- 不改变 Access 15 分钟／Refresh 7 天的仓库默认时长及滑动续期语义，不新增空闲超时或绝对会话上限；实际部署仍使用经验证的现有 JWT 时长配置。
- 不修改密码策略、验证码、注册、角色、组织、管理权限、授权版本唯一事实来源或业务请求的自动重试策略。
- 不新增全设备普通退出、MySQL 会话表、会话管理大盘、维护清理任务或新的基础设施依赖。
- 不重设计登录／注册及工作台视觉，不撤回已有顶部账号菜单；不回写已归档审计、不修复历史 Public API Metadata 或清理历史审计数据。

## Capabilities

### New Capabilities

无。新增行为由现有认证、工作台、Route Catalog 和日志能力分别持有，避免第二套会话或接口契约。

### Modified Capabilities

- `auth`：登录会话、Cookie 凭据传递、一次性续期与幂等恢复、当前会话撤销、错误区分、部署安全和旧 Token 切换。
- `admin-workbench`：启动恢复、运行中失效与手动恢复、跨标签协调、退出待确认和浏览器能力失败，不改变现有业务编辑及主题职责。
- `route-catalog`：Cookie 会话控制端点的静态访问声明、Header／Cookie／响应文档及完整安全错误来源；保持单一 Snapshot 和其余路由的 JWT 最低身份边界。
- `logging`：把浏览器会话 Cookie、Set-Cookie 和续期凭据纳入既有敏感数据保护，不新增敏感日志字段或扩大运行日志白名单。
- `modular-layered-architecture`：经用户确认，仅把两处过时的黑名单校验／归属描述同步为活动登录会话及其撤销；不改变分层、Principal 或授权版本归属。

## Impact

- **后端代码**：`internal/identity/{application,domain,adapters}`、`internal/platform/config`、`internal/app`、`internal/routecatalog`、`internal/apidoc` 及相关测试。遵守现有分层单体与唯一组合根，不恢复退役的顶层 handler/service/router 目录。
- **前端代码**：`web/src/lib/session-state.ts`、`session.tsx`、集中 API Client、认证 DTO、认证页与工作台会话守卫及相关现有测试；页面不自行读写 Cookie 或另建鉴权 Client。
- **路由**：不增加或重命名业务 URL。修改 `POST /api/login`、`POST /api/refresh`、`POST /api/user/logout`；退出改为不依赖有效 Access Token 的 Cookie 会话控制入口，静态标为 Public，由 Identity Handler 强制校验 Origin／CSRF 并完成会话撤销。其余 `/api/user/**` 继续 Authenticated，`/api/admin/**` 继续 PermissionControlled；`GET /api/user/context` 继续要求有效 Access 身份。验证码与注册保持原协议。
- **HTTP 契约**：统一四字段信封保持不变；登录 data 为 `access_token / session_id / user`，续期 data 为 `access_token / session_id`，`session_id` 仅是非授权性的会话关联标识。使用 `admin_refresh` Cookie、认证控制请求 Header `X-Admin-Request: 1` 和续期 Header `X-Refresh-Attempt-ID`。新增唯一公开错误 `403 / AUTHN_CSRF_INVALID` 与 `409 / AUTHN_REFRESH_SUPERSEDED`，后者只表示已有幂等结果被更高代取代，禁止返回旧 Cookie 覆盖新代；不改变已有错误码所有权。
- **状态与模型**：新增 Identity 登录会话、续期代和短期幂等结果模型；JWT 增加不可省略的会话绑定和 Refresh 凭据标识。新增 Redis 命名空间 `auth:session:<sid>` 与 `auth:refresh-attempt:<sid>:<attempt-id>`；不用原始 JWT 作新 key，不新建 MySQL 表，不批量提升 `user_access_versions`。已有验证码、失败计数、锁定 key 保持原样。
- **浏览器与配置**：只允许会话 Cookie 持有 Refresh Token；Web Storage 仅新增不含凭据、用户资料或权限的协调与退出待确认标记。新增明确的可信页面 Origin 和仅回环开发 HTTP 例外配置；Cookie 不设置长期 Expires／Max-Age。具体名称、原子性和能力前提见 design。
- **保持不变**：MinIO bucket、业务表、管理员 Permission Code、其他路由访问等级、已保存 API Metadata 管理配置及既有运行日志事件名均不改。API Metadata 默认值和 OpenAPI 消费同一新 Snapshot，不回填历史 Public 记录。
- **交付**：提案阶段只生成 change-local 工件；实施时迁移所有仓库调用者，更新 README、认证／配置运行说明、领域术语及相关 ADR。完成真实 MySQL／Redis／HTTP／浏览器联合验收后，同步协议主规格及经确认的架构归属；归档另行执行，不沿用此前四项修复的 `--skip-specs` 策略。
