# 组织管理

> 本文只保留组织模块边界和数据范围注意事项。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/organization-management/spec.md`](../../openspec/specs/organization-management/spec.md) 为准。

## 模块边界

`internal/organization` 负责组织单位、组织树、组织成员关系和资源 Scope 查询。

## 关键规则

- `parent_id=0` 表示根组织；组织编码全局唯一。
- 创建或修改组织时，父组织必须存在，且不能形成自引用或循环。
- 组织树按 `sort asc, id asc` 排序。
- 删除存在子组织的节点必须拒绝；删除组织时清理成员关联。
- 成员分配是覆盖式操作，提交空列表表示清空该组织成员。
- 组织列表、组织树和成员查询均应用当前用户的数据范围。
- `self` 数据范围没有可见组织；树查询会保留可见节点所需的祖先节点。
- 数据范围只限制可见数据，不替代 API 动态权限。

## 接口入口

Swagger UI 的 `organization` 标签提供：

- 组织单位列表、树、创建、修改、删除。
- 组织成员分配和查询。

规则变化以 OpenSpec 为准；本页不重复维护请求和响应示例。
