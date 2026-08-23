# modular-layered-architecture Delta Specification

## MODIFIED Requirements

### Requirement: 固定业务模块边界

系统 SHALL 将业务代码组织在固定的 `internal` 一级模块中，并为每项业务数据和用例指定唯一所有者。

#### Scenario: 检查一级模块
- **WHEN** 架构边界测试扫描 `internal` 下的系统模块
- **THEN** 系统只使用 `app`、`platform`、`identity`、`authorization`、`navigation`、`apimetadata`、`organization`、`dictionary`、`files`、`messaging`、`audit`、`routecatalog`、`apidoc` 和 `uploadsecurity`
- **AND** 系统不为角色、权限、菜单、按钮、邮件或短信继续增加独立一级业务模块

#### Scenario: 检查业务所有权
- **WHEN** 代码访问消息主体、分类、受众、收件箱状态、Outbox 或消息事件幂等数据
- **THEN** 该数据的写入用例 SHALL 由 `messaging` 唯一拥有
- **AND** Identity、Organization、Authorization、Files、Audit 和 Platform SHALL 通过显式 Contract 提供能力

### Requirement: App 统一装配系统

`internal/app` SHALL 是唯一系统组合根，并由 `internal/platform` 提供配置、数据库、缓存、对象存储、RabbitMQ 和日志等基础设施实例。

#### Scenario: 启动系统
- **WHEN** 服务启动
- **THEN** App SHALL 创建 RabbitMQ Publisher、Messaging Repository、Application Service、HTTP Adapter、Outbox Worker 和 WebSocket Consumer
- **AND** App SHALL 注入 Identity、Organization、Authorization、Files 和 Audit Contract
- **AND** 业务模块 SHALL NOT 自行创建数据库、Redis、MinIO 或 RabbitMQ 客户端

#### Scenario: RabbitMQ 暂时不可用
- **WHEN** RabbitMQ 连接或拓扑声明暂时失败
- **THEN** App SHALL 保持 MySQL 消息事实写入能力
- **AND** App SHALL 将 Outbox Worker 标记为延迟并记录受控运行状态
- **AND** App SHALL NOT 由 Messaging 直接创建替代 Broker 或全局客户端

#### Scenario: 执行迁移和 Seed
- **WHEN** 服务执行 AutoMigrate 或 Seed
- **THEN** App SHALL 集中编排 Messaging Model、消息分类基础数据和消息拓扑声明
- **AND** 各模块 SHALL 只维护自身拥有的数据定义和基础数据

## ADDED Requirements

### Requirement: Messaging 核心层隔离异步基础设施

系统 SHALL 将 RabbitMQ、Redis、GORM 和 WebSocket 细节限制在 Messaging Adapter、Platform Adapter 或 App 组合根。

#### Scenario: Application 使用消息事件能力
- **WHEN** Messaging Application 写入消息状态并请求发布事件
- **THEN** Application SHALL 依赖抽象的 Outbox/Publisher Contract
- **AND** Domain/Application SHALL NOT 导入 RabbitMQ Client、Gin、GORM Model、Redis Client 或 MinIO Client

#### Scenario: 未来邮件短信接入
- **WHEN** 后续能力消费内部消息事件
- **THEN** 邮件和短信 SHALL 通过独立 Consumer Adapter 订阅事件
- **AND** 邮件、短信 SHALL NOT 直接写入 Messaging Model 或修改 Messaging 核心事务

### Requirement: 调用方拥有消息跨模块 Contract

跨模块同步调用 SHALL 使用 Messaging 或调用方定义的最小 Contract，不得共享持久化模型。

#### Scenario: 消息查询组织与授权
- **WHEN** Messaging 需要判断动态受众或管理员组织范围
- **THEN** Messaging SHALL 通过显式 Contract 获取组织成员、角色关系和数据范围
- **AND** Contract SHALL 只返回业务值类型和资源 Scope

#### Scenario: 消息图片读写
- **WHEN** Messaging 创建或读取消息图片
- **THEN** Messaging SHALL 调用 Files 的消息图片 Contract
- **AND** Contract SHALL 不暴露 MinIO URL、object key、GORM Model 或存储客户端
