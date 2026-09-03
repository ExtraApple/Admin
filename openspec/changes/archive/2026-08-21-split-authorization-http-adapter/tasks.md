## 1. 固化拆分基线

- [x] 1.1 增加 Authorization Route Descriptor 有序契约测试，覆盖现有 20 条路由的 Method、Path、Access Level、名称、分组、Permission Code、审计分类、Handler 和 OpenAPI Schema
- [x] 1.2 增加基于 Go AST 的 HTTP Adapter 结构测试，要求 `routes.go` 只保留路由聚合且禁止用行数或源码文本判断
- [x] 1.3 运行现有 Authorization HTTP、App Route Catalog 和 OpenAPI 测试，记录拆分前基线

## 2. 按能力拆分 HTTP Adapter

- [x] 2.1 将 `Handler`、`authRoute`、共享成功/错误 Schema、分页、path ID、JSON binding 和当前 bad-request 辅助移动到 `shared.go`，保持名称和行为不变
- [x] 2.2 将角色 CRUD、角色用户、角色数据范围及所属请求/响应 DTO 和映射移动到 `roles.go`
- [x] 2.3 将权限 CRUD、权限码列表、路由权限同步、角色权限及所属请求/响应 DTO 和映射移动到 `permissions.go`
- [x] 2.4 将权限分组 CRUD 及所属请求/响应 DTO 移动到 `permission_groups.go`
- [x] 2.5 将 `routes.go` 收敛为单一有序 Descriptor 字面量，逐项保持现有 20 条路由及相对顺序
- [x] 2.6 删除拆分后重复或失去调用点的声明并格式化 Authorization HTTP Adapter，不新增子包或跨模块 HTTP Helper

## 3. 验证纯结构变更

- [x] 3.1 运行 Authorization HTTP Adapter 的结构和有序 Descriptor 契约测试
- [x] 3.2 运行 Authorization Application、HTTP 和 App Authorization 集成测试，确认角色、权限、用户关系、数据范围和权限同步行为不变
- [x] 3.3 运行 Route Catalog、API Metadata 同步、RBAC 同步、OpenAPI 和 App 路由快照测试，确认 Method/Path/顺序/Schema 无漂移
- [x] 3.4 运行 `go test ./... -count=1` 并确认请求/响应 JSON、状态码、消息、权限码和外部行为无变化
- [x] 3.5 对照 proposal、design 和 modular-layered-architecture Delta Spec 完成 Change 验收
