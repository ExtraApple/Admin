## Why

管理工作台目前仅支持角色、用户、组织的查看与既有对象的授权/归属编辑，缺少角色与组织的新增和删除入口，且后端缺少管理员直接创建用户的受控接口（仅有带验证码与固定默认角色的公开注册接口 `POST /api/register`）。补齐后端 `POST /api/admin/users` 与工作台三模块的新增/删除闭环后，管理员可在同一权限模型与数据范围约束下完成角色、用户、组织的完整创建与清理。

## What Changes

- **后端新增管理员创建用户接口**：
  - 在 `identity` 模块新增 `POST /api/admin/users`（权限码 `admin.users.post`，审计分类 `identity`），复用共享密码策略与既有稳定错误码契约。
  - 仅允许 `all` 数据范围管理员调用（非 `all` 范围返回 `403 IDENTITY_PERMISSION_DENIED`），避免产生创建者自身立刻不可见的孤儿用户，也不绕过 `admin.users.id.roles.put` / `admin.users.id.organizations.put` 权限边界。
  - 必填 `username`（3–100）、`email`（全局唯一，检查现有邮箱、待验证邮箱与软删唯一约束，冲突返回 `409 IDENTITY_CONFLICT`）、`password`（复用共享密码策略，≥6 Unicode 字符、≤72 UTF-8 字节、至少三类字符）；可选 `nickname`（≤100）、`status`（`0`/`1`，默认 `1`）；旧兼容列 `users.role` 固定为 `"user"` 不可由请求指定。
  - 创建时不触发 SMTP 验证邮件，初始保持未验证（`email_verified_at = NULL`）；成功后返回与 `GET /api/admin/users/:id` 一致的脱敏 `AdminUser` 结构（含 `roles: []`、`organizations: []`、`access_version`）。
  - 通过 Route Catalog 声明路由，启动 Seed 自动同步 `apis`、`permissions` 及 `admin` 角色授权，并同步更新 OpenAPI / 路由契约测试。
- **前端新增角色、用户、组织独立创建子路由**：
  - `/roles/new`（`admin.roles.post`）：对接 `POST /api/admin/roles`，支持 `name`、`code`、`description`、`sort`（默认 `0`）、`status`（显式默认 `1` 启用）、`data_scope`（默认 `self`，可选 `self` / `org` / `org_and_children` / `all`，并提示 `custom` 需创建后在详情页「编辑数据范围」配置）。
  - `/users/new`（`admin.users.post`）：对接 `POST /api/admin/users`，支持 `username`、`email`、`password` + 本地「确认密码」校验、`nickname`、`status`（默认 `1`）。
  - `/organizations/new`（`admin.organizations.tree.get` + `admin.organizations.post`）：对接 `POST /api/admin/organizations`，支持 `parent_id`、`name`、`code`、`remark`、`sort`（默认 `0`）、`status`（默认 `1`）；列表页提供「新建组织」，可管理组织详情页提供「新建子组织」（`/organizations/new?parentId=:id` 预选上级）；上级选择仅允许可管理节点，并在存在只读祖先（非全量范围）时限制根组织选择、同时依赖后端范围校验兜底。
  - 三个新建页均复用 `PageHeading`、`UnsavedGuard` 与 `useSavedNavigation`，并更新 `workbench-navigation.ts` 的面包屑与模块页签准入判定；创建成功后具备详情读权限时直达新建对象详情页，否则返回对应列表页。
- **前端补齐角色与组织删除交互（用户删除保持现状）**：
  - 角色管理在列表行操作浮层（含窄屏卡片）和详情页底部提供「删除角色」（受 `admin.roles.id.delete` 控制，`admin` 角色标明受保护不可删）；确认弹窗在具备 `admin.roles.id.users.get` 时实时读取并展示受影响用户数，说明将级联解除角色与用户/权限/菜单/数据范围关联并使相关用户旧会话失效；详情页删除后跳回 `/roles`，列表页删除后刷新并在尾页删空时自动回退上一页。
  - 组织管理在可管理组织详情页提供「删除组织」（受 `admin.organizations.id.delete` 控制，范围外只读祖先不显示）；前端基于已加载组织树预先拦截仍有下级子组织的节点并说明原因，其余引用冲突（被角色数据范围引用、含受保护/范围外成员等）沿用现有后端错误码展示；确认弹窗说明将解除成员绑定、使成员旧会话失效且软删不可在界面撤销；删除成功后跳回 `/organizations`。
- **非目标（显式排除）**：
  - 不接入角色、用户、组织的基础信息编辑（`PUT /api/admin/roles/:id`、`PUT /api/admin/users/:id`、除调整上级外的 `PUT /api/admin/organizations/:id`）。
  - 不修改现有用户删除交互与入口（[UserPages.tsx](file:///D:/WORK/GO/admin/web/src/pages/UserPages.tsx) 已满足现行契约）。
  - 不修改后端角色删除级联语义，也不细化或改动后端组织删除错误码契约。
  - 不在 `POST /api/admin/users` 中混入初始角色或初始组织写入，也不新增 Redis key、MinIO bucket 或数据库表结构变更。

## Capabilities

### New Capabilities
- 无（全部变更落在既有能力规格增量中）。

### Modified Capabilities
- `user-management`: 新增管理员创建用户接口 `POST /api/admin/users`（权限码 `admin.users.post`）的字段、数据范围、共享密码策略、唯一性冲突、邮箱未验证状态与响应契约。
- `admin-workbench`: 扩展角色管理、用户管理、组织管理的新增独立子路由页面、角色与组织删除确认与拦截交互、创建/删除后的导航流转，以及新增页的未保存离开确认与路由面包屑/页签准入。

## Impact

- **后端代码**：`internal/identity/`（DTO、`AdminUserService` / `UserService`、GORM Repository 唯一性检查、HTTP Route/Handler、`internal/app/identity.go` 装配及单元测试/契约测试）。
- **权限与路由元数据**：新增 1 条权限受控路由 `POST /api/admin/users`（默认权限码 `admin.users.post`），无表结构迁移；既有部署在下次启动 Seed（或手动执行路由/权限同步并赋权）后生效，不新增或修改其他模块路由与错误码。
- **前端代码**：`web/src/pages/ManagementPages.tsx`、`web/src/pages/RolePages.tsx`、`web/src/pages/UserPages.tsx`、`web/src/pages/OrganizationPages.tsx`、`web/src/lib/workbench-navigation.ts`、`web/src/lib/models.ts`、`web/src/management.css` 及对应 Vitest 单测。
