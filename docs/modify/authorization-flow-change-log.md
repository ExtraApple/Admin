# 权限链路修改记录

本文记录后台权限体系从简单 `role = admin` 判断，逐步演进到 RBAC、动态 API 权限、菜单联动、数据权限和 Token 实时控制的过程。

## 当前目标

- 使用 `roles` / `permissions` / `apis` / `menus` 作为新的权限核心。
- 不再继续依赖旧的单字段 `role` 兜底判断。
- 管理员权限通过新 RBAC 体系判断，保证后续权限可以精细化扩展。
- 菜单、按钮、API、数据范围可以逐步联动，减少前后端权限不一致。

## 已完成修改

### 1. API 管理

- 新增 API 元数据管理，用于维护接口路径、请求方法、权限码和状态。
- 支持 API 创建、修改、删除、查询、同步。
- API 权限码可以作为动态权限判断依据。

### 2. 动态权限中间件

- 新增基于请求方法和路径匹配的动态权限校验。
- 登录用户访问接口时，会根据用户绑定角色、角色绑定权限、API 权限码判断是否允许访问。
- 路由侧只需要挂载统一权限中间件，不再为每个接口手写固定角色判断。

### 3. 菜单权限和按钮权限

- 菜单支持目录、菜单、按钮等类型。
- 菜单可以绑定权限码，用于前端控制可见菜单和按钮。
- 用户登录后可以根据角色权限返回可访问菜单树。

### 4. Token 实时生效

- Authorization 拥有授权版本，独立表 `user_access_versions` 是唯一读取事实来源。
- JWT 中继续携带授权版本，Access Token 和 Refresh Token 使用同一版本来源。
- 登录通过 `EnsureVersion` 幂等初始化版本；非登录认证缺行时拒绝 Token，不回退旧字段。
- 用户权限、状态、密码等关键安全信息变化后，通过 `EnsureAndIncrement` 创建或提升版本，
  使旧 Access Token 和 Refresh Token 立即失效。

### 5. 用户强制下线

- 管理员可以强制指定用户下线。
- 强制下线通过 Authorization 的 `EnsureAndIncrement` 创建或提升授权版本。
- 用户下次携带旧 Access Token 和 Refresh Token 时会被拒绝，需要重新登录。

### 6. 新体系超级管理员兜底

- 启动时检查是否存在新 RBAC 体系下的超级管理员。
- 如果不存在，则根据环境配置自动创建超级管理员账号。
- 超级管理员不再依赖旧 `role = admin` 字段判断。

### 7. 配置迁移到 `.env`

- 将核心配置从 `config.yaml` 迁移到 `.env`。
- 管理员初始账号、密码、数据库、Redis、MinIO、JWT 等配置统一通过环境变量读取。
- 避免把敏感配置直接写入仓库。

### 8. 数据权限

- 新增组织数据范围判断。
- 支持按本人、本组织、本组织及子组织、全部数据等范围控制查询。
- 为后续组织管理、用户管理、日志管理等模块提供统一数据边界。

### 9. 菜单与 API 联动

- 新增菜单与 API 绑定关系。
- 支持从 API 快速生成菜单按钮。
- API 权限码更新后，可以同步影响绑定菜单按钮。
- 删除菜单或 API 时，会清理对应联动关系，避免残留脏数据。

### 10. API 管理收尾

- 补齐 API 详情接口。
- 补齐 API 分组列表接口，供前端筛选和下拉选项使用。
- 补齐 HTTP 方法列表接口，供前端创建、修改、筛选 API 使用。
- API 管理文档补充 Apifox 测试顺序。
- 路由测试覆盖 API 管理关键路由，避免 Gin 路由冲突。

### 11. 自动化 API 文档

- 新增 `/docs` Swagger UI 页面。
- 新增 `/docs/openapi.json` OpenAPI 3.0 JSON。
- OpenAPI 文档基于 Gin 路由和 `apis` 表元数据生成。
- 文档会结合 `apis.name`、`apis.api_group`、`apis.remark`、`apis.status`、`apis.need_auth` 生成标题、分组、描述、废弃状态和认证要求。
- JSON 请求体根据路由对应 DTO 生成 Schema，避免 Swagger UI 显示 `additionalProp`。
- 没有请求体的同步、退出登录、强制下线等接口不再显示空的通用 Request body。
- 上传接口保留 `multipart/form-data` 的 `file` 字段。
- 自动文档由 `config.yaml` 中的 `api_docs.enabled` 控制，生产环境可关闭。

## 当前验证

- `go test ./...` 已通过。
- `openspec validate --all` 已通过。

## 后续维护

后续工程化差距、模块优先级和未实现功能不在本文重复维护，统一查看 `mature-admin-system-comparison-recommendations.md`。
