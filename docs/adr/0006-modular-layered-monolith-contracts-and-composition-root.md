# 分层单体的业务模块、调用方 Contract 与唯一组合根

## 状态

已接受。

## 背景

项目原先以 `handler`、`service`、`dto`、`model`、`router` 等技术目录横向组织业务。一个业务能力需要跨多个目录阅读，且全局数据库、缓存、对象存储、集中式 Gin 路由和跨模块 Adapter 使边界与事务所有权难以验证。

`restructure-layered-monolith` 需要在不改变既有 HTTP 契约、认证行为、Redis key、MinIO bucket 和数据库表事实来源的前提下完成渐进迁移。授权版本存储已经由 ADR 0004/0005 完成退出，当前重构只能消费 `user_access_versions` Contract，不得恢复 `users.token_version` 或退役迁移状态。

## 决策

### 1. 固定一级业务模块

业务代码最终只放在以下 `internal` 一级模块中：

- `app`
- `platform`
- `identity`
- `authorization`
- `navigation`
- `apimetadata`
- `organization`
- `dictionary`
- `files`
- `audit`
- `routecatalog`
- `apidoc`
- `uploadsecurity`

不再为角色、权限、菜单或按钮增加一级模块；这些数据和用例由 Authorization、Navigation 或 API Metadata 的唯一所有者提供。

### 2. 模块内部按复杂度分层

复杂模块可在自身目录内使用 `domain`、`application` 和 `adapter`；简单模块保持扁平。依赖方向固定为：

```text
HTTP / Infrastructure Adapter → Application → Domain
```

Domain 不依赖 Application、Adapter、Gin、GORM Model、Redis、MinIO 或全局运行时状态。Application 只依赖领域类型和最小 Contract，不导入其他模块 Adapter，不暴露 GORM 连接。

### 3. 跨模块能力由调用方拥有 Contract

需要其他模块能力的 Application 在本模块定义最小输入/输出接口；被调用模块只实现该接口，App 在组合根中显式注入。Contract 不暴露 GORM Model、SQL、Gin Context、Redis client、MinIO client 或 ORM 查询对象。

Authorization 对外提供 Principal/AccessSnapshot、授权版本提升和资源 Scope 能力；Organization 提供组织层级与成员查询能力；Navigation 提供菜单导航能力；API Metadata 提供运行时元数据能力。各模块不得通过全局函数、共享 Model 或跨模块 Adapter 旁路调用。

### 4. Route Catalog 是静态路由事实来源

每个含 HTTP 入口的模块返回 Route Descriptor，包含规范化 Method、Path、最低 Access Level、Handler、API 元数据默认值、审计分类和完整 OpenAPI operation/schema。Descriptor 不直接调用 Gin 注册方法。

Route Catalog 在启动注册前收集、规范化、校验并拒绝重复或不完整描述，生成只读 Snapshot。只有 App 能把 Snapshot 注册到 Gin Engine；API Metadata 同步、RBAC 权限同步、启动 Seed 和 API Doc 统一消费这份 Snapshot，不扫描 Gin Engine。

最低访问等级固定为 `Public`、`Authenticated`、`PermissionControlled`。运行时 API Metadata 的 `need_auth` 只能控制受保护路由内的元数据策略，不能把静态受保护路由降级为匿名访问。首次权限同步例外必须是显式、可测试的 Route Catalog 策略。

### 5. App 是唯一组合根与唯一 Gin 注册点

`internal/app` 负责读取配置、创建 Platform 基础设施、装配模块 Repository/Application/Adapter、收集 Route Descriptor、构造 Route Catalog Snapshot、注册 Gin 路由、编排 AutoMigrate/Seed 和后台任务。除 App 外的代码不得创建系统级 Gin Engine 或向其注册业务路由。

`internal/platform` 负责配置、日志、数据库、缓存、对象存储和共享 TransactionRunner 的构造。事务 Runner 通过 Context 传递当前事务连接；跨模块用例只有一个最外层事务，内部 Capability 必须加入已有事务，不得自行提交或回退全局 DB。

### 6. 迁移完成后只保留模块正式路径

迁移期兼容 Adapter 已完成删除。系统不再保留顶层业务 `handler`、`service`、`dto`、`model`、`router`、`middleware`、`utils`、`global`，也不保留 `legacyglobal`。后续业务变更只能进入固定 `internal` 模块；模块不得重新引入旧入口、全局运行时状态或其他模块 Adapter。

## 约束与验证

- HTTP Method、Path、请求/响应结构、状态码和稳定错误码保持兼容；菜单/API/Permission Code/角色授权/Token 失效的批准原子性增强除外。
- `user_access_versions` 是授权版本唯一读取来源；Seed 不创建授权版本，不声明或恢复 `users.token_version` 和退役迁移状态。
- Route Catalog 必须验证 Method + Path 唯一、Access Level 完整和 OpenAPI 描述完整。
- 架构测试扫描固定一级模块、核心层禁止依赖、唯一 Gin 注册点和旧目录旁路；SQLite 使用隔离临时库，MySQL/Redis/MinIO 关键语义由真实集成门禁验收。
- 每个迁移阶段必须保留兼容测试、只有一条正式调用路径，并有明确的 Adapter 删除节点。

## 后果

业务边界和事务所有权已由固定模块、调用方 Contract 和 App 组合根固化；路由、API 元数据、RBAC 同步和 OpenAPI 文档共享稳定静态事实来源。新增模块或跨模块用例必须通过架构测试保持这些边界，避免旧技术目录、全局状态和第二条正式调用路径回流。
