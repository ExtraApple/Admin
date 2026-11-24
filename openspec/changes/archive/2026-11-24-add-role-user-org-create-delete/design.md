## Context

管理工作台（`web/`）当前支持角色、用户、组织的列表查看、详情查看和既有对象的授权/归属编辑，但存在三处缺口：
1. 角色和组织后端已具备 `POST /api/admin/roles`、`DELETE /api/admin/roles/:id`、`POST /api/admin/organizations`、`DELETE /api/admin/organizations/:id`，但前端尚未接入对应创建与删除交互。
2. 用户前端已完成删除交互（列表操作浮层与详情页底部），但后端缺少受 RBAC 与数据范围控制的管理员创建用户接口（仅存在公开自助注册 `POST /api/register`，依赖验证码且触发 SMTP 邮箱验证）。
3. 现有工作台导航壳层与多页签路由匹配器（`web/src/lib/workbench-navigation.ts`）尚未覆盖 `/roles/new`、`/users/new`、`/organizations/new` 的静态路由判定、面包屑与模块页签归属。

## Goals / Non-Goals

**Goals:**
- 在 `identity` 模块中以增量方式新增 `POST /api/admin/users`（权限码 `admin.users.post`），复用既有分层、共享密码策略、稳定错误码、审计脱敏和 `AdminUser` 脱敏响应结构。
- 在 `web/` 工作台中为角色、用户、组织分别增加独立创建子路由页面（`/roles/new`、`/users/new`、`/organizations/new`），统一复用 `PageHeading`、`UnsavedGuard` 与 `useSavedNavigation`。
- 在 `web/` 工作台中补齐角色删除（列表操作浮层 + 详情页）与组织删除（可管理组织详情页），并保持用户现有删除实现不变。
- 同步 Route Catalog、OpenAPI / 契约测试、前后端单元测试，并完成真实浏览器联调验证。

**Non-Goals:**
- 不实现角色、用户、组织的基础信息编辑页面。
- 不调整用户删除现有 UI 与交互。
- 不改变后端角色级联删除逻辑，也不改动或细分后端组织删除错误码契约。
- 不在 `POST /api/admin/users` 中写入角色或组织归属，不引入表结构迁移或新的存储依赖。

## Decisions

### 1. `POST /api/admin/users` 归属 `identity` `UserService`，且仅允许 `all` 数据范围管理员调用
- **方案**：在 `internal/identity/application/user_service.go` 新增 `CreateByAdmin(ctx, operatorID, AdminCreateUserRequest) (AdminUser, error)`，在 `internal/identity/adapters/http/routes.go` 注册 `POST /api/admin/users`（权限码 `admin.users.post`，审计分类 `identity`）。
- **权限与范围顺序**：
  1. 先调用 `access.UserScope(ctx, operatorID)` 快速拒绝非 `all` 范围操作者（返回 `403 IDENTITY_PERMISSION_DENIED`），杜绝非全量管理员枚举已有用户名/邮箱或消耗 bcrypt CPU。
  2. 校验可选 `status`（`nil` 默认 `1`，否则须为 `0` 或 `1`）、用户名非纯空白与昵称非纯空白（允许空昵称），并调用共享 `validatePassword(request.Password)`（≤72 UTF-8 字节、≥3 类字符，超限/复杂度失败返回 `422 IDENTITY_VALIDATION_INVALID` + `password / IDENTITY_PASSWORD_INVALID`）。
  3. 在数据库事务外执行 `passwords.Hash(request.Password)`，避免长耗时哈希持有行锁。
  4. 在单一数据库事务内通过 `access.ManagementUserScope(tx, operatorID)` 再次锁定确认当前 `scope.All`（防并发降权），检查 `ExistsIdentityConflict(tx, username, email)`（同时覆盖 `Unscoped()` 的 `username` / `email` 唯一约束与活跃用户的 `pending_email`），写入新用户（固定 `Role: "user"`、`EmailVerifiedAt: nil`、`PendingEmail: ""`），并调用 `access.EnsureVersions(tx, []uint{user.ID})` 初始化授权版本。若写入撞并发唯一索引错误，则重查冲突并归类为 `409 IDENTITY_CONFLICT`。
  5. 返回脱敏 `AdminUser` 结构（`avatar: "/api/avatars/default"`、`roles: []`、`organizations: []`、`has_unmanaged_organizations: false`、`access_version`）。
- **备选方案与取舍**：
  - 备选 B：在 `POST /api/admin/users` 同事务绑定初始 `organization_ids`。放弃原因：会跨模块耦合写事务，并允许仅持有 `admin.users.post` 而无 `admin.users.id.organizations.put` 的角色写入 `user_organizations`，破坏既有按维分权边界。
  - 备选 C：一步同时写入 `role_ids` 与 `organization_ids`。放弃原因：重复授权模块保护角色与乐观版本逻辑，与工作台按维度独立保存契约冲突。

### 2. 管理员创建用户不触发 SMTP 验证邮件、不标记已验证
- **方案**：`CreateByAdmin` 不调用 `emailVerificationIssuer` / `verificationEmailSender`，`email_verified_at` 保持 `NULL`（未验证）。
- **理由**：避免 SMTP 未配置或临时故障导致管理建号返回 503；同时不伪造邮箱所有权已验证事实，用户可在个人资料页按既有流程完成验证。

### 3. 前端新建表单采用独立子路由并显式规避后端隐式默认值陷阱
- **路由与门禁**：
  - `/roles/new`：`Guard(module="roles", permissions=['admin.roles.post'])`
  - `/users/new`：`Guard(module="users", permissions=['admin.users.post'])`
  - `/organizations/new`：`Guard(module="organizations", permissions=['admin.organizations.tree.get', 'admin.organizations.post'])`
  - 在 `web/src/pages/ManagementPages.tsx` 中显式声明静态子路由，并在 `web/src/lib/workbench-navigation.ts` 中于 `:id` 分支之前优先匹配 `segments[1] === 'new'`，生成对应面包屑（如「工作台 / 角色管理 / 新建角色」）并归属对应模块页签。
- **表单默认值与防踩坑**：
  - 角色新建页：后端 `POST /api/admin/roles` 省略 `status` 会落成 `0`（停用）、省略 `data_scope` 会落成 `all` 且拒绝 `custom`。前端显式默认 `status = 1`、`sort = 0`、`data_scope = 'self'`（最小权限默认，仅提供 `self`/`org`/`org_and_children`/`all`，并说明 `custom` 需创建后在详情页「编辑数据范围」配置）。
  - 用户新建页：必填 `username`（3–100）、`email`、`password` + 本地「确认密码」一致性校验；可选 `nickname`（≤100）、`status`（默认 `1`）；提示密码需满足至少 6 位、至多 72 UTF-8 字节及三类字符规则。
  - 组织新建页：支持列表页「新建组织」及可管理详情页「新建子组织」（`/organizations/new?parentId=:id` 自动预选对应可管理节点）；上级选择器仅允许选择 `manageable: true` 节点；当树中存在 `manageable: false` 的只读祖先（表明当前操作者非全量数据范围）时禁用「作为根组织」选项并引导选择可管理上级，同时仍以服务端 `CurrentScope` 校验为最终事实源。
- **创建成功后跳转**：具备对应详情读权限（角色 `admin.roles.id.get`、用户 `admin.users.id.get`、组织 `admin.organizations.tree.get`）时跳往新建对象详情页并传递保存成功状态；否则跳回列表页。

### 4. 角色与组织删除交互设计
- **角色删除**：
  - 列表页桌面表格新增操作列（窄屏卡片底部增加操作浮层，风格与用户列表操作浮层对称），对 `admin` 角色显示受保护标识，对其他角色在具备 `admin.roles.id.delete` 时提供「删除角色」；详情页底部同样提供危险删除区。
  - 打开确认弹窗时，若具备 `admin.roles.id.users.get`，实时请求 `GET /api/admin/roles/:id/users` 并展示受影响用户数；读取失败时明确提示无法读取绑定人数（不伪报 0）；弹窗说明删除将级联解除角色与用户/权限/菜单/数据范围的关联并使关联用户旧会话失效、且不可撤销。
  - 详情页删除后跳回 `/roles`；列表页删除后重新拉取，若当前页仅剩被删这一条且 `page > 1` 则自动回退 `page - 1`。
- **组织删除**：
  - 仅在 `node.manageable === true` 的组织详情页且具备 `admin.organizations.id.delete` 时提供删除区（列表页与只读祖先节点不提供）。
  - 若已加载组织树中当前节点存在可见 `children`，前置拦截并提示需先移动或删除下级组织，不误导提交；其余后端拒绝原因（被非保护角色自定义数据范围引用 `409 ORG_CONFLICT`、含受保护/范围外成员或并发新增下级 `422 ORG_VALIDATION_INVALID` 等）通过 `ErrorNotice` 原样本地化展示。
  - 确认弹窗说明将解除该组织现有成员绑定、使受影响成员旧会话失效且删除不可在界面撤销；删除成功后跳回 `/organizations`。

## Risks / Trade-offs

- **[Risk] 非全量范围管理员仍可能持有 `admin.users.post` 权限码**：`/api/user/context` 当前不返回操作者数据范围，前端在具备 `admin.users.post` 时仍会展示新建用户入口。
  - **Mitigation**：后端在 `CreateByAdmin` 中以锁定后的真实 `scope.All` 做最终裁决并返回 `403 IDENTITY_PERMISSION_DENIED`；新建用户页静态说明仅全量数据范围管理员可新建账号，提交被拒时通过统一 `ErrorNotice` 呈现，保留表单输入不假报成功。
- **[Risk] 组织树中「无只读祖先」不绝对等价于全量范围**：若部门管理员恰好管理的是顶层根组织，前端树中也没有 `manageable: false` 节点，仍可能看到「作为根组织」选项。
  - **Mitigation**：服务端 `CreateUnit` 已在锁定拓扑事务内校验 `if request.ParentID == 0 && !scope.All`，返回 `422 ORG_VALIDATION_INVALID`；前端保留草稿并展示服务端错误提示，不引入新的上下文探测接口。
- **[Risk] 既有部署升级后需同步新权限码 `admin.users.post`**：
  - **Mitigation**：`POST /api/admin/users` 注册在 Route Catalog 中，服务启动时 `SeedCatalog` 自动写入 `apis`、`permissions` 并授予 `admin` 角色；无数据库表结构迁移，无需人工数据脚本。
