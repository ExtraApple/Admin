# 角色管理

> 本文只保留角色模块边界和不可忽略的约束。接口路径、字段和响应 Schema 以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/rbac/spec.md`](../../openspec/specs/rbac/spec.md) 为准。

## 模块边界

角色、用户角色、角色权限和数据范围由 `internal/authorization` 拥有。角色菜单用例和菜单树由 `internal/navigation` 拥有，即使接口路径包含 `/roles/:id/menus`。

## 关键规则

- 角色名称和编码必须唯一。
- 编码为 `admin` 的超级管理员角色不可修改或删除，并固定拥有全部数据范围。
- 用户、权限和菜单分配均为全量替换；空列表表示清空关联。
- 查询角色用户、角色权限和角色菜单时返回脱敏或启用数据。
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
