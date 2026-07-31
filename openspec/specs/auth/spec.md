# auth Specification

## Purpose

认证模块负责后台系统的验证码、注册、登录、JWT 校验、登出黑名单和登录防刷保护。它定义公开入口、登录安全约束、令牌签发内容，以及已登出 token 被拒绝的行为。

## Requirements

### Requirement: 验证码生成
系统 SHALL 提供公开验证码接口，生成一次性数字图片验证码，并将验证码答案短期存入 Redis。

#### Scenario: 客户端请求验证码
- **WHEN** 客户端调用 `GET /api/captcha`
- **THEN** 系统返回验证码 ID 和 base64 图片内容
- **AND** 系统将验证码答案存入 Redis 的 `captcha:<id>`
- **AND** 验证码在 5 分钟后自动过期

### Requirement: 验证码校验
注册和登录 SHALL 提交有效验证码，并且验证码无论校验成功还是失败都必须被消费。

#### Scenario: 验证码正确
- **WHEN** 用户提交匹配的 `captcha_id` 和 `captcha_code`
- **THEN** 系统继续执行注册或登录流程
- **AND** 系统从 Redis 删除该验证码答案

#### Scenario: 验证码错误或过期
- **WHEN** 用户提交错误、已使用或已过期的验证码
- **THEN** 系统拒绝本次请求
- **AND** 用户必须重新获取验证码后才能重试

### Requirement: 密码复杂度
系统 SHALL 在注册和修改密码时校验密码复杂度。

#### Scenario: 密码满足规则
- **WHEN** 密码长度不少于 6 位
- **AND** 密码包含大写字母、小写字母、数字、特殊符号中的至少 3 类
- **THEN** 系统允许继续后续流程

#### Scenario: 密码不满足规则
- **WHEN** 密码长度少于 6 位，或字符类型少于 3 类
- **THEN** 系统拒绝请求并返回校验错误

### Requirement: 登录锁定
系统 SHALL 基于用户名使用 Redis 记录登录失败次数，并在达到阈值后进行渐进式锁定。

#### Scenario: 未达到锁定阈值时登录失败
- **WHEN** 用户提交有效验证码但密码错误，且连续失败次数少于 5 次
- **THEN** 系统递增 `fail:<username>`
- **AND** 系统返回模糊的用户名或密码错误提示，并提示剩余尝试次数
- **AND** 系统不暴露用户名是否存在

#### Scenario: 登录失败达到锁定阈值
- **WHEN** 登录失败次数达到配置阈值
- **THEN** 系统在 Redis 中设置 `lock:<username>`
- **AND** 锁定存在期间拒绝继续登录

#### Scenario: 账号处于锁定状态
- **WHEN** Redis 中存在 `lock:<username>`
- **THEN** 系统拒绝登录请求
- **AND** 系统返回剩余锁定时间

#### Scenario: 登录成功
- **WHEN** 验证码正确、账号启用、密码匹配
- **THEN** 系统清理 `fail:<username>` 和 `lock:<username>`
- **AND** 系统签发 access token 和 refresh token

### Requirement: JWT 会话
系统 SHALL 在登录成功后签发包含用户 ID、token 版本、角色码、权限码的 JWT。

#### Scenario: 登录成功后返回令牌
- **WHEN** 用户登录成功
- **THEN** 响应包含 access token、refresh token 和脱敏后的用户信息
- **AND** access token 可用于认证受保护的 `/api/user` 和 `/api/admin` 请求

### Requirement: Token 实时失效
系统 SHALL 使用 Authorization 持有的用户授权版本使权限和账号状态变更后的旧
Token 失效。

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
- **AND** 系统 SHALL NOT 回退其他授权版本存储

#### Scenario: 认证时授权版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到 `user_access_versions`
- **THEN** 系统拒绝该 Token
- **AND** 系统不跳过授权版本检查

#### Scenario: 权限相关变更提升版本
- **WHEN** 用户密码、用户状态、角色分配、角色权限、角色菜单、菜单记录或组织成员归属发生影响登录态的变更
- **THEN** 系统通过 `EnsureAndIncrement` 提升受影响用户的授权版本
- **AND** 这些用户已持有的旧 Access Token 和 Refresh Token 在下一次使用时失效

### Requirement: 登出黑名单
系统 SHALL 通过 Redis 黑名单使已登出的 access token 失效。

#### Scenario: 用户登出
- **WHEN** 已认证用户调用 `POST /api/user/logout`
- **THEN** 系统将当前 token 写入 Redis 的 `blacklist:<token>`
- **AND** 黑名单 TTL 与 token 剩余有效期对齐

#### Scenario: 复用已登出的 token
- **WHEN** 请求携带已存在于黑名单中的 token
- **THEN** JWT 中间件拒绝该请求


