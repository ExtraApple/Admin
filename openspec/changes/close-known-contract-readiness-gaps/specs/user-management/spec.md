## MODIFIED Requirements

### Requirement: 管理员修改用户
系统 SHALL 允许管理员修改非保护用户，同时保护自己和管理员用户不被管理端修改。管理员用户更新 SHALL NOT 代改邮箱；邮箱所有权变更由用户本人经密码校验与邮箱验证流程完成。

#### Scenario: 管理员修改普通用户
- **WHEN** 管理员修改另一个非管理员用户的昵称、角色或状态
- **THEN** 系统应用提交的字段
- **AND** 响应返回更新后的脱敏用户信息
- **AND** 当角色或状态变化影响登录态时，系统 SHALL 创建或提升目标用户授权版本
- **AND** 系统使目标用户旧 Access Token 和 Refresh Token 失效

#### Scenario: 管理员尝试修改自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝修改

#### Scenario: 管理员提交邮箱更新
- **WHEN** 管理员更新另一个可管理且非保护的用户，并提交非空 `email`
- **THEN** 系统 SHALL 拒绝整次更新，并返回既有邮箱字段错误 `IDENTITY_ADMIN_EMAIL_NOT_WRITABLE`
- **AND** 系统 SHALL NOT 改变目标用户的邮箱、待验证邮箱或邮箱验证状态
- **AND** 其他同时提交的资料字段 SHALL NOT 部分生效，目标用户授权版本 SHALL NOT 因该被拒绝请求而改变
