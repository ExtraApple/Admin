## 1. 前置条件与行为基线

- [x] 1.1 确认 `migrate-access-version-storage` 已完成切换和退出里程碑，并记录本 Change 只消费 Authorization 授权版本 Contract、不迁移、镜像或恢复 `users.token_version`；当前复核证据见 `evidence/2026-08-02-access-version-cutover-current.md`
- [x] 1.2 固化当前 Gin 业务路由的 Method、Path、中间件等级和 Permission Code 快照，并记录 Public、Authenticated、PermissionControlled 映射、首次同步例外以及 `POST /api/admin/permissions/sync` 的现有行为
- [x] 1.3 补齐 HTTP Method、Path、请求/响应结构、状态码和稳定错误码兼容测试
- [x] 1.4 补齐认证与用户主规格回归：验证码一次性消费、密码复杂度、登录锁定、JWT 用途和存量 Token 兼容、Refresh Token 稳定 401、黑名单、用户上下文、头像安全、API 状态、Permission Code、菜单树和数据范围
- [x] 1.5 建立 SQLite 快速数据库测试环境，以及真实 MySQL、Redis 和 MinIO 关键链路集成测试环境
- [x] 1.6 创建模块边界、禁止依赖和唯一 Gin 注册点的架构测试骨架
- [x] 1.7 将固定一级模块、调用方 Contract、Route Catalog 和组合根决策同步为 ADR
- [x] 1.8 创建隔离的 SQLite 测试 Helper，使用独立临时库或唯一 shared-memory DSN，防止多连接和并行测试污染
- [x] 1.9 使用当前授权版本前置证据复核：代码和测试只依赖 `user_access_versions` Contract，Seed 不创建授权版本，重构任务不等待或恢复已退役的旧字段迁移能力

## 2. App 与 Platform 基础结构

- [x] 2.1 创建 `internal/platform/config` 和配置加载 Adapter，并保持现有配置字段兼容
- [x] 2.2 创建 `internal/platform/logging`，由 App 构造和关闭 Zap Logger；新模块不导入 `global`，旧日志入口只由 App 边缘兼容绑定
- [x] 2.3 创建 `internal/platform/database`，集中构造 GORM/MySQL 连接并保持现有连接配置
- [x] 2.4 创建 `internal/platform/cache` 和 `internal/platform/objectstorage`，封装现有 Redis 与 MinIO 客户端创建
- [x] 2.5 实现基于 Context 的 GORM `TransactionRunner`，支持加入已有事务和有界 MySQL 死锁/锁等待重试；明确最外层回调重试、嵌套 Capability 不自行提交、上下文取消和提交后副作用不重复执行的边界
- [x] 2.6 创建 `internal/app` 组合根，集中装配 Platform 资源、迁移、Seed、HTTP、旧 Application Service 兼容入口、后台任务和资源关闭顺序；`main` 只构建并运行 App
- [x] 2.7 将 AutoMigrate Model 收集迁入 `internal/app/migrate.go`，保持迁移失败阻止启动
- [x] 2.8 将 Seed 的两个事务阶段及中间 API/权限同步顺序迁入 `internal/app/seed.go`，保持幂等和默认数据行为；Descriptor 路径由 App 捕获单次 Route Catalog Snapshot，尚未静态描述的旧路由仅通过命名明确的 `SeedLegacyRoutes` 兼容入口接入，并绑定后续统一来源删除节点
- [x] 2.9 创建仅供 App 使用的 `internal/app/legacyglobal` 兼容 Adapter，并通过架构测试保证它是 `internal` 新代码唯一允许导入 `global` 的位置且只有 App 组合根可导入该 Adapter
- [x] 2.10 通过真实 `httptest` 监听验证新 App/Platform 装配下旧 HTTP 契约和 `/ping` 可启动，并通过 App、Logging、MySQL 构造、Redis、MinIO、Seed、Service 和架构边界回归验证现有行为；真实外部基础设施强门禁仍由 12.6 统一执行

## 3. Route Catalog 与统一路由注册

- [x] 3.1 在 `internal/routecatalog` 定义 Route Descriptor、Public、Authenticated 和 PermissionControlled
- [x] 3.2 实现 Descriptor Method、Path、Handler、Access Level、Permission Code 和 OpenAPI operation 组合校验
- [x] 3.3 实现规范化 `method + path` 重复检测，并在冲突时阻止 HTTP 服务启动
- [x] 3.4 实现稳定的只读 Route Catalog Snapshot，供 API Metadata、RBAC 路由权限同步、API Doc 和启动 Seed 消费
- [x] 3.5 创建 `internal/app/http.go`，按 Access Level 统一挂载中间件和注册 Gin 路由
- [x] 3.6 调整权限中间件，固定 Public、Authenticated、PermissionControlled 路由映射，使 PermissionControlled 始终要求登录且 `need_auth = 0` 只能跳过 Permission Code 检查；保留两个 API Metadata 首次同步接口的 `admin` 引导例外，并由 App HTTP 公共边界测试覆盖匿名、登录、动态权限和引导例外
- [x] 3.7 增加非法 Descriptor、规范化重复路由、缺少 OpenAPI 描述/Schema、数据库元数据不能动态创建路由的单元和 App HTTP 集成测试
- [x] 3.8 增加 Route Catalog Descriptor 集合与 Gin 实际业务路由集合一致性测试，严格拒绝额外路由，并覆盖技术路由、路径参数、通配路径和文档开关
- [x] 3.9 增加 Route Catalog 统一来源测试：启动 Seed、`POST /api/admin/apis/sync`、`POST /api/admin/permissions/sync` 和 OpenAPI 均不得通过 `Gin Engine.Routes()` 另行发现业务路由；`POST /api/admin/apis/sync-permissions` 保持基于 API Metadata 的显式用例
- [x] 3.10 验证缺少 OpenAPI operation、请求/响应 Schema 或上传字段描述的 Descriptor 在 Gin 注册前失败，禁止静默生成不完整文档

## 4. Dictionary 模块模板验证

- [x] 4.1 创建 `internal/dictionary`，迁入字典 Model 和所属 AutoMigrate/Seed 定义
- [x] 4.2 将字典请求/响应 DTO 和校验逻辑迁入 Dictionary 模块
- [x] 4.3 将字典 Service 用例迁入模块内 Application 或扁平用例文件，并显式注入 Repository
- [x] 4.4 将字典 GORM 查询迁入模块 Adapter，移除对 `global.DB` 的依赖
- [x] 4.5 将字典 Handler 迁入 HTTP Adapter并输出 Route Descriptor
- [x] 4.6 由 App 装配 Dictionary 并切换唯一正式路由入口
- [x] 4.7 运行字典 CRUD、公开读取和 HTTP 兼容测试后删除字典旧 Handler、Service、DTO、Model 和 Router 入口

## 5. Organization 层级 Capability

- [x] 5.1 在 `internal/organization` 定义组织单位、组织成员和层级查询的模块所有权
- [x] 5.2 迁入组织 Model、DTO、GORM Repository 和现有 AutoMigrate/Seed 定义
- [x] 5.3 实现不暴露 GORM Model 的 Organization Hierarchy 只读 Capability
- [x] 5.4 迁入组织 CRUD、成员管理和组织树 Application 用例
- [x] 5.5 迁入组织 HTTP Adapter并输出 Route Descriptor
- [x] 5.6 由 App 装配 Organization，并验证组织 CRUD、成员关系和组织树行为兼容

## 6. Authorization 核心迁移

- [x] 6.1 在 `internal/authorization` 迁入角色、权限、用户角色、角色权限和数据范围 Model
- [x] 6.2 将角色、权限、关联关系和数据范围 GORM 操作迁入 Authorization Adapter
- [x] 6.3 在 Authorization Domain 中实现角色保护和 Permission Code 值对象规则
- [x] 6.4 定义只携带用户 ID 的 Principal，以及不暴露持久化实现的 Access Snapshot
- [x] 6.5 定义 UserScope 和 OrganizationScope，固化 `All`、空集合和 `self` 语义
- [x] 6.6 在 `authorization/application/contracts.go` 定义调用方所需的 Organization Hierarchy 等最小 Contract
- [x] 6.7 迁入角色、权限、用户角色、角色权限、数据范围和 Access Snapshot Application 用例
- [x] 6.8 让 Authorization 通过 Organization Hierarchy Contract 解析 Scope，不直接访问 Organization GORM Adapter
- [x] 6.9 在用户和组织 Repository 中分别应用 UserScope 和 OrganizationScope，禁止传递 SQL 或 GORM Scope
- [x] 6.10 通过正式 Contract 接入授权版本读取和 `EnsureAndIncrement`，不得重新实现 TokenVersion 存储迁移
- [x] 6.11 迁入 Authorization HTTP DTO、Handler 和 Route Descriptor
- [x] 6.12 由 App 注入 Organization Capability、事务能力和 Repository，并消除 Authorization 对其他模块 Adapter 的导入
- [x] 6.13 运行角色、权限、数据范围、空 Scope、`self`、Token 失效和超级管理员保护测试
- [x] 6.14 补齐 RBAC 兼容迁移：权限分组 CRUD、超级管理员 Seed 保护、角色/权限关系失效以及 `POST /api/admin/permissions/sync` 的 Route Catalog 消费和公开路由跳过规则

## 7. Navigation 与 API Metadata 迁移

- [x] 7.1 创建 `internal/navigation`，迁入菜单、`role_menus` 和 `menu_apis` Model 及 Repository
- [x] 7.2 迁入菜单 CRUD、角色菜单分配、用户可见菜单树、前端路由同步和菜单 API 查询用例
- [x] 7.3 创建 `internal/apimetadata`，迁入 `apis` Model、DTO、Repository、状态和策略查询
- [x] 7.4 迁入 API 元数据 CRUD、分组选项、方法选项和权限同步用例
- [x] 7.5 拆分不依赖 Navigation 的 API Metadata Storage/Policy Core，并由 App 先行构造
- [x] 7.6 在 Navigation Application 中实现菜单/API/Permission Code 跨模块绑定的最外层事务用例
- [x] 7.7 在事务内锁定 API、菜单、Permission 和关联行，并计算变更前后受影响用户
- [x] 7.8 实现 Permission Code 已存在时合并并去重 `role_permissions`，仅在无引用时清理旧 Permission
- [x] 7.9 将 API、菜单、`menu_apis`、权限、角色授权和相关用户授权版本写入同一数据库事务
- [x] 7.10 将缓存清理移动到事务提交后，并增加失败指标和重试入口
- [x] 7.11 让 API Metadata 的 Permission Code 管理操作委托 Navigation 跨模块绑定用例
- [x] 7.12 迁入 Navigation 与 API Metadata 的 HTTP Handler、DTO 和 Route Descriptor
- [x] 7.13 由 App 完成 Authorization、Navigation 和 API Metadata 的分阶段构造与 Contract 注入
- [x] 7.14 增加菜单/API绑定、API Permission Code 修改、按钮生成每个写入点失败时的完整回滚测试
- [x] 7.15 增加并发 API 更新、菜单绑定、Permission 合并和 MySQL 死锁有界重试测试
- [x] 7.16 增加菜单完整兼容矩阵：菜单路径/父子约束、角色菜单分配、用户权限过滤和父级保留、前端路由同步、删除清理以及每个联动写入点的授权版本失效

## 8. API 路由同步与 OpenAPI 生成

- [x] 8.1 将启动 Seed 及 `POST /api/admin/apis/sync` 的路由来源从 Gin Engine 扫描切换为静态 Route Source；迁移模块仍由 Route Catalog Snapshot 装配，Legacy 适配器使用经过兼容测试的静态路由清单
- [x] 8.2 保持已存在 API 元数据、软删除恢复、默认 Permission Code、Public 路由兼容规则以及 `POST /api/refresh` 错误认证标记的纠正行为
- [x] 8.3 保持 API Metadata 权限同步和 RBAC 路由权限同步均为显式用例；前者读取 API Metadata，后者读取 Route Catalog，不在普通路由注册或 Catalog 收集中隐式创建 Permission
- [x] 8.4 迁入 `internal/apidoc`，使 OpenAPI 生成消费 Route Catalog 和必要的 API Metadata Snapshot
- [x] 8.5 保持 `/docs`、`/docs/openapi.json`、DTO Schema 和上传接口文档行为兼容，并验证每个已注册业务路由都出现在 OpenAPI 文档中
- [x] 8.6 验证 API/权限同步、启动 Seed 不扫描 Gin Engine，且 Route Catalog、API Metadata、OpenAPI 和 RBAC 权限同步的 Method/Path 集合一致
- [x] 8.7 验证管理员修改 API Metadata Method/Path 后真实路由按 API 未配置拒绝访问，且同步可重新创建正确记录
- [x] 8.8 验证 `need_audit` 保持当前日志主规格语义：`/api/*` 请求继续异步记录，既有元数据的该字段不被 Catalog 同步覆盖，且该字段不改变静态访问等级
- [x] 8.9 增加首次同步和访问等级测试：匿名请求不能绕过 `PermissionControlled`，已登录 `admin` 可初始化两个 API Metadata 同步接口，`need_auth = 0` 不会把受保护路由降级为匿名

## 9. Identity 与用户上下文迁移

- [x] 9.1 创建 `internal/identity`，迁入用户 Model、认证 DTO 和用户资料 DTO
- [x] 9.2 迁入验证码、注册、登录、JWT、Refresh Token、Redis 黑名单和登录锁定 Application 用例
- [x] 9.3 迁入用户资料、密码、头像、管理员用户操作和用户状态 Application 用例
- [x] 9.4 在 `identity/application/contracts.go` 定义 Authorization Snapshot、Navigation Menu 和必要组织能力 Contract
- [x] 9.5 迁入 `/api/user/context` 编排，使 Identity 组合角色、权限、菜单和用户资料
- [x] 9.6 迁入 Identity 的 GORM、Redis、对象存储和 HTTP Adapter并输出 Route Descriptor
- [x] 9.7 由 App 注入 Authorization、Navigation、Organization、Redis 和对象存储能力
- [x] 9.8 删除旧 JWT、用户上下文和数据范围旁路入口
- [x] 9.9 运行认证与用户上下文兼容测试：登录、注册、验证码消费、密码复杂度、登录锁定、JWT 用途/存量 Token、Refresh Token、登出黑名单、禁用、软删除、恢复、Kick、头像和用户上下文
- [x] 9.10 验证 Identity 只通过 Authorization Contract 获取授权快照和授权版本；Seed 不创建版本，运行时不恢复 `users.token_version` 或迁移状态依赖

## 10. Files、Upload Security 与 Audit 迁移

- [x] 10.1 创建 `internal/files`，迁入文件 Model、DTO、Application 用例、GORM Adapter 和 HTTP Adapter
- [x] 10.2 让 Files 只依赖调用方声明的对象存储和 Upload Security Contract，删除全局 MinIO 旁路
- [x] 10.3 迁入 `internal/uploadsecurity` 并保持管理员文件与头像的现有验证、重编码和 SHA-256 规则
- [x] 10.4 创建 `internal/audit`，迁入审计 Model、Repository、Application 用例和 HTTP Adapter
- [x] 10.5 保持 Audit 当前提交后异步记录语义和稳定错误处理
- [x] 10.6 由 App 装配 Files、Upload Security 和 Audit，并收集各模块 Route Descriptor
- [x] 10.7 运行文件上传、列表、详情、改名、下载、预览拒绝、重新验证、浏览、删除、轮转、配置校验、头像安全、对象清理和审计日志回归测试
- [x] 10.8 验证 Audit 保持请求字段、敏感字段脱敏、multipart 不读文件内容、上传安全 metadata、分类查询、异步提交和冷热归档兼容

## 11. 旧入口删除与架构收口

- [x] 11.1 将剩余业务 Handler、Service、DTO、Model 和路由逐模块迁入对应 `internal` 模块
- [x] 11.2 为每个完成迁移的模块切断 legacy Adapter，并确认只有一条正式调用路径
- [x] 11.3 删除无调用点的旧 middleware、`initialize` 旁路和 `utils` 业务 Helper
- [x] 11.4 删除 `legacyglobal` 和本 Change 引入的全部兼容 Adapter
- [x] 11.5 删除全局业务 `handler`、`service`、`dto`、`model` 和 `router` 目录
- [x] 11.6 强化架构测试，禁止新业务代码重新引入旧目录、全局状态或跨模块 Adapter 依赖
- [x] 11.7 验证 App 是唯一组合根、唯一 Gin 注册点和唯一迁移/Seed 编排入口

## 12. 文档与最终验证

- [x] 12.1 更新 README 和模块文档，提供按业务模块阅读代码的导航
- [x] 12.2 同步 ADR、`CONTEXT.md`、背景方案和受影响 OpenSpec 主规格所需的术语与长期决策；明确授权版本迁移已归档完成，当前 Change 不再依赖旧字段或观察期
- [x] 12.3 运行全部架构边界、Domain、Application Fake Contract 和 SQLite Repository 快速测试
- [x] 12.4 运行 Route Catalog、路由访问等级、API Metadata 动态策略和 Gin 路由一致性测试
- [x] 12.5 运行菜单/API/Permission 强事务、角色授权保留、Token 失效和失败回滚测试
- [x] 12.6 使用真实 MySQL 验证行锁、死锁重试、并发事务、AutoMigrate 和数据库结构兼容性，并使用真实 Redis 与必要 MinIO 运行关键认证授权端到端测试
- [x] 12.7 运行 `go test ./... -count=1` 并确认无不稳定测试、数据竞争或旧包引用
- [x] 12.8 对照 proposal、design 和全部 delta specs 完成验收，确认除批准的原子性增强外没有 HTTP 行为变化
- [x] 12.9 对照全部主规格和 Delta Spec 完成覆盖矩阵，重点确认 API/权限同步来源、首次同步例外、`/api/refresh` 兼容修正、事务重试边界、审计语义和文件安全边界均有可执行验收


## 13. Identity 用户目录与头像边界修复

- [x] 13.1 按最新设计在现有 `internal/identity/domain` 定义最小 `DirectoryUser` 业务值类型和共享头像可信规则；不得新增顶层 `internal/avatar` 或 `internal/identity/avatar` 业务模块。
- [x] 13.2 先补充 Identity UserDirectory 应用接缝的失败测试，再在现有 `internal/identity/application` 实现批量用户查询、脱敏和可信头像映射；Repository 只增加窄接口，不返回 GORM Model、旧 `avatar` 字段、MinIO URL 或 object key。
- [x] 13.3 修改 Authorization 和 Organization 的调用方 Contract，使角色成员和组织成员消费 Identity-owned `DirectoryUser`，并由 App 注入同一个 Identity Directory 实现。
- [x] 13.4 将 Navigation 所需的全部 Identity 用户 ID 查询接入 UserDirectory，按 Identity Core → Authorization/Organization/Navigation → 完整 Identity 用例的顺序调整 App 装配，移除 `internal/app` 对 `identity.User` 的直接 GORM 查询和头像规则旁路。
- [x] 13.5 补充两个管理员 HTTP 接口的回归测试：可信头像返回 `/api/avatars/<user-id>`，历史 URL、旧字段和不可信元数据统一返回 `/api/avatars/default`，并覆盖空成员列表和批量查询顺序。
- [x] 13.6 强化架构门禁和 Contract 绑定测试，禁止 App 重新读取 Identity 持久化模型或复制头像信任规则；验证模块实现只通过正式 UserDirectory 接缝连接。
- [x] 13.7 运行 Identity Directory、角色成员、组织成员和架构边界的针对性测试，再运行 `go test ./... -count=1`，确认 HTTP 契约、旧入口引用和数据竞争无回归。
- [x] 13.8 删除 `identityComposition.directory` 冗余字段和构造赋值；`DirectoryService` 只由 `identityCore` 持有，并由 App 将同一实例注入 Authorization、Organization 和 Navigation。
- [x] 13.9 按最新设计保持 `UserDirectory` 为跨模块批量安全读取接缝；Identity 自有注册、登录、用户资料、管理员用户列表和头像操作继续通过 Identity Application 与 DTO 输出，不得为展示映射产生重复数据库查询。
- [x] 13.10 补充 Identity DTO 头像映射回归测试，覆盖可信对象、历史 `avatar` URL、错误用户归属、未验证状态、MIME 与后缀不一致，确认只输出用户头像路由或默认头像路由。
- [x] 13.11 提取 App Identity 依赖分类门禁并补充允许与拒绝用例：允许 Identity 自有组合文件装配 Adapter，拒绝跨模块组合文件导入 Identity 持久化模型或基础设施 Adapter，并确认路径前缀旁路不能绕过检查。
- [x] 13.12 增加 Identity 公开用户投影的最终验收门禁：使用同一组可信头像、历史 URL、错误用户归属、未验证状态和 MIME/后缀不一致用户，分别经过 Identity DTO 与 UserDirectory 两条正式路径，确认公开用户字段和头像路由保持一致；同时验证 Identity 自有 HTTP 路径不调用 UserDirectory 产生重复数据库查询，并将该门禁纳入 `go test ./... -count=1`。

## 14. 归档前验证修正

- [x] 14.1 统一 Design 中的通用模块装配顺序与 Identity Core 专项装配阶段，明确 Identity Directory 先于 Authorization、Organization 和 Navigation，完整 Identity 用例在其消费者之后装配。
- [x] 14.2 增加 API 硬删除的直接事务回归测试，覆盖 `menu_apis` 清理、API 记录硬删除、删除回调失败回滚和授权版本提升失败回滚。
- [x] 14.3 更新最终验收矩阵中的架构测试路径，并重新运行 OpenSpec 严格校验、相关测试和全仓测试。