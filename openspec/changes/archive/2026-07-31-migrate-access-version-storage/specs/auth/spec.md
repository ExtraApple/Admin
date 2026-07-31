## MODIFIED Requirements

### Requirement: Token 实时失效
系统 SHALL 使用 Authorization 持有的用户授权版本使权限和账号状态变更后的旧 Token 失效。

#### Scenario: Token 版本一致
- **WHEN** 已认证请求携带有效 JWT
- **AND** JWT 中的 `token_version` 等于 `user_access_versions.version`
- **AND** 用户状态为启用
- **THEN** JWT 中间件允许请求继续

#### Scenario: Token 版本不一致
- **WHEN** Access Token 或 Refresh Token 中的 `token_version` 与 `user_access_versions.version` 不一致
- **THEN** 系统拒绝该 Token
- **AND** 用户需要重新登录获取新 Token

#### Scenario: 账号被禁用或删除
- **WHEN** 已认证请求对应用户不存在或状态不是启用
- **THEN** JWT 中间件拒绝请求

#### Scenario: 登录初始化新用户版本
- **WHEN** 用户通过身份和密码校验但 `user_access_versions` 不存在
- **THEN** 系统幂等创建版本 1
- **AND** 系统重新读取当前版本
- **AND** 系统签发包含该版本的 Access Token 和 Refresh Token

#### Scenario: 登录初始化失败
- **WHEN** 系统无法创建或读取用户授权版本
- **THEN** 系统拒绝登录
- **AND** 系统不签发 Token

#### Scenario: 使用 Refresh Token 刷新
- **WHEN** 客户端向 `POST /api/refresh` 提交签名、有效期、用户状态和授权版本均有效的 `refresh_token`
- **THEN** 系统返回新的 Access Token 和 Refresh Token
- **AND** 两个新 Token SHALL 使用 `user_access_versions` 中的当前版本
- **AND** 成功响应 SHALL 保持 `code=200`、`msg=刷新成功` 和 `data.access_token`、`data.refresh_token`

#### Scenario: Refresh Token 刷新失败
- **WHEN** Refresh Token 无效、过期、用户不存在、用户被禁用、新表缺行或授权版本不一致
- **THEN** `POST /api/refresh` SHALL 返回 HTTP 401 和稳定 `code=401`
- **AND** 系统 SHALL NOT 签发新 Token
- **AND** 系统 SHALL NOT 回退读取 `users.token_version`

#### Scenario: 认证时授权版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到 `user_access_versions`
- **THEN** 系统拒绝该 Token
- **AND** 系统不回退读取 `users.token_version`

#### Scenario: 权限相关变更提升版本
- **WHEN** 用户密码、用户状态、角色分配、角色权限、角色菜单、菜单记录或组织成员归属发生影响登录态的变更
- **THEN** 系统通过 `EnsureAndIncrement` 提升受影响用户的授权版本
- **AND** 这些用户已持有的旧 Access Token 和 Refresh Token 在下一次使用时失效
