# user-management Specification Delta

## ADDED Requirements

### Requirement: 管理员查询已软删除用户列表
系统 SHALL 提供受 `admin.deleted-users.get` 权限码保护的 `GET /api/admin/deleted-users?page=&size=&keyword=` 接口，允许具有 `all` 数据范围的管理员分页查询处于可恢复软删除状态的用户（`deleted_at IS NOT NULL AND anonymized_at IS NULL`），按 `deleted_at desc, id desc` 排序。列表 SHALL 返回脱敏用户资料、删除时间 `deleted_at`、当前保留的真实 `roles[]` 与 `organizations[]`、受保护标志 `protected` 与 `access_version`，SHALL NOT 返回密码哈希、可信头像对象键或摘要，也 SHALL NOT 包含活跃用户或已匿名化墓碑。

#### Scenario: 全量数据范围管理员查询可恢复软删除用户
- **WHEN** 具有 `admin.deleted-users.get` 权限且当前数据范围为 `all` 的管理员调用 `GET /api/admin/deleted-users` 并可选提供 `keyword`、`page`、`size`
- **THEN** 系统 SHALL 仅返回 `deleted_at IS NOT NULL AND anonymized_at IS NULL` 且匹配用户名、昵称或邮箱关键词的分页记录及对应 `total`
- **AND** 每项头像 SHALL 返回 `/api/avatars/default`，且响应永远不包含密码哈希或头像存储元数据
- **AND** 关联了 `admin` 角色的记录 SHALL 标记 `protected = true`

#### Scenario: 已匿名化墓碑不出现在回收站列表
- **WHEN** 某用户已完成受控匿名化（`anonymized_at IS NOT NULL`）
- **THEN** `GET /api/admin/deleted-users` SHALL NOT 返回该墓碑记录，且 `total` SHALL NOT 计入该记录

#### Scenario: 非全量数据范围管理员被拒绝查询已软删除用户
- **WHEN** 操作者持有 `admin.deleted-users.get` 权限，但其当前数据范围不是 `all`
- **THEN** 系统 SHALL 返回 HTTP 403 和 `IDENTITY_PERMISSION_DENIED`
- **AND** 系统 SHALL NOT 泄露任何已软删除用户的身份或归属信息

### Requirement: 管理员恢复软删除用户
系统 SHALL 提供受 `admin.users.id.restore.post` 权限码保护的 `POST /api/admin/users/:id/restore` 接口，允许具有 `all` 数据范围的管理员将非本人、非受保护且处于可恢复软删除状态的用户恢复为活跃状态（`deleted_at = NULL`）。恢复操作 SHALL 沿用软删除时已经提升的 `access_version`，保持 `pending_email` 为空与既有已清理的关系事实，返回与 `GET /api/admin/users/:id` 一致的脱敏 `AdminUser`。

#### Scenario: 全量数据范围管理员恢复软删除普通用户
- **WHEN** 具有 `admin.users.id.restore.post` 权限且在事务锁内确认当前数据范围为 `all` 的管理员对一个非本人、未绑定 `admin` 角色且处于可恢复软删除状态的用户调用 `POST /api/admin/users/:id/restore`
- **THEN** 系统 SHALL 在单一数据库事务中清除目标用户的 `deleted_at`，保留其既有 `status`、`email`、`email_verified_at` 与当前仍保留的角色/组织关系
- **AND** 系统 SHALL 沿用删除时已提升的 `access_version`，删除前签发的 Access Token、Refresh Token 与未消费邮箱验证凭据 SHALL NOT 重新生效
- **AND** 软删期间已被受控清理的角色或组织归属 SHALL NOT 被自动恢复
- **AND** 响应 SHALL 返回 HTTP 200 及脱敏 `AdminUser`

#### Scenario: 恢复活跃用户或已匿名化墓碑被拒绝
- **WHEN** 目标用户当前处于活跃状态（`deleted_at IS NULL`）或已完成受控匿名化（`anonymized_at IS NOT NULL`）
- **THEN** 系统 SHALL 返回 HTTP 409 和 `IDENTITY_CONFLICT`
- **AND** 系统 SHALL NOT 修改目标用户状态、归属或授权版本

#### Scenario: 恢复不存在用户、本人、受保护身份或非全量范围被拒绝
- **WHEN** 目标用户 ID 不存在
- **THEN** 系统 SHALL 返回 HTTP 404 和 `IDENTITY_USER_NOT_FOUND`
- **WHEN** 目标用户为操作者本人
- **THEN** 系统 SHALL 返回 HTTP 422 和 `IDENTITY_VALIDATION_INVALID`
- **WHEN** 操作者当前数据范围不是 `all`（含并发降权）或目标用户绑定了 `admin` 角色（受保护身份）
- **THEN** 系统 SHALL 返回 HTTP 403 和 `IDENTITY_PERMISSION_DENIED`
- **AND** 系统 SHALL NOT 改变任何用户记录或授权版本

### Requirement: 管理员受控匿名化已软删除用户
系统 SHALL 提供受 `admin.users.id.anonymize.post` 权限码保护的 `POST /api/admin/users/:id/anonymize` 接口，允许具有 `all` 数据范围的管理员对非本人、非受保护且已软删除但尚未匿名化的用户执行不可逆受控匿名化。请求体 SHALL 必填 `confirm_username` 并与事务锁内目标当前 `username` 精确一致。匿名化 SHALL 保留 `users.id` 墓碑行（维持 `audit_logs`、`audit_log_archives`、`files`、`messages` 系列表引用完整）与 `user_access_versions` 行，并在同一数据库事务中释放原 `username` 与 `email` 唯一键、清理 PII 与关联归属；事务提交后再对原可信自定义头像执行受控存储清理。

#### Scenario: 全量数据范围管理员受控匿名化软删除普通用户
- **WHEN** 具有 `admin.users.id.anonymize.post` 权限且在事务锁内确认当前数据范围为 `all` 的管理员对非本人、非受保护且处于可恢复软删除状态的用户提交匹配的 `confirm_username`
- **THEN** 系统 SHALL 在同一数据库事务中：
  - 移除该用户的全部 `user_roles` 与 `user_organizations` 归属关系
  - 将该用户全部剩余 `used_at IS NULL` 的 `email_verification_credentials` 标记为已失效
  - 将 `username` 改写为唯一不可逆占位值 `deleted_<id>_<unix_nano>`、将 `email` 改写为唯一不可逆占位值 `deleted_<id>_<unix_nano>@deleted.invalid`，从而立即释放原用户名与原邮箱的唯一索引占用
  - 将 `password` 置为不可匹配哨兵值 `"!"`，清空 `nickname`、`pending_email`、`email_verified_at` 及全部头像字段（含 `avatar`、`avatar_object_name`、`avatar_content_type`、`avatar_content_sha256`、`avatar_validation_status`、`avatar_validated_at`），将 `status` 置为 `0`，写入 `anonymized_at` 并保持 `deleted_at` 非空
  - 对 `user_access_versions` 执行 `EnsureAndIncrement` 并永久保留该版本记录
- **AND** 事务任一步失败时，系统 SHALL 完整回滚上述全部数据库变更，且 SHALL NOT 提前删除对象存储中的头像对象
- **AND** 匿名化提交后，新用户注册、管理员创建用户与邮箱换绑 SHALL 可以重新使用原用户名与原邮箱，且历史 `audit_logs`、`files`、`messages` 对该 `user_id` 的引用保持完整

#### Scenario: 匿名化提交后受控清理可信头像与忽略历史未验证头像
- **WHEN** 受控匿名化数据库事务成功提交，且目标用户匿名化前具有经系统验证的可信自定义头像对象（`TrustedAvatarObjectName`）
- **THEN** 系统 SHALL 在事务提交后尝试删除私有 `image` bucket 中对应的可信头像对象
- **AND** 若对象删除失败，系统 SHALL 保持匿名化成功响应并记录不含 object key 或底层存储原始错误的受控运行日志
- **WHEN** 目标用户匿名化前仅有历史未验证 `avatar` URL 或默认头像
- **THEN** 系统 SHALL NOT 尝试从对象存储或外部地址删除该历史来源

#### Scenario: 确认用户名不匹配或请求结构无效
- **WHEN** 请求体缺少必填 `confirm_username` 或为空字符串
- **THEN** 系统 SHALL 返回 HTTP 400 和 `HTTP_REQUEST_INVALID`
- **WHEN** `confirm_username` 与事务锁内目标用户当前 `username` 不一致
- **THEN** 系统 SHALL 返回 HTTP 422、`IDENTITY_VALIDATION_INVALID` 及 `confirm_username / IDENTITY_CONFIRM_USERNAME_MISMATCH` 字段错误
- **AND** 系统 SHALL NOT 执行任何清理、改写或版本变更

#### Scenario: 禁止直接匿名化活跃用户或重复匿名化墓碑
- **WHEN** 目标用户仍处于活跃状态（`deleted_at IS NULL`）或已经完成匿名化（`anonymized_at IS NOT NULL`）
- **THEN** 系统 SHALL 返回 HTTP 409 和 `IDENTITY_CONFLICT`
- **AND** 系统 SHALL NOT 改写目标用户、清理归属或删除头像对象

#### Scenario: 匿名化不存在用户、本人、受保护身份或非全量范围被拒绝
- **WHEN** 目标用户 ID 不存在
- **THEN** 系统 SHALL 返回 HTTP 404 和 `IDENTITY_USER_NOT_FOUND`
- **WHEN** 目标用户为操作者本人
- **THEN** 系统 SHALL 返回 HTTP 422 和 `IDENTITY_VALIDATION_INVALID`
- **WHEN** 操作者当前数据范围不是 `all`（含并发降权）或目标用户绑定了 `admin` 角色（受保护身份）
- **THEN** 系统 SHALL 返回 HTTP 403 和 `IDENTITY_PERMISSION_DENIED`
- **AND** 系统 SHALL NOT 修改目标用户、角色/组织归属、授权版本或头像对象

#### Scenario: 启动 Seed 不恢复已匿名化墓碑
- **WHEN** 系统启动执行 `SeedAdminUser` 检查配置管理员用户
- **THEN** Seed 查询 SHALL 仅匹配 `anonymized_at IS NULL` 的记录，绝不恢复任何已匿名化墓碑

## MODIFIED Requirements

### Requirement: 管理员删除用户
系统 SHALL 允许管理员软删除非保护用户，并在同一数据库事务中清理待验证邮箱与未使用邮箱验证凭据，防止删除前会话 Token 和邮箱验证凭据在用户恢复后重新生效。软删除将用户移入可恢复软删除状态（`deleted_at IS NOT NULL, anonymized_at IS NULL`）；在经受控匿名化释放前，软删除用户的 `username` 与 `email` 仍保留唯一索引占用；用户一旦进入匿名化墓碑终态（`anonymized_at IS NOT NULL`），SHALL 不可恢复。

#### Scenario: 管理员删除普通用户
- **WHEN** 管理员删除另一个非管理员用户
- **THEN** 系统 SHALL 在同一数据库事务中清空该用户的 `pending_email`、将该用户全部 `used_at IS NULL` 的邮箱验证凭据标记为已失效、软删除用户并执行 `EnsureAndIncrement`
- **AND** 系统 SHALL 保留该用户的授权版本记录以及主邮箱与用户名的软删占位，并使其进入可恢复软删除状态
- **AND** 该清理 SHALL NOT 依赖 SMTP 是否启用，且 SHALL NOT 影响其他用户的凭据或已消费凭据的历史时间
- **AND** 任一步失败时系统 SHALL 回滚待验证邮箱清理、凭据失效、用户删除和授权版本变化

#### Scenario: 软删除用户被恢复
- **WHEN** 系统恢复处于可恢复软删除状态（`deleted_at IS NOT NULL, anonymized_at IS NULL`）的用户
- **THEN** 系统 SHALL 沿用删除时已经提升的授权版本
- **AND** 删除前签发的 Access Token、Refresh Token 与未消费邮箱验证凭据 SHALL NOT 重新生效

#### Scenario: 管理员尝试删除自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝删除
