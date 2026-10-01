# 角色管理

> 本文只保留角色模块边界和不可忽略的约束。接口路径、字段和响应 Schema 以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/rbac/spec.md`](../../openspec/specs/rbac/spec.md) 为准。

## 模块边界

角色、用户角色、角色权限和数据范围由 `internal/authorization` 拥有。角色菜单用例和菜单树由 `internal/navigation` 拥有，即使接口路径包含 `/roles/:id/menus`。

## 关键规则

- 角色名称和编码必须唯一。
- 编码为 `admin` 的超级管理员角色不可修改或删除，并固定拥有全部数据范围。
- 角色中心的用户、权限和菜单分配为全量替换；用户定向角色 PUT 是另一种粒度，不覆盖其他用户。
- 角色权限读取按启用规则处理；管理员角色菜单读取则返回所有显式已分配项，包括停用菜单，不可与用户运行时菜单混用。
- `all`、`self`、`org`、`org_and_children`、`custom` 是支持的数据范围值。
- `custom` 数据范围使用角色与组织单位关联记录。
- 角色、权限、菜单或数据范围变更会使受影响用户的旧 Access Token 和 Refresh Token 失效。
- 受影响用户的授权版本提升必须和授权关系变化处于同一事务。

## 接口入口

Swagger UI 中主要分为：

- 角色 CRUD。
- 角色用户分配和查询。
- 角色权限分配和查询。
- 角色数据范围配置和查询。

权限码、角色菜单和组织数据范围的详细行为分别以 RBAC、菜单和组织 OpenSpec 为准。

## 工作台集成

现有工作台路由为 `/roles`、`/roles/:id` 及 `/roles/:id/permissions`、`/roles/:id/menus`、`/roles/:id/data-scope`。列表需要 `admin.roles.get`，详情需要 `admin.roles.id.get`；`GET /api/admin/roles/:id` 的 `data` 含 `id,name,code,description,sort,status,data_scope`。

每个授权维度独立读取与保存，权限码分别为 `admin.roles.id.permissions.get/post`、`admin.roles.id.menus.get/post`、`admin.roles.id.data-scope.get/post`（斜线表示两个独立权限码）。编辑还需角色详情读取；权限候选读取需 `admin.permissions.get`，菜单候选读取需 `admin.menus.get`，自定义组织选择需 `admin.organizations.tree.get`。数据范围读取为 `{role_id,data_scope,organization_ids}`；勾选父组织不隐式勾选后代，`manageable=false` 不可选。

保存前确认本维度变更与旧会话失效；失败保留未提交选择。受保护 `admin` 角色只读。上述新增详情行为仍由 [`rbac delta`](../../openspec/changes/build-admin-workbench/specs/rbac/spec.md) 追踪，不以代码存在宣称验收完成。
