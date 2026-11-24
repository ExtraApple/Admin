# user-management Specification Delta

## ADDED Requirements

### Requirement: 管理员创建用户
系统 SHALL 提供受 `admin.users.post` 权限码保护的 `POST /api/admin/users` 接口，允许具有 `all` 数据范围的管理员直接创建用户身份，不依赖公开注册验证码，也不触发验证邮件发送。该接口 SHALL 只创建用户身份本身，SHALL NOT 写入角色或组织归属，旧兼容列 `users.role` SHALL 固定写入 `"user"` 且不可由请求指定。新设密码 SHALL 复用与公开注册、修改密码一致的共享密码策略（至少 6 个 Unicode 字符、最多 72 个 UTF-8 字节、包含大写/小写/数字/特殊字符中的至少三类）。

#### Scenario: 全量数据范围管理员创建用户成功
- **WHEN** 具有 `admin.users.post` 权限且当前数据范围为 `all` 的管理员向 `POST /api/admin/users` 提交唯一用户名（3–100 字符）、唯一合法邮箱（最多 100 字符）、符合共享密码策略的密码，以及可选昵称（最多 100 字符）和状态（`0` 或 `1`，省略时默认 `1`）
- **THEN** 系统 SHALL 使用 bcrypt 存储密码哈希，创建固定 `role = "user"` 且 `email_verified_at = NULL`（未验证）的用户记录，并在同一事务中确保该用户具有有效初始授权版本
- **AND** 系统 SHALL NOT 触发 SMTP 验证邮件发送
- **AND** 系统 SHALL 返回 HTTP 200 及与 `GET /api/admin/users/:id` 一致的脱敏 `AdminUser` 结构（包含 `id`、默认头像 `/api/avatars/default`、空 `roles[]`、空 `organizations[]`、`has_unmanaged_organizations: false` 和 `access_version`），且永远不包含密码哈希

#### Scenario: 非全量数据范围管理员被拒绝创建用户
- **WHEN** 操作者持有 `admin.users.post` 权限，但其当前数据范围不是 `all`（包括在并发降权后不再具备 `all` 范围）
- **THEN** 系统 SHALL 返回 HTTP 403 和 `IDENTITY_PERMISSION_DENIED`
- **AND** 系统 SHALL NOT 创建用户记录，且 SHALL NOT 通过冲突错误暴露用户名或邮箱是否已被占用

#### Scenario: 用户名或邮箱与现存记录、待验证邮箱或软删记录冲突
- **WHEN** 全量数据范围管理员提交的用户名或邮箱已被现有用户（含软删除用户的唯一索引占用）或其他用户的 `pending_email` 占用
- **THEN** 系统 SHALL 返回 HTTP 409 和 `IDENTITY_CONFLICT`
- **AND** 系统 SHALL NOT 创建重复用户或把唯一约束冲突返回为 HTTP 500

#### Scenario: 请求结构或密码策略不合法
- **WHEN** 请求缺少必填字段、用户名长度不在 3–100、密码不足 6 字符、邮箱格式或长度不合法、昵称超长或状态不是 `0`/`1`
- **THEN** 系统 SHALL 按既有结构校验分层返回 HTTP 400 和 `HTTP_REQUEST_INVALID`
- **WHEN** 请求通过结构校验，但密码不满足复杂度规则或超过 72 个 UTF-8 字节，或用户名/非空昵称为全空白
- **THEN** 系统 SHALL 返回 HTTP 422 和 `IDENTITY_VALIDATION_INVALID`（密码错误携带 `password / IDENTITY_PASSWORD_INVALID` 字段错误）
- **AND** 系统 SHALL NOT 调用 bcrypt Hash 或创建用户
