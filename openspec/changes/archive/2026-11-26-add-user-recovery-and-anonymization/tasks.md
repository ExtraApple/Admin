## 1. 数据模型、DTO 与跨模块清理契约

- [x] 1.1 在 `internal/identity/model.go` 为 `User` 新增可空索引字段 `AnonymizedAt *time.Time`，并在 `internal/identity/domain/entities.go` 补充 `DeletedAt` / `AnonymizedAt` 生命周期只读投影字段
- [x] 1.2 在 `internal/identity/dto.go` 定义 `AdminAnonymizeUserRequest`（必填 `confirm_username`）以及已软删除用户列表响应结构（含 `deleted_at`、`protected`、`roles`、`organizations`、`access_version` 且不含敏感凭据/头像存储字段）
- [x] 1.3 在 `internal/identity/application/contracts.go` 定义单用户角色与组织事务内清理窄接口、可信头像事后删除接口及 `UserManagementRepository` / `AdminUserRepository` 所需方法（`FindByIDForUpdateUnscoped`、`ListDeleted`、`Restore`、`Anonymize`）
- [x] 1.4 在 `internal/authorization/` 与 `internal/organization/` 分别补充基于 `platformdatabase.FromContext(ctx, db)` 的单用户归属清理方法（清除指定用户的 `user_roles` 与 `user_organizations`），确保不跨模块耦合 ORM 模型

## 2. 仓储、应用服务与组合根装配

- [x] 2.1 在 `internal/identity/adapters/gorm/repository.go` 实现 `ListDeleted`（过滤 `deleted_at IS NOT NULL AND anonymized_at IS NULL`、支持用户名/昵称/邮箱关键词筛选并按 `deleted_at desc, id desc` 排序）、`FindByIDForUpdateUnscoped`、`Restore` 与 `Anonymize`
- [x] 2.2 更新 `internal/identity/adapters/gorm/seed.go`，在 `SeedAdminUser` 查询配置管理员时附加 `anonymized_at IS NULL` 条件，确保匿名化墓碑绝不被启动 Seed 恢复
- [x] 2.3 在 `internal/identity/application/admin_users.go` 实现 `ListDeleted`：校验 `UserScope.All`（非全量返回 `403 IDENTITY_PERMISSION_DENIED`），组装脱敏已软删除用户列表、真实角色/组织归属、`protected` 标志与 `access_version`
- [x] 2.4 在 `internal/identity/application/user_service.go` 实现 `Restore` 与 `Anonymize`：
  - 按既有锁序（`IsProtectedUser` -> `FindByIDForUpdateUnscoped` -> `ManagementUserScope`）校验本人、全量范围、受保护身份与生命周期状态（活跃或已匿名化返回 `409 IDENTITY_CONFLICT`，缺失返回 `404 IDENTITY_USER_NOT_FOUND`）
  - `Restore` 清除 `deleted_at`、沿用已提升的 `access_version` 且不恢复删前凭据或已清理归属，返回脱敏 `AdminUser`
  - `Anonymize` 在锁内校验 `confirm_username` 精确匹配（不匹配返回 `422 IDENTITY_VALIDATION_INVALID` 及 `confirm_username / IDENTITY_CONFIRM_USERNAME_MISMATCH`），在单一事务内清除 `user_roles` / `user_organizations`、作废剩余未消费邮箱验证凭据、改写 `username` / `email` 为唯一墓碑占位（`deleted_<id>_<unix_nano>` 与 `deleted_<id>_<unix_nano>@deleted.invalid`）、置 `password = "!"`、清空 PII 与头像元数据、写入 `anonymized_at` 并执行 `EnsureAndIncrement`
  - 仅在 `Anonymize` 事务提交后，对原可信自定义头像（`TrustedAvatarObjectName`）向私有 `image` bucket 执行受控删除；失败时保留匿名化成功并记录不含 object key 与原始存储错误的受控 Zap error 日志，绝不删除历史未验证头像 URL
- [x] 2.5 在 `internal/app/identity.go` 完成角色清理、组织清理、头像对象清理器与运行日志器在组合根的注入装配

## 3. HTTP Handler、Route Catalog 与 OpenAPI

- [x] 3.1 在 `internal/identity/adapters/http/admin_users.go` 与 `internal/identity/adapters/http/routes.go` 注册 3 条权限受控路由（组 `identity`，默认审计分类 `identity`）：
  - `GET /api/admin/deleted-users`（权限码 `admin.deleted-users.get`）
  - `POST /api/admin/users/:id/restore`（权限码 `admin.users.id.restore.post`）
  - `POST /api/admin/users/:id/anonymize`（权限码 `admin.users.id.anonymize.post`）
- [x] 3.2 完善对应 Handler 的查询/请求体绑定、字段错误映射（含 `IDENTITY_CONFIRM_USERNAME_MISMATCH`）及 OpenAPI 描述与错误响应声明

## 4. 管理工作台前端页面与导航

- [x] 4.1 在 `web/src/lib/models.ts` 补充已软删除用户列表项、分页响应与匿名化请求类型；在 `web/src/lib/errors.ts` 补充 `IDENTITY_CONFIRM_USERNAME_MISMATCH` 等本地化文案
- [x] 4.2 在 `web/src/lib/workbench-navigation.ts` 与 `web/src/pages/ManagementPages.tsx` 于 `/users/:id` 之前注册静态子路由 `/users/deleted`（权限守卫 `admin.deleted-users.get`，面包屑「工作台 / 用户管理 / 已删除用户」，归属用户管理模块页签）
- [x] 4.3 在 `web/src/pages/UserPages.tsx` 与 `web/src/management.css` 实现 `/users/deleted` 回收站页面（URL 驱动搜索与分页、桌面表格与窄屏卡片、受保护标记、「恢复用户」确认弹窗、须手输目标 `username` 解锁的「彻底注销」高风险确认弹窗及非全量范围 403 明确提示），并在 `/users` 列表添加条件入口及同步软删除确认文案

## 5. 文档、领域术语与自动化验证

- [x] 5.1 更新 `CONTEXT.md`，补充「匿名化墓碑（Anonymized User Tombstone）」领域术语并对齐软删除用户与受控匿名化的边界描述
- [x] 5.2 补充后端单元测试与集成测试(覆盖回收站查询过滤、恢复后旧 Token 与旧邮箱凭据仍失效、软删期间已清归属不复活、匿名化原子回滚、唯一键立即释放并可重新注册/换绑、可信头像事后删除与失败降级日志、历史未验证头像不被删除、非全量/本人/受保护/状态冲突/确认用户名不匹配拦截,以及 Route Catalog / OpenAPI / Seed 契约测试)
- [x] 5.3 补充前端 Vitest 单测（覆盖 `/users/deleted` 路由导航、权限门禁、搜索分页、受保护禁用、恢复确认、手输用户名解锁与提交失败提示），并完成 `go test`、`go build ./...`、TypeScript 检查、Vite 生产构建及真实前后端浏览器联调验证
