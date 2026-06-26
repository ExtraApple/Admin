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
系统 SHALL 在登录成功后签发包含用户 ID、角色码、权限码的 JWT。

#### Scenario: 登录成功后返回令牌
- **WHEN** 用户登录成功
- **THEN** 响应包含 access token、refresh token 和脱敏后的用户信息
- **AND** access token 可用于认证受保护的 `/api/user` 和 `/api/admin` 请求

### Requirement: 登出黑名单
系统 SHALL 通过 Redis 黑名单使已登出的 access token 失效。

#### Scenario: 用户登出
- **WHEN** 已认证用户调用 `POST /api/user/logout`
- **THEN** 系统将当前 token 写入 Redis 的 `blacklist:<token>`
- **AND** 黑名单 TTL 与 token 剩余有效期对齐

#### Scenario: 复用已登出的 token
- **WHEN** 请求携带已存在于黑名单中的 token
- **THEN** JWT 中间件拒绝该请求


