## 1. 后端管理员创建用户接口（identity）

- [x] 1.1 在 `internal/identity/dto.go` 定义 `AdminCreateUserRequest` DTO（`username`、`password`、`email`、`nickname`、`status`），保持密码无上限 tag 以便超 72 字节由共享密码策略返回 422
- [x] 1.2 在 `internal/identity/application/contracts.go` 和 `internal/identity/adapters/gorm/repository.go` 补齐 `ExistsIdentityConflict`（覆盖 `Unscoped` 用户名/邮箱与活跃 `pending_email`）及 `AccessManager.EnsureVersions`
- [x] 1.3 在 `internal/identity/application/user_service.go` 实现 `CreateByAdmin`：前置/事务内双重 `all` 数据范围校验、共享密码策略校验、事务外 bcrypt 哈希、唯一冲突检测与并发冲突归类、固定 `role = "user"`、不发验证邮件、事务内初始化授权版本并返回脱敏 `AdminUser`
- [x] 1.4 在 `internal/identity/adapters/http/routes.go` 注册 `POST /api/admin/users`（权限码 `admin.users.post`，审计分类 `identity`）与对应 HTTP Handler，补充单元测试与 HTTP 路由/契约测试

## 2. 前端路由、模型与导航壳层

- [x] 2.1 在 `web/src/lib/models.ts` 补齐角色、用户、组织创建请求类型；在 `web/src/lib/workbench-navigation.ts` 优先匹配 `/roles/new`、`/users/new`、`/organizations/new` 的面包屑、模块归属与权限准入
- [x] 2.2 在 `web/src/pages/ManagementPages.tsx` 注册 `/roles/new`、`/users/new`、`/organizations/new` 静态路由及对应 `Guard` 权限门禁

## 3. 前端角色、用户、组织新增与删除页面

- [x] 3.1 在 `web/src/pages/RolePages.tsx` 实现 `RoleCreate` 页面（显式默认 `status = 1`、`sort = 0`、`data_scope = 'self'`，提示 `custom` 需创建后配置，`UnsavedGuard` + `useSavedNavigation`），并在 `RolesList`（含桌面操作浮层与窄屏卡片）和 `RoleDetail` 实现受保护校验与带绑定用户数读取的二次确认删除
- [x] 3.2 在 `web/src/pages/UserPages.tsx` 实现 `UserCreate` 页面（对接 `POST /api/admin/users`，含本地确认密码校验、密码规则提示、`UnsavedGuard` + 创建后跳转）及用户列表新建入口，保持现有用户删除交互不变
- [x] 3.3 在 `web/src/pages/OrganizationPages.tsx` 实现 `OrganizationCreate` 页面（支持列表新建与详情「新建子组织」`?parentId=:id` 预选，仅允许选 `manageable: true` 上级，非全量树禁用根组织），并在可管理 `OrganizationDetail` 实现前置子组织拦截与二次确认删除
- [x] 3.4 按需补充 `web/src/management.css` 表单与操作区紧凑响应式样式，确保 375px / 850px / 1440px 视口无横向溢出且符合亮/暗色基线

## 4. 自动化测试与真实页面验证

- [x] 4.1 补充并运行后端 Go 测试（`internal/identity/...`、`internal/app/...` 及受影响的 OpenAPI / Route Catalog 契约测试）
- [x] 4.2 补充并运行前端 Vitest 单测、TypeScript 类型检查与 Vite 生产构建
- [x] 4.3 启动真实后端与前端服务，在浏览器中完成角色/用户/组织新建与删除（含权限控制、确认提示、错误拦截及响应式视口）端到端验证
