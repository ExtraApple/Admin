# Messaging 与 DLQ Recorder 运维

> 本文描述当前 RabbitMQ Messaging 的部署检查和 Consumer DLQ Recorder 处置边界。运行日志字段约束见 [`runtime-logging.md`](runtime-logging.md)。

## 组件与队列

App 启用 RabbitMQ 配置后由组合根声明 Topic Exchange、Quorum Queue、Publisher Confirm 和后台 Consumer。WebSocket Consumer 名称为 `websocket`。

| 用途 | 名称 |
|---|---|
| 事件 Exchange | `admin.events.v1` |
| 重试 Exchange | `admin.events.retry` |
| 死信 Exchange | `admin.events.dlx` |
| Consumer Queue | `admin.messaging.websocket` |
| Consumer DLQ | `admin.messaging.websocket.dlq` |
| Consumer DLQ Recorder Queue | `admin.messaging.websocket.dlq.recorder.2` ～ `.5` |
| Recorder 重试 Queue | `admin.messaging.websocket.dlq.recorder.retry.1` ～ `.5` |
| Recorder 告警 Queue | `admin.messaging.websocket.dlq.recorder.alert` |

主消费队列和重试/死信队列均为 durable Quorum Queue。事件发布使用 Publisher Confirm；Outbox 发布失败按 `1s`、`2s`、`4s`、`8s`、`16s` 受控重试。

## 消息长度上限

标题与正文的 Unicode 长度上限来自配置，由组合根在启动时转换为 Messaging 领域的
`ContentLimits`；领域层不读取配置文件。

| 配置项 | 默认值 | 允许范围 | 生效位置 |
|---|---|---|---|
| `messaging.max_title_runes` | 100 | 1 ～ 1000 | 标题的 Unicode 字符数 |
| `messaging.max_body_runes` | 20000 | 1 ～ 100000 | Markdown 正文的 Unicode 字符数 |

- 计数单位是 Unicode 字符（rune），不是字节；多字节字符按 1 个字符计。
- 值为 `0` 或键缺失表示"未配置"，取默认值；负数或超出上界会在**配置加载阶段**直接失败，
  应用不会以静默默认值启动。
- 上限在**启动时读取一次**，修改配置需要重启应用，不支持热更新。
- **调小上限的影响**：编辑既有公告时会按新上限重新校验正文
  （`EditAnnouncement`）。若新上限低于既有消息长度，该消息将无法再被编辑或重新发布，
  只能先撤销后重建。调小前请先评估库中既有消息的长度分布。
- 清洗后 HTML 的 128 KiB 上限与上面两项无关，**不可通过配置调整**；
  超限时返回 `MSG_HTML_TOO_LARGE`。

## 健康与就绪

- `GET /api/health` 只表示进程存活。
- `GET /api/ready` 只返回应用状态、RabbitMQ 受控状态、Outbox pending 数量和 `last_error_code`。
- Consumer DLQ pending 数量、最旧年龄和告警状态不加入 `/api/ready`，通过注入的 `MessagingMetrics.RecordConsumerDLQPending` 交给部署监控。
- `admin.messages.outbox.replay` 只允许超级管理员重放死信 Outbox。
- `admin.messages.dead-letter.manage` 只允许超级管理员查询、重放或丢弃 Consumer DLQ 投影。

## 日常检查

1. 检查 App 运行日志中的 `messaging_outbox_*`、`messaging_rabbitmq_*` 和 `messaging_consumer_dlq_*` 稳定事件。
2. 检查 RabbitMQ Management UI 的队列深度、Ready/Unacked 数量、消费者数和告警队列积压。
3. 看到 Outbox pending 增长时先确认 Broker 连接和 Confirm，不直接删除 Outbox 记录。
4. 看到 Consumer DLQ pending 增长时确认 Recorder 是否仍能写入 MySQL；不要通过重新入主队列绕过受控重试。
5. RabbitMQ 不可用时保留 MySQL 事实和 Outbox，恢复 Broker 后由 Worker 补发。

## DLQ Recorder 告警队列处置

Recorder 写入 Consumer DLQ 投影失败时，消息保持未 ACK，由 Broker 按 Recorder 自身 TTL 重试链处理。达到最后一次重试后消息进入 `admin.messaging.websocket.dlq.recorder.alert`，该队列不可自动丢弃。

### RabbitMQ Management UI

1. 以受限运维账号打开 **Queues and Streams**。
2. 选择对应 `*.dlq.recorder.alert` 队列，确认消息数量和最后投递时间。
3. 先查看应用日志中的 `consumer`、`event_id`、`failure_code`、`retry_attempt`；不要从消息正文、HTTP 日志或审计日志复制正文和凭据。
4. 修复 MySQL、连接或权限问题后，使用管理界面的 **Get messages** 进行小批量人工重投；每次只处理已确认的消息，并观察 Recorder Queue 和 MySQL 投影是否恢复。
5. 无法安全重投时，将消息导出到受限的合规存档后再使用 **Purge**；记录队列、事件 ID、动作、结果和稳定原因码到部署运行日志。
6. 处置结束后确认告警队列为零、投影记录存在且 RabbitMQ Consumer 已重新 ACK。应用 HTTP 审计不得伪造该运维动作。

管理界面的 Get、Requeue 和 Purge 权限必须独立授予；生产环境禁止向普通管理员开放告警队列正文读取。

## Consumer DLQ 投影处置

优先使用受保护的 HTTP 入口，不直接浏览 RabbitMQ 队列：

- `GET /api/admin/message-dead-letters`
- `POST /api/admin/message-dead-letters/:id/replay`
- `DELETE /api/admin/message-dead-letters/:id`

重放持有 30 秒投影租约，Confirm 成功后标记 `replayed`；发布失败或租约失效返回 `pending`。同一事件再次第五次失败时复用原投影并递增 `replay_cycle`，不创建重复投影。旧聚合版本会被 Consumer 标记为 `superseded`，不会倒置刷新。

处置请求只返回受控元数据。正文、HTML、图片、外链 URL、Token、凭据和 Broker 原始错误不得出现在 HTTP 响应、审计记录或运行日志。
