## MODIFIED Requirements

### Requirement: 登出黑名单
系统 SHALL 通过 Redis 黑名单使已登出的 access token 失效。

#### Scenario: 用户登出
- **WHEN** 已认证用户调用 `POST /api/user/logout`
- **THEN** 系统将当前 token 写入 Redis 的 `blacklist:<token>`
- **AND** 黑名单 TTL SHALL 取 `jwt.expire` 配置的固定时长，而非该 token 的剩余有效期

#### Scenario: 黑名单 TTL 长于剩余有效期的情形
- **WHEN** 登出的 token 剩余有效期短于 `jwt.expire`
- **THEN** 系统 SHALL 仍按 `jwt.expire` 设置黑名单 TTL
- **AND** 该 token 在自然过期后由签名与过期校验拒绝，黑名单条目 SHALL NOT 影响该结果
- **AND** 黑名单条目 SHALL 在 TTL 到期后由 Redis 自动清除

#### Scenario: 复用已登出的 token
- **WHEN** 请求携带已存在于黑名单中的 token
- **THEN** JWT 中间件拒绝该请求
