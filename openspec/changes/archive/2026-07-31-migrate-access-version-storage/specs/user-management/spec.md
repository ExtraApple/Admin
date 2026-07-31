## MODIFIED Requirements

### Requirement: 管理员修改用户
系统 SHALL 允许管理员修改非保护用户，同时保护自己和管理员用户不被管理端修改。

#### Scenario: 管理员修改普通用户
- **WHEN** 管理员修改另一个非管理员用户的昵称、邮箱、角色或状态
- **THEN** 系统应用提交的字段
- **AND** 响应返回更新后的脱敏用户信息
- **AND** 当角色或状态变化影响登录态时，系统 SHALL 创建或提升目标用户授权版本
- **AND** 系统使目标用户旧 Access Token 和 Refresh Token 失效

#### Scenario: 管理员尝试修改自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝修改

### Requirement: 管理员删除用户
系统 SHALL 允许管理员软删除非保护用户，并防止删除前 Token 在用户恢复后重新生效。

#### Scenario: 管理员删除普通用户
- **WHEN** 管理员删除另一个非管理员用户
- **THEN** 系统 SHALL 在同一数据库事务中软删除用户并执行 `EnsureAndIncrement`
- **AND** 系统 SHALL 保留该用户的授权版本记录
- **AND** 任一步失败时系统 SHALL 回滚用户删除和授权版本变化

#### Scenario: 软删除用户被恢复
- **WHEN** 系统恢复已软删除用户
- **THEN** 系统 SHALL 沿用删除时已经提升的授权版本
- **AND** 删除前签发的 Access Token 和 Refresh Token SHALL NOT 重新生效

#### Scenario: 管理员尝试删除自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝删除

### Requirement: 管理员切换用户状态
系统 SHALL 允许管理员切换非保护用户的启用状态。

#### Scenario: 管理员切换普通用户状态
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/status`
- **THEN** 系统将状态从 `1` 改为 `0`，或从 `0` 改为 `1`
- **AND** 系统 SHALL 通过 `EnsureAndIncrement` 创建或提升目标用户授权版本
- **AND** 系统使目标用户旧 Access Token 和 Refresh Token 失效

#### Scenario: 管理员尝试切换自己或其他管理员状态
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作

### Requirement: 管理员强制用户下线
系统 SHALL 允许管理员主动使非保护用户的旧 Token 失效。

#### Scenario: 管理员强制普通用户下线
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/kick`
- **THEN** 系统 SHALL 通过 `EnsureAndIncrement` 创建或提升目标用户授权版本
- **AND** 系统不修改目标用户资料和用户状态
- **AND** 目标用户旧 Access Token 和 Refresh Token 在后续请求中失效

#### Scenario: 管理员尝试强制下线自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作

