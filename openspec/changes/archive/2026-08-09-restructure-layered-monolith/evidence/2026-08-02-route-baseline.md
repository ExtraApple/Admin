# Gin 业务路由基线（2026-08-02）

本快照来自当前 `router/router.go`，用于 `restructure-layered-monolith` 迁移前对照。方法和路径按 Gin 注册值记录；权限码按当前 `service.deriveAPIPermissionCode` 规则计算（去除 `/api/` 前缀、去除路径参数冒号、斜杠转点、追加小写 HTTP 方法）。

## 访问等级与中间件

| 访问等级 | 当前路由形态 | 当前中间件链 |
| --- | --- | --- |
| Public | `/ping`、可选 `/docs` 路由、`/api` 下验证码/登录/注册/刷新/公开字典/头像 | `/ping` 和 `/docs` 无业务中间件；`/api` 路由经过 `AuditLog` |
| Authenticated | `/api/user/*` | `AuditLog` → `JWTAuth` |
| PermissionControlled | `/api/admin/*` | `AuditLog` → `JWTAuth` → `APIPermission` |

`PermissionControlled` 路由即使 API 元数据的 `need_auth=0`，仍先经过 `JWTAuth`；元数据只决定 `APIPermission` 后续是否需要权限码。`APIPermission` 在元数据不存在、禁用或受保护接口缺少权限码时分别返回 403；管理员角色只绕过权限码检查，不绕过元数据存在性和启用状态检查。

## Public 路由

| Method | Path | Middleware | Permission Code |
| --- | --- | --- | --- |
| GET | `/ping` | none | — |
| GET | `/docs`（`api_docs.enabled=true`） | none | — |
| GET | `/docs/openapi.json`（`api_docs.enabled=true`） | none | — |
| GET | `/api/captcha` | AuditLog | — |
| POST | `/api/register` | AuditLog | — |
| POST | `/api/login` | AuditLog | — |
| POST | `/api/refresh` | AuditLog | — |
| GET | `/api/dicts/:type_code/items` | AuditLog | — |
| GET | `/api/avatars/default` | AuditLog | — |
| GET | `/api/avatars/:user_id` | AuditLog | — |

## Authenticated 路由

| Method | Path | Middleware | Permission Code |
| --- | --- | --- | --- |
| GET | `/api/user/context` | AuditLog → JWTAuth | — |
| GET | `/api/user/info` | AuditLog → JWTAuth | — |
| PUT | `/api/user/info` | AuditLog → JWTAuth | — |
| PUT | `/api/user/password` | AuditLog → JWTAuth | — |
| POST | `/api/user/avatar` | AuditLog → JWTAuth | — |
| DELETE | `/api/user/avatar` | AuditLog → JWTAuth | — |
| POST | `/api/user/logout` | AuditLog → JWTAuth | — |

## PermissionControlled 路由

| Method | Path | Middleware | Permission Code |
| --- | --- | --- | --- |
| GET | `/api/admin/users` | AuditLog → JWTAuth → APIPermission | `admin.users.get` |
| PUT | `/api/admin/users/:id` | AuditLog → JWTAuth → APIPermission | `admin.users.id.put` |
| DELETE | `/api/admin/users/:id` | AuditLog → JWTAuth → APIPermission | `admin.users.id.delete` |
| PUT | `/api/admin/users/:id/status` | AuditLog → JWTAuth → APIPermission | `admin.users.id.status.put` |
| PUT | `/api/admin/users/:id/kick` | AuditLog → JWTAuth → APIPermission | `admin.users.id.kick.put` |
| GET | `/api/admin/roles` | AuditLog → JWTAuth → APIPermission | `admin.roles.get` |
| POST | `/api/admin/roles` | AuditLog → JWTAuth → APIPermission | `admin.roles.post` |
| PUT | `/api/admin/roles/:id` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.put` |
| DELETE | `/api/admin/roles/:id` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.delete` |
| POST | `/api/admin/roles/:id/users` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.users.post` |
| GET | `/api/admin/roles/:id/users` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.users.get` |
| POST | `/api/admin/roles/:id/data-scope` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.data-scope.post` |
| GET | `/api/admin/roles/:id/data-scope` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.data-scope.get` |
| GET | `/api/admin/permissions` | AuditLog → JWTAuth → APIPermission | `admin.permissions.get` |
| POST | `/api/admin/permissions` | AuditLog → JWTAuth → APIPermission | `admin.permissions.post` |
| PUT | `/api/admin/permissions/:id` | AuditLog → JWTAuth → APIPermission | `admin.permissions.id.put` |
| DELETE | `/api/admin/permissions/:id` | AuditLog → JWTAuth → APIPermission | `admin.permissions.id.delete` |
| GET | `/api/admin/permission-codes` | AuditLog → JWTAuth → APIPermission | `admin.permission-codes.get` |
| POST | `/api/admin/permissions/sync` | AuditLog → JWTAuth → APIPermission | `admin.permissions.sync.post` |
| POST | `/api/admin/roles/:id/permissions` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.permissions.post` |
| GET | `/api/admin/roles/:id/permissions` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.permissions.get` |
| GET | `/api/admin/permission-groups` | AuditLog → JWTAuth → APIPermission | `admin.permission-groups.get` |
| POST | `/api/admin/permission-groups` | AuditLog → JWTAuth → APIPermission | `admin.permission-groups.post` |
| PUT | `/api/admin/permission-groups/:id` | AuditLog → JWTAuth → APIPermission | `admin.permission-groups.id.put` |
| DELETE | `/api/admin/permission-groups/:id` | AuditLog → JWTAuth → APIPermission | `admin.permission-groups.id.delete` |
| POST | `/api/admin/files` | AuditLog → JWTAuth → APIPermission | `admin.files.post` |
| GET | `/api/admin/files` | AuditLog → JWTAuth → APIPermission | `admin.files.get` |
| GET | `/api/admin/files/:id` | AuditLog → JWTAuth → APIPermission | `admin.files.id.get` |
| PUT | `/api/admin/files/:id` | AuditLog → JWTAuth → APIPermission | `admin.files.id.put` |
| DELETE | `/api/admin/files/:id` | AuditLog → JWTAuth → APIPermission | `admin.files.id.delete` |
| GET | `/api/admin/files/:id/download` | AuditLog → JWTAuth → APIPermission | `admin.files.id.download.get` |
| GET | `/api/admin/files/:id/preview` | AuditLog → JWTAuth → APIPermission | `admin.files.id.preview.get` |
| POST | `/api/admin/files/:id/revalidate` | AuditLog → JWTAuth → APIPermission | `admin.files.id.revalidate.post` |
| GET | `/api/admin/files-browse` | AuditLog → JWTAuth → APIPermission | `admin.files-browse.get` |
| GET | `/api/admin/menus` | AuditLog → JWTAuth → APIPermission | `admin.menus.get` |
| POST | `/api/admin/menus` | AuditLog → JWTAuth → APIPermission | `admin.menus.post` |
| PUT | `/api/admin/menus/:id` | AuditLog → JWTAuth → APIPermission | `admin.menus.id.put` |
| DELETE | `/api/admin/menus/:id` | AuditLog → JWTAuth → APIPermission | `admin.menus.id.delete` |
| POST | `/api/admin/menus/sync` | AuditLog → JWTAuth → APIPermission | `admin.menus.sync.post` |
| POST | `/api/admin/menus/:id/apis` | AuditLog → JWTAuth → APIPermission | `admin.menus.id.apis.post` |
| GET | `/api/admin/menus/:id/apis` | AuditLog → JWTAuth → APIPermission | `admin.menus.id.apis.get` |
| POST | `/api/admin/roles/:id/menus` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.menus.post` |
| GET | `/api/admin/roles/:id/menus` | AuditLog → JWTAuth → APIPermission | `admin.roles.id.menus.get` |
| GET | `/api/admin/audit-logs` | AuditLog → JWTAuth → APIPermission | `admin.audit-logs.get` |
| GET | `/api/admin/login-logs` | AuditLog → JWTAuth → APIPermission | `admin.login-logs.get` |
| GET | `/api/admin/operation-logs` | AuditLog → JWTAuth → APIPermission | `admin.operation-logs.get` |
| GET | `/api/admin/permission-logs` | AuditLog → JWTAuth → APIPermission | `admin.permission-logs.get` |
| GET | `/api/admin/data-access-logs` | AuditLog → JWTAuth → APIPermission | `admin.data-access-logs.get` |
| GET | `/api/admin/dict-types` | AuditLog → JWTAuth → APIPermission | `admin.dict-types.get` |
| POST | `/api/admin/dict-types` | AuditLog → JWTAuth → APIPermission | `admin.dict-types.post` |
| PUT | `/api/admin/dict-types/:id` | AuditLog → JWTAuth → APIPermission | `admin.dict-types.id.put` |
| DELETE | `/api/admin/dict-types/:id` | AuditLog → JWTAuth → APIPermission | `admin.dict-types.id.delete` |
| GET | `/api/admin/dict-items` | AuditLog → JWTAuth → APIPermission | `admin.dict-items.get` |
| POST | `/api/admin/dict-items` | AuditLog → JWTAuth → APIPermission | `admin.dict-items.post` |
| PUT | `/api/admin/dict-items/:id` | AuditLog → JWTAuth → APIPermission | `admin.dict-items.id.put` |
| DELETE | `/api/admin/dict-items/:id` | AuditLog → JWTAuth → APIPermission | `admin.dict-items.id.delete` |
| GET | `/api/admin/organizations` | AuditLog → JWTAuth → APIPermission | `admin.organizations.get` |
| GET | `/api/admin/organizations/tree` | AuditLog → JWTAuth → APIPermission | `admin.organizations.tree.get` |
| POST | `/api/admin/organizations` | AuditLog → JWTAuth → APIPermission | `admin.organizations.post` |
| PUT | `/api/admin/organizations/:id` | AuditLog → JWTAuth → APIPermission | `admin.organizations.id.put` |
| DELETE | `/api/admin/organizations/:id` | AuditLog → JWTAuth → APIPermission | `admin.organizations.id.delete` |
| POST | `/api/admin/organizations/:id/users` | AuditLog → JWTAuth → APIPermission | `admin.organizations.id.users.post` |
| GET | `/api/admin/organizations/:id/users` | AuditLog → JWTAuth → APIPermission | `admin.organizations.id.users.get` |
| GET | `/api/admin/api-groups` | AuditLog → JWTAuth → APIPermission | `admin.api-groups.get` |
| GET | `/api/admin/api-methods` | AuditLog → JWTAuth → APIPermission | `admin.api-methods.get` |
| GET | `/api/admin/apis` | AuditLog → JWTAuth → APIPermission | `admin.apis.get` |
| GET | `/api/admin/apis/:id` | AuditLog → JWTAuth → APIPermission | `admin.apis.id.get` |
| POST | `/api/admin/apis` | AuditLog → JWTAuth → APIPermission | `admin.apis.post` |
| PUT | `/api/admin/apis/:id` | AuditLog → JWTAuth → APIPermission | `admin.apis.id.put` |
| DELETE | `/api/admin/apis/:id` | AuditLog → JWTAuth → APIPermission | `admin.apis.id.delete` |
| POST | `/api/admin/apis/:id/menu-button` | AuditLog → JWTAuth → APIPermission | `admin.apis.id.menu-button.post` |
| POST | `/api/admin/apis/sync` | AuditLog → JWTAuth → APIPermission | `admin.apis.sync.post` |
| POST | `/api/admin/apis/sync-permissions` | AuditLog → JWTAuth → APIPermission | `admin.apis.sync-permissions.post` |

## 首次同步例外与现有行为

- `POST /api/admin/apis/sync` 和 `POST /api/admin/apis/sync-permissions` 在 `middleware.isPermissionSyncRoute` 中属于首次同步例外。请求仍需先通过 `JWTAuth`；当上下文角色包含 `admin` 时，`APIPermission` 在查询 `apis` 元数据前直接放行。
- `POST /api/admin/permissions/sync` **不在**上述例外集合中。当前请求仍查询 `apis(method='POST', path='/api/admin/permissions/sync')`；缺少元数据时返回 `403 {"code":403,"msg":"API未配置权限"}`。即使请求用户是 `admin`，也不能绕过“元数据不存在”这一分支。
- 当 `/api/admin/permissions/sync` 已有启用元数据且 `need_auth=1` 时，管理员角色只绕过权限码检查；普通用户仍需拥有该元数据的 `permission_code`。
- `SyncPermissions` 当前实现从 `h.Engine.Routes()` 扫描 Gin 路由，再调用服务层同步权限；这属于迁移前事实，后续应改为消费 Route Catalog Snapshot。
- `SyncAPIs` 和 `SyncAPIPermissions` 当前也由 API Handler 持有的 Gin Engine 路由集合驱动；公开路由由 `inferAPINeedAuth` 判定，`POST /api/refresh` 还会修复既有错误元数据为 `need_auth=0` 且清空权限码。

## 迁移对照约束

1. Route Catalog 必须保留以上 Method + Path 集合（文档开关关闭时排除两条 `/docs` 路由）。
2. Public、Authenticated、PermissionControlled 映射必须保持；PermissionControlled 不得被 API Metadata 的 `need_auth` 改成匿名路由。
3. API Metadata、RBAC 权限同步和 API Doc 后续必须消费同一份静态 Route Catalog Snapshot，不得扫描 Gin Engine。
4. `/api/admin/permissions/sync` 的当前缺少元数据行为需由兼容测试固定；首次同步例外只能按明确的 Route Catalog 迁移决策调整。
