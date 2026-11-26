## Why

当前系统对用户仅提供单向软删除（`DELETE /api/admin/users/:id`），既缺少将误删用户从软删状态救回的管理入口，也缺少在确认注销后释放 `username` / `email` 唯一键并清理个人资料、凭据、头像与角色/组织归属的终态出口。在正式上线场景下，这会导致误删无法恢复、已删用户的邮箱和用户名被永久锁死、以及孤儿头像与归属关系长期滞留；同时因 `audit_logs`、`audit_log_archives`、`files.uploader_id`、`messages`、`message_recipients`、`message_user_states`、`message_event_deliveries` 与 `user_access_versions` 均直接引用 `users.id`，不能直接物理删除 `users` 行。

本变更通过引入“**软删可恢复 + 受控匿名化释放唯一键（保留 `users.id` 墓碑与授权版本）**”两阶段生命周期闭环，在保持历史审计、文件与消息引用完整且旧凭据永不复活的前提下，完整解决上述上线缺陷。

## What Changes

- **用户模型增量字段（无破坏性迁移）**：
  - 在 `internal/identity/model.go` 的 `User` 模型新增可空索引字段 `anonymized_at *time.Time`，通过既有 `AutoMigrate` 自动补列；存量活跃与软删记录默认 `anonymized_at = NULL`，无需人工数据回填。
  - 在领域术语表 `CONTEXT.md` 补充“匿名化墓碑（Anonymized User Tombstone）”定义，明确区分活跃用户、软删除用户与不可逆匿名化墓碑。
- **后端新增三个全量范围管理接口（Route Catalog 声明，Seed 自动同步权限码）**：
  - `GET /api/admin/deleted-users`（权限码 `admin.deleted-users.get`，审计分类 `identity`）：仅限 `all` 数据范围管理员分页查询**可恢复的软删除用户**（`deleted_at IS NOT NULL AND anonymized_at IS NULL`），支持 `page`、`size`、`keyword` 筛选并按 `deleted_at desc, id desc` 排序，返回脱敏信息、删除时间、当前保留的角色/组织摘要、受保护标识及 `access_version`；不包含已匿名化墓碑。
  - `POST /api/admin/users/:id/restore`（权限码 `admin.users.id.restore.post`，审计分类 `identity`）：仅限 `all` 数据范围管理员将非本人、非受保护（无 `admin` 角色）且处于可恢复软删态的用户恢复为活跃状态（`deleted_at = NULL`）；沿用软删时已提升的 `access_version`（删前 Token 与未消费邮箱验证凭据永不复活），不复活软删期间已清理的角色或组织归属；返回脱敏 `AdminUser`。
  - `POST /api/admin/users/:id/anonymize`（权限码 `admin.users.id.anonymize.post`，审计分类 `identity`）：仅限 `all` 数据范围管理员对非本人、非受保护且**已软删除但尚未匿名化**的用户执行不可逆受控匿名化，请求体必填 `confirm_username` 与锁定后的目标当前用户名精确匹配：
    - 在单一数据库事务中清除该用户的 `user_roles` 与 `user_organizations` 归属、将剩余 `used_at IS NULL` 的 `email_verification_credentials` 置为失效、改写 `users` 为不可逆墓碑（`username` 改写为 `deleted_<id>_<unix_nano>`、`email` 改写为 `deleted_<id>_<unix_nano>@deleted.invalid` 以立即释放原唯一键，`password` 置为不可匹配哨兵 `"!"`，清空 `nickname`/`pending_email`/`email_verified_at` 及全部头像元数据，`status = 0`，写入 `anonymized_at` 且保留 `deleted_at`），并对 `user_access_versions` 执行 `EnsureAndIncrement` 且保留该版本行。
    - 事务提交后，仅当目标原持有经系统验证的可信自定义头像对象（`TrustedAvatarObjectName`）时，向私有 `image` bucket 执行受控对象清理；删除失败时保留已提交匿名化结果并记录不含 object key/原始 MinIO 错误的受控运行日志，绝不删除历史未验证头像 URL。
  - 保护既有超级管理员启动恢复契约：`SeedAdminUser` 仅匹配 `anonymized_at IS NULL` 的记录，且受保护 `admin` 用户始终禁止经管理端匿名化。
- **前端管理工作台补齐已删除用户回收站与交互**：
  - 在 `/users` 列表页为具备 `admin.deleted-users.get` 权限的管理员提供「已删除用户」入口，并新增静态子路由 `/users/deleted`（路由守卫 `module="users"` + `admin.deleted-users.get`，面包屑「工作台 / 用户管理 / 已删除用户」，归属用户管理模块页签）。
  - 支持关键词搜索、服务端分页、桌面表格与窄屏卡片响应式展示；针对可恢复非保护用户按权限提供「恢复用户」确认弹窗与「彻底注销（匿名化）」高风险二次确认弹窗（须手输目标当前用户名解锁提交，并完整说明释放唯一键、清理资料/归属/头像、保留墓碑与不可恢复影响）。
  - 调整既有 `/users` 软删除确认提示文案，准确说明软删除后账号进入可恢复状态且在彻底注销前继续保留用户名与邮箱占用。
- **基础设施与外部依赖影响**：
  - 路由：新增 `GET /api/admin/deleted-users`、`POST /api/admin/users/:id/restore`、`POST /api/admin/users/:id/anonymize`。
  - 权限码：新增 `admin.deleted-users.get`、`admin.users.id.restore.post`、`admin.users.id.anonymize.post`（由 Route Catalog + 启动 Seed 自动同步至 `apis`、`permissions` 并授予 `admin` 角色）。
  - 模型：`users` 表新增可空索引列 `anonymized_at`。
  - Redis key：无新增。
  - MinIO bucket：无新增（复用现有私有 `image` bucket 清理可信自定义头像）。
- **非目标（显式排除）**：
  - 不执行 `DELETE FROM users` 物理删行，不破坏 `audit_logs`、`audit_log_archives`、`files`、`messages` 系列表及 `user_access_versions` 的 `user_id` 引用完整性。
  - 不改写、擦除或脱敏历史 `audit_logs` / `audit_log_archives` 中的既有记录与历史 `username` 快照。
  - 不允许跳过软删除直接对活跃用户执行匿名化（强制两阶段防误操作）。
  - 不允许恢复已匿名化墓碑，也不允许对操作者本人或受保护 `admin` 身份执行恢复或匿名化（超级管理员灾备恢复继续仅由 `SeedAdminUser` 承担）。
  - 不在本变更中引入按保留天数自动定时匿名化的后台任务（仅提供管理员显式触发；自动清理策略如需可另立独立变更）。
  - 不支持删除历史未验证外部头像 URL，也不新增跨租户/局部数据范围的回收站管理模式。

## Capabilities

### New Capabilities
- 无（全部能力作为既有规格的生命周期扩展增量落地）。

### Modified Capabilities
- `user-management`: 新增已软删除用户回收站分页查询、软删除用户恢复与受控匿名化释放唯一键的接口、数据范围、锁序、事务原子性、可信头像清理与状态冲突契约；完善软删除两阶段生命周期衔接条款。
- `admin-workbench`: 新增 `/users/deleted` 已软删除用户回收站页面（搜索、分页、恢复确认、手输用户名彻底注销确认、受保护用户与非全量范围提示），并对齐用户软删除确认文案及导航面包屑/页签归属。
- `access-version-storage`: 明确受控匿名化时执行 `EnsureAndIncrement` 并永久保留 `user_access_versions` 记录，防止旧凭据复活。

## Impact

- **后端代码**：`internal/identity/`（`model.go`、`domain`、`dto.go`、`application/contracts.go`、`application/user_service.go`、`application/admin_users.go`、`adapters/gorm/repository.go`、`adapters/gorm/seed.go`、`adapters/http/routes.go`、`adapters/http/admin_users.go`）、`internal/authorization/`（新增事务内清理单用户 `user_roles` 的窄能力）、`internal/organization/`（新增事务内清理单用户 `user_organizations` 的窄能力）、`internal/app/identity.go`（组合根装配与 MinIO 头像清理依赖注入）及对应单元测试、集成测试与 OpenAPI / Route Catalog 契约测试。
- **数据库与权限元数据**：`users` 表通过 `AutoMigrate` 新增可空索引列 `anonymized_at`；新增 3 条权限受控路由与对应默认权限码，启动 Seed 自动同步并授予 `admin` 角色。
- **前端代码**：`web/src/lib/models.ts`、`web/src/lib/workbench-navigation.ts`、`web/src/pages/ManagementPages.tsx`、`web/src/pages/UserPages.tsx`、`web/src/management.css` 及对应 Vitest 单测。
- **领域文档**：`CONTEXT.md` 补充匿名化墓碑术语定义。
