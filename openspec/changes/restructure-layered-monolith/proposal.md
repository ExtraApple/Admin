## Why

当前项目按 `handler/service/dto/model/router` 技术职责横向拆分，阅读或修改一个业务能力通常需要跨越三到四个目录，并且全局基础设施、集中式路由和跨模块数据库写入使边界难以验证。现在需要在保持既有 HTTP 契约和权限行为稳定的前提下，重构为业务模块优先的分层单体，为后续功能扩展和测试隔离建立清晰边界。

## What Changes

- **BREAKING（内部代码结构）**：将业务代码迁入固定的 `internal/app`、`internal/platform`、`internal/identity`、`internal/authorization`、`internal/navigation`、`internal/apimetadata`、`internal/organization`、`internal/dictionary`、`internal/files`、`internal/audit`、`internal/routecatalog`、`internal/apidoc` 和 `internal/uploadsecurity` 一级模块。
- 复杂模块在模块内部使用 Domain、Application、Adapter 分层；简单模块允许保持扁平，避免为形式统一创建空层。
- 将 Authorization 有限拆分为授权核心、Navigation 和 API Metadata，明确角色/权限、菜单关系和 API 元数据的唯一所有者。
- 建立 `internal/app` 组合根和 `internal/platform` 基础设施装配，逐步移除业务代码对 `global`、`initialize` 及跨模块 Adapter 的依赖。
- 业务模块输出 Route Descriptor，由 Route Catalog 收集、校验并提供给 App；只有 App 可以向 Gin Engine 注册路由。
- 将路由最低访问级别固定为 Public、Authenticated 和 PermissionControlled；API Metadata 只能在 PermissionControlled 范围内控制启用状态和权限码检查，不能把受保护路由改成匿名路由。
- 区分 Route Catalog 的代码静态事实和 API Metadata 的管理员运行时事实；保留 API Method/Path 可编辑行为以及现有 `NeedAudit` 语义。
- 跨模块调用改为调用方定义的最小 Contract，由 App 显式注入；Application 不得导入其他模块 Adapter 或暴露 GORM Model。
- Authorization 将数据范围解析为资源类型明确的 `UserScope`、`OrganizationScope` 等值类型，由拥有数据的业务模块应用过滤；空集合不得解释为全部数据。
- 菜单、API、Permission Code、角色授权关系和相关用户会话失效在同一数据库事务中完成；失败时不得留下部分更新。
- 通过兼容 Adapter 渐进迁移，迁移完成后删除旧的全局 `handler/service/dto/model/router` 业务目录和旁路入口。
- 保持现有 HTTP Method、Path、请求/响应结构、状态码、稳定错误码、Redis key、MinIO bucket 和文件安全策略不变。
- 本 Change 不负责把 `users.token_version` 迁移到 `user_access_versions`；该数据库演进由 `migrate-access-version-storage` Change 处理，本 Change 在其达到切换里程碑后开始。
- 非目标：微服务化、引入 Casbin、切换数据库迁移工具、重写全部业务行为、增加新的一级业务模块或一次性移动整个仓库。

## Capabilities

### New Capabilities

- `modular-layered-architecture`: 定义固定业务模块、模块内分层、依赖方向、跨模块 Contract、组合根、兼容迁移和旧结构退出约束。
- `route-catalog`: 定义模块路由描述、重复校验、静态访问等级、App 唯一 Gin 注册点，以及 API Metadata、RBAC 路由权限同步、启动 Seed 和 API Doc 对路由目录的消费边界。

### Modified Capabilities

- `api-management`: API 路由同步和自动化 API 文档改为消费 Route Catalog；修改已绑定 API 的 Permission Code 时，菜单、权限及角色授权合并和会话失效必须原子完成。
- `menu-management`: 菜单与 API 绑定、Permission Code 同步及相关用户会话失效必须在同一事务中完成，失败时全部回滚。
- `rbac`: RBAC 路由权限同步改为消费 Route Catalog，保持公开路由跳过、默认权限码生成和不重复创建行为。
- `logging`: Zap 运行日志改由 App/Platform 装配并注入，保持启动、请求、文件轮转和错误日志的可观察字段及敏感信息保护。

## Impact

- 主要影响 Go 包结构、构造函数、Repository/Contract、Gin 路由装配、GORM 事务传递、迁移/Seed 编排和架构测试。
- HTTP API 对调用方保持兼容；明确批准的可观察增强是跨菜单、API、权限和会话失效操作不再出现部分成功。
- `apis`、`menus`、`menu_apis`、`permissions`、`role_permissions`、`role_menus` 等现有表继续使用，不在本 Change 中增加业务表。
- Redis 黑名单 key、权限缓存策略、MinIO bucket 和对象命名规则不变。
- API Metadata 同步、RBAC 路由权限同步、启动 Seed 和 OpenAPI 统一消费 Route Catalog Snapshot；这改变内部路由事实来源，不改变对外 HTTP 契约。
- 实施需要同步 README、模块文档、ADR 和 OpenSpec，并以兼容测试保护渐进迁移。
