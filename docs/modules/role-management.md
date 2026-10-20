# 角色管理

> 本文只保留角色模块边界和不可忽略的约束。接口路径、字段和响应 Schema 以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/rbac/spec.md`](../../openspec/specs/rbac/spec.md) 为准。

## 模块边界

角色、用户角色、角色权限和数据范围由 `internal/authorization` 拥有。角色菜单用例和菜单树由 `internal/navigation` 拥有，即使接口路径包含 `/roles/:id/menus`。

## 关键规则

- 角色名称和编码必须唯一。
- 编码为 `admin` 的超级管理员角色不可修改或删除，并固定拥有全部数据范围。
- 角色PUT的`description`遗漏保持、显式空字符串清空、非空替换；`name`／`code`的显式空或纯空白整次拒绝。DTO与Application均保留存在性，JSON null沿原解析表示未提交；创建默认不改。真实说明清空继续与成员版本同事务，后续版本失败不留部分写入。[S1定向验收](../reviews/evidence/authorization-field-clearing-2026-10-18.json)已完成，主规格已按依赖同步，未归档。
- 角色中心的用户、权限和菜单分配为全量替换；用户定向角色 PUT 是另一种粒度，不覆盖其他用户。
- 管理员的角色权限／菜单配置读取不等同于运行时授权；角色菜单返回所有显式已分配项，包括停用菜单。运行时仍按角色、用户、菜单的适用启停规则处理。
- `all`、`self`、`org`、`org_and_children`、`custom` 是支持的数据范围值。
- `custom` 数据范围使用角色与组织单位关联记录。
- 角色共享字段、权限、菜单或数据范围的真实配置变化会使当前依赖用户的旧Access／Refresh失效；名称、说明、排序等真实展示变化也不因另一角色提供重复授权而豁免。
- 合法同值字段和等价配置在现有保护、引用及并发前提满足后成功，不重建等价关联、不创建／提升版本。权限比较读取原始`role_permissions`，不按有效权限过滤；非custom模式仍有必须清除的旧custom关联时属于真实变化。此目标已完成[配置no-op验收](../reviews/evidence/configuration-noop-2026-10-18.json)，delta未同步、未归档，不扩展为成员差量或最终有效权限diff。
- 受影响用户的授权版本提升必须和授权关系变化处于同一事务。
- 直接编辑`admin`角色的用户、权限或菜单关联均拒绝，包括同值和空集合；保护身份来自现存`admin`实际关联，角色停用不解除保护，也不因此授予运行时权限。
- 角色下用户是完整替换：正常目标须合法、存在、未删除且在范围内；只有实际移除的软删普通旧成员可在范围、本人和保护检查全部满足时受控清理。清空不豁免旧对象检查，用户定向角色PUT仍保留CAS。
- 纯角色成员完整目标只增删差量：A/B→B/C仅使A/C失效，B关系及版本保持，等价集合不补／升任何成员版本。逐人角色写入仍先Ensure及CAS；等价目标返回current，真实归属变化只升目标一次，包括停用普通角色等不改变最终有效权限的归属变化。实际原始目标／删除对象的保护和范围前提不因no-op豁免；见[成员差量验收](../reviews/evidence/membership-delta-2026-10-18.json)，delta未同步、未归档。
- 组织真正移动父级时，适用`org_and_children`角色与成员根的后代边界变化会使依赖用户失效；另一`all/admin`角色或重复授权不能抵消独立层级依赖。角色本身固定`all`不自动制造该依赖，普通固定`org/self/custom`也不因此扩大。[I4专项验收](../reviews/evidence/organization-hierarchy-dependents-2026-10-18.json)包含角色启用／删除与移动的双向真实交错，不改变角色配置或纯成员差量边界。
- 停用普通用户／角色仍可合法预配置；权限／菜单配置不新增一套组织数据范围。合法普通共享配置可以间接影响操作者，仍按当前规则失效其旧会话，不能套用直接本人写入禁令。

上述关联保护新目标已完成本地定向验收，尚不表示生产发布或其他新目标完成；完整拒绝、停用admin身份、预配置与旧会话边界见[去敏证据](../reviews/evidence/protected-authorization-assignments-2026-10-17.json)。

- 普通角色删除锁定当前角色及实际成员，调用Navigation清理`role_menus`，与`user_roles`、`role_permissions`、`role_data_scopes`、角色实体及成员版本共同提交；共享菜单和权限不删除，第二个成员版本失败也整体回滚。
- 绑定先提交后合法删除可两次成功，删除必须包含新成员；删除先提交后不能重建引用。R4-R6已在隔离原程序中定向验收，见[生命周期证据](../reviews/evidence/authorization-reference-lifecycle-2026-10-18.json)，不代表其他C6目标或生产切换完成。

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

保存前确认本维度的实际变化及相应旧会话失效；合法等价配置不触发失效，失败保留未提交选择。受保护 `admin` 角色只读。上述新增详情行为仍由 [`rbac delta`](../../openspec/changes/build-admin-workbench/specs/rbac/spec.md) 追踪，不以代码存在宣称验收完成。
