# Zap 运行日志

> 本文只保留当前运行日志架构和运维约束。已确认行为以 [`openspec/specs/logging/spec.md`](../../openspec/specs/logging/spec.md) 为准。

## 当前架构

运行日志由 `internal/platform/logging` 创建，App 将 Zap Logger 显式注入模块和后台任务。业务代码不依赖 `global.Logger`，也不从旧的 `initialize` 或 `middleware` 目录获取日志能力。

```text
internal/platform/logging
→ 创建 Zap Logger 和 lumberjack 输出
→ internal/app 统一装配
→ 模块与后台任务通过显式依赖记录日志
```

## 配置

```yaml
logger:
  level: "debug"
  format: "console"
  output: "logs/app.log"
  max_size: 100
  max_backups: 7
  max_age: 30
  compress: true
```

日志同时写入标准输出和轮转文件。配置决定级别、编码格式、输出路径、文件大小、保留数量、保留天数和压缩策略。

## 使用边界

- 运行日志用于启动、基础设施、HTTP 请求、后台任务和故障排查。
- HTTP 请求日志记录 method、path、status、latency、client IP 和 user agent。
- 不记录请求体、Authorization header、密码、验证码或 Token。
- 日志中不得暴露 MinIO 凭据、对象 key、数据库密码或 JWT Secret。
- 审计日志用于安全业务追踪，写入数据库；不能用运行日志替代审计日志。
- 模块需要日志能力时通过构造参数或最小 Contract 注入，不新增全局状态。

## Messaging 运行日志

Messaging 的 Application 层使用供应商无关的 `RuntimeLogger` Contract；`internal/app` 只在组合根将受控字段适配到 Zap。Outbox、事件 Consumer、RabbitMQ Consumer、DLQ Recorder 和 Consumer DLQ Replay 记录稳定事件名、阶段、事件或投影引用、失败分类和受控重试次数。

允许的运行字段仅包括 `consumer`、`event_id`、`failure_code`、`stage`、`retry_attempt`、`worker_id`、`outbox_id`、`dead_lettered`、`state`、`audience_observed_count`、`projection_id`、`replay_cycle`、`result` 和 `pending_count`。原始 Broker、数据库、Redis、MinIO、解析器和 SMTP 错误不得作为日志字段或消息写入。

Consumer DLQ 投影提交后，只有同一 `(consumer, failure_code)` 从零变为非零的 `pending` 才调用无返回值 `MessagingMetrics.RecordConsumerDLQPending`。该 Adapter 是 best-effort；异常只记录 `messaging_consumer_dlq_metrics_failed`，不阻塞投影提交、RabbitMQ ACK 或触发 AMQP 重试。

RabbitMQ 不可用时 `/api/ready` 只报告受控组件状态和 `last_error_code`；Consumer DLQ 数量、最旧年龄和处置动作不写入就绪响应。Outbox 死信和 Consumer DLQ 只能通过受保护的管理员入口或受限运维队列处置。

## 排查入口

1. 检查标准输出和 `logger.output` 指定文件。
2. 检查目录写权限和轮转参数。
3. 区分运行故障与审计事件；审计查询见 [`audit-logging.md`](audit-logging.md)。
