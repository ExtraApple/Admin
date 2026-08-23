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

### Requirement: WebSocket ticket 脱敏

系统 SHALL 防止 WebSocket ticket 进入普通请求日志、审计日志和错误响应。

#### Scenario: 请求携带 ticket
- **WHEN** 客户端调用 WebSocket ticket 签发或升级接口
- **THEN** 请求日志 SHALL 对 ticket 查询参数和协议字段脱敏
- **AND** 审计和错误响应 SHALL NOT 回显 ticket
