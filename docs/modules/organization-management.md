# 组织管理

> 本文只保留组织模块边界和数据范围注意事项。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/organization-management/spec.md`](../../openspec/specs/organization-management/spec.md) 为准。

## 模块边界

`internal/organization` 负责组织单位、组织树、组织成员关系和资源 Scope 查询。

## 关键规则

- `parent_id=0` 表示根组织；组织编码全局唯一。
- 创建或修改组织时，父组织必须存在，且不能形成自引用或循环。
- 组织树按 `sort asc, id asc` 排序。
- 删除存在子组织的节点必须拒绝；删除组织时清理成员关联。
- 组织中心的成员分配是覆盖式操作，空列表清空该组织成员；工作台逐人成员维护不得调用这一接口。
- 组织列表、组织树和成员查询均应用当前用户的数据范围。
- `self` 数据范围没有可见组织；树查询保留可见节点所需祖先，`manageable=false` 的祖先仅用于定位，不可读成员、移动或作为归属／自定义范围选择目标。
- 数据范围只限制可见数据，不替代 API 动态权限。

## 接口入口

Swagger UI 的 `organization` 标签提供：

- 组织单位列表、树、创建、修改、删除。
- 组织成员分配和查询。

规则变化以 OpenSpec 为准；本页不重复维护请求和响应示例。

## 工作台集成

路由为 `/organizations`、`/organizations/:id`、`/organizations/:id/move`。列表需要 `admin.organizations.get` 和 `admin.organizations.tree.get`，详情从受 `admin.organizations.tree.get` 保护的树定位可管理节点，成员读取另需 `admin.organizations.id.users.get`；移动需 `admin.organizations.id.put`。

树节点包含 `id,parent_id,name,code,remark,sort,status,manageable,children`。范围外祖先只是路径占位，不是授权目标。移动确认展示旧路径／新路径，由后端最终拒绝循环及越权。

加入／移除一名成员需要用户详情读取与 `admin.users.id.organizations.put`，候选用户列表另需 `admin.users.get`。先读取目标用户最新详情，在其可管理组织集合增删当前组织，再提交显式数组与 `expected_access_version`；不覆盖整组成员、不移除其他归属。409 必须重新读取核对，不显示成功或自动覆盖。隐藏归属、加入时间与受保护用户规则见 [用户管理](user-management.md)；实施中的新增契约见 [`组织 delta`](../../openspec/changes/build-admin-workbench/specs/organization-management/spec.md)。
