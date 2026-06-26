# user-management Specification

## Purpose

用户管理覆盖账号注册、登录返回的用户信息、个人资料维护、头像上传、修改密码、前端初始化上下文，以及管理员对用户的管理操作。

## Requirements

### Requirement: 用户注册
系统 SHALL 允许用户在通过验证码和密码策略校验后公开注册账号。

#### Scenario: 注册成功
- **WHEN** 请求向 `POST /api/register` 提交唯一用户名、唯一邮箱、有效密码和有效验证码
- **THEN** 系统创建一个角色为 `user`、状态为 `1` 的用户
- **AND** 系统使用 bcrypt 存储密码
- **AND** 响应返回不包含密码的脱敏用户信息

#### Scenario: 用户名或邮箱已存在
- **WHEN** 请求提交的用户名或邮箱已被其他用户使用
- **THEN** 系统拒绝注册

### Requirement: 当前用户资料
系统 SHALL 允许已认证用户读取和修改自己的非敏感资料字段。

#### Scenario: 查询当前用户资料
- **WHEN** 已认证用户调用 `GET /api/user/info`
- **THEN** 系统返回脱敏后的用户信息
- **AND** 响应永远不包含密码哈希

#### Scenario: 修改当前用户资料
- **WHEN** 已认证用户调用 `PUT /api/user/info` 并提交昵称、邮箱或头像
- **THEN** 系统只更新允许修改的字段
- **AND** 如果邮箱已被其他用户使用，系统拒绝请求
- **AND** 如果没有任何可更新字段，系统拒绝请求

### Requirement: 修改密码
系统 SHALL 允许已认证用户在验证旧密码后修改密码。

#### Scenario: 修改密码成功
- **WHEN** 旧密码正确
- **AND** 新密码和确认密码一致
- **AND** 新密码与旧密码不同
- **AND** 新密码满足密码复杂度规则
- **THEN** 系统存储新的 bcrypt 密码哈希

#### Scenario: 修改密码失败
- **WHEN** 任一密码条件不满足
- **THEN** 系统拒绝修改密码

### Requirement: 头像上传
系统 SHALL 允许已认证用户上传头像图片，并将头像存储到 MinIO。

#### Scenario: 头像上传成功
- **WHEN** 用户向 `POST /api/user/avatar` 上传受支持的图片文件
- **THEN** 系统将图片存入 `image` bucket
- **AND** 系统更新用户头像 URL
- **AND** 响应返回更新后的脱敏用户信息

#### Scenario: 头像不满足约束
- **WHEN** 上传文件过大或格式不受支持
- **THEN** 系统拒绝上传

### Requirement: 前端初始化上下文
系统 SHALL 提供一个已认证接口，一次性返回前端初始化所需数据。

#### Scenario: 客户端请求初始化上下文
- **WHEN** 已认证用户调用 `GET /api/user/context`
- **THEN** 系统返回脱敏用户信息
- **AND** 系统返回 JWT 上下文中的角色码和权限码
- **AND** 系统返回该用户可访问的启用菜单树

### Requirement: 管理员用户列表
系统 SHALL 允许管理员分页查询用户列表。

#### Scenario: 管理员查询用户列表
- **WHEN** 管理员调用 `GET /api/admin/users`
- **THEN** 系统返回分页后的脱敏用户列表
- **AND** 响应包含总数、页码和每页数量

### Requirement: 管理员修改用户
系统 SHALL 允许管理员修改非保护用户，同时保护自己和管理员账号不被管理端修改。

#### Scenario: 管理员修改普通用户
- **WHEN** 管理员修改另一个非管理员用户的昵称、邮箱、角色或状态
- **THEN** 系统应用提交的字段
- **AND** 响应返回更新后的脱敏用户信息

#### Scenario: 管理员尝试修改自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝修改

### Requirement: 管理员删除用户
系统 SHALL 允许管理员软删除非保护用户。

#### Scenario: 管理员删除普通用户
- **WHEN** 管理员删除另一个非管理员用户
- **THEN** 系统软删除该用户记录

#### Scenario: 管理员尝试删除自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝删除

### Requirement: 管理员切换用户状态
系统 SHALL 允许管理员切换非保护用户的启用状态。

#### Scenario: 管理员切换普通用户状态
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/status`
- **THEN** 系统将状态从 `1` 改为 `0`，或从 `0` 改为 `1`

#### Scenario: 管理员尝试切换自己或其他管理员状态
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作


