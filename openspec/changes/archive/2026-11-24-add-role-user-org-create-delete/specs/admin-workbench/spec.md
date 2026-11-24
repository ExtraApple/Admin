# admin-workbench Specification Delta

## ADDED Requirements

### Requirement: 角色管理新增与删除
工作台 SHALL 在角色管理模块提供独立的新建角色页面（`/roles/new`）以及受权限控制的删除角色交互，并保护 `admin` 角色不可删除。

#### Scenario: 新建角色
- **WHEN** 具有 `admin.roles.post` 权限的管理员从角色列表进入 `/roles/new` 并填写名称、编码、可选描述、排序（默认 `0`）、状态（显式默认 `1` 启用）与数据范围（默认 `self`，可选 `self` / `org` / `org_and_children` / `all`）后提交
- **THEN** 页面 SHALL 向 `POST /api/admin/roles` 提交表单，并明确说明自定义组织数据范围（`custom`）需在创建后前往角色详情页「编辑数据范围」配置
- **AND** 创建成功后，具备 `admin.roles.id.get` 权限时 SHALL 跳转到新建角色详情页并提示已创建，否则返回 `/roles`
- **AND** 提交失败（如名称/编码冲突 `409 AUTHZ_CONFLICT` 或校验失败）时 SHALL 停留当前页保留输入并展示本地化错误

#### Scenario: 在角色列表或详情页删除角色
- **WHEN** 具有 `admin.roles.id.delete` 权限的管理员在角色列表行操作菜单（含窄屏卡片）或角色详情页选择删除一个非 `admin` 角色
- **THEN** 页面 SHALL 打开二次确认弹窗；若管理员同时具备 `admin.roles.id.users.get` 权限，SHALL 实时读取并展示该角色当前绑定用户数（读取失败时明确提示无法核对人数，不得伪报为 0）
- **AND** 确认弹窗 SHALL 说明删除将级联解除该角色的用户、权限、菜单和数据范围关联，并使关联用户旧会话在后续请求失效且不可撤销
- **AND** 确认后 SHALL 调用 `DELETE /api/admin/roles/:id`；详情页删除成功后跳回 `/roles`，列表页删除成功后刷新列表并在当前页删空且 `page > 1` 时自动回退上一页

#### Scenario: 受保护的 admin 角色不可删除
- **WHEN** 管理员在角色列表或详情页查看编码为 `admin` 的角色
- **THEN** 页面 SHALL 标明受保护，SHALL NOT 提供可执行的角色删除动作

### Requirement: 用户管理新增页面
工作台 SHALL 在用户管理模块提供受 `admin.users.post` 权限控制的独立新建用户页面（`/users/new`），对接 `POST /api/admin/users` 完成纯用户身份创建；既有用户删除交互与入口 SHALL 保持不变。

#### Scenario: 管理员新建用户
- **WHEN** 具有 `admin.users.post` 权限的管理员从用户列表进入 `/users/new` 并提交用户名、邮箱、符合规则的密码（及一致的确认密码）、可选昵称与启用状态（默认 `1`）
- **THEN** 页面 SHALL 在本地校验两次密码一致后调用 `POST /api/admin/users`，不发送多余的角色或组织字段
- **AND** 创建成功后，具备 `admin.users.id.get` 权限时 SHALL 跳转到新建用户详情页并展示成功提示以便继续分配角色与组织，否则返回 `/users`
- **AND** 当服务端返回字段错误（如 `password / IDENTITY_PASSWORD_INVALID`）、冲突（`409 IDENTITY_CONFLICT`）或非全量范围拒绝（`403 IDENTITY_PERMISSION_DENIED`）时，页面 SHALL 停留当前页保留草稿并展示集中本地化错误

### Requirement: 组织管理新增与删除
工作台 SHALL 在组织管理模块提供独立新建组织页面（`/organizations/new`）与可管理组织详情页的删除交互；范围外只读祖先节点 SHALL NOT 提供新建子组织或删除动作。

#### Scenario: 新建组织与新建子组织
- **WHEN** 具有 `admin.organizations.tree.get` 与 `admin.organizations.post` 权限的管理员从组织列表进入 `/organizations/new`，或从可管理组织详情页点击「新建子组织」进入 `/organizations/new?parentId=:id`
- **THEN** 页面 SHALL 读取组织树，上级选择仅允许选择 `manageable: true` 的节点；从详情页带入的有效可管理 `parentId` SHALL 自动预选
- **AND** 当组织树包含 `manageable: false` 只读祖先（表明非全量范围）时，页面 SHALL 禁止选择「作为根组织」并提示须挂在可管理上级下；最终范围校验仍以服务端响应为准
- **AND** 提交成功调用 `POST /api/admin/organizations` 后 SHALL 跳转到新建组织详情页 `/organizations/:id` 并展示成功提示；失败时 SHALL 停留当前页保留输入并展示错误

#### Scenario: 在可管理组织详情页删除组织
- **WHEN** 具有 `admin.organizations.id.delete` 权限的管理员在 `manageable: true` 的组织详情页查看删除区
- **THEN** 若已加载组织树显示该节点仍存在可见下级子组织，页面 SHALL 前置阻止删除并明确说明需先调整或删除下级组织
- **AND** 若无可见下级子组织，点击删除 SHALL 打开二次确认弹窗，说明将解除该组织成员绑定、使受影响成员旧会话失效且删除不可在界面撤销
- **AND** 确认后调用 `DELETE /api/admin/organizations/:id`，成功后跳回 `/organizations`；若服务端因角色自定义数据范围引用、受保护/范围外成员或并发子组织拒绝删除，SHALL 留在详情页并展示本地化错误提示

## MODIFIED Requirements

### Requirement: 工作台导航分组与响应式壳层
工作台 SHALL 使用基于 shadcn/ui 与 Radix 的导航壳层，在不改变后端菜单、权限码和路由事实的前提下，按展示层分组呈现当前操作者可访问的工作台入口。展示层分组 SHALL 包含“授权核对”“身份与组织”“配置管理”和“审计”；配置管理只承载菜单管理与接口管理入口；审计只承载审计日志查询入口。工作台顶栏 SHALL 吸顶展示静态层级面包屑、全局操作及桌面端已打开页面栏。新建子路由 `/roles/new`、`/users/new` 与 `/organizations/new` SHALL 归属对应一级管理模块页签，并在面包屑中展示「新建角色」「新建用户」「新建组织」，不得被误解析为对象详情 `#new`。

#### Scenario: 新建子路由面包屑与模块页签归属
- **WHEN** 具备对应创建权限的管理员访问 `/roles/new`、`/users/new` 或 `/organizations/new`
- **THEN** 顶栏面包屑 SHALL 分别展示「工作台 / 角色管理 / 新建角色」、「工作台 / 用户管理 / 新建用户」或「工作台 / 组织管理 / 新建组织」
- **AND** 桌面端已打开页面栏 SHALL 将其归入对应的「角色管理」「用户管理」或「组织管理」单一模块页签，并按对应模块菜单与写权限判定页签可访问性

### Requirement: 编辑离开确认与一致的失败呈现
工作台 SHALL 对角色授权、用户归属、组织移动、个人资料编辑，以及角色、用户、组织新建表单中的真实未保存修改提供离开确认，并复用 `standardize-api-response-contract` 所定义的集中 API Client、稳定错误码本地化、字段错误和原生响应路径；本能力 SHALL NOT 另立信封或错误码契约。个人资料页的密码草稿 SHALL 遵循敏感输入清理规则，不改变取消离开必须保留草稿的保证。

#### Scenario: 带未保存更改导航
- **WHEN** 用户在上述任一编辑页或新建页修改输入后点击返回、导航到其他页面、使用浏览器后退或直接离开
- **THEN** 页面 SHALL 先询问是否放弃未保存修改；取消 SHALL 保留当前表单输入，确认 SHALL 才允许离开

#### Scenario: 字段校验或未知错误
- **WHEN** 请求返回可定位的字段错误或未知稳定错误码
- **THEN** 页面 SHALL 使用集中 Client 的字段级提示及已知错误码的中文本地化；未知码 SHALL 使用安全英文消息作回退
- **AND** 页面 SHALL 不展示原始内部错误、伪造的成功结果或第二套组件内信封解析

#### Scenario: 原生资源与错误边界
- **WHEN** 工作台加载头像等原生资源或处理管理操作的网络失败
- **THEN** 头像读取和上传等原生路径 SHALL 保留各自既定接口语义
- **AND** 网络异常、响应格式异常或会话失效 SHALL 通过集中 Client 一致处理，不出现未捕获错误页
