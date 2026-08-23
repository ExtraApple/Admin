# logging Delta Specification

## ADDED Requirements

### Requirement: 内部消息操作审计

系统 SHALL 将内部消息的关键业务操作和权限拒绝写入现有 Audit Log，并只保存受控元数据。

#### Scenario: 消息发送和状态操作被审计
- **WHEN** 用户或管理员发送、编辑、发布、撤销消息，或修改消息分类
- **THEN** Audit SHALL 记录操作者 ID、目标消息副本 ID、组织 ID、动作、结果、稳定原因码和时间
- **AND** Audit SHALL NOT 记录 Markdown、清洗后的 HTML、图片内容或完整外链 URL

#### Scenario: 消息权限拒绝被审计
- **WHEN** 用户因消息权限、组织范围、受众、分类或状态规则被拒绝
- **THEN** Audit SHALL 记录请求用户、目标资源、稳定拒绝原因和结果
- **AND** Audit SHALL NOT 记录数据库、RabbitMQ、MinIO、解析器或代理的原始错误

#### Scenario: RabbitMQ 和 Outbox 运行失败
- **WHEN** Outbox 发布、RabbitMQ 消费、重试或死信处理失败
- **THEN** 系统 SHALL 写入受控运行日志
- **AND** 运行日志 SHALL 包含事件 ID、消费者或发布阶段、稳定错误分类和重试信息
- **AND** 日志 SHALL NOT 包含消息正文、HTML、图片内容、Token 或外链 URL

#### Scenario: 死信 Outbox 重放被审计
- **WHEN** 管理员通过受控 HTTP 入口重放死信 Outbox
- **THEN** Audit SHALL 记录操作者、Outbox 事件 ID、原聚合版本、结果和稳定原因码
- **AND** Audit SHALL NOT 记录消息正文、投递投影、凭据或 Broker 原始错误

#### Scenario: Consumer 死信处置被审计
- **WHEN** 超级管理员查询、重放或丢弃 Consumer DLQ 消息，或重放事件再次第五次失败并重新打开原投影
- **THEN** Audit SHALL 记录操作者（如有）、消费者名称、事件 ID、`replay_cycle`、动作、结果和稳定原因码
- **AND** Audit SHALL NOT 记录消息正文、投递投影、凭据或 Broker 原始错误

#### Scenario: DLQ Recorder 持久化失败
- **WHEN** DLQ Recorder 无法写入 Consumer 死信投影或进入运维告警队列
- **THEN** 运行日志 SHALL 记录消费者、事件 ID、稳定失败分类和受控重试次数
- **AND** 日志 SHALL NOT 记录消息正文、投递投影、凭据或 Broker 原始错误

#### Scenario: Consumer 死信立即告警
- **WHEN** 任一 `(Consumer, 稳定失败码)` 的 `pending` Consumer DLQ 投影计数从零变为非零且 MySQL 投影已提交
- **THEN** Messaging SHALL 以 best-effort 调用 App 注入的 `MessagingMetrics.RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)`，记录仅含 Consumer、稳定失败码、待处置计数和最旧 `pending` 年龄的 Observation，且方法不返回错误
- **AND** 部署监控系统 SHALL 基于该观测触发实际运维告警；本 Change SHALL NOT 直接调用 Prometheus、OTel、Alertmanager、Webhook 或 SMTP
- **WHEN** Contract Adapter 自身异常
- **THEN** 系统 SHALL 写入受控运行日志，且 DLQ Recorder SHALL 继续 ACK RabbitMQ 消息而不重试 AMQP 消息
- **AND** Observation 和日志 SHALL NOT 包含事件载荷、事件 ID、用户标识、消息正文、凭据或 Broker 原始错误

#### Scenario: 运维告警队列处置
- **WHEN** 运维人员通过受限 CLI 或 RabbitMQ 管理界面处置 DLQ Recorder 告警队列
- **THEN** 部署运行日志 SHALL 记录队列、事件 ID、动作、结果和稳定原因码
- **AND** 应用 HTTP 日志 SHALL NOT 伪造该运维操作已由系统处理

#### Scenario: Consumer 死信投影重放状态
- **WHEN** 超级管理员重放 Consumer 死信投影且 Confirm 失败、租约到期、Confirm 后进程崩溃，或原 Consumer 将旧版本重放标记为 `superseded`
- **THEN** 运行日志 SHALL 记录投影 ID、事件 ID、稳定重放或 Consumer 结果状态和原因码
- **AND** 日志 SHALL NOT 记录最小事件载荷、凭据或 Broker 原始错误

#### Scenario: 受众快照容量超限
- **WHEN** Consumer 解析到超过 100,000 用户的动态受众
- **THEN** 运行日志 SHALL 记录事件 ID、消费者、稳定容量错误码、观察到的受众数量和受控重试次数
- **AND** 日志 SHALL NOT 记录用户 ID 列表、消息正文或受众规则细节

#### Scenario: 消息事件快照围栏丢失
- **WHEN** Consumer 无法续租动态受众快照租约或发现 `snapshot_fence` 不匹配
- **THEN** 运行日志 SHALL 记录事件 ID、消费者、稳定围栏失败码、处理阶段和受控重试次数
- **AND** 日志 SHALL NOT 记录用户 ID 列表、消息正文、完整受众快照或凭据

### Requirement: 邮箱验证安全审计

系统 SHALL 对邮箱验证请求、SMTP 投递结果、确认和节流拒绝记录受控审计与运行日志。

#### Scenario: 邮箱验证操作
- **WHEN** 用户请求、重发或确认邮箱验证，或 SMTP 投递成功或失败
- **THEN** Audit SHALL 记录用户 ID、操作、结果、稳定原因码和时间
- **AND** Audit 与运行日志 SHALL NOT 记录邮箱地址、token、token 摘要、SMTP 用户名、密码、服务器地址或 SMTP 原始错误

#### Scenario: 邮箱验证节流拒绝
- **WHEN** 用户在滚动一小时内超过三次验证邮件请求
- **THEN** Audit SHALL 记录稳定节流原因码和拒绝结果
- **AND** 错误响应 SHALL NOT 回显邮箱地址或凭据

### Requirement: WebSocket ticket 脱敏

系统 SHALL 防止 WebSocket ticket 进入普通请求日志、审计日志和错误响应。

#### Scenario: 请求携带 ticket
- **WHEN** 客户端调用 WebSocket ticket 签发或升级接口
- **THEN** 请求日志 SHALL 对 ticket 查询参数和协议字段脱敏
- **AND** 审计和错误响应 SHALL NOT 回显 ticket
