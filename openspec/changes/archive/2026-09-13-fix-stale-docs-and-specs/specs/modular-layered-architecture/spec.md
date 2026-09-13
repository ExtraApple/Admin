## MODIFIED Requirements

### Requirement: App 统一装配系统
`internal/app` SHALL 是唯一系统组合根，并由 `internal/platform` 提供配置、数据库、缓存、对象存储、RabbitMQ 和日志等基础设施实例。

#### Scenario: 启动系统
- **WHEN** 服务启动
- **THEN** App SHALL 创建基础设施、Repository 和 Application Service
- **AND** App SHALL 注入跨模块 Capability、收集路由并启动后台任务
- **AND** 业务模块 SHALL NOT 自行创建数据库、Redis 或 MinIO 客户端
#### Scenario: Messaging 启动装配
- **WHEN** 服务启动
- **THEN** App SHALL 创建 RabbitMQ Publisher、Messaging Repository、Application Service、HTTP Adapter、Outbox Worker、WebSocket Consumer 和 `MessagingMetrics` Adapter
- **AND** App SHALL 注入 Identity、Organization、Authorization、Files、Audit 和 MessagingMetrics Contract
- **AND** 业务模块 SHALL NOT 自行创建数据库、Redis、MinIO 或 RabbitMQ 客户端

#### Scenario: RabbitMQ 暂时不可用
- **WHEN** RabbitMQ 连接或拓扑声明暂时失败
- **THEN** App SHALL 保持 MySQL 消息事实写入能力并将 Outbox Worker 标记为延迟
- **AND** Messaging SHALL NOT 创建替代 Broker 或全局客户端

#### Scenario: RabbitMQ 降级就绪
- **WHEN** RabbitMQ 不可用但 MySQL 消息事务仍可接受写入
- **THEN** `GET /ping` SHALL 继续只报告进程存活，`GET /api/ready` SHALL 返回 HTTP 200 及受控 RabbitMQ 状态字段
- **AND** 总体和 RabbitMQ 组件状态 SHALL 为 `degraded`
- **AND** 响应 SHALL NOT 泄露 Broker 地址、凭据、时间戳、Worker 身份或原始错误

#### Scenario: Consumer DLQ 不改变就绪
- **WHEN** Messaging 存在待处置 Consumer DLQ 投影
- **THEN** `GET /api/ready` SHALL 保持 HTTP 200 和原 status
- **AND** 待处置统计 SHALL 通过受保护查询和监控指标提供，而非公开就绪响应

#### Scenario: 执行迁移和 Seed
- **WHEN** 服务执行 AutoMigrate 或 Seed
- **THEN** App SHALL 集中编排模块提供的 Model 和 Seed 能力
- **AND** 各模块 SHALL 只维护自身拥有的数据定义和基础数据
