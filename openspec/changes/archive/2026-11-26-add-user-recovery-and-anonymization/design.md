## Context

当前 `identity` 模块对用户管理仅提供软删除（`DELETE /api/admin/users/:id`）：
1. 软删除在同一数据库事务中清空 `pending_email`、作废该用户全部 `used_at IS NULL` 的 `email_verification_credentials`、写入 `users.deleted_at` 并执行 `user_access_versions` 的 `EnsureAndIncrement`；软删后 `users.username` 与 `users.email` 继续保留唯一索引占用，公开注册、管理员建号、本人换绑与确认命中软删唯一键均返回 `409 IDENTITY_CONFLICT`。
2. 全库中 `audit_logs`、`audit_log_archives`、`files.uploader_id`、`messages`、`message_recipients`、`message_user_states`、`message_event_deliveries` 与 `user_access_versions` 均直接引用 `users.id`，且 `access-version-storage` 规格要求永不删除或级联删除授权版本记录。
3. 正式上线后，仅保留单向软删除存在三个结构性缺口：误删用户无法从工作台恢复；离职或测试用户确认注销后其 `username` 与 `email` 永久无法释放；个人资料 PII（昵称、原邮箱、密码哈希、可信头像对象）及残留 `user_roles` / `user_organizations` 归属缺少终态清理入口。

## Goals / Non-Goals

**Goals:**
- 建立清晰的**三态用户生命周期**：活跃（`deleted_at IS NULL, anonymized_at IS NULL`）→ 可恢复软删（`deleted_at IS NOT NULL, anonymized_at IS NULL`）→ 不可逆匿名化墓碑（`deleted_at IS NOT NULL, anonymized_at IS NOT NULL`）。
- 在 `identity` 模块以增量方式提供已软删除用户回收站查询、恢复与受控匿名化三个全量范围管理接口，遵循 `handler -> application service -> repository/ports` 分层与既定跨模块锁序。
- 匿名化在单一事务中原子清理归属、凭据、PII、头像元数据、改写唯一键占位并提升且保留 `user_access_versions`，事务提交后再受控清理私有 `image` bucket 中的可信自定义头像对象。
- 在 `web/` 管理工作台补齐 `/users/deleted` 回收站子路由（搜索、分页、恢复确认、手输用户名彻底注销确认），并同步 `CONTEXT.md`、Route Catalog、OpenAPI 与自动化测试。

**Non-Goals:**
- 不执行 `DELETE FROM users` 物理删除。
- 不修改、抹除或重算历史 `audit_logs` / `audit_log_archives` 记录与历史 `username` 快照。
- 不允许跳过软删除直接匿名化活跃用户，不允许恢复已匿名化墓碑。
- 不支持局部数据范围管理员访问或操作回收站，也不允许管理端恢复或匿名化操作者本人及受保护 `admin` 身份。
- 不在本变更中实现定时自动匿名化任务，不新增 Redis key 或 MinIO bucket。
- 不删除历史未验证外部头像 URL。

## Decisions

### 1. 用可空索引列 `users.anonymized_at` 显式区分可恢复软删与匿名化墓碑
- **方案**：在 `internal/identity/model.go` 的 `User` 模型新增 `AnonymizedAt *time.Time`（`gorm:"index;comment:受控匿名化完成时间"`），由既有 `AutoMigrate` 自动添加可空列。
  - 活跃用户：`deleted_at IS NULL AND anonymized_at IS NULL`
  - 可恢复软删用户：`deleted_at IS NOT NULL AND anonymized_at IS NULL`
  - 匿名化墓碑：`deleted_at IS NOT NULL AND anonymized_at IS NOT NULL`
- **数据迁移说明**：新增列为可空列，存量活跃与软删记录自动保持 `anonymized_at = NULL`，无需停机、无需历史数据回填脚本。
- **备选方案与取舍**：
  - 备选 B：通过 `username LIKE 'deleted_%'` 前缀推断是否已匿名化。放弃原因：字符串约定脆弱且与正常用户命名空间耦合，无法作为索引过滤条件与状态机事实源。
  - 备选 C：物理 `DELETE FROM users` 或拆分独立归档表。放弃原因：破坏 `audit_logs`、`files`、`messages`、`user_access_versions` 的 `user_id` 引用完整性，且与既有 `access-version-storage` 规格冲突。

### 2. 路由命名避免 Gin 前缀树冲突，统一收归 `all` 数据范围
- **方案**：新增 3 条 Route Catalog 权限受控路由（组 `identity`，默认审计分类 `identity`）：
  1. `GET /api/admin/deleted-users`（权限码 `admin.deleted-users.get`）：查询 `deleted_at IS NOT NULL AND anonymized_at IS NULL` 的分页列表（`page`、`size`、`keyword`），按 `deleted_at desc, id desc` 排序。
  2. `POST /api/admin/users/:id/restore`（权限码 `admin.users.id.restore.post`）：恢复软删除用户。
  3. `POST /api/admin/users/:id/anonymize`（权限码 `admin.users.id.anonymize.post`）：对已软删除用户执行不可逆匿名化，请求体必填 `{ "confirm_username": "..." }`。
- **路由路径理由**：Gin（`httprouter`）禁止在同一层为相同方法同时注册静态段 `/api/admin/users/deleted` 与参数段 `/api/admin/users/:id`；采用 `/api/admin/deleted-users` 与仓库内 `/api/admin/api-groups`、`/api/admin/message-categories`、`/api/admin/dict-types` 风格一致，且动作子路由 `/api/admin/users/:id/restore` 与 `/api/admin/users/:id/anonymize` 与既有 `/api/admin/users/:id/status`、`/api/admin/users/:id/kick` 保持对称。
- **全量数据范围理由**：软删用户可能已无组织归属或同时带有范围外隐藏组织（`has_unmanaged_organizations`）；受控匿名化会全局清除全部 `user_roles` 与 `user_organizations` 并全局释放唯一键，若开放给局部范围管理员将破坏范围外组织保护边界。因此三接口均沿用 `POST /api/admin/users` 模式：前置检查 `UserScope` 必须为 `All`，写操作在事务内锁定后再以 `ManagementUserScope` 二次校验 `All`（非全量范围返回 `403 IDENTITY_PERMISSION_DENIED`）。

### 3. 严格状态机校验、跨模块锁序与受控清理端口
- **跨模块边界**：遵循模块化分层架构，`identity` 不直接导入 `authorization/adapters/gorm` 或 `organization/adapters/gorm`。在 `internal/identity/application/contracts.go` 定义窄清理接口（或扩展现有接口），分别由 `authorization` 暴露事务内删除指定用户全部 `user_roles` 的方法、由 `organization` 暴露事务内删除指定用户全部 `user_organizations` 的方法，在 `internal/app/identity.go` 组合根通过 `platformdatabase.FromContext(ctx, db)` 注入，确保在同一最外层数据库事务中原子提交或回滚。
- **恢复（`Restore`）事务步骤**：
  1. 拒绝 `operatorID == targetID`（`422 IDENTITY_VALIDATION_INVALID`）；前置校验 `UserScope.All`（非全量返回 `403 IDENTITY_PERMISSION_DENIED`）。
  2. 在最外层 `transactions.Run` 内：
     - 调用 `access.IsProtectedUser(tx, operatorID, targetID)`（按既定锁序先以 ID 升序锁定 `ManagementUsersByIDs` 中的 `users` 行，再锁定 `user_roles` / `roles`）。
     - 调用 `users.FindByIDForUpdateUnscoped(tx, targetID)` 读取包含 `deleted_at` 与 `anonymized_at` 的锁定事实；不存在返回 `404 IDENTITY_USER_NOT_FOUND`。
     - 调用 `access.ManagementUserScope(tx, operatorID)` 复核 `scope.All`；非全量或 `protected == true` 返回 `403 IDENTITY_PERMISSION_DENIED`。
     - 校验状态：`deleted_at == nil` 或 `anonymized_at != nil` 均返回 `409 IDENTITY_CONFLICT`。
     - 执行 `Unscoped()` 清除 `deleted_at = NULL`（保持原 `status`、`email`、`email_verified_at`、`pending_email == ""` 与当前已保留的 `user_roles` / `user_organizations`；沿用软删时已提升的 `access_version`，不回退也不复活删前凭据）。
  3. 事务提交后返回与 `GET /api/admin/users/:id` 一致的脱敏 `AdminUser`。
- **受控匿名化（`Anonymize`）事务与事务外清理步骤**：
  1. 结构体校验请求体 `confirm_username` 非空（缺失或空值返回 `400 HTTP_REQUEST_INVALID`）；拒绝 `operatorID == targetID`（`422 IDENTITY_VALIDATION_INVALID`）；前置校验 `UserScope.All`。
  2. 在最外层 `transactions.Run` 内按相同锁序：
     - `IsProtectedUser(tx, operatorID, targetID)` -> `FindByIDForUpdateUnscoped(tx, targetID)` -> `ManagementUserScope(tx, operatorID)`。
     - 不存在返回 `404 IDENTITY_USER_NOT_FOUND`；非全量或 `protected == true` 返回 `403 IDENTITY_PERMISSION_DENIED`。
     - 校验必须处于可恢复软删态（`deleted_at != nil && anonymized_at == nil`），否则返回 `409 IDENTITY_CONFLICT`（禁止对活跃用户直接匿名化，也禁止重复匿名化）。
     - 在锁内比对 `request.ConfirmUsername == lockedUser.Username`，不匹配返回 `422 IDENTITY_VALIDATION_INVALID` 并携带字段错误 `{field: "confirm_username", error_code: "IDENTITY_CONFIRM_USERNAME_MISMATCH"}`。
     - 记录待清理可信头像对象名：`oldAvatarObject, hasTrustedAvatar := domain.TrustedAvatarObjectName(lockedUser, targetID)`。
     - 清除该用户全部 `user_roles` 与 `user_organizations` 关系。
     - 调用 `InvalidateEmailCredentials(tx, targetID, now)` 将该用户残留 `used_at IS NULL` 邮箱验证凭据置为失效。
     - 构造不可逆且满足 `varchar(100)` 与唯一索引的墓碑占位值：
       - `username = fmt.Sprintf("deleted_%d_%d", targetID, now.UnixNano())`
       - `email = fmt.Sprintf("deleted_%d_%d@deleted.invalid", targetID, now.UnixNano())`（RFC 2606 保留不可路由域名）
       - `password = "!"`（bcrypt 永不匹配的不可用哨兵）
       - `nickname = ""`、`status = 0`、`pending_email = ""`、`email_verified_at = nil`
       - 清空 `avatar`、`avatar_object_name`、`avatar_content_type`、`avatar_content_sha256`、`avatar_validation_status`、`avatar_validated_at`
       - 写入 `anonymized_at = &now`，保留既有 `deleted_at` 非空。
     - 调用 `access.IncrementVersions(tx, []uint{targetID})`（执行 `EnsureAndIncrement` 并永久保留 `user_access_versions` 行）。
  3. **仅在数据库事务成功提交后**（绝不在 `transactions.Run` 回调内，遵循 `TransactionRunner` 重试安全契约）：
     - 若 `hasTrustedAvatar == true`，调用 `AvatarStorage.Delete(ctx, AvatarBucket, oldAvatarObject)` 删除私有 `image` bucket 中的可信头像对象。
     - 若对象删除失败，保持匿名化成功响应，并写入受控 Zap error 日志（不暴露 `object_name` 或 MinIO 原始错误）；若原记录仅有历史未验证 `avatar` URL，则不执行任何存储删除。
- **Seed 与查询一致性**：
  - `SeedAdminUser` 查询配置管理员时附加 `anonymized_at IS NULL` 条件，确保匿名化墓碑永远不会被启动 Seed 恢复。
  - 既有 `ExistsIdentityConflict`、`EmailExists`、`Confirm` 无需改写即可自动感知唯一键释放（因墓碑行 `username` 与 `email` 已改写为 `deleted_<id>_<ts>` 占位值，原用户名与原邮箱立即释放）。
  - 既有 `ManagementUsersByIDs` 继续将所有 `deleted_at IS NOT NULL`（含已匿名化墓碑）标记为 `Deleted: true`，`ListUsersByIDs`、`ListUserIDs`、`FindByID`、`FindByUsername` 与公开头像读取继续天然排除墓碑行。

### 4. 管理工作台 `/users/deleted` 交互与防误操作设计
- **路由与导航**：在 `web/src/pages/ManagementPages.tsx` 的 `/users/:id` 之前注册静态子路由 `/users/deleted`（`Guard(module="users", permissions=['admin.deleted-users.get'])`），在 `web/src/lib/workbench-navigation.ts` 于 `:id` 分支前优先匹配 `segments[1] === 'deleted'`，面包屑展示「工作台 / 用户管理 / 已删除用户」并归属用户管理模块页签。
- **入口与列表**：在 `/users` 列表页顶部操作区按 `admin.deleted-users.get` 条件展示「已删除用户」入口；`/users/deleted` 使用 URL 查询参数（`keyword`、`page`、`size`）驱动真实分页，展示用户名、昵称、邮箱、删除时间、保留角色/组织及受保护标记，桌面表格与窄屏卡片响应式适配 375px / 850px / 1440px。
- **高风险二次确认**：
  - 「恢复用户」确认弹窗说明：恢复后用户回到正常用户列表、删除前的登录会话与邮箱验证链接仍保持永久失效、软删期间已清理的角色或组织归属不会自动恢复。
  - 「彻底注销（匿名化）」确认弹窗要求管理员手动输入目标用户的精确 `username` 后方能启用提交按钮，清晰列出释放用户名/邮箱、清空资料与归属、删除可信头像、保留历史审计/文件/消息墓碑引用且不可撤销等后果。
- **既有软删文案同步**：将 `/users` 列表与详情页软删除确认弹窗文案更新为准确描述“软删除后账号进入已删除用户回收站，恢复或彻底注销前仍保留用户名与邮箱占用”，避免误导管理员以为软删除会释放邮箱或彻底删除。

## Risks / Trade-offs

- **[Risk] 匿名化事务提交后 MinIO 删除可信头像对象瞬时失败，可能残留孤儿对象**：
  - **Mitigation**：严格遵循“先提交数据库事务，后最佳努力清理对象存储”原则（与 `DELETE /api/user/avatar` 契约一致），绝不把 MinIO 删除放进可重试的数据库事务内；失败时记录不含 object key 与底层存储敏感信息的受控 Zap error 日志供运维排查，不影响用户身份匿名化与唯一键释放的确定性提交。
- **[Risk] 非全量范围管理员仍可能被授予 `admin.deleted-users.get` / `restore` / `anonymize` 权限码**：
  - **Mitigation**：`/api/user/context` 不新增范围探测字段；后端三个接口均在前置和事务锁内双重校验 `scope.All` 并返回 `403 IDENTITY_PERMISSION_DENIED`，前端回收站页面与确认弹窗使用统一 `ErrorNotice` 原样本地化提示需要全量数据范围，不伪报空列表或成功。
- **[Risk] 历史 `audit_logs` 中仍保留匿名化前记录的 `username` 快照**：
  - **Mitigation**：按安全合规审计不可篡改原则显式列为非目标，在设计文档、规格和前端确认文案中透明告知管理员“历史审计日志与文件/消息的 `user_id` 引用保持完整且不改写历史日志”。
- **[Risk] 既有部署升级后需同步 3 个新权限码并添加 `users.anonymized_at` 列**：
  - **Mitigation**：新列为可空列，`AutoMigrate` 无损升级；3 条路由通过 Route Catalog 注册，服务启动 `SeedCatalog` 自动写入 `apis`、`permissions` 并授予 `admin` 角色，无需手工 SQL 或停机迁移脚本。
