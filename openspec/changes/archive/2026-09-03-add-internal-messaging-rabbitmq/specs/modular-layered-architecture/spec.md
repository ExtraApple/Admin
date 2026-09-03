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
- **THEN** App SHALL 创建 RabbitMQ Publisher、Messaging Repository、Application Service、HTTP Adapter、Outbox Worker、WebSocket Consumer 和 `MessagingMetrics` Adapter
- **AND** App SHALL 注入 Identity、Organization、Authorization、Files、Audit 和 `MessagingMetrics` Contract
- **AND** 业务模块 SHALL NOT 自行创建数据库、Redis、MinIO 或 RabbitMQ 客户端

#### Scenario: RabbitMQ 暂时不可用
- **WHEN** RabbitMQ 连接或拓扑声明暂时失败
- **THEN** App SHALL 保持 MySQL 消息事实写入能力
- **AND** App SHALL 将 Outbox Worker 标记为延迟并记录受控运行状态
- **AND** App SHALL NOT 由 Messaging 直接创建替代 Broker 或全局客户端

#### Scenario: RabbitMQ 降级就绪状态
- **WHEN** RabbitMQ 不可用但 MySQL 消息事务仍可接受写入
- **THEN** 既有 `GET /api/health` SHALL 继续只报告进程存活
- **AND** 公开 `GET /api/ready` SHALL 返回 HTTP 200，以及 `{status, components.rabbitmq.status, components.rabbitmq.outbox_pending, components.rabbitmq.last_error_code}`
- **AND** RabbitMQ 降级时总体和 RabbitMQ 组件状态 SHALL 为 `degraded`
- **AND** 就绪响应 SHALL NOT 泄露 Broker 地址、凭据、时间戳、Worker 身份或原始连接错误

#### Scenario: Consumer 死信不降级就绪
- **WHEN** Messaging 存在待处置的 Consumer DLQ 投影
- **THEN** App SHALL 将其作为运行告警而非接流量就绪失败
- **AND** `GET /api/ready` SHALL NOT 因待处置 Consumer DLQ 改变 HTTP 200 或 `status`
- **AND** App SHALL 通过受保护管理查询和监控指标提供待处置总数、按 Consumer/稳定失败码计数及最旧 `pending` 年龄

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

#### Scenario: Messaging 记录 Consumer DLQ 告警观测
- **WHEN** Consumer DLQ 的 `(Consumer, 稳定失败码)` `pending` 计数从零变为非零且 MySQL 投影已提交
- **THEN** Messaging SHALL 仅以 best-effort 依赖 App 注入的供应商无关 `MessagingMetrics.RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)`
- **AND** Observation SHALL 仅包含 `consumer_name`、稳定 `failure_code`、`pending_count` 和 `oldest_pending_age`，该方法 SHALL 不返回错误
- **AND** Adapter 异常 SHALL NOT 阻止 DLQ Recorder ACK 或导致 AMQP 重试
- **AND** Messaging SHALL NOT 导入 Prometheus、OTel、Alertmanager、Webhook、SMTP 或其他告警提供商 SDK，也不得实现告警发送器

#### Scenario: 未来邮件短信接入
- **WHEN** 后续能力消费内部消息事件
- **THEN** 邮件和短信 SHALL 作为 App 注入的进程内 Consumer Adapter，通过 Messaging Projection Contract 查询当前允许投递的内容和用户标识，再通过 Identity Contract 解析当前渠道地址
- **AND** 邮件、短信 SHALL NOT 直接写入 Messaging Model、调用网络 Projection API 或修改 Messaging 核心事务

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
