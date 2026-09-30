## Why

`web/preview.html` 已确定角色、用户、组织和认证入口的交互基线，但它只有内存示例数据，不能实际登录或管理授权。现有后端缺少真实多角色／组织归属的管理员读取和逐人修改契约；直接照搬样稿或用整组覆盖接口接线会造成错误授权和成员丢失。

## What Changes

- 在 `web/` 构建 React + TypeScript + Vite 管理工作台，以 Tailwind CSS、按需引入的 shadcn/ui（Radix 基元）实现样稿的视觉与响应式行为；真实数据取代示例数据，路由可直达角色、用户和组织详情及独立编辑页。
- 接入验证码、公开注册、登录／刷新／退出、当前用户资料及授权上下文；菜单和操作入口按实际权限显示，无可用管理菜单时进入说明页，仍可访问个人资料和退出。
- 提供角色分页列表、独立详情和权限／已分配菜单／数据范围的分别编辑；用户分页筛选、详情、状态／下线／删除、逐人角色与组织归属修改；组织树、详情、层级调整和逐人成员维护，包含空结果、失败、确认、未保存修改与移动端状态。
- 新增 `GET /api/admin/roles/:id`、`GET /api/admin/users/:id`、`PUT /api/admin/users/:id/roles`、`PUT /api/admin/users/:id/organizations`；扩充 `GET /api/admin/users` 的筛选和真实归属、`GET /api/admin/organizations/tree` 的祖先节点可操作标识。
- **BREAKING**：管理员 `GET /api/admin/roles/:id/menus` 从“仅已启用菜单树”改为按 ID 显式分配的平铺菜单列表（包含已停用项、`parent_id` 与 `status`），避免单独分配子菜单时被树组装丢弃；`GET /api/user/context` 的最终可见菜单过滤规则不变。新增逐人写入以授权版本作并发前置条件，过期写入返回 409 而不覆盖他人变更。
- 与进行中的 `standardize-api-response-contract` 共用其前端 API Client／验收成果并回填其待办，不复制第二套信封解析或错误映射；完成该 change 的前端和联合验收后才视为可发布。
- 非目标：用旧 `users.role` 推断真实角色、管理员创建用户、把整组用户覆盖接口用于单人编辑、引入额外资源模块、把样稿假数据或演示状态带入正式产品。

## Capabilities

### New Capabilities

- `admin-workbench`: 正式管理端的认证入口、权限导航、角色／用户／组织界面、编辑与失败状态及响应式交互。

### Modified Capabilities

- `rbac`: 管理员可按 ID 读取角色详情以支持独立页面和直接访问。
- `menu-management`: 管理员可读取完整的角色已分配菜单，区分已停用配置与用户实际可见菜单。
- `user-management`: 管理员分页筛选返回真实角色及组织归属、读取用户详情；逐人修改角色归属有版本并发保护。
- `organization-management`: 组织树标示仅供定位的祖先；逐人修改组织归属并保留其他成员及既有加入时间。

## Impact

- 前端：新建 `web/` 正式应用；`web/preview.html` 保留为视觉参照而非运行入口。依赖 React、TypeScript、Vite、Tailwind CSS、shadcn/ui（Radix）；不引入现成后台模板。
- 后端：Identity 的用户列表／详情读取与跨域查询端口、Authorization 的角色详情及逐人角色写入、Navigation 的角色菜单读取、Organization 的树标记及逐人组织写入，以及路由目录、OpenAPI 与测试。
- 新增受控路由与权限码：`GET /api/admin/roles/:id` → `admin.roles.id.get`；`GET /api/admin/users/:id` → `admin.users.id.get`；`PUT /api/admin/users/:id/roles` → `admin.users.id.roles.put`；`PUT /api/admin/users/:id/organizations` → `admin.users.id.organizations.put`。其余角色、权限、菜单、用户、组织接口复用既有路由和权限码。
- 使用现有 `user_roles`、组织成员关系和 `user_access_versions`；不新增数据表、Redis key、MinIO bucket，也不迁移历史数据。逐人写入需复用现有授权版本失效规则，并符合正在推进的四字段响应契约。
