# 权限管理

> 本文只保留权限模块边界和同步规则。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/rbac/spec.md`](../../openspec/specs/rbac/spec.md) 为准。

## 模块边界

`internal/authorization` 负责角色、权限、权限分组、角色权限关系和访问快照。API 元数据由 `internal/apimetadata` 负责，菜单由 `internal/navigation` 负责。

## 关键规则

- 权限码必须唯一；创建后权限码不可通过普通修改接口变更。
- 权限分组、权限记录和角色权限关系支持管理接口。
- 角色权限分配是全量替换，空列表表示清空。
- 用户有效权限码从全部可用角色权限关系推导，并在授权上下文中去重。
- `admin` 角色拥有超级管理员兜底能力，但禁用 API 仍然拒绝访问。
- API 权限同步、启动 Seed 和 API 文档都消费同一份 Route Catalog Snapshot，不扫描 Gin Engine。
- 角色权限、菜单/API 联动等变更会使受影响用户的旧 Token 失效。

## 接口入口

Swagger UI 的 `authorization` 标签提供：

- 权限和权限分组 CRUD。
- 权限码列表。
- Route Catalog 权限同步。
- 角色权限分配和查询。

新增或修改权限行为时，先更新 OpenSpec，再更新本页摘要。
