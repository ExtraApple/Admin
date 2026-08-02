## 1. 前置条件与行为基线

- [x] 1.1 确认 `migrate-access-version-storage` 已完成切换和退出里程碑，并记录本 Change 只消费 Authorization 授权版本 Contract、不迁移、镜像或恢复 `users.token_version`；当前复核证据见 `evidence/2026-08-02-access-version-cutover-current.md`
- [ ] 1.2 固化当前 Gin 业务路由的 Method、Path、中间件等级和 Permission Code 快照，并记录 Public、Authenticated、PermissionControlled 映射、首次同步例外以及 `POST /api/admin/permissions/sync` 的现有行为
- [ ] 1.3 补齐 HTTP Method、Path、请求/响应结构、状态码和稳定错误码兼容测试
- [ ] 1.4 补齐认证与用户主规格回归：验证码一次性消费、密码复杂度、登录锁定、JWT 用途和存量 Token 兼容、Refresh Token 稳定 401、黑名单、用户上下文、头像安全、API 状态、Permission Code、菜单树和数据范围
- [ ] 1.5 建立 SQLite 快速数据库测试环境，以及真实 MySQL、Redis 和 MinIO 关键链路集成测试环境
- [ ] 1.6 创建模块边界、禁止依赖和唯一 Gin 注册点的架构测试骨架
- [ ] 1.7 将固定一级模块、调用方 Contract、Route Catalog 和组合根决策同步为 ADR
- [ ] 1.8 创建隔离的 SQLite 测试 Helper，使用独立临时库或唯一 shared-memory DSN，防止多连接和并行测试污染
- [ ] 1.9 使用当前授权版本前置证据复核：代码和测试只依赖 `user_access_versions` Contract，Seed 不创建授权版本，重构任务不等待或恢复已退役的旧字段迁移能力

## 2. App 与 Platform 基础结构

- [ ] 2.1 创建 `internal/platform/config` 和配置加载 Adapter，并保持现有配置字段兼容
- [ ] 2.2 创建 `internal/platform/logging`，由 App 构造 Zap Logger 并移除新增全局日志依赖
- [ ] 2.3 创建 `internal/platform/database`，集中构造 GORM/MySQL 连接并保持现有连接配置
- [ ] 2.4 创建 `internal/platform/cache` 和 `internal/platform/objectstorage`，封装现有 Redis 与 MinIO 客户端创建
- [ ] 2.5 实现基于 Context 的 GORM `TransactionRunner`，支持加入已有事务和有界 MySQL 死锁/锁等待重试；明确最外层回调重试、嵌套 Capability 不自行提交、上下文取消和提交后副作用不重复执行的边界
- [ ] 2.6 创建 `internal/app` 组合根，显式装配 Platform、Repository、Application Service 和后台任务
- [ ] 2.7 将 AutoMigrate Model 收集迁入 `internal/app/migrate.go`，保持迁移失败阻止启动
- [ ] 2.8 将 Seed 顺序和事务编排迁入 `internal/app/seed.go`，保持 Seed 幂等和现有默认数据行为，并让 API/权限相关 Seed 消费 Route Catalog Snapshot 而非扫描 Gin Engine
- [ ] 2.9 创建仅供 App 使用的 `legacyglobal` 兼容 Adapter，并通过架构测试禁止新增业务代码导入 `global`
- [ ] 2.10 验证新 App/Platform 装配下服务可启动，且现有健康检查、日志、MySQL、Redis 和 MinIO 行为不变

## 3. Route Catalog 与统一路由注册

- [ ] 3.1 在 `internal/routecatalog` 定义 Route Descriptor、Public、Authenticated 和 PermissionControlled
- [ ] 3.2 实现 Descriptor Method、Path、Handler、Access Level、Permission Code 和 OpenAPI operation 组合校验
- [ ] 3.3 实现规范化 `method + path` 重复检测，并在冲突时阻止 HTTP 服务启动
- [ ] 3.4 实现稳定的只读 Route Catalog Snapshot，供 API Metadata、RBAC 路由权限同步、API Doc 和启动 Seed 消费
- [ ] 3.5 创建 `internal/app/http.go`，按 Access Level 统一挂载中间件和注册 Gin 路由
- [ ] 3.6 调整权限中间件，固定 Public、Authenticated、PermissionControlled 路由映射，使 PermissionControlled 始终要求登录且 `need_auth = 0` 只能跳过 Permission Code 检查；保留两个 API Metadata 首次同步接口的 `admin` 引导例外
- [ ] 3.7 增加非法 Descriptor、重复路由、缺少 OpenAPI 描述/Schema、数据库元数据不能动态创建路由的单元和集成测试
- [ ] 3.8 增加 Route Catalog Descriptor 集合与 Gin 实际业务路由集合一致性测试，并覆盖技术路由、路径参数、通配路径和文档开关
- [ ] 3.9 增加 Route Catalog 统一来源测试：启动 Seed、`POST /api/admin/apis/sync`、`POST /api/admin/permissions/sync` 和 OpenAPI 均不得通过 `Gin Engine.Routes()` 另行发现业务路由；`POST /api/admin/apis/sync-permissions` 保持基于 API Metadata 的显式用例
- [ ] 3.10 验证缺少 OpenAPI operation、请求/响应 Schema 或上传字段描述的 Descriptor 在 Gin 注册前失败，禁止静默生成不完整文档

## 4. Dictionary 模块模板验证

- [ ] 4.1 创建 `internal/dictionary`，迁入字典 Model 和所属 AutoMigrate/Seed 定义
- [ ] 4.2 将字典请求/响应 DTO 和校验逻辑迁入 Dictionary 模块
- [ ] 4.3 将字典 Service 用例迁入模块内 Application 或扁平用例文件，并显式注入 Repository
- [ ] 4.4 将字典 GORM 查询迁入模块 Adapter，移除对 `global.DB` 的依赖
- [ ] 4.5 将字典 Handler 迁入 HTTP Adapter并输出 Route Descriptor
- [ ] 4.6 由 App 装配 Dictionary 并切换唯一正式路由入口
- [ ] 4.7 运行字典 CRUD、公开读取和 HTTP 兼容测试后删除字典旧 Handler、Service、DTO、Model 和 Router 入口

## 5. Organization 层级 Capability

- [ ] 5.1 在 `internal/organization` 定义组织单位、组织成员和层级查询的模块所有权
- [ ] 5.2 迁入组织 Model、DTO、GORM Repository 和现有 AutoMigrate/Seed 定义
- [ ] 5.3 实现不暴露 GORM Model 的 Organization Hierarchy 只读 Capability
- [ ] 5.4 迁入组织 CRUD、成员管理和组织树 Application 用例
- [ ] 5.5 迁入组织 HTTP Adapter并输出 Route Descriptor
- [ ] 5.6 由 App 装配 Organization，并验证组织 CRUD、成员关系和组织树行为兼容

## 6. Authorization 核心迁移

- [ ] 6.1 在 `internal/authorization` 迁入角色、权限、用户角色、角色权限和数据范围 Model
- [ ] 6.2 将角色、权限、关联关系和数据范围 GORM 操作迁入 Authorization Adapter
- [ ] 6.3 在 Authorization Domain 中实现角色保护和 Permission Code 值对象规则
- [ ] 6.4 定义只携带用户 ID 的 Principal，以及不暴露持久化实现的 Access Snapshot
- [ ] 6.5 定义 UserScope 和 OrganizationScope，固化 `All`、空集合和 `self` 语义
- [ ] 6.6 在 `authorization/application/contracts.go` 定义调用方所需的 Organization Hierarchy 等最小 Contract
- [ ] 6.7 迁入角色、权限、用户角色、角色权限、数据范围和 Access Snapshot Application 用例
- [ ] 6.8 让 Authorization 通过 Organization Hierarchy Contract 解析 Scope，不直接访问 Organization GORM Adapter
- [ ] 6.9 在用户和组织 Repository 中分别应用 UserScope 和 OrganizationScope，禁止传递 SQL 或 GORM Scope
- [ ] 6.10 通过正式 Contract 接入授权版本读取和 `EnsureAndIncrement`，不得重新实现 TokenVersion 存储迁移
- [ ] 6.11 迁入 Authorization HTTP DTO、Handler 和 Route Descriptor
- [ ] 6.12 由 App 注入 Organization Capability、事务能力和 Repository，并消除 Authorization 对其他模块 Adapter 的导入
- [ ] 6.13 运行角色、权限、数据范围、空 Scope、`self`、Token 失效和超级管理员保护测试
- [ ] 6.14 补齐 RBAC 兼容迁移：权限分组 CRUD、超级管理员 Seed 保护、角色/权限关系失效以及 `POST /api/admin/permissions/sync` 的 Route Catalog 消费和公开路由跳过规则

## 7. Navigation 与 API Metadata 迁移

- [ ] 7.1 创建 `internal/navigation`，迁入菜单、`role_menus` 和 `menu_apis` Model 及 Repository
- [ ] 7.2 迁入菜单 CRUD、角色菜单分配、用户可见菜单树、前端路由同步和菜单 API 查询用例
- [ ] 7.3 创建 `internal/apimetadata`，迁入 `apis` Model、DTO、Repository、状态和策略查询
- [ ] 7.4 迁入 API 元数据 CRUD、分组选项、方法选项和权限同步用例
- [ ] 7.5 拆分不依赖 Navigation 的 API Metadata Storage/Policy Core，并由 App 先行构造
- [ ] 7.6 在 Navigation Application 中实现菜单/API/Permission Code 跨模块绑定的最外层事务用例
- [ ] 7.7 在事务内锁定 API、菜单、Permission 和关联行，并计算变更前后受影响用户
- [ ] 7.8 实现 Permission Code 已存在时合并并去重 `role_permissions`，仅在无引用时清理旧 Permission
- [ ] 7.9 将 API、菜单、`menu_apis`、权限、角色授权和相关用户授权版本写入同一数据库事务
- [ ] 7.10 将缓存清理移动到事务提交后，并增加失败指标和重试入口
- [ ] 7.11 让 API Metadata 的 Permission Code 管理操作委托 Navigation 跨模块绑定用例
- [ ] 7.12 迁入 Navigation 与 API Metadata 的 HTTP Handler、DTO 和 Route Descriptor
- [ ] 7.13 由 App 完成 Authorization、Navigation 和 API Metadata 的分阶段构造与 Contract 注入
- [ ] 7.14 增加菜单/API绑定、API Permission Code 修改、按钮生成每个写入点失败时的完整回滚测试
- [ ] 7.15 增加并发 API 更新、菜单绑定、Permission 合并和 MySQL 死锁有界重试测试
- [ ] 7.16 增加菜单完整兼容矩阵：菜单路径/父子约束、角色菜单分配、用户权限过滤和父级保留、前端路由同步、删除清理以及每个联动写入点的授权版本失效

## 8. API 路由同步与 OpenAPI 生成

- [ ] 8.1 将启动 Seed 及 `POST /api/admin/apis/sync` 的路由来源从 Gin Engine 扫描切换为 Route Catalog Snapshot
- [ ] 8.2 保持已存在 API 元数据、软删除恢复、默认 Permission Code、Public 路由兼容规则以及 `POST /api/refresh` 错误认证标记的纠正行为
- [ ] 8.3 保持 API Metadata 权限同步和 RBAC 路由权限同步均为显式用例；前者读取 API Metadata，后者读取 Route Catalog，不在普通路由注册或 Catalog 收集中隐式创建 Permission
- [ ] 8.4 迁入 `internal/apidoc`，使 OpenAPI 生成消费 Route Catalog 和必要的 API Metadata Snapshot
- [ ] 8.5 保持 `/docs`、`/docs/openapi.json`、DTO Schema 和上传接口文档行为兼容，并验证每个已注册业务路由都出现在 OpenAPI 文档中
- [ ] 8.6 验证 API/权限同步、启动 Seed 不扫描 Gin Engine，且 Route Catalog、API Metadata、OpenAPI 和 RBAC 权限同步的 Method/Path 集合一致
- [ ] 8.7 验证管理员修改 API Metadata Method/Path 后真实路由按 API 未配置拒绝访问，且同步可重新创建正确记录
- [ ] 8.8 验证 `need_audit` 保持当前日志主规格语义：`/api/*` 请求继续异步记录，Catalog 默认审计分类只用于同步新记录，且该字段不改变静态访问等级
- [ ] 8.9 增加首次同步和访问等级测试：匿名请求不能绕过 `PermissionControlled`，已登录 `admin` 可初始化两个 API Metadata 同步接口，`need_auth = 0` 不会把受保护路由降级为匿名

## 9. Identity 与用户上下文迁移

- [ ] 9.1 创建 `internal/identity`，迁入用户 Model、认证 DTO 和用户资料 DTO
- [ ] 9.2 迁入验证码、注册、登录、JWT、Refresh Token、Redis 黑名单和登录锁定 Application 用例
- [ ] 9.3 迁入用户资料、密码、头像、管理员用户操作和用户状态 Application 用例
- [ ] 9.4 在 `identity/application/contracts.go` 定义 Authorization Snapshot、Navigation Menu 和必要组织能力 Contract
- [ ] 9.5 迁入 `/api/user/context` 编排，使 Identity 组合角色、权限、菜单和用户资料
- [ ] 9.6 迁入 Identity 的 GORM、Redis、对象存储和 HTTP Adapter并输出 Route Descriptor
- [ ] 9.7 由 App 注入 Authorization、Navigation、Organization、Redis 和对象存储能力
- [ ] 9.8 删除旧 JWT、用户上下文和数据范围旁路入口
- [ ] 9.9 运行认证与用户上下文兼容测试：登录、注册、验证码消费、密码复杂度、登录锁定、JWT 用途/存量 Token、Refresh Token、登出黑名单、禁用、软删除、恢复、Kick、头像和用户上下文
- [ ] 9.10 验证 Identity 只通过 Authorization Contract 获取授权快照和授权版本；Seed 不创建版本，运行时不恢复 `users.token_version` 或迁移状态依赖

## 10. Files、Upload Security 与 Audit 迁移

- [ ] 10.1 创建 `internal/files`，迁入文件 Model、DTO、Application 用例、GORM Adapter 和 HTTP Adapter
- [ ] 10.2 让 Files 只依赖调用方声明的对象存储和 Upload Security Contract，删除全局 MinIO 旁路
- [ ] 10.3 迁入 `internal/uploadsecurity` 并保持管理员文件与头像的现有验证、重编码和 SHA-256 规则
- [ ] 10.4 创建 `internal/audit`，迁入审计 Model、Repository、Application 用例和 HTTP Adapter
- [ ] 10.5 保持 Audit 当前提交后异步记录语义和稳定错误处理
- [ ] 10.6 由 App 装配 Files、Upload Security 和 Audit，并收集各模块 Route Descriptor
- [ ] 10.7 运行文件上传、列表、详情、改名、下载、预览拒绝、重新验证、浏览、删除、轮转、配置校验、头像安全、对象清理和审计日志回归测试
- [ ] 10.8 验证 Audit 保持请求字段、敏感字段脱敏、multipart 不读文件内容、上传安全 metadata、分类查询、异步提交和冷热归档兼容

## 11. 旧入口删除与架构收口

- [ ] 11.1 将剩余业务 Handler、Service、DTO、Model 和路由逐模块迁入对应 `internal` 模块
- [ ] 11.2 为每个完成迁移的模块切断 legacy Adapter，并确认只有一条正式调用路径
- [ ] 11.3 删除无调用点的旧 middleware、`initialize` 旁路和 `utils` 业务 Helper
- [ ] 11.4 删除 `legacyglobal` 和本 Change 引入的全部兼容 Adapter
- [ ] 11.5 删除全局业务 `handler`、`service`、`dto`、`model` 和 `router` 目录
- [ ] 11.6 强化架构测试，禁止新业务代码重新引入旧目录、全局状态或跨模块 Adapter 依赖
- [ ] 11.7 验证 App 是唯一组合根、唯一 Gin 注册点和唯一迁移/Seed 编排入口

## 12. 文档与最终验证

- [ ] 12.1 更新 README 和模块文档，提供按业务模块阅读代码的导航
- [ ] 12.2 同步 ADR、`CONTEXT.md`、背景方案和受影响 OpenSpec 主规格所需的术语与长期决策；明确授权版本迁移已归档完成，当前 Change 不再依赖旧字段或观察期
- [ ] 12.3 运行全部架构边界、Domain、Application Fake Contract 和 SQLite Repository 快速测试
- [ ] 12.4 运行 Route Catalog、路由访问等级、API Metadata 动态策略和 Gin 路由一致性测试
- [ ] 12.5 运行菜单/API/Permission 强事务、角色授权保留、Token 失效和失败回滚测试
- [ ] 12.6 使用真实 MySQL 验证行锁、死锁重试、并发事务、AutoMigrate 和数据库结构兼容性，并使用真实 Redis 与必要 MinIO 运行关键认证授权端到端测试
- [ ] 12.7 运行 `go test ./... -count=1` 并确认无不稳定测试、数据竞争或旧包引用
- [ ] 12.8 对照 proposal、design 和全部 delta specs 完成验收，确认除批准的原子性增强外没有 HTTP 行为变化
- [ ] 12.9 对照全部主规格和 Delta Spec 完成覆盖矩阵，重点确认 API/权限同步来源、首次同步例外、`/api/refresh` 兼容修正、事务重试边界、审计语义和文件安全边界均有可执行验收
