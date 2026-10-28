## Why

当前后端已经提供菜单类型校验、菜单/API 绑定、Route Catalog 状态、影响预览和 fingerprint 提交契约，但正式 React 工作台仍没有菜单管理页或接口管理页。管理员无法在真实工作台中维护菜单树、诊断接口元数据、完成 API 绑定或安全地执行同步操作，导致已完成的后端能力没有可用的管理入口。

现在补齐前端页面，可以在不复制路由和权限事实的前提下，把现有后端契约接入正式工作台，并统一处理预览确认、陈旧冲突、未保存修改、响应式和无障碍交互。

## What Changes

- 在正式工作台新增“配置管理”导航组，提供 `/menus` 和 `/apis` 两个受权限控制的入口。
- 新增菜单管理页面：菜单树、节点详情、目录/页面/按钮类型表单、菜单 CRUD、API 绑定/显式清空和 Route Catalog 菜单同步预览。
- 新增接口管理页面：服务端分页、筛选、API 详情、RouteState 诊断、元数据编辑、API 路由同步、按钮菜单生成和删除预览。
- 通过现有 impact preview 与 `preview_fingerprint` 完成高影响操作确认和提交，陈旧时显示冲突并要求重新读取，不自动重放。
- 复用现有 `api.ts`、统一错误信封、会话失效处理、`WorkbenchShell`、主题和管理页响应式样式。
- **BREAKING** 前端首版不提供手工创建 API，也不编辑 API Method、Path 或 `NeedAuth`；API 路由事实继续来自 Route Catalog。
- **BREAKING** 菜单表单严格遵循目录、页面、按钮的类型字段和 API 绑定边界，不再提供会被后端拒绝的组合。
- 只有在真实浏览器验收完成后，才勾选 `TODO.md` 中的菜单管理页面和接口管理页面事项。

## Capabilities

### New Capabilities

- `menu-api-management-ui`: 正式 React 工作台中的菜单管理和接口管理页面、真实 API 操作、影响预览、响应式和无障碍交互。

### Modified Capabilities

- `admin-workbench`: 新增配置管理导航组、菜单/接口页面入口、页面权限过滤、直接地址访问和工作台响应式验收约束。
- `menu-management`: 为前端管理页面明确菜单类型表单、菜单树维护、API 绑定/清空、同步预览和 fingerprint 提交流程。
- `api-management`: 为前端管理页面明确分页筛选、RouteState 诊断、只读路由字段、元数据编辑、同步预览和按钮生成流程。

## Impact

- **Frontend:** `web/src/lib/models.ts`、`web/src/lib/api.ts`、`web/src/lib/workbench-navigation.ts`、React Router 配置、菜单/接口页面、共享表单/预览/状态组件和 `web/src/management.css`。
- **Backend APIs consumed:** existing menu CRUD, menu API list/bind/clear, menu sync/impact preview, API list/detail/update/delete, API sync/impact preview and menu-button endpoints. No new backend route is required by this change.
- **Permissions:** reuse existing `admin.menus.*` and `admin.apis.*` permissions; no new permission code.
- **Persistence/runtime:** no database migration, Redis key, MinIO bucket, route registration source, or token storage change.
- **Verification:** frontend component/request tests plus real browser checks at desktop and narrow-screen sizes, including preview-to-commit, stale fingerprint, permission denial, empty/error/loading, theme and keyboard flows.
