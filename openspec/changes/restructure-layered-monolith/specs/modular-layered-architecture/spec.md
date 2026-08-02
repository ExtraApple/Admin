## ADDED Requirements

### Requirement: 固定业务模块边界
系统 SHALL 将业务代码组织在固定的 `internal` 一级模块中，并为每项业务数据和用例指定唯一所有者。

#### Scenario: 检查一级模块
- **WHEN** 架构边界测试扫描 `internal` 下的系统模块
- **THEN** 系统只使用 `app`、`platform`、`identity`、`authorization`、`navigation`、`apimetadata`、`organization`、`dictionary`、`files`、`audit`、`routecatalog`、`apidoc` 和 `uploadsecurity`
- **AND** 系统不为角色、权限、菜单或按钮继续增加独立一级业务模块

#### Scenario: 检查业务所有权
- **WHEN** 代码访问角色、权限、菜单、API 元数据、用户身份或组织数据
- **THEN** 该数据的写入用例 SHALL 由其唯一所有模块提供
- **AND** 其他模块 SHALL 通过显式 Contract 使用该能力

### Requirement: 模块内部按复杂度分层
复杂模块 SHALL 在模块内部按 Domain、Application 和 Adapter 分离职责，简单模块 SHALL 能够保持扁平而不创建空层。

#### Scenario: 复杂模块依赖方向
- **WHEN** 架构测试检查复杂模块的包依赖
- **THEN** HTTP 和基础设施 Adapter 可以依赖 Application
- **AND** Application 可以依赖 Domain
- **AND** Domain SHALL NOT 依赖 Application 或 Adapter

#### Scenario: 简单模块保持局部性
- **WHEN** 模块没有稳定领域规则、多个入口 Adapter 或复杂跨模块依赖
- **THEN** 模块 SHALL 能够在自身目录内保持扁平
- **AND** 模块 SHALL NOT 为形式统一创建无职责的 Domain、Application 或 Repository 包

### Requirement: 核心层隔离框架和全局状态
Domain 和 Application SHALL 与 HTTP 框架、ORM Model、基础设施 SDK 及全局运行时状态隔离。

#### Scenario: Domain 依赖检查
- **WHEN** 架构测试扫描 Domain 包
- **THEN** Domain SHALL NOT 导入 Gin、GORM、Redis、MinIO、`global` 或 `initialize`

#### Scenario: Application 依赖检查
- **WHEN** 架构测试扫描 Application 包
- **THEN** Application SHALL NOT 导入 Gin、GORM Model、`global`、`initialize` 或其他业务模块的 Adapter

### Requirement: App 统一装配系统
`internal/app` SHALL 是唯一系统组合根，并由 `internal/platform` 提供配置、数据库、缓存、对象存储和日志等基础设施实例。

#### Scenario: 启动系统
- **WHEN** 服务启动
- **THEN** App SHALL 创建基础设施、Repository 和 Application Service
- **AND** App SHALL 注入跨模块 Capability、收集路由并启动后台任务
- **AND** 业务模块 SHALL NOT 自行创建数据库、Redis 或 MinIO 客户端

#### Scenario: 执行迁移和 Seed
- **WHEN** 服务执行 AutoMigrate 或 Seed
- **THEN** App SHALL 集中编排模块提供的 Model 和 Seed 能力
- **AND** 各模块 SHALL 只维护自身拥有的数据定义和基础数据

### Requirement: 调用方拥有最小跨模块 Contract
跨模块同步调用 SHALL 使用调用方定义的最小接口，并由 App 显式注入提供方实现。

#### Scenario: Application 调用其他模块能力
- **WHEN** 一个 Application 用例需要组织层级、授权快照、菜单树或 API 元数据能力
- **THEN** 调用方 SHALL 在单一 `contracts.go` 中声明所需最小接口
- **AND** Contract SHALL NOT 暴露 GORM Model、Gin Context 或基础设施客户端

#### Scenario: 提供方实现 Contract
- **WHEN** App 组装调用方和提供方
- **THEN** 提供方 SHALL 通过 Go 隐式接口满足调用方 Contract
- **AND** 提供方 SHALL NOT 为实现该 Contract 导入调用方包

### Requirement: 跨模块值类型保持稳定和最小
跨模块 Contract SHALL 使用业务值类型传递身份、授权快照和数据范围，不得泄漏持久化或 HTTP 实现。

#### Scenario: 创建请求 Principal
- **WHEN** Identity 完成 Token、黑名单、用户存在性和启用状态校验
- **THEN** 系统 SHALL 创建只携带用户 ID 的 Principal
- **AND** JWT 中的旧角色或权限快照 SHALL NOT 绕过运行时授权校验

#### Scenario: 返回 Access Snapshot
- **WHEN** Identity 请求用户初始化上下文或其他模块请求授权快照
- **THEN** Authorization SHALL 返回最小的角色、权限和授权版本值
- **AND** 返回值 SHALL NOT 包含 GORM Model、SQL 或基础设施客户端

### Requirement: 模块所有权保持分离
Authorization、Navigation、API Metadata、Identity 和 Organization SHALL 按各自变化原因维护业务能力。

#### Scenario: 授权和导航数据归属
- **WHEN** 系统维护角色、权限、用户角色、角色权限、数据范围、菜单、角色菜单或菜单 API 关联
- **THEN** 角色、权限、用户角色、角色权限和数据范围 SHALL 由 Authorization 拥有
- **AND** 菜单、`role_menus`、`menu_apis` 和可见菜单树 SHALL 由 Navigation 拥有

#### Scenario: 角色菜单路由归属
- **WHEN** 系统处理 `POST /api/admin/roles/:id/menus` 或 `GET /api/admin/roles/:id/menus`
- **THEN** 路由和用例 SHALL 由 Navigation 拥有
- **AND** URL 中出现角色资源 SHALL NOT 改变 `role_menus` 的数据所有权

#### Scenario: 身份和 API 元数据归属
- **WHEN** 系统处理 JWT、Refresh Token、用户状态、用户上下文或 API 运行时策略
- **THEN** JWT、Refresh Token、黑名单、用户状态和用户上下文编排 SHALL 由 Identity 拥有
- **AND** `apis`、状态、认证标记、审计标记和 Permission Code 策略 SHALL 由 API Metadata 拥有

#### Scenario: 组织、文件和系统能力归属
- **WHEN** 系统维护组织树、组织成员、字典、管理员文件、用户头像、审计、OpenAPI 或上传验证
- **THEN** 组织树和成员关系 SHALL 由 Organization 拥有
- **AND** 字典类型和条目 SHALL 由 Dictionary 拥有
- **AND** 管理员文件 SHALL 由 Files 拥有，用户头像 SHALL 由 Identity 拥有
- **AND** 审计采集和记录 SHALL 由 Audit 拥有
- **AND** OpenAPI 生成 SHALL 由 API Doc 拥有，文件内容验证 SHALL 由 Upload Security 提供

### Requirement: 数据范围使用资源明确的 Scope
Authorization SHALL 解析数据范围，拥有数据的业务模块 SHALL 将资源类型明确的 Scope 应用到自身查询。

#### Scenario: 应用用户数据范围
- **WHEN** 用户管理查询需要数据范围过滤
- **THEN** Authorization SHALL 返回 UserScope
- **AND** 用户 Repository SHALL 使用 `All` 或 `UserIDs` 应用过滤

#### Scenario: 应用组织数据范围
- **WHEN** 组织管理查询需要数据范围过滤
- **THEN** Authorization SHALL 返回 OrganizationScope
- **AND** Organization Repository SHALL 使用 `All` 或 `OrganizationIDs` 应用过滤

#### Scenario: Scope 表示全部数据
- **WHEN** Scope 的 `All = true`
- **THEN** 对应 ID 集合 SHALL 为空
- **AND** 资源 Repository SHALL 不增加 ID 限制

#### Scenario: Scope 表示无可见数据
- **WHEN** Scope 的 `All = false` 且对应 ID 集合为空
- **THEN** 资源 Repository SHALL 返回空结果
- **AND** 空集合 SHALL NOT 被解释为全部数据

#### Scenario: self 数据范围
- **WHEN** Authorization 解析 `self` 数据范围
- **THEN** UserScope SHALL 只包含当前用户 ID
- **AND** OrganizationScope SHALL 返回无可见组织的空集合

#### Scenario: Scope 保持存储无关
- **WHEN** Authorization 返回任意资源 Scope
- **THEN** Scope SHALL NOT 包含 SQL、GORM Scope 或数据库字段名

### Requirement: 渐进迁移只有一条正式调用路径
系统 SHALL 通过有删除节点的兼容 Adapter 逐模块迁移，并在迁移完成后删除旧技术目录和旁路入口。

#### Scenario: 模块处于迁移期
- **WHEN** 新 App 仍需调用尚未迁移的旧实现
- **THEN** App SHALL 通过名称明确的 legacy Adapter 调用旧实现
- **AND** 新 Domain 和 Application SHALL NOT 调用该 Adapter
- **AND** 不得新增旧入口调用点

#### Scenario: 模块完成切换
- **WHEN** 一个模块的兼容测试和回归测试通过
- **THEN** App SHALL 只装配该模块的新实现
- **AND** 系统 SHALL 删除该模块对应的旧正式入口

#### Scenario: 全部模块完成迁移
- **WHEN** 本 Change 的模块迁移任务全部完成
- **THEN** 系统 SHALL 不再保留全局业务 `handler`、`service`、`dto`、`model` 和 `router` 目录
- **AND** 系统 SHALL 不再保留 `legacyglobal` 或本 Change 引入的兼容 Adapter
