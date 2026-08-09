# 项目分层重构方案

## 文档状态

- 最近更新：2026-08-02
- 状态：阶段 0 至阶段 6 的代码迁移已完成；当前进行阶段 7 的最终门禁与规格验收
- 适用范围：目录结构、模块归属、依赖方向、路由装配、跨模块事务、授权版本 Contract、测试与渐进实施
- 当前行为事实来源：`openspec/specs/`
- 后续实施入口：`openspec/changes/restructure-layered-monolith/`

本文整合 `docs/modify` 中已有的项目对比、优化路线、权限链路和文件上传安全结论，并记录本轮目录重构访谈中已经确认的决策。

本文不是当前系统行为规格。涉及数据库结构、事务原子性或认证行为的变化，必须先进入对应 OpenSpec change，再进行代码实施。
授权版本迁移、镜像观察和旧字段退出的历史方案已由 `migrate-access-version-storage` 完成并归档；当前状态以 `openspec/specs/access-version-storage/spec.md`、ADR 0004 和归档退出证据为准，本文不再把恢复旧字段或观察期作为分层重构前置条件。

---

## 1. 背景

### 1.1 当前项目已经具备的能力

项目已经具备后台管理系统的主要能力：

- 注册、登录、验证码、JWT、Refresh Token和Redis黑名单。
- 用户、角色、权限、菜单和API元数据管理。
- 组织树、组织成员和数据范围。
- 字典、文件、头像和审计日志。
- MySQL、Redis、MinIO、Zap、AutoMigrate和幂等Seed。
- 基于Gin路由和API元数据生成OpenAPI文档。

当前主要问题不是业务模块缺失，而是业务能力按技术职责横向分散，缺少清晰的模块所有者。

### 1.2 当前目录带来的阅读成本

当前代码主要分散在：

```text
handler/
service/
dto/
model/
middleware/
router/
initialize/
global/
utils/
```

阅读一个角色管理功能，通常需要在以下位置往返：

```text
handler/role.go
service/role.go
dto/role.go
model/role.go
service/permission.go
service/token_version.go
router/router.go
```

主要问题：

1. `service`是巨型平铺package，文件名代替了真正的模块边界。
2. Handler、DTO、Model和业务规则分散，修改局部性较差。
3. 多数实现直接依赖`global.DB`、`global.Redis`、`global.Minio`和`global.Logger`。
4. 路由注册、API同步、权限策略、OpenAPI和审计分类缺少稳定边界。
5. 包级函数、全局helper和可注入Service并存。
6. 测试需要替换全局变量，难以隔离和并行执行。

### 1.3 与现有优化路线的关系

现有路线已经确定：

```text
先封堵安全风险
  → 再建立回归测试保护
  → 再补可观测能力
  → 再调整核心结构
  → 再扩展业务模块
```

因此目录重构不能用“移动文件”代替行为验证。登录、JWT、RBAC、菜单/API联动、数据范围、文件生命周期和审计行为必须先有测试保护。

文件上传安全V1已经形成较成熟的seam：

- `uploadsecurity.Validator`
- `objectstorage.Store`
- 文件及头像repository interface
- Service constructor
- 稳定错误码
- Adapter级测试

后续迁移应复用这种设计，不得重新引入全局MinIO旁路。

---

## 2. 总体决策

### 2.1 架构形态

采用：

> 业务模块优先、模块内部再分层的模块化分层单体。

组织顺序为：

```text
先按业务能力聚合
  → 再按复杂度决定是否物理拆分Domain、Application和Adapter
```

不采用：

- 微服务拆分。
- 每张表一个模块。
- 全部模块强制使用完全相同的目录模板。
- 继续扩大全局`handler/service/dto/model`技术目录。

### 2.2 设计目标

- 阅读单一业务能力时，主要停留在一个模块目录。
- 每项业务数据和业务用例只有一个明确所有者。
- 跨模块调用使用最小业务Contract，不读取对方GORM Model。
- Domain和Application不依赖Gin、GORM、Redis或MinIO SDK。
- App是唯一系统级组合根。
- 业务模块定义路由，Route Catalog描述和校验，App统一注册。
- 复杂模块通过较少但较深的接口隐藏实现细节。
- 迁移完成后删除全局`handler/service/dto/model/router`等传统技术目录。

### 2.3 非目标

本方案不包含：

- 微服务化。
- 切换Casbin。
- 引入goose或golang-migrate。
- PostgreSQL或多数据库适配。
- 修改现有HTTP路径和响应结构。
- 修改文件上传安全策略。
- 修改现有`NeedAudit`运行时行为。
- 为形式统一而创建空Domain、空Repository或单函数目录。
- 在一次提交中完成整个仓库迁移。

授权版本数据库演进已由 `migrate-access-version-storage` 独立完成：`user_access_versions` 是唯一长期事实来源，`users.token_version`、迁移状态和镜像能力均已退出。本方案只负责把现有 Authorization 授权版本能力迁入目标模块边界并通过 Contract 注入，不重新执行迁移、回填或删列。

---

## 3. 已确认的一级模块

`internal`下的一级模块已经固定，后续不再通过拆分角色、菜单或权限等方式增加新的一级目录：

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

项目现有入口文件是否移动不作为本次模块边界设计的必要条件，不为追求目录形式强制新增`cmd/admin`。

### 3.1 推荐目标目录树

```text
internal/
├── app/
│   ├── app.go
│   ├── http.go
│   ├── migrate.go
│   ├── seed.go
│   └── jobs.go
│
├── platform/
│   ├── config/
│   ├── database/
│   ├── cache/
│   ├── objectstorage/
│   └── logging/
│
├── identity/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   ├── gorm/
│   │   └── redis/
│   └── module.go
│
├── authorization/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   └── gorm/
│   └── module.go
│
├── navigation/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   └── gorm/
│   └── module.go
│
├── apimetadata/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   ├── gorm/
│   │   └── middleware/
│   └── module.go
│
├── organization/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   └── gorm/
│   └── module.go
│
├── dictionary/
│   ├── module.go
│   ├── domain.go
│   ├── usecases.go
│   ├── handler.go
│   ├── routes.go
│   ├── repository_gorm.go
│   └── dto.go
│
├── files/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   ├── gorm/
│   │   └── objectstorage/
│   └── module.go
│
├── audit/
│   ├── domain/
│   ├── application/
│   │   └── contracts.go
│   ├── adapters/
│   │   ├── http/
│   │   │   └── routes.go
│   │   ├── gorm/
│   │   └── middleware/
│   └── module.go
│
├── routecatalog/
│   ├── catalog.go
│   ├── descriptor.go
│   └── validation.go
│
├── apidoc/
│   ├── generator.go
│   ├── schema.go
│   └── routes.go
│
└── uploadsecurity/
    ├── validator.go
    ├── policy.go
    └── errors.go
```

目录树表达目标边界，不要求一次性创建所有文件。没有实际职责时不得创建占位层。

### 3.2 物理分层判断

| 模块 | 初始结构 | 原因 |
| --- | --- | --- |
| Identity | Domain/Application/Adapter | 凭据、JWT、Redis、用户状态、头像协作复杂 |
| Authorization | Domain/Application/Adapter | RBAC、访问快照、数据范围、授权版本不变量较多 |
| Navigation | Domain/Application/Adapter | 菜单树、角色菜单、菜单/API跨模块事务 |
| API Metadata | Domain/Application/Adapter | API管理、路由同步和运行时策略 |
| Organization | Domain/Application/Adapter | 组织层级、成员关系和双向能力协作 |
| Files | Domain/Application/Adapter | 上传、下载、补偿、轮转和对象存储 |
| Audit | Domain/Application/Adapter | 中间件采集、脱敏、持久化、查询和归档 |
| Dictionary | 扁平 | 主要是字典类型和条目管理 |
| Route Catalog | 扁平 | 静态描述、集合校验和Snapshot |
| API Doc | 扁平 | 输入到文档的转换 |
| Upload Security | 扁平深模块 | 现有Validator已经隐藏复杂实现 |

判断标准：

| 信号 | 保持扁平 | 物理分层 |
| --- | --- | --- |
| 业务规则 | 少量局部规则 | 多组关联不变量或状态机 |
| 用例 | 少量独立CRUD | 多步骤事务、补偿或并发语义 |
| Adapter | HTTP和GORM为主 | 同时包含Redis、对象存储或中间件 |
| 跨模块协作 | 很少 | 需要多个稳定Contract |
| 导航成本 | 单目录易读 | 单目录出现大量实现和测试 |

---

## 4. 分层职责与依赖规则

### 4.1 Domain

Domain负责：

- 业务实体和值对象。
- 状态转换和不变量。
- 权限、数据范围等纯业务规则。
- 稳定业务错误。

Domain不得依赖：

- Gin。
- GORM。
- Redis或MinIO SDK。
- HTTP状态码。
- 配置读取。
- `global`或`initialize`。

### 4.2 Application

Application负责：

- 完整业务用例编排。
- 调用Domain规则。
- 声明原子事务范围。
- 调用Repository和跨模块Contract。
- 返回稳定业务结果和错误。

推荐使用业务语言：

```go
ReplaceRolePermissions(ctx, roleID, permissionIDs)
BindMenuAPIs(ctx, menuID, apiIDs, permissionCode)
ResolveUserScope(ctx, principal)
InvalidateUsers(ctx, userIDs)
```

不推荐暴露底层操作：

```go
DeleteRolePermissionRows(...)
InsertMenuAPIRow(...)
UpdateTokenVersionColumn(...)
```

Application不接收或返回：

- `*gorm.DB`
- `gin.Context`
- Redis SDK类型
- MinIO SDK类型

### 4.3 Adapter

#### HTTP Adapter

负责：

- Path、Query、Header、JSON和multipart解析。
- HTTP DTO与Application输入输出转换。
- 从请求上下文读取Principal。
- 业务错误到HTTP响应的映射。
- 定义本模块的Route Descriptor。

HTTP Adapter不得直接修改Gin Engine。

#### GORM Adapter

负责：

- GORM Model。
- 查询与持久化。
- Model与Domain类型映射。
- 关联表维护。
- 参与Application声明的事务。
- 数据库错误归一化。

#### Redis、对象存储和中间件Adapter

负责隐藏外部技术细节，不得把SDK类型暴露给Application。

### 4.4 依赖方向

```text
HTTP Adapter ───────┐
GORM Adapter ───────┤
Redis Adapter ──────┤
                    ▼
               Application
                    ▼
                  Domain

App
  └── 创建Platform和各模块，并完成跨模块注入
```

允许：

```text
domain       → 标准库、稳定值对象
application  → 自身domain、调用方声明的最小Contract
adapters     → application、domain、外部框架
module.go    → 本模块application和adapters
app          → platform和所有业务模块
```

禁止：

```text
domain       → Gin、GORM、Redis、MinIO、global
application  → Gin、GORM Model、global、initialize
业务模块      → 其他模块的adapters目录
platform     → 具体业务模块
handler      → global.DB
```

---

## 5. 模块所有权

### 5.1 Identity

Identity拥有：

- 注册和登录。
- 验证码。
- JWT签发与解析。
- Access Token、Refresh Token和Redis黑名单。
- 用户存在性和启用状态检查。
- 当前用户资料、密码和头像。
- 管理员维护用户基础资料和状态。
- `/api/user/context`响应编排。

Identity不拥有：

- 用户角色关系。
- 权限码。
- 数据范围。
- 菜单树。
- 授权版本持久化。

`GET /api/user/context`由Identity拥有并保持现有响应格式：

```text
Identity读取用户资料
→ Authorization返回AccessSnapshot
→ Navigation生成可见菜单树
→ Identity组装现有响应
```

认证流程：

```text
Identity解析JWT
→ 校验Redis黑名单
→ 校验用户存在且启用
→ Authorization校验授权版本
→ 创建Principal
```

### 5.2 Authorization

Authorization采用有限拆分，不继续拆成`roles`、`permissions`等一级模块。

Authorization拥有：

- 角色。
- 权限和权限分组。
- 用户角色`user_roles`。
- 角色权限`role_permissions`。
- 角色数据范围配置。
- Access Snapshot。
- 权限码判断。
- 数据范围解析。
- 授权版本`user_access_versions`。
- 受影响用户计算和授权版本提升。

Authorization不拥有：

- 菜单和按钮。
- `role_menus`。
- `menu_apis`。
- `apis`表。
- JWT编码和Redis黑名单。
- 组织树本身。

### 5.3 Navigation

Navigation拥有：

- 菜单和按钮。
- 菜单树。
- 可见菜单树生成。
- 角色菜单`role_menus`。
- 菜单/API关系`menu_apis`。
- 菜单与API绑定用例。
- 从API生成按钮菜单。

即使HTTP路径表现为角色资源：

```http
POST /api/admin/roles/:id/menus
GET  /api/admin/roles/:id/menus
```

路由和用例仍由Navigation拥有。URL形式不决定数据所有权。

Navigation通过Contract调用Authorization完成角色校验、权限创建和受影响用户失效；通过Contract调用API Metadata读取或更新API策略。

### 5.4 API Metadata

API Metadata拥有：

- `apis`表。
- API元数据管理。
- `Status`、`NeedAuth`、`NeedAudit`和`PermissionCode`运行时策略。
- Route Catalog到API记录的同步。
- 运行时API策略中间件。

API Metadata不拥有：

- Gin路由注册。
- 菜单/API关联。
- Permission实体。
- JWT解析。

Route Catalog提供代码静态事实；API Metadata保存管理员可配置的运行时事实。

两者按字段区分所有权：

| 字段或能力 | 事实来源 | 说明 |
| --- | --- | --- |
| Handler绑定 | Route Catalog | 数据库不能替换或新增Handler |
| 实际Gin Method/Path | Route Catalog | 表示代码真实注册的路由 |
| 静态AccessLevel | Route Catalog | 决定Public、Authenticated或PermissionControlled |
| API记录Method/Path | API Metadata | 作为运行时策略匹配键，保留当前可编辑行为 |
| Name、Group、Remark、Sort | API Metadata | 管理员可维护的展示信息 |
| Status、NeedAuth、NeedAudit | API Metadata | 运行时策略，其中NeedAudit本轮不扩展语义 |
| PermissionCode | API Metadata记录，受Authorization不变量约束 | 修改必须进入跨模块权限码变更用例 |
| 新记录默认值 | Route Catalog | 仅在同步创建记录时使用 |

当管理员把API Metadata中的Method或Path改离真实Route Catalog路由时，真实路由按现有安全语义成为“API未配置”并拒绝访问；后续路由同步可以重新创建正确匹配的记录。本文不把Method或Path改为只读字段，以保持当前OpenSpec行为。

`Status`和动态`NeedAuth`只参与PermissionControlled路由的运行时策略。Public和Authenticated的最低身份要求由Route Catalog决定，不能被数据库降低或替换。

### 5.5 Organization

Organization拥有：

- 组织树和父子关系。
- 组织成员。
- 组织编码。
- 组织层级和成员查询能力。
- 在Organization查询中应用已解析Scope。

Authorization解析Scope，Organization不自行解释角色或权限表。

### 5.6 Dictionary

Dictionary拥有字典类型和字典条目。初期保持扁平，不为简单CRUD建立大量抽象层。

### 5.7 Files

Files拥有：

- 管理员文件上传。
- 文件记录。
- 下载和内容完整性。
- 删除补偿。
- 文件轮转。
- 文件访问签名。

用户头像属于Identity，不属于Files。两者可以分别声明所需对象存储Contract，并共同使用Platform提供的MinIO实现。

### 5.8 Audit

Audit拥有：

- 请求审计采集。
- 分类和脱敏。
- 审计写入。
- 查询和归档。

本轮保持现有异步写入和退出语义。队列可靠性、flush和graceful shutdown属于后续独立变更。

### 5.9 Route Catalog

Route Catalog拥有代码中的静态路由事实：

- HTTP方法和路径。
- Handler绑定。
- 路由访问等级。
- API名称、分组和默认权限码。
- 默认审计分类。
- OpenAPI请求和响应描述。

Route Catalog不从数据库动态创建Gin路由，也不保存运行时权限开关。

### 5.10 API Doc

API Doc消费Route Catalog和必要的API Metadata Snapshot，生成OpenAPI文档。核心生成逻辑应尽量保持纯转换，不直接扫描Gin Engine或查询全局数据库。

### 5.11 Upload Security

Upload Security提供文件内容验证能力。管理员文件和头像继续使用不同Policy：

- 管理员文件允许PDF、UTF-8 TXT、UTF-8 CSV。
- 头像允许JPEG、PNG、WebP输入，并执行解码、尺寸限制和重新编码。
- SHA-256表示实际存储内容的完整性基线。

---

## 6. 路由设计

采用：

> 业务模块定义路由，Route Catalog描述和校验，App统一注册。

### 6.1 路由放置

复杂模块：

```text
internal/<module>/adapters/http/routes.go
```

扁平模块：

```text
internal/<module>/routes.go
```

`routes.go`返回Route Descriptor，不直接调用`engine.GET`或`group.POST`。

示意：

```go
type AccessLevel string

const (
	Public               AccessLevel = "public"
	Authenticated        AccessLevel = "authenticated"
	PermissionControlled AccessLevel = "permission_controlled"
)

type Descriptor struct {
	Method         string
	Path           string
	Access         AccessLevel
	PermissionCode string
	Handler        gin.HandlerFunc
}
```

### 6.2 唯一注册点

`internal/app/http.go`是唯一允许修改Gin Engine的位置：

```text
模块创建Route Descriptor
→ Route Catalog收集和校验
→ App按访问等级挂载中间件
→ App注册到Gin Engine
```

业务模块、Route Catalog、API Metadata和API Doc均不得绕过App注册路由。

### 6.3 静态访问等级与动态策略

Route Catalog静态确定路由最低访问等级：

| 等级 | 身份要求 | 权限要求 |
| --- | --- | --- |
| Public | 可匿名 | 无 |
| Authenticated | 必须登录 | 不检查权限码 |
| PermissionControlled | 必须登录 | 由API Metadata决定是否检查权限码 |

PermissionControlled不命名为Admin，因为普通角色也可以凭权限访问管理员路径。

对于PermissionControlled路由：

```text
Identity
  → 始终要求有效登录身份

API Metadata
  → 校验API启用状态
  → NeedAuth=1时提供Permission Code策略
  → NeedAuth=0时跳过权限码检查

Authorization
  → NeedAuth=1时判断Principal是否拥有Permission Code
```

兼容性约束：

- `NeedAuth=0`不能把受保护路由变为匿名路由。
- PermissionControlled路由即使`NeedAuth=0`也必须登录。
- `NeedAudit`暂时不改变当前审计行为。
- 数据库配置不能把代码声明为Public的Handler替换成其他Handler，也不能新增Gin路由。

### 6.4 Route Catalog与API Metadata同步

同步方向：

```text
Route Catalog
  → API Metadata
```

同步原则：

- 新记录使用Catalog默认值创建。
- 已存在记录按现有兼容规则保留管理员配置。
- 软删除记录按现有规则恢复。
- 权限记录补齐由明确用例负责，不隐式混入普通路由注册。

具体覆盖字段必须以OpenSpec当前规格和回归测试为准。

---

## 7. 跨模块Contract与App装配

### 7.1 Contract归属

采用调用方定义最小接口：

```text
identity/application/contracts.go
authorization/application/contracts.go
navigation/application/contracts.go
organization/application/contracts.go
```

规则：

1. 接口由能力调用方定义。
2. 每个模块最多一个`application/contracts.go`。
3. 不为每个用例创建一个接口文件。
4. 提供方通过Go隐式接口实现能力。
5. App负责把实现注入调用方。
6. Application不得导入其他模块的Adapter。
7. 禁止Service Locator和全局模块注册表。

扁平模块确实需要跨模块能力时，只保留一个根级`contracts.go`；模块升级为物理分层后再移动到`application/contracts.go`。

示例：

```go
// authorization/application/contracts.go
type OrganizationHierarchy interface {
	OrganizationIDsForUser(ctx context.Context, userID uint) ([]uint, error)
	DescendantOrganizationIDs(
		ctx context.Context,
		organizationIDs []uint,
	) ([]uint, error)
}
```

Organization的公开capability满足接口，但不需要导入Authorization。

### 7.2 公开值类型

跨模块值类型必须稳定、最小，不暴露GORM Model。

例如Authorization公开：

```go
type Principal struct {
	UserID uint
}

type AccessSnapshot struct {
	Roles        []string
	Permissions  []string
	TokenVersion int
}
```

Principal只携带请求身份。角色和权限是否放入Principal应根据实际缓存与一致性策略决定，不能让JWT中的旧权限快照绕过运行时校验。

### 7.3 App装配

App是唯一系统级composition root：

```text
加载配置
→ 创建Logger、DB、Redis和MinIO
→ 创建共享数据库事务Adapter
→ 创建各模块Repository
→ 创建模块Application Service
→ 注入跨模块Capability
→ 收集Route Descriptor
→ Route Catalog校验
→ 注册中间件和Gin路由
→ 启动后台任务
```

`module.go`只负责模块内部factory，不读取全局配置，也不自行创建数据库、Redis或MinIO客户端。

### 7.4 推荐装配顺序

```text
Platform
→ Organization Hierarchy Core
→ Authorization Core
→ API Metadata Storage/Policy Core
→ Navigation Core
→ API Metadata Management
→ Identity Core
→ Files、Audit、Dictionary、API Doc
→ 各模块HTTP Adapter
→ Route Catalog
→ App注册Gin路由
```

Navigation需要API Metadata的读写Capability，而API Metadata修改权限码时又需要Navigation同步关联菜单。为避免构造循环，先构造不依赖Navigation的API Metadata存储/策略Core，再构造Navigation，最后构造依赖Navigation Capability的API Metadata管理用例。

如果其他模块间出现类似构造循环，也应拆分“基础Capability构造”和“HTTP/管理用例构造”，不得创建可变的半初始化Module再回填字段。

---

## 8. 菜单、API与权限的强事务

### 8.1 所有权

菜单/API绑定用例归Navigation：

```text
menus、role_menus、menu_apis  → Navigation
apis                           → API Metadata
permissions                    → Authorization
```

### 8.2 原子范围

以下操作必须属于同一个数据库事务：

```text
事务内重新读取并锁定目标API、菜单和现有关联
→ 捕获变更前受影响用户
→ 校验Permission Code变更
→ 维护或合并Permission及role_permissions
→ 更新apis.permission_code
→ 更新menus.permission_code
→ 替换menu_apis
→ 捕获变更后受影响用户
→ 提升新旧受影响用户的user_access_versions
```

当前`AssignAPIsToMenu`已经在一个事务中执行主要写入；当前`UpdateAPI`先更新API、再启动另一个事务同步菜单，仍存在部分成功风险。目标设计必须补齐完整原子性，并作为OpenSpec中的明确行为增强记录。

Permission Code视为稳定业务编码，不能被普通Repository更新绕过。API管理接口修改Permission Code时，必须委托Navigation拥有的跨模块绑定用例；Authorization提供权限重命名或合并能力，并保证：

- 既有`role_permissions`不会静默丢失。
- 新权限码已存在时合并角色授权并去重。
- 旧Permission仅在不再被菜单、API或角色引用时清理。
- 受影响用户在同一事务中提升授权版本。

### 8.3 事务seam

Application不接触`*gorm.DB`：

```go
type TransactionRunner interface {
	WithinTransaction(
		ctx context.Context,
		fn func(context.Context) error,
	) error
}
```

GORM事务Adapter把当前事务保存到私有Context值中。Authorization、Navigation和API Metadata的GORM Adapter从Context取得同一事务连接。

约束：

- Context中的事务实现细节只能由数据库Adapter读取。
- Application只能传递Context。
- 任一数据库写入失败，全部回滚。
- 不允许某个Capability在事务中回退使用全局DB。
- 校验读取、关联读取和受影响用户计算必须在事务内完成。
- GORM Adapter应对需要串行修改的API、菜单、Permission和关联行使用行锁。
- 一个业务用例只能存在一个最外层事务，内部Capability必须加入现有事务，不得自行提交新事务。
- 唯一约束负责最终冲突保护；TransactionRunner对MySQL死锁只允许有界重试。

### 8.4 事务内失效与提交后副作用

授权版本提升是可持久化的安全事实，必须包含在第8.2节的数据库事务中。提交后只执行不能加入数据库事务的副作用：

```text
事务内提升授权版本并镜像旧字段
→ 提交业务事务
→ 清理权限缓存或执行其他非数据库副作用
```

这样可以保证权限关系成功提交时，旧Token对应版本已经失效。提交后的缓存清理失败不能回滚数据库事务；应记录错误和指标，并通过重试处理。运行时校验必须以数据库授权版本为最终依据，不能仅依赖可能清理失败的缓存。

---

## 9. 数据范围

### 9.1 职责划分

采用：

> Authorization解析Scope，拥有数据的业务模块应用Scope过滤。

流程：

```text
业务模块取得Principal
→ Authorization根据角色和数据范围配置解析Scope
→ 业务模块Repository把Scope应用到自己的查询
```

Authorization不返回SQL、GORM Scope或数据库字段名。

### 9.2 资源类型明确的Scope

不使用一个包含任意资源ID的通用Scope。

```go
type UserScope struct {
	All     bool
	UserIDs []uint
}

type OrganizationScope struct {
	All             bool
	OrganizationIDs []uint
}
```

语义：

- `All=true`时ID集合必须为空。
- `All=false`且ID集合为空表示没有可见数据。
- 空集合绝不能解释成全部数据。
- `self`对应的`UserScope`只包含本人。
- `self`对应的`OrganizationScope`为空。
- Scope不包含GORM或SQL条件。

当前只为用户管理和组织管理接入明确Scope；新资源需要数据范围时，新增对应资源类型，而不是复用含义模糊的ID集合。

### 9.3 组织层级能力

Authorization通过自己声明的`OrganizationHierarchy`Contract读取组织成员和后代组织，不直接查询Organization的GORM Adapter。

Organization拥有组织结构；Authorization拥有数据范围解释。

---

## 10. 授权版本独立表

### 10.1 所有权调整

Identity继续拥有：

- 用户存在性。
- 用户启用状态。
- JWT和Refresh Token。
- Redis黑名单。

Authorization拥有：

- 授权版本。
- 版本校验。
- 版本提升。
- 角色、权限或数据范围变化导致的会话失效。

### 10.2 目标表

```text
user_access_versions
├── user_id        primary key
├── version        int not null default 1
├── created_at
└── updated_at
```

表不需要`deleted_at`。用户软删除后版本记录继续保留。

如果建立数据库外键：

- 不得使用`ON DELETE CASCADE`。
- 物理删除用户时必须由显式清理流程处理。

### 10.3 Token校验

Access Token和Refresh Token继续携带版本号。

```text
Identity解析Token
→ Identity校验黑名单
→ Identity校验用户存在且启用
→ Authorization读取user_access_versions
→ 比较Token版本
```

Authorization不再重复判断用户状态，避免Identity和Authorization共同拥有用户状态规则。

### 10.4 新用户懒初始化与缺行处理

采用首次登录时懒初始化：

```text
Identity确认用户存在且启用
→ Authorization执行EnsureVersion
→ 不存在则幂等创建version=1
→ 返回提交后的当前版本
→ Identity签发Token
```

并发首次登录使用等价于：

```go
tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&version)
```

然后重新读取，避免唯一键冲突。

约束：

- 只能在Identity完成用户有效性检查后初始化。
- 记录缺失不是跳过版本校验的理由。
- 初始化失败时登录失败。
- 应记录异常缺失和初始化指标，避免长期数据问题被静默掩盖。

所有会话失效操作统一使用 Authorization 提供的 `EnsureAndIncrement` 语义，包括用户禁用、软删除、Kick、角色调整、权限调整、菜单/API 绑定和数据范围变化：

```text
加入调用方最外层数据库事务
→ 幂等确保 user_access_versions 至少存在 version=1
→ 锁定版本记录
→ version 原子 +1
→ 提交数据库事务
→ 提交后再清理缓存
```

缺行时不能直接执行 `UPDATE version = version + 1` 后把零影响行当作成功，也不能回退到已删除的旧字段。`EnsureAndIncrement` 必须通过唯一键和行锁处理并发，使“首次登录初始化”和“首次登录前发生失效操作”最终只保留一行、版本不倒退。

### 10.5 Seed行为

Seed不创建超级管理员的授权版本：

```text
Seed创建超级管理员
→ 首次登录时由Authorization创建version=1
```

数据库迁移负责迁移前已经存在的用户；Seed不扫描全部用户补数据，也不承担历史修复职责。

### 10.6 用户删除和恢复

当前管理员删除用户为GORM软删除。目标行为：

```text
Identity软删除User
→ Authorization执行EnsureAndIncrement
→ 同一数据库事务提交
```

版本记录继续保留。恢复用户时沿用提升后的版本，因此删除前Token不会重新生效。

用户软删除与版本提升任一步失败，整个删除事务回滚。

---

## 11. 授权版本迁移前置已完成

`migrate-access-version-storage` 已在 2026-07-31 完成切换、加速耐久性资格验收、退出版本、独立删列和过渡能力退役。当前永久状态为：

- `user_access_versions` 是授权版本唯一持久化事实来源。
- Identity 负责 Token 解析、签名、黑名单和用户状态；Authorization 负责 `EnsureVersion`、当前版本读取和 `EnsureAndIncrement`。
- `users.token_version` 已从运行时、User Model、AutoMigrate 和数据库结构中移除。
- `access_version_migration_states`、镜像写、迁移锁、观察指标和旧版本回滚代码已退役。
- Seed 不创建、扫描或修复授权版本记录。

### 11.1 本次分层重构的消费边界

本次 Change 只把现有授权版本能力收口到 Authorization 模块，并由调用方通过最小 Contract 使用：

```text
Identity / Navigation / Organization 等调用方
→ 调用方声明授权版本 Contract
→ App 注入 Authorization 实现
→ Authorization Adapter 访问 user_access_versions
```

不得重新引入：

- 对 `users.token_version` 的读取、写入、Model 字段或 AutoMigrate 声明。
- 新旧字段双读、镜像写或取最大值。
- 迁移状态表、旧字段观察期或回滚到旧字段读取的路径。
- Seed 预创建或批量修复授权版本。

### 11.2 当前证据

- `openspec/changes/archive/2026-07-31-migrate-access-version-storage/evidence/2026-07-31-exit-milestone/README.md`
- `docs/adr/0004-authorization-owns-access-version-storage.md`
- `openspec/specs/access-version-storage/spec.md`
- `openspec/changes/restructure-layered-monolith/evidence/2026-08-02-access-version-cutover-current.md`

---
## 12. AutoMigrate与Seed

`internal/app/migrate.go`负责集中收集各模块需要迁移的Model，并保持当前启动失败策略。

`internal/app/seed.go`负责Seed顺序和事务编排，各模块提供所属Seed能力：

```text
Authorization：角色、权限、角色权限和数据范围默认值
Navigation：菜单和角色菜单
API Metadata：API元数据同步
Dictionary：字典类型和条目
Identity：超级管理员用户
```

必须保持：

- Seed幂等。
- 软删除默认数据按现有规则恢复。
- 用户已经修改的业务字段不被无条件覆盖。
- API、权限、菜单、角色关系和超级管理员初始化顺序有明确测试。
- Seed不承担`user_access_versions`历史回填。

---

## 13. 测试与验收策略

采用风险驱动的分层测试，并保留少量真实基础设施端到端测试。

### 13.1 架构边界测试

自动检查：

```text
Domain不导入Gin、GORM、Redis、MinIO、global
Application不导入Gin、GORM Model、global、initialize
业务模块不导入其他模块的adapters
只有internal/app修改Gin Engine
新增业务代码不导入旧handler/service/dto/model
```

### 13.2 Domain测试

- 纯Go测试。
- 不启动Gin、GORM、Redis或MinIO。
- 覆盖业务规则、Scope语义和值对象约束。

### 13.3 Application测试

通过Fake Contract覆盖：

- `/api/user/context`编排。
- Access Snapshot。
- UserScope和OrganizationScope解析。
- 菜单与API绑定。
- 用户软删除和授权版本提升。
- 首次登录懒初始化。
- 首次登录前发生禁用、软删除、Kick或授权变化时的`EnsureAndIncrement`。
- 事务内授权版本提升与提交后缓存清理调用。

### 13.4 数据库集成测试

覆盖：

- `user_access_versions` AutoMigrate、主键、非空版本、时间戳和无软删除字段。
- 授权版本记录在用户软删除后保留。
- `EnsureVersion`、`EnsureAndIncrement` 的幂等初始化、行锁、并发和版本不倒退。
- 用户软删除与授权版本提升的强事务。
- 菜单/API/Permission 强事务和失败回滚。
- Permission Code 变更保留或合并既有角色授权。
- 并发 API 更新和菜单绑定不会部分覆盖。
- TransactionRunner 的 MySQL 锁等待/死锁有界重试，以及非 MySQL 错误不重试。

SQLite 可用于快速反馈；涉及 MySQL 事务、约束或并发差异的关键场景必须使用 MySQL 集成测试。

### 13.5 HTTP兼容测试

重构前后保持：

```text
HTTP Method不变
Path不变
请求结构不变
响应结构不变
状态码不变
稳定错误码不变
权限行为不变，明确批准的原子性增强除外
```

Route Catalog Descriptor集合必须与Gin实际路由集合一致，不允许路由遗漏或重复。

### 13.6 认证与授权回归

覆盖：

- Access Token。
- Refresh Token。
- Redis黑名单。
- 用户禁用。
- 用户软删除与恢复。
- 授权版本提升。
- Public、Authenticated和PermissionControlled。
- `NeedAuth=0`不能匿名访问PermissionControlled路由。
- API禁用状态。
- 角色、权限、菜单和数据范围变化后的失效。

### 13.7 少量端到端测试

使用真实MySQL、Redis和必要的MinIO环境验证关键主链路：

```text
登录
→ 用户上下文
→ 权限访问
→ 菜单/API绑定
→ Token失效
→ Refresh Token拒绝旧版本
```

不要求所有接口都通过完整E2E覆盖，避免反馈过慢和失败定位困难。

---

## 14. OpenSpec拆分

采用两个有依赖关系的Change，不把数据库风险和全仓库目录移动放在同一个实施单元。

### 14.1 Change 1：`migrate-access-version-storage`

该 Change 已于 2026-07-31 完成并归档。它负责创建和切换 `user_access_versions`、完成授权版本验证、退出旧字段、执行独立删列 DDL，并退役迁移状态、镜像写、观察和旧版本回滚能力。

当前结论：

- `user_access_versions` 是唯一长期事实来源。
- `users.token_version` 已从运行时、User Model、AutoMigrate 和数据库结构中移除。
- `restructure-layered-monolith` 只依赖 Authorization 授权版本 Contract，不等待观察期、不恢复旧字段，也不重复执行 Change 1 的迁移任务。

证据以归档 Change、ADR 0004 和 `openspec/specs/access-version-storage/spec.md` 为准。

### 14.2 Change 2：`restructure-layered-monolith`

依赖 Change 1 已完成退出里程碑后开始；不再存在观察窗口或旧字段能力阻塞 Change 2。

范围：

- 建立`internal/app`和`internal/platform`。
- 按已确认一级模块迁移代码。
- 将Authorization有限拆分为Authorization、Navigation和API Metadata。
- 建立调用方拥有的最小Contract。
- 建立Route Catalog和模块路由Descriptor。
- 让App成为唯一Gin注册点。
- 完成菜单/API/Permission强事务。
- 删除旧技术目录和全部本 Change 产生的兼容 Adapter。

完成标准：

- 业务行为通过兼容测试。
- 明确批准的事务增强通过回滚测试。
- 不存在新旧两套正式调用路径。
- 全量测试稳定通过。

---

## 15. 渐进实施顺序

> 实施状态（2026-08-05）：阶段 0 至阶段 6 已完成。系统入口已切换到 `internal/app`，顶层技术目录、`global`、`legacyglobal` 和迁移期兼容 Adapter 已删除；当前只剩阶段 7 的最终门禁和规格验收。授权版本迁移已归档，本重构只消费现行 Authorization Contract，不依赖 `users.token_version` 或旧迁移观察期。

### 阶段0：前置复核与行为基线

1. 复核 `migrate-access-version-storage` 的退出证据和 `user_access_versions` 唯一事实来源。
2. 固化当前路由、响应、权限码、Seed、认证、文件和审计行为。
3. 补齐认证、RBAC、菜单/API、数据范围、文件和事务回归测试。
4. 建立隔离 SQLite、真实 MySQL、Redis 和必要 MinIO 测试门禁。

### 阶段1：建立 App、Platform 和 Route Catalog

1. 建立唯一 composition root。
2. 将配置和基础设施创建迁移到 Platform。
3. 建立 Descriptor 收集、重复校验、访问等级校验和只读 Snapshot。
4. 让 API Metadata、RBAC 路由权限同步、启动 Seed 和 API Doc 消费同一 Snapshot。
5. 由 App 统一注册 Gin 路由，保留必要的短期 `legacyglobal` 兼容 Adapter。
6. 禁止新增 `global` 访问和 Gin Engine 路由扫描旁路。

### 阶段2：以 Dictionary 验证模块模板

1. 把 Handler、用例、DTO、Model 和测试迁入同一模块。
2. 输出 Route Descriptor。
3. 由 App 注册路由。
4. 删除字典旧入口。

### 阶段3：迁移 Organization、Authorization、Navigation 和 API Metadata

1. 提取 Organization Hierarchy 只读 Capability。
2. 迁移 Authorization Core、Scope、授权版本 Contract 和 RBAC 路由权限同步。
3. 迁移 Navigation、菜单/API 绑定和前端菜单树。
4. 迁移 API Metadata、运行时策略、API 同步和 OpenAPI。
5. 建立共享事务 Context seam，补齐菜单/API/Permission 强事务。

### 阶段4：迁移 Identity

1. 迁移登录、JWT、验证码、用户资料、头像和 Refresh Token。
2. 通过正式 Contract 使用 Authorization、Navigation 和 Organization。
3. 接入 `/api/user/context` 编排。
4. 删除旧 JWT、用户上下文和数据范围入口。

### 阶段5：迁移 Files、Audit、API Doc 和 Upload Security

1. 迁移 Files 并删除全局 MinIO 旁路。
2. 迁移 Audit 并保持当前异步语义和敏感字段保护。
3. 让 API Doc 消费 Route Catalog。
4. 保持 Upload Security 现有深接口和文件安全策略。

### 阶段6：删除旧结构

1. 删除已经迁移的顶层 `handler/service/dto/model/router` 代码。
2. 删除 `legacyglobal` 和本 Change 产生的全部兼容 Adapter。
3. 删除无调用点的旧 middleware、initialize 旁路和 utils helper。
4. 更新 README、模块文档、OpenSpec、ADR 和代码导航。

### 阶段7：最终验收

1. 完成架构边界、Domain、Application、SQLite、MySQL、Redis 和必要 MinIO 测试。
2. 完成 Route Catalog、API/权限同步来源、事务回滚和 HTTP 兼容矩阵。
3. 对照全部主规格和 Delta Spec，确认除批准的事务原子性增强外没有 HTTP 行为变化。

前一阶段未满足完成标准时，不进入下一阶段；任何新的数据库表、历史回填或外部行为变化必须创建独立 OpenSpec Change。

## 16. 兼容Adapter规则

渐进迁移允许在App装配边缘存在短期兼容Adapter，但必须满足：

1. 名称包含`legacy`，用途清晰。
2. 只能由App装配。
3. 新Domain和Application不得调用旧包。
4. 不得新增旧入口调用点。
5. 每个兼容Adapter有唯一删除阶段。
6. 不允许兼容Adapter变成长期公共层。

允许：

```text
新App → legacy Adapter → 尚未迁移的旧实现
```

禁止：

```text
新Application → 旧service包
新Domain → global
旧代码和新代码同时成为事实来源
```

---

## 17. 风险与控制

### 17.1 大爆炸迁移

风险：同时移动所有目录导致路由遗漏、循环依赖和审查困难。

控制：按阶段和模块迁移，每个阶段有行为测试和兼容入口删除条件。

### 17.2 形式化分层导致膨胀

风险：每张表都创建Entity、DTO、Repository、Mapper和UseCase。

控制：简单模块保持扁平；接口只在存在替换、跨模块或测试价值时引入。

### 17.3 循环依赖

风险：Identity、Authorization、Navigation、API Metadata和Organization相互导入。

控制：调用方声明Contract，Go隐式实现，App负责注入；禁止导入其他模块Adapter。

### 17.4 路由与权限语义回归

风险：路由分组、`NeedAuth`和权限码的组合发生变化。

控制：静态AccessLevel和动态API策略分离，并建立HTTP兼容和认证矩阵测试。

### 17.5 跨模块事务失效

风险：Capability内部使用全局DB，导致看似同一事务、实际部分提交。

控制：使用Context事务seam，Adapter集成测试强制制造每一步失败并验证全部回滚。

### 17.6 授权版本边界回退

风险：分层迁移时重新让 Identity、Seed 或旧 Service 拥有授权版本存储，或恢复已退役的旧字段依赖。

控制：只允许 Authorization Adapter 访问 `user_access_versions`，调用方通过最小 Contract 使用 `EnsureVersion`、当前版本读取和 `EnsureAndIncrement`；架构测试禁止旧字段、迁移状态和旁路 Service 重新进入运行时。

### 17.7 路由事实来源分叉

风险：API 同步、RBAC 权限同步、Seed 或 OpenAPI 继续扫描 Gin Engine，形成与 Route Catalog 不一致的第二套路由事实。

控制：四个消费者使用同一 Route Catalog Snapshot，并通过集合一致性和禁止 `Engine.Routes()` 扫描测试锁定。

### 17.8 Seed职责扩张

风险：Seed承担业务修复和全表扫描。

控制：新用户版本由首次登录初始化，Seed 只维护基础数据且不得创建、扫描或修复授权版本。

### 17.9 文件安全退化

风险：目录迁移时绕过Validator或重新调用全局MinIO helper。

控制：保持文件和头像策略测试，业务模块只依赖使用方声明的对象存储Contract。

---

## 18. 最终验收标准

### 18.1 结构

- `internal`一级模块与第3节一致。
- 复杂模块按Domain/Application/Adapter分层。
- 简单模块保持扁平。
- 迁移完成后不存在全局业务`handler/service/dto/model/router`目录。
- App是唯一系统组合根和Gin注册点。

### 18.2 所有权

- Authorization不拥有菜单和API元数据。
- Navigation拥有`role_menus`和`menu_apis`。
- API Metadata拥有`apis`和运行时策略。
- Identity拥有JWT和用户状态。
- Authorization拥有`user_access_versions`和Scope解析。

### 18.3 依赖

- Domain不依赖框架和基础设施。
- Application不依赖Gin、GORM Model和全局变量。
- 业务模块不导入其他模块Adapter。
- 跨模块Contract由调用方集中定义。
- 基础设施通过constructor显式注入。

### 18.4 路由

- 业务模块定义Route Descriptor。
- Route Catalog校验方法和路径唯一性。
- App统一注册Gin路由。
- `NeedAuth=0`不能使PermissionControlled路由匿名。

### 18.5 数据和事务

- `user_access_versions` 保持授权版本唯一持久化事实来源。
- 运行时、Model、AutoMigrate 和测试不得恢复 `users.token_version` 或迁移状态依赖。
- 用户软删除和授权版本提升属于同一事务。
- 菜单、API 和 Permission 绑定满足完整原子性。
- 数据 Scope 不携带 GORM 或 SQL。

### 18.6 测试

- 架构边界测试通过。
- Application使用Fake Contract测试。
- Repository、并发事务和失败回滚测试通过。
- HTTP兼容测试通过。
- 关键认证端到端测试通过。
- `go test ./... -count=1`稳定通过。

### 18.7 可维护性

- 阅读单一业务能力主要停留在一个模块目录。
- 新增路由不需要修改集中式超大Router。
- 新功能有明确模块所有者。
- 兼容 Adapter 均有明确删除节点，完成迁移后不保留旧字段或双轨调用路径。

---

## 19. 决策记录

| 决策项 | 结论 |
| --- | --- |
| 架构形态 | 模块化分层单体 |
| 一级模块 | 固定为第3节列出的13个`internal`模块 |
| Authorization拆分 | 有限拆为Authorization、Navigation、API Metadata |
| 路由 | 模块定义、Route Catalog校验、App注册 |
| `role_menus` | Navigation拥有 |
| 用户上下文 | Identity拥有并编排Authorization和Navigation |
| 运行时权限 | Route Catalog静态等级 + Identity认证 + API Metadata策略 + Authorization权限判断 |
| 数据范围 | Authorization解析，资源模块应用 |
| Scope类型 | UserScope和OrganizationScope等资源明确类型 |
| TokenVersion | JWT Claim 保持兼容；持久化唯一来源为 `user_access_versions` |
| 新用户版本 | 首次登录懒初始化；首次登录前发生失效操作时原子创建并提升 |
| 用户删除 | 保留版本记录并在删除事务中提升 |
| 授权版本迁移 | `migrate-access-version-storage` 已完成并归档，不属于当前 Change |
| Seed | 不创建、扫描或修复用户授权版本 |
| 路由事实来源 | Route Catalog Snapshot 统一服务 API Metadata、RBAC 权限同步、Seed 和 API Doc |
| OpenSpec | Change 1 已归档；`restructure-layered-monolith` 为当前实施 Change |
| 跨模块接口 | 调用方定义最小 Contract |
| 测试 | 风险驱动的分层测试，保留少量E2E |

---

## 20. 与其他文档的关系

| 文档 | 定位 |
| --- | --- |
| [mature-admin-system-comparison-recommendations](mature-admin-system-comparison-recommendations.md) | 历史导航和项目阶段结论 |
| [mature-admin-system-comparison-overview](mature-admin-system-comparison-overview.md) | 已有能力、工程化差距和候选业务模块 |
| [project-improvement-roadmap](project-improvement-roadmap.md) | 全项目长期优先级和前置关系 |
| [authorization-flow-change-log](authorization-flow-change-log.md) | 权限体系演进历史 |
| [file-upload-security-recommendations](file-upload-security-recommendations.md) | 文件安全 V1 背景和后续方向 |
| [layered-monolith-restructuring](layered-monolith-restructuring.md) | 当前目录结构、模块所有权和迁移方案 |

出现冲突时，优先级为：

```text
当前OpenSpec change
  → OpenSpec主规格
  → ADR
  → CONTEXT
  → 本文
  → 其他docs/modify历史文档
```

本方案确认后，实施阶段应把难以撤销的长期决策同步为ADR，把行为变化写入OpenSpec，不继续让多个`docs/modify`文件重复维护同一执行计划。
