## MODIFIED Requirements

### Requirement: WebSocket 刷新提示

系统 SHALL 通过一次性、60 秒有效且只存储摘要的 WebSocket ticket 建立已认证 Gateway。ticket SHALL 是 `GET /api/user/messages/ws` 握手升级的唯一身份凭据：该路由 SHALL 使用现有 `Public` 访问等级，不经过统一 Bearer 中间件，但 Handler SHALL 原子消费 ticket 并取得绑定用户；`Public` 不代表匿名。签发 ticket 的 `POST /api/user/messages/ws-ticket` 仍 SHALL 要求已认证 Bearer。升级 SHALL 校验精确的可信 Origin，并重新检查 ticket 绑定用户仍处于启用状态。刷新 payload 只包含事件 ID、游标、消息副本引用和用户引用，不得包含正文、HTML、图片、URL 或 Token。Gateway SHALL 支持 24 小时 Redis Stream 游标恢复、事件 ID 幂等和超窗全量刷新控制事件；慢连接关闭不得删除 MySQL 事实或恢复事件。

#### Scenario: 有效 ticket 升级

- **WHEN** 已认证用户申请一次性 ticket，并使用该 ticket 从可信 Origin 发起 WebSocket 升级
- **THEN** 系统 SHALL 原子消费 ticket、绑定 ticket 所属用户并返回 `101 Switching Protocols`
- **AND** 连接 SHALL NOT 要求浏览器提供 `Authorization` header

#### Scenario: 无效、过期或已消费 ticket

- **WHEN** 客户端使用不存在、已过期、已消费或属于其他用户的 ticket 发起升级
- **THEN** 系统 SHALL 在协议升级前返回 `401 MSG_WS_TICKET_INVALID`
- **AND** Redis ticket 不存在 SHALL NOT 被归类为内部错误或返回 500

#### Scenario: 不可信 Origin 或停用用户

- **WHEN** 升级请求来自非 `browser_session.origin` 的 Origin，或 ticket 所属用户已被停用
- **THEN** 系统 SHALL 拒绝升级且不建立 WebSocket 连接
- **AND** 不得以匿名方式继续连接

#### Scenario: 收件箱刷新提示

- **WHEN** 消息状态变化导致收件箱可能变化
- **THEN** 系统 SHALL 向已认证客户端提供不含正文的可恢复刷新提示

#### Scenario: 断线恢复

- **WHEN** 客户端携带最后游标重新申请 ticket 并升级
- **THEN** 系统 SHALL 按当前用户可见范围补发 24 小时内的事件
- **AND** 重复事件 SHALL 保持幂等
