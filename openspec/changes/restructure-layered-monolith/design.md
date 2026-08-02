## Context

项目当前以顶层 `handler`、`service`、`dto`、`model`、`router`、`initialize`、`global` 和 `utils` 组织代码。该结构能够表达技术职责，但同一业务能力被拆散在多个目录；服务之间经常通过包级函数、GORM Model 或全局 DB/Redis/MinIO 实例耦合，路由注册、API 元数据同步、权限判断和 OpenAPI 生成也共同依赖集中式 Gin 路由状态。

本 Change 将项目调整为“业务模块优先、模块内部再分层”的模块化分层单体。它是跨全仓库的渐进式结构迁移，不修改外部 HTTP 契约，也不改变 Redis key、MinIO bucket 或文件安全策略。菜单、API 和 Permission Code 联动当前存在部分操作不在同一事务的问题，本次将其作为明确的行为增强一并修复。

授权版本存储迁移不属于本 Change。`migrate-access-version-storage` 必须先达到切换里程碑，使授权版本已有稳定的新读取来源和失效能力；本 Change 只通过 Authorization Contract 使用该能力。

主要约束：

- 一级模块已经确认，不再继续拆出 `roles`、`permissions`、`menus` 等一级目录。
- 迁移必须渐进进行，不能通过一次提交移动整个仓库。
- HTTP Method、Path、请求/响应结构、状态码和稳定错误码保持兼容。
- MySQL、Redis、MinIO、Gin、GORM、Zap、AutoMigrate 和现有 Seed 机制继续使用。
- OpenSpec 是行为事实来源；长期架构决策在实施时同步 ADR。

## Goals / Non-Goals

**Goals:**

- 让阅读和修改单一业务能力时主要停留在一个模块目录。
- 为业务数据、用例、路由和跨模块能力建立唯一所有者。
- 让 Domain 和 Application 脱离 Gin、GORM Model、Redis、MinIO 和全局变量。
- 建立唯一系统组合根、唯一 Gin 注册点和可验证的 Route Catalog。
- 通过调用方拥有的最小 Contract 解除跨模块 Adapter 依赖。
- 使菜单、API、Permission Code、角色授权合并和相关用户会话失效原子完成。
- 使用兼容 Adapter 和回归测试逐模块迁移，最终删除旧技术目录。
- 保持当前用户和前端可观察的 API 行为，除明确批准的事务原子性增强外。

**Non-Goals:**

- 不拆分微服务。
- 不引入 Casbin、事件总线、Service Locator 或新的依赖注入框架。
- 不切换数据库、ORM 或数据库迁移工具。
- 不在本 Change 中迁移 `users.token_version`。
- 不修改 JWT 声明、Redis 黑名单 key、MinIO bucket 和对象命名。
- 不增加新的一级业务模块。
- 不强制简单模块创建空 Domain、Application 或 Repository 目录。
- 不借重构修改无关业务规则、HTTP 路径或响应模型。
- 不在本 Change 中增强审计队列可靠性、flush 或 graceful shutdown。

## Decisions

### 1. 采用固定一级模块的模块化分层单体

目标一级目录为：

```text
internal/
├── app/
├── platform/
├── identity/
├── authorization/
├── navigation/
├── apimetadata/
├── organization/
├── dictionary/
├── files/
├── audit/
├── routecatalog/
├── apidoc/
└── uploadsecurity/
```

其中：

- `app` 是系统组合根、启动编排、迁移/Seed 编排和唯一 Gin 注册点。
- `platform` 创建配置、数据库、缓存、对象存储和日志等基础设施。
- 其余目录按业务能力聚合代码。

选择该方案是因为当前主要问题是业务局部性不足，而不是缺少技术层。相比继续维护全局技术目录，模块优先能让代码所有权和依赖边界直接反映在目录中；相比微服务，它不引入网络、分布式事务和独立部署成本。

备选方案：

- 继续按全局技术层组织：移动成本较低，但不能解决跨目录阅读和所有权模糊。
- 每个实体一个模块：目录数量过多，角色、权限、菜单和 API 联动会更加分散。
- 微服务：当前规模和部署需求不足以抵消分布式复杂度。

### 2. 根据复杂度决定模块内部是否物理分层

复杂模块使用：

```text
domain/
application/
adapters/http/
adapters/gorm/
adapters/redis/
module.go
```

简单模块允许在模块根目录放置用例、DTO、Adapter 和 `routes.go`。升级条件是出现稳定业务规则、多个入口/基础设施 Adapter、明显跨模块 Contract 或测试隔离需要。

依赖方向为：

```text
HTTP/Infrastructure Adapter → Application → Domain
App → 模块 factory 与 Adapter
```

禁止方向为：

- Domain 导入 Gin、GORM、Redis、MinIO、`global` 或 `initialize`。
- Application 导入 Gin、GORM Model、`global`、`initialize` 或其他模块 Adapter。
- 业务模块直接修改 Gin Engine。

备选方案是所有模块强制相同三层模板。该方案形式整齐，但会产生大量空目录和单函数包装，降低而不是提高可读性。

### 3. 明确 Authorization、Navigation 和 API Metadata 所有权

Authorization 拥有：

- 角色、权限、`user_roles`、`role_permissions`。
- 数据范围及其解析。
- Access Snapshot 和授权版本能力。

Navigation 拥有：

- 菜单、按钮、`role_menus`、`menu_apis`。
- 角色菜单分配和用户可见菜单树。
- 菜单/API/Permission Code 跨模块绑定用例。

API Metadata 拥有：

- `apis` 记录。
- `status`、`need_auth`、`need_audit` 和 `permission_code` 运行时策略。
- API 元数据管理和 Route Catalog 同步。

Identity 拥有 JWT、Refresh Token、Redis 黑名单、用户状态和 `/api/user/context` 编排。Organization 拥有组织树和成员关系，并向 Authorization 提供最小层级查询能力。

其他模块所有权保持明确：

- Dictionary 拥有字典类型和字典条目。
- Files 拥有管理员文件、文件轮转、访问签名、完整性和删除补偿；用户头像仍由 Identity 拥有。
- Audit 拥有审计采集、分类、脱敏、写入、查询和归档，本轮保持现有异步语义。
- API Doc 消费 Route Catalog 和 API Metadata Snapshot，不拥有真实路由。
- Upload Security 提供文件内容验证，继续区分管理员文件与头像 Policy。

该有限拆分在保持一级目录数量可控的同时，将变化原因不同的数据分开。将角色、权限、菜单和 API 全留在一个 Authorization 模块会继续形成巨型模块；进一步拆成更多一级模块则会使单一授权流程跨越过多目录。

### 4. App 是唯一组合根，Platform 只提供基础设施

`internal/app` 按以下顺序装配系统：

```text
加载配置
→ 创建 Logger、DB、Redis、MinIO
→ 创建共享事务 Adapter
→ 创建各模块 Repository
→ 创建 Application Service
→ 注入跨模块 Capability
→ 收集 Route Descriptor
→ Route Catalog 校验
→ 注册中间件和 Gin 路由
→ 启动后台任务
```

模块 `module.go` 只能构造模块内部对象，不读取全局配置，也不自行创建基础设施客户端。`platform` 不包含角色、菜单、文件等业务规则。

推荐的模块构造顺序为：

```text
Platform
→ Organization Hierarchy Core
→ Authorization Core
→ API Metadata Storage/Policy Core
→ Navigation Core
→ API Metadata Management
→ Identity Core
→ Files、Audit、Dictionary、API Doc
→ 各模块 HTTP Adapter
→ Route Catalog
→ App 注册 Gin 路由
```

Navigation 需要 API Metadata 读写能力，而 API Metadata 的 Permission Code 管理又需要 Navigation 同步关联菜单，因此先构造不依赖 Navigation 的 Storage/Policy Core，再构造 Navigation，最后构造 API Metadata Management。类似循环必须通过拆分基础 Capability 和管理用例解决，不得创建可变的半初始化模块后回填字段。

该决策让依赖图在一个位置可见，并允许测试使用 Fake Contract 或测试 Adapter。备选的全局容器或 Service Locator 会隐藏依赖并继续制造运行时耦合，因此不采用。

### 5. 模块定义 Route Descriptor，App 统一注册路由

复杂模块在 `adapters/http/routes.go` 输出 Descriptor；简单模块可在根级 `routes.go` 输出。Descriptor 至少包含：

- HTTP Method。
- Path。
- 静态 Access Level。
- 默认 Permission Code。
- Handler。
- API 名称、分组和默认审计分类。
- OpenAPI 请求和响应描述。

Route Catalog 负责：

- 收集全部 Descriptor。
- 规范化并校验 `method + path` 唯一性。
- 校验 Access Level 和 Permission Code 组合。
- 向 API Metadata、RBAC 路由权限同步、API Doc 和启动 Seed 提供只读快照。
- 在 Gin 注册前校验 OpenAPI operation、请求/响应 Schema 和上传字段描述的完整性；缺少必需描述时启动校验失败，不允许 API Doc 静默遗漏路由或 Schema。

`internal/app/http.go` 是唯一调用 Gin 注册方法的位置。Route Catalog 不从数据库动态创建 Handler，也不直接修改 Gin Engine。

访问等级固定为：

- `Public`：允许匿名，不进行权限码检查。
- `Authenticated`：必须登录，不进行权限码检查。
- `PermissionControlled`：始终必须登录，再由 API Metadata 决定启用状态及是否检查 Permission Code。

`need_auth = 0` 只能跳过 Permission Code 检查，不能把 `PermissionControlled` 路由变成匿名。数据库元数据也不能新增路由或替换代码 Handler。
当前业务路由的静态访问等级映射固定为：`/api/captcha`、`/api/register`、`/api/login`、`/api/refresh`、公开字典和公开头像为 `Public`；`/api/user/**` 为 `Authenticated`；`/api/admin/**` 为 `PermissionControlled`。`/ping`、`/docs` 和 `/docs/openapi.json` 是 App 拥有的技术路由，不通过 API Metadata 动态创建。
`POST /api/admin/apis/sync` 和 `POST /api/admin/apis/sync-permissions` 保留现有首次初始化例外：请求必须先通过有效登录，`admin` 角色在 API Metadata 尚未完整同步时可以进入；该例外不得扩展为匿名访问，也不得自动适用于 `POST /api/admin/permissions/sync`。

字段事实来源固定为：

| 字段或能力 | 事实来源 |
| --- | --- |
| Handler、真实 Method/Path、静态 Access Level | Route Catalog |
| API 名称、分组、默认 Permission Code、默认审计分类 | Route Catalog，仅用于同步新记录默认值 |
| API Metadata 的 Method/Path、Name、Group、Remark、Sort | API Metadata |
| `status`、`need_auth`、`need_audit`、`permission_code` | API Metadata |

管理员仍可按现有行为编辑 API Metadata Method/Path。若编辑后不再匹配真实 Route Catalog 路由，真实受保护路由按“API 未配置”拒绝访问；后续同步可以重新创建真实 Method/Path 对应记录。本轮不扩展 `need_audit` 的运行时语义。
按 `logging` 主规格，`/api/*` 请求继续由 Audit 异步记录；`need_audit` 只保留既有运行时语义，不参与静态访问等级、路由注册或匿名降级判断。本 Change 不引入按 `need_audit` 抑制审计的新增行为。

相比继续扫描已注册 Gin 路由，Descriptor 先于注册存在，因而可以在启动前检查重复、测试路由完整性，并同时服务 API Metadata 与 OpenAPI。相比让每个模块自行注册 Gin，App 统一注册可以稳定中间件顺序和访问等级。

### 6. 跨模块接口由调用方定义并由 App 注入

复杂模块的跨模块接口集中放在：

```text
internal/<caller>/application/contracts.go
```

每个调用方模块最多一个该文件。简单模块确有跨模块依赖时使用根级 `contracts.go`。Contract 只暴露调用方所需的最小业务值类型，不暴露 GORM Model、Gin Context 或基础设施客户端。

跨模块公开值类型保持稳定和最小。请求身份使用只包含用户 ID 的 `Principal`；Authorization 通过 `AccessSnapshot` 返回角色、权限和授权版本。JWT 中已有的角色或权限 Claim 不能绕过运行时授权版本和权限校验。

Go 的隐式接口实现允许提供方不导入调用方。App 将提供方实现注入调用方，Application 只依赖接口。

备选方案：

- 提供方声明公共大接口：容易随提供方实现膨胀，调用方被迫依赖无关方法。
- 共享 `common/contracts`：会形成新的全局杂物目录。
- 直接导入其他模块 Repository/Adapter：破坏所有权并使事务和测试边界不可控。

### 7. 使用资源类型明确的数据范围

Authorization 负责根据角色和数据范围配置解析 Scope，拥有数据的模块负责把 Scope 应用到自己的 Repository 查询：

```text
业务模块取得 Principal
→ Authorization 返回资源类型明确的 Scope
→ 业务模块 Repository 应用过滤
```

首批公开值类型为：

```text
UserScope
├── All
└── UserIDs

OrganizationScope
├── All
└── OrganizationIDs
```

约束：

- `All = true` 时 ID 集合必须为空。
- `All = false` 且 ID 集合为空表示无可见数据，不能解释为全部数据。
- `self` 对 UserScope 只包含本人，对 OrganizationScope 返回空集合。
- Scope 不包含 SQL、GORM Scope 或数据库字段名。
- 新资源需要数据范围时新增对应资源 Scope，不复用含义模糊的通用 ID 集合。

Authorization 通过自己声明的 Organization Hierarchy Contract 获取用户组织和后代组织，不直接查询 Organization GORM Adapter。

相比返回 GORM Scope 或 SQL，该方案保持 Authorization 与资源存储解耦；相比一个通用 ID Scope，资源类型明确可以防止把用户 ID 错当成组织 ID。

### 8. 使用共享 TransactionRunner 完成跨模块强事务

Application 依赖最小事务接口，GORM Adapter 将当前事务连接保存到私有 Context 值；各模块 GORM Adapter 从 Context 取得同一连接。Application 只能传递 Context，不能操作 `*gorm.DB`。

菜单/API/Permission Code 变更的最外层用例由 Navigation 编排，事务内顺序为：

1. 锁定目标 API、菜单、Permission 和现有关联。
2. 计算变更前受影响用户。
3. 校验新 Permission Code。
4. 创建、重命名或合并 Permission，并保留、去重 `role_permissions`。
5. 更新 API 和菜单 Permission Code。
6. 替换 `menu_apis`。
7. 计算变更后受影响用户。
8. 通过 Authorization 提升新旧受影响用户的授权版本。
9. 提交后清理权限缓存等非数据库副作用。

约束：

- 一个业务用例只有一个最外层事务。
- 内部 Capability 必须加入已有事务，不得自行提交新事务或回退到全局 DB。
- 新 Permission Code 已存在时合并角色授权并去重。
- 旧 Permission 仅在不再被菜单、API 或角色引用时删除。
- 任一数据库写入或授权版本提升失败时全部回滚。
- 对需要串行修改的行使用 MySQL 行锁和唯一约束。
- TransactionRunner 只对 MySQL 死锁进行有界重试。
TransactionRunner 只在最外层事务回调尚未提交且错误明确属于 MySQL 死锁或锁等待时进行有界重试；每次重试必须使用新的数据库事务。加入已有事务的 Capability 不得自行提交、重试或开启第二个事务；提交后的缓存清理和其他外部副作用不得由数据库重试重复执行。上下文取消、非 MySQL 错误和超过上限时立即返回。

缓存清理不能加入数据库事务，失败时记录错误、指标并重试；数据库授权版本仍是安全事实来源。

备选的 Saga 或最终一致性不适用于同一 MySQL 中的安全敏感写入。让各模块各自开启事务会保留部分成功窗口，因此不采用。

### 9. Route Catalog 是 API Metadata、RBAC 路由权限同步和 API Doc 的共同来源

同步方向为：

```text
Route Catalog → API Metadata
Route Catalog → RBAC 路由权限同步
Route Catalog + API Metadata Snapshot → API Doc
```

Catalog 提供代码声明的 Method、Path、Access Level、默认 Permission Code 和文档描述。API Metadata 继续拥有管理员可配置的状态及策略字段。同步时新记录使用 Catalog 默认值，已有记录按既有兼容规则保留管理员配置，软删除记录按当前规则恢复。

API Doc 不再扫描 Gin Engine，而是将 Catalog 与必要的 API Metadata Snapshot 转换为 OpenAPI。`/docs` 和 `/docs/openapi.json` 的开关和 HTTP 行为保持不变。
启动 Seed、`POST /api/admin/apis/sync` 和 `POST /api/admin/permissions/sync` 必须消费同一份只读 Snapshot，不得通过 `Gin Engine.Routes()` 另行发现路由。`POST /api/admin/apis/sync-permissions` 仍是基于 API Metadata 的显式权限同步用例。

Route Catalog 不隐式创建 Permission；路由权限同步仍由明确的 API Metadata 或 RBAC 用例负责。

### 10. 使用有删除节点的兼容 Adapter 渐进迁移

迁移期间允许：

```text
新 App → legacy Adapter → 尚未迁移的旧实现
```

兼容 Adapter 必须：

- 名称包含 `legacy`。
- 只在 App 装配边缘使用。
- 不被新 Domain 或 Application 调用。
- 不新增旧入口调用点。
- 在任务中绑定唯一删除阶段。

每迁移一个模块，即切换该模块的唯一正式调用路径并删除对应旧入口。不得长期保留新旧两套业务路径，也不得通过双写维持两个业务事实来源。

备选的一次性全仓库移动会使行为回归难以定位，回滚粒度过大；长期双轨则会让测试和所有权失真。

### 11. 以风险驱动的分层测试保护结构迁移

测试层次包括：

- 架构边界测试：检查禁止依赖、App 唯一 Gin 注册和旧目录新增引用。
- Domain 纯 Go 测试。
- Application Fake Contract 测试。
- SQLite 数据库测试：Repository CRUD、Model 映射、通用唯一约束、普通事务回滚和失败注入的快速反馈。
- MySQL 数据库集成测试：真实 AutoMigrate、事务隔离、行锁、死锁、并发冲突、方言差异和失败回滚。
- HTTP 兼容测试：Method、Path、请求、响应、状态码和稳定错误码。
- 认证授权回归：Public、Authenticated、PermissionControlled、API 状态和 Permission Code。
- 少量真实 MySQL、Redis、MinIO 端到端测试。
兼容矩阵还必须覆盖主规格中未改变但容易在迁移中丢失的边界：验证码消费和登录锁定、JWT 用途与存量 Token 兼容、Refresh Token 稳定错误、RBAC 权限分组和路由权限同步、组织树祖先保留、菜单前端路由同步、文件重验证/下载/预览拒绝/轮转、上传配置校验、审计字段脱敏和归档。

SQLite 是默认的快速数据库测试环境。每个测试使用独立临时数据库，或使用唯一命名的 shared-memory DSN 并正确限制连接生命周期，避免 `:memory:` 多连接产生不同数据库以及并行测试互相污染。

SQLite 不能作为唯一验收数据库。以下场景必须使用真实 MySQL：

- `SELECT ... FOR UPDATE` 等行锁行为。
- TransactionRunner 的死锁识别和有界重试。
- 并发菜单/API/Permission Code 修改。
- MySQL 隔离级别、唯一约束冲突时序和方言 SQL。
- AutoMigrate、索引、外键及部署相关 DDL。

能够在两种数据库运行的 Repository 行为测试应复用同一组测试用例，先在 SQLite 快速执行，再在 MySQL 验证方言和并发差异。

## Risks / Trade-offs

- [模块迁移期间出现循环依赖] → 先拆分基础 Capability 和管理用例构造，使用调用方 Contract，由 App 完成注入，禁止半初始化模块回填字段。
- [为了分层增加大量样板代码] → 简单模块保持扁平，只在复杂度达到阈值时物理分层。
- [兼容 Adapter 长期存在] → 每个 Adapter 在 tasks 中标记唯一删除阶段，并通过架构测试禁止新增旧入口。
- [路由遗漏或重复] → 对 Descriptor 唯一性、Catalog 集合与 Gin 实际路由集合建立自动测试，启动校验失败时拒绝启动。
- [API Metadata 动态配置降低认证等级] → 静态 Access Level 决定最低身份要求，`need_auth = 0` 不得使受保护路由匿名。
- [空数据范围被错误解释为全部数据] → 使用资源类型明确的 Scope，并覆盖 `All`、空集合和 `self` 语义测试。
- [跨模块事务看似统一但内部使用全局 DB] → Context 事务 Adapter 集成测试在每个写入点注入失败并验证完全回滚。
- [Permission Code 变更丢失角色授权] → 在事务内锁定、合并并去重 `role_permissions`，只在无引用时清理旧 Permission。
- [提交后缓存清理失败导致短暂旧缓存] → 授权版本在事务内提升，运行时以数据库版本为准；缓存清理失败记录指标并重试。
- [全仓库重构周期较长] → 以模块为迁移单元，保持每个阶段可构建、可测试、可回滚。
- [结构重构夹带行为变化] → 除强事务外使用 HTTP 兼容测试锁定行为，其他行为变化必须单独创建 OpenSpec Change。

## Migration Plan

### 前置条件

1. `migrate-access-version-storage` 达到切换里程碑。
2. 当前路由、响应、权限码、Seed、认证和文件行为有回归基线。
3. 菜单/API/Permission 强事务的失败场景已有可执行测试设计。

### 实施阶段

1. **建立 App、Platform 和最小 Route Catalog**
   - 建立唯一组合根和基础设施创建入口。
   - 收集 Descriptor、校验重复并由 App 注册 Gin 路由。
   - 对尚未迁移代码提供最小 `legacyglobal` Adapter。
   - 禁止新增 `global` 使用。

2. **以 Dictionary 验证模块模板**
   - 将 Handler、DTO、用例、Model、路由和测试迁入同一模块。
   - 切换为 Descriptor 路由并删除字典旧入口。
   - 验证简单模块无需强制三层目录。

3. **迁移 Authorization、Navigation 和 API Metadata**
   - 先提供 Organization Hierarchy 只读 Contract。
   - 迁移授权核心、菜单导航和 API 元数据。
   - 建立 TransactionRunner，完成菜单/API/Permission 强事务。
   - 扩展 Route Catalog 的 API 同步和 OpenAPI 描述。

4. **迁移 Identity 和 Organization**
   - 迁移登录、JWT、用户状态、资料、头像和用户上下文编排。
   - 通过正式 Contract 使用 Authorization、Navigation 和 Organization。
   - 删除旧 JWT 和数据范围入口。

5. **迁移 Files、Audit、API Doc 和 Upload Security**
   - 保持现有对象存储 Contract 和上传安全策略。
   - 保持 Audit 当前异步语义。
   - 让 API Doc 消费 Route Catalog。

6. **删除旧结构**
   - 删除已经迁移的顶层 `handler/service/dto/model/router` 业务代码。
   - 删除 `legacyglobal` 和本 Change 引入的兼容 Adapter。
   - 删除无调用点的旧 middleware、initialize 旁路和 utils helper。
   - 更新 README、模块文档、ADR、OpenSpec 和代码导航。

每个阶段都必须满足：全量构建和测试通过、HTTP 兼容、只有一条正式调用路径、对应兼容 Adapter 有明确保留或删除结论。

### 数据迁移

本 Change 不创建业务表、不执行历史数据回填，也不迁移 `users.token_version`。现有表仅因 Repository 和模块所有权调整而更换访问入口。若实施中发现必须新增表或补数据，应停止对应任务并创建独立 OpenSpec Change。

### 回滚

- 单模块切换失败时，将 App 装配恢复到该模块的 legacy Adapter，并回滚该模块的新路由 Descriptor；其他已迁移模块保持不变。
- 强事务上线失败时整体回滚对应用例实现，不允许保留部分启用的跨模块写入链路。
- 已删除旧入口前必须完成兼容和回归验证；旧入口删除后不通过恢复双轨代码进行长期回滚。
- 回滚不得改变 HTTP 契约、数据库事实来源或 `migrate-access-version-storage` 已完成的读取切换。

## Open Questions

无阻断实施的开放问题。一级模块、Authorization 有限拆分、路由归属、Contract 归属、事务边界和测试策略已在本 Change、`openspec/specs/` 及 ADR 中确认；授权版本迁移和旧字段退出已由 `migrate-access-version-storage` 完成，不再以 `docs/modify/项目分层重构方案.md` 中的旧迁移计划作为前置事实。
