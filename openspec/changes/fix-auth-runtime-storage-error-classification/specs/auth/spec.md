## MODIFIED Requirements

### Requirement: Token 实时失效
系统 SHALL 使用 Authorization 持有的用户授权版本使权限和用户状态变更后的旧 Token 失效，并通过稳定认证错误契约拒绝无效会话。

Access 认证 SHALL 将“唯一授权版本行缺失”分类为不可验证会话，而非内部故障；仅该分类以及明确确认的用户不存在身份，才映射为认证拒绝。数据库连接、查询、表不可用或其他真实基础设施失败 SHALL 继续返回 HTTP 500 和稳定 `IDENTITY_INTERNAL_ERROR`，响应 `data` SHALL 为 `null`，不得泄露底层信息、伪装成凭据无效或成功。

#### Scenario: Token 版本一致
- **WHEN** 已认证请求携带用途为 Access Token 的有效 JWT
- **AND** JWT 中的 `token_version` 等于 `user_access_versions.version`
- **AND** 用户状态为启用
- **THEN** JWT 中间件允许请求继续

#### Scenario: Token 版本不一致
- **WHEN** Access Token 或 Refresh Token 中的 `token_version` 与 `user_access_versions.version` 不一致，或令牌用途不符合请求场景
- **THEN** 系统拒绝该 Token
- **AND** Access Token 请求 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 用户需要重新登录获取新 Token

#### Scenario: 用户被禁用或删除
- **WHEN** Access Token 对应用户不存在或状态不是启用
- **THEN** JWT 中间件拒绝请求
- **AND** 不存在或不可验证身份 SHALL 返回 HTTP 401 和稳定认证错误码
- **AND** 已建立身份但账号被禁用 SHALL 返回 HTTP 403 和稳定 `IDENTITY_*` 错误码

#### Scenario: Access 身份查询真实基础设施失败
- **WHEN** Access 认证查询用户身份时发生数据库连接、查询、表不可用或其他真实基础设施故障
- **AND** 故障不是已确认的用户不存在
- **THEN** JWT 中间件 SHALL 返回 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** `data` SHALL 为 `null`
- **AND** 响应不得包含底层数据库、驱动或连接错误
- **AND** 系统不得将该故障伪装为 `AUTHN_TOKEN_INVALID` 或签发新 Token

#### Scenario: 登录初始化新用户版本
- **WHEN** 用户通过身份和密码校验但 `user_access_versions` 不存在
- **THEN** 系统幂等创建版本 1
- **AND** 系统重新读取当前版本
- **AND** 系统签发包含该版本的 Access Token 和 Refresh Token

#### Scenario: 登录初始化失败
- **WHEN** 系统无法创建或读取用户授权版本
- **THEN** 系统拒绝登录
- **AND** 系统不签发 Token
- **AND** 响应 SHALL 使用 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** 响应不得包含底层数据库错误

#### Scenario: 使用 Refresh Token 刷新
- **WHEN** 客户端向 `POST /api/refresh` 提交用途、签名、有效期、用户状态和授权版本均有效的 `refresh_token`
- **THEN** 系统返回新的 Access Token 和 Refresh Token
- **AND** 两个新 Token SHALL 使用 `user_access_versions` 中的当前版本并携带对应用途
- **AND** 成功响应 SHALL 使用 `code=200`、空 `error_code`、`msg=success` 和 `data.access_token`、`data.refresh_token`

#### Scenario: Refresh Token 刷新失败
- **WHEN** Refresh Token 无效、过期、用途错误、用户不存在、用户被禁用、新表缺行或授权版本不一致
- **THEN** `POST /api/refresh` SHALL 返回 HTTP 401 和 `AUTHN_REFRESH_TOKEN_INVALID`
- **AND** 安全英文消息 SHALL NOT 区分具体拒绝原因
- **AND** `data` SHALL 为 `null`
- **AND** 系统 SHALL NOT 签发新 Token
- **AND** 系统 SHALL NOT 回退其他授权版本存储

#### Scenario: Refresh 真实基础设施失败保持既有拒绝契约
- **WHEN** Refresh Token 校验过程中发生数据库、Redis、连接、查询或其他真实基础设施故障
- **THEN** `POST /api/refresh` SHALL 返回 HTTP 401 和 `AUTHN_REFRESH_TOKEN_INVALID`
- **AND** `data` SHALL 为 `null`
- **AND** 安全英文消息 SHALL NOT 区分具体故障原因
- **AND** 系统 SHALL NOT 签发新 Token

#### Scenario: 认证时授权版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到 `user_access_versions`
- **THEN** 系统拒绝该 Token
- **AND** Access 请求返回 HTTP 401 和 `AUTHN_TOKEN_INVALID`，Refresh 按其既有稳定拒绝契约处理
- **AND** 系统不跳过授权版本检查

#### Scenario: 权限相关变更提升版本
- **WHEN** 用户密码、用户状态、角色分配、角色权限、角色菜单、菜单记录或组织成员归属发生影响登录态的变更
- **THEN** 系统通过 `EnsureAndIncrement` 提升受影响用户的授权版本
- **AND** 这些用户已持有的旧 Access Token 和 Refresh Token 在下一次使用时失效

#### Scenario: Access 缺版本不补行
- **WHEN** 用途、签名、有效期和用户状态合法的 Access JWT 对应 `user_access_versions` 行不存在
- **THEN** 系统 SHALL 返回 HTTP 401 和 `AUTHN_TOKEN_INVALID`
- **AND** 非登录认证不得 Ensure、跳过版本检查、读取旧 `users.token_version` 或签发新 Token

#### Scenario: Access 读取真实基础设施失败
- **WHEN** Access 认证读取版本发生数据库连接、查询、表不可用或其他真实基础设施故障而不是已确认缺行
- **THEN** 系统 SHALL 返回 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** `data` SHALL 为 `null`
- **AND** 不得回退其他存储、创建版本或签发新 Token
- **AND** 响应不得包含底层错误

#### Scenario: Refresh 缺行保持既有结果
- **WHEN** 有效 Refresh JWT 的唯一授权版本行缺失
- **THEN** 系统 SHALL 继续返回 HTTP 401 与 `AUTHN_REFRESH_TOKEN_INVALID`，且不签发 Token
