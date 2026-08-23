## Context

Admin 已有 Identity、Authorization、Organization、Files、Audit、Route Catalog、API Metadata 和统一 HTTP 响应契约，但没有内部消息能力。需求包含三类消息：一对一私信、管理员群发和通知公告；群发和公告的受众按当前组织成员/角色关系动态计算，跨组织发送按组织创建独立副本。实时能力只需要向在线客户端提示“收件箱需要刷新”，消息正文和收件箱状态必须由 MySQL 保存。

RabbitMQ 是新增的基础设施选择，目标不仅是承载站内消息事件，也为后续邮件、短信等消费者提供统一事件出口。仓库目前没有 RabbitMQ 配置或 Go 客户端依赖；现有 Redis 仍可用于短期 WebSocket ticket、游标恢复缓存和在线节点间提示分发。

本 Change 遵循现有模块化架构：业务模块由 App 组合根装配，跨模块使用调用方定义的最小 Contract，HTTP 路由由 Route Catalog 声明并由 App 注册，业务 JSON 使用四字段响应信封。

## Goals / Non-Goals

**Goals:**

- 建立内部消息模块，覆盖私信、管理员群发、通知公告、组织级分类、收件箱、已读、撤销和动态受众。
- 通过 MySQL 保存消息事实，通过 Transactional Outbox 将业务事务与异步事件发布解耦。
- 通过 RabbitMQ Topic Exchange、独立 Durable Queue、Publisher Confirm、手动 ACK、重试和死信支持可扩展通知消费者。
- 实现 WebSocket Gateway、一次性 60 秒 ticket 和 24 小时事件游标恢复；事件只提示收件箱刷新，不携带正文。
- 为后续邮件、短信消费者保留稳定事件发布 Contract，但不实现其适配器。
- 保持普通文件接口拒绝图片，同时增加消息图片专用上传用途并服从消息可见性。
- 对 Markdown、HTML、图片和外链实施服务端安全策略。
- 让 RabbitMQ 故障只造成实时事件异步延迟，不阻止消息业务事务成功提交。

**Non-Goals:**

- 不实现邮件发送、短信发送、第三方供应商配置、模板或发送回执。
- 不把 RabbitMQ 用作消息正文、收件箱、已读或撤销状态数据库。
- 不允许浏览器直接连接 RabbitMQ Web STOMP、Web MQTT 或 AMQP WebSocket 入口。
- 不实现群聊会话、在线状态、Typing、移动端推送或通用文件附件。
- 不改变普通文件上传接口对图片的拒绝行为。
- 不在本 Change 创建前端 `web/` 页面；本 Change 的前端验证边界是后端协议和 WebSocket 可运行性验证。

## Decisions

### 1. 消息模块与跨模块 Contract

新增 `internal/messaging` 作为一级业务模块，拥有消息主体、分类、受众、私信收件关系、用户消息状态、Outbox 和事件消费幂等事实。模块内部按复杂度划分 Domain、Application、HTTP Adapter 和 Infrastructure Adapter；不导入其他模块 Adapter。

调用方 Contract：

- Messaging → Identity：当前用户 Principal、启用状态、用户基础信息，以及仅供进程内通知 Adapter 按用户 ID 解析当前渠道地址的最小 Contract。
- Messaging → Organization：组织成员、组织树、组织及子组织范围、成员关系变化查询。
- Messaging → Authorization：当前用户的角色关系、权限校验和消息管理范围。
- Messaging → Files：消息图片专用上传、对象元数据和受消息可见性约束的读取能力。
- Messaging → Audit：写入受控消息操作元数据。
- App → Messaging：注入 Repository、TransactionRunner、RabbitMQ Publisher、Redis、Logger 和调用方定义的 `MessagingMetrics` 观测 Contract。

Contract 不暴露 GORM Model、Gin Context、SQL、Redis Client、RabbitMQ Channel 或 MinIO Client。

### 2. 消息存储与动态受众

建议新增以下 MySQL 表，最终字段以 Model 和迁移任务为准：

- `message_categories`：`organization_id`、编码、名称、排序、启用状态、时间；组织内编码唯一。
- `messages`：逻辑消息 ID、组织副本 ID、类型、发送者、分类、标题、清洗后的 HTML、草稿/发布/撤销/过期状态、发布时间和有效期。
- `message_audiences`：消息副本与组织、角色、全体受众的关系；受众类型和目标 ID 组合唯一。
- `message_recipients`：私信实际收件人关系，保存已读和当前用户删除墓碑；只用于私信。
- `message_user_states`：动态群发/公告的用户查看和已读状态；按消息副本、用户唯一。
- `message_outboxes`：事件 ID、事件类型/版本、聚合类型和 ID、组织/消息副本引用、最小事件载荷、重试次数、下次重试时间、Worker ID、租约到期时间、发布时间和最后安全错误码；Worker 用短事务的 `FOR UPDATE SKIP LOCKED` 抢占当前副本下一版本并写入有限租约。
- `message_event_consumptions`：本模块消费者的事件 ID 幂等记录，按消费者名称和事件 ID 唯一；快照建立前使用 `snapshotting` 状态、Worker ID、30 秒可续租租约和单调递增 `snapshot_fence` 抢占，并发 Consumer 还按消息副本和聚合版本拒绝过时的处理结果。租约和围栏只保存在该记录：抢占及续租使用独立短 MySQL 事务，长 `REPEATABLE READ` 快照事务不得锁定该记录；快照状态变更和完成标记均须以 `snapshot_fence` 与未过期租约条件更新。续租失败或围栏不匹配时，当前 Consumer 停止处理并由重投递取得新围栏后接管。受众超过上限时不持久化用户快照，只记录稳定 `audience_capacity_exceeded` 失败码和本次观察到的受众数量；后续重试重新计算当前受众。
- `message_event_consumer_cursors`：Consumer 名称与消息副本唯一，保存持久化的已处理最大聚合版本；与事件消费记录在同一事务更新，以便并发 Consumer 将迟到事件（包括已处理更高版本后的 Consumer DLQ 旧版本重放）标记为 `superseded` 后安全 ACK，不产生新的 Redis Stream 刷新。
- `message_event_deliveries`：Consumer、事件 ID 与用户 ID 唯一，保存 Consumer 首次处理事件时不超过 100,000 用户的当前动态受众完整快照；重试仅使用该快照，不成为消息主体的静态受众。普通终态保留 24 小时；关联 Consumer DLQ 投影时保留至投影 30 天终态清理。受众容量超限事件绝不写入部分用户快照。
- `message_consumer_dead_letters`：由 DLQ Recorder 持久化的 Consumer、事件 ID、原队列、最小事件载荷、受控重试头、末次稳定失败码和观察到的受众数量、状态、`replay_cycle`、重放 Worker、30 秒重放租约、关联的完整受众快照、终态时间和处置审计引用；`(consumer_name, event_id)` 唯一，每次重放后的第五次失败复用原投影，从 `replayed` 重新打开为 `pending` 并递增 `replay_cycle`、更新末次失败事实，所有循环保留受审计处置历史。它是 Consumer DLQ HTTP 查询、重放和丢弃的事实来源，不依赖 RabbitMQ Management API 浏览队列。只有最终保持 `replayed` 或 `discarded` 的投影及关联快照保留 30 天后删除，`pending` 与 `replaying` 不按时间自动删除。

跨组织发送在一个最外层 MySQL 事务内为每个目标组织创建独立消息副本；按分类编码匹配。任一组织分类缺失或任一副本写入失败，整批回滚。同一用户同时属于多个目标组织时，系统 SHALL 将每个组织副本视为独立收件箱项、已读状态和 Outbox 事件；不得按逻辑消息去重。单一组织副本内命中多个受众规则的同一用户只保留一个用户状态和一次该副本事件投递。

私信发送时校验双方共享至少一个组织；发送后组织关系变化不删除既有私信。群发和公告按当前组织成员/角色关系判断可见性；成员关系变化在下一次收件箱查询、未读查询或 WebSocket 恢复时生效。用户跨多个目标组织的各副本分别计算当前可见性和未读状态。

为避免依赖组织成员加入时间：

- 私信使用实际收件关系，默认未读。
- 管理员群发首次进入可见范围的用户在首次收件箱/未读查询时懒创建未读状态。
- 公告对发布前历史内容在用户首次进入可见范围时自动视为已读；发布后的公告按未读处理。
- 群发和公告不允许收件人删除主体，只能标记已读。

### 3. 生命周期与权限

消息类型和状态：

- 私信：发送后不可编辑；发送者可撤销自己的私信；收件人可删除自己的收件关系。
- 管理员群发：发送后不可编辑；收件人不能删除；组织管理员可在所属组织及子组织范围内撤销普通用户消息；不能撤销管理员或超级管理员消息。
- 公告：`draft → scheduled/published → expired/revoked`；发布后允许编辑标题、正文、分类、受众和有效期；已撤销或已过期公告不可恢复为可见发布状态；收件人不能删除。

新增稳定权限码：

- `admin.messages.private.send`
- `admin.messages.broadcast.manage`
- `admin.messages.announcement.manage`
- `admin.messages.category.manage`
- `admin.messages.revoke.all`
- `admin.messages.outbox.replay`
- `admin.messages.dead-letter.manage`

用户收件箱和私信发送使用 `Authenticated`，管理员管理入口使用 `PermissionControlled`。超级管理员遵守现有 `admin` 角色兜底，但仍不得绕过消息状态、组织副本和审计约束。

### 4. HTTP 和 WebSocket 入口

初始 HTTP 路由：

技术入口：

- `GET /api/health`：既有公开存活检查，只表示进程存活。
- `GET /api/ready`：公开就绪检查；RabbitMQ 降级时仍返回 HTTP 200 与安全的 `degraded` 状态，消息写入继续可用。待处置 Consumer DLQ 不改变该端点的 HTTP 状态或 `status`，只经受保护的死信管理查询和监控观测。

用户入口：

- `POST /api/user/messages/private`
- `GET /api/user/messages`
- `GET /api/user/messages/unread-count`
- `GET /api/user/messages/:id`
- `PUT /api/user/messages/:id/read`
- `DELETE /api/user/messages/:id/inbox`
- `POST /api/user/messages/:id/revoke`
- `POST /api/user/messages/images`
- `POST /api/user/messages/ws-ticket`

管理员入口：

- `GET /api/admin/messages`
- `POST /api/admin/messages/broadcast`
- `POST /api/admin/messages/:id/revoke`
- `GET /api/admin/announcements`
- `POST /api/admin/announcements`
- `GET /api/admin/announcements/:id`
- `PUT /api/admin/announcements/:id`
- `POST /api/admin/announcements/:id/publish`
- `POST /api/admin/announcements/:id/revoke`
- `GET /api/admin/message-categories`
- `POST /api/admin/message-categories`
- `PUT /api/admin/message-categories/:id`
- `DELETE /api/admin/message-categories/:id`
- `POST /api/admin/messages/images`
- `GET /api/admin/message-outboxes`
- `POST /api/admin/message-outboxes/:id/replay`
- `GET /api/admin/message-dead-letters`
- `POST /api/admin/message-dead-letters/:id/replay`
- `DELETE /api/admin/message-dead-letters/:id`

WebSocket 升级入口：

- `GET /api/user/messages/ws?ticket=<short-lived-ticket>&cursor=<optional-cursor>`

WebSocket ticket 只允许使用一次，有效期 60 秒，服务端只存 ticket 摘要到 Redis，TTL 60 秒；访问日志、审计日志和错误日志必须对 `ticket` 查询参数脱敏。升级成功后，连接绑定用户身份，不接受 RabbitMQ 凭据或长期 JWT。

### 5. RabbitMQ 拓扑和 Outbox

RabbitMQ 拓扑：

- Topic Exchange：`admin.events.v1`，durable。每个消息生命周期事件使用 `messaging.message.<action>.v1` Routing Key；`<action>` 固定为 `created`、`published`、`edited`、`revoked` 或 `expired`。
- WebSocket Queue：`admin.messaging.websocket`，durable quorum queue，绑定全部 `messaging.message.*.v1` 生命周期事件；每个 Consumer 使用独立的 durable quorum TTL 重试队列，依次等待 `1s`、`2s`、`4s`、`8s`、`16s` 后再返回主队列，避免直接重投递形成热循环。应用实例作为竞争 Consumer，以 `prefetch=1` 消费；Consumer 在同一事务更新持久化版本游标与事件消费记录，迟到事件标记 `superseded` 后 ACK。
- Dead Letter Exchange：`admin.events.dlx`，持久化；Consumer 第五次处理失败后进入按 Consumer 与事件类型隔离的 DLX 队列并保留 7 天。每个 Consumer DLQ 配有竞争的 DLQ Recorder，Recorder 将最小事件载荷和受控头持久化至 MySQL 后才 ACK；写入失败时不 ACK，并经独立 TTL 重试后进入不可自动丢弃的运维告警队列。告警队列仅能通过受限运维 CLI 或 RabbitMQ 管理界面处置，标准重放将消息重置受控重试头并定向返回原 DLQ Recorder，不经公共 Topic Exchange。Consumer DLQ HTTP 重放通过 durable direct exchange `admin.events.replay`，按 `consumer.<consumer-name>` 定向返回原 Consumer，不经 `admin.events.v1` 广播。Publisher 无法连接或未获 Confirm 时不能假定 DLX 可用。
- 邮件、短信队列本次不声明具体消费者；未来 Consumer 作为 App 注入的进程内 Adapter 调用 Messaging Projection Contract，以最小事件引用查询当前可发送且未读的用户 ID、显示名、标题、清洗 HTML、服务端派生纯文本、消息副本 ID 和事件版本。Adapter 通过 Identity Contract 仅解析当前已验证渠道地址，不从 RabbitMQ 事件读取正文、邮箱或手机号。若 RabbitMQ 延迟、重试或重放时目标用户已在站内读取消息，Projection 不返回该用户，Adapter 不得再发起外部通知；未读资格只在 Projection 查询时判定，查询完成后才发生的已读不会撤回已在途的 SMTP/短信发送。本 Change 不增加外部投递预约、发送状态或取消机制。用户存在 `pending_email` 时，当前已验证 `email` 继续是唯一可投递邮箱；候选地址确认并原子提升后，后续查询才返回新邮箱。只有私信 `created` 以及管理员群发/公告 `published` 事件允许触发外部通知。

事件最小字段：`event_id`、`event_name`、`event_version`、`message_copy_id`、`organization_id`、`occurred_at`、`aggregate_version`。不得携带 Markdown、HTML、图片二进制、长期 Token 或外链 URL。

Outbox Worker 使用 Publisher Confirms；AMQP 连接超时为 5 秒、Confirm 超时为 10 秒、Worker 租约为 30 秒。Worker 用短 MySQL 事务以 `FOR UPDATE SKIP LOCKED` 仅抢占当前 `message_copy_id` 的下一聚合版本，并写入 Worker ID 与有限租约；等待 Confirm 时可以续租，崩溃或超时后其他 Worker 可以接管。确认成功后标记 Outbox 已发布。连接失败、Confirm 超时或 Broker 拒绝时按 `1s`、`2s`、`4s`、`8s`、`16s` 重试；第五次失败后将 Outbox 标记为 `dead` 并记录稳定错误码。管理员只能通过受审计的重放操作把死信 Outbox 恢复为待发布；不得将 Publisher 失败直接写入 RabbitMQ DLX。应用启动不因 RabbitMQ 暂时不可用而失败，但必须产生受控运行日志和健康状态。

同一 `message_copy_id` 的事件按单调 `aggregate_version` 严格发布。前一版本尚未发布或已进入 `dead` 时，Worker 不得发布后一版本；重放前一版本后才能继续该副本的后续事件。

WebSocket Consumer 收到未过时事件时，先在 `message_event_consumptions` 中以独立短 MySQL 事务使用 `snapshotting` 状态、30 秒可续租租约和单调递增 `snapshot_fence` 抢占；续租同样使用独立短事务。持有有效围栏的 Consumer 在不锁定该消费记录的单个 `REPEATABLE READ` MySQL 事务中固定首次消费时的读视图，并以每批最多 500 用户持久化动态受众至 `message_event_deliveries`。快照状态变更及完成提交必须同时匹配该围栏和未过期租约。构建阶段续租失败或发现围栏不匹配时，当前 Consumer 必须回滚未完成事务，不写 Redis Stream、不 ACK；重投递取得新围栏后接管。已完整提交快照后的 Consumer 失租时不得继续任何新的 Redis 写入或 ACK，并由新围栏 Consumer 复用该快照；已发生的 Lua 幂等写入由接管者补齐。单事件快照上限为 100,000 用户。首次读视图解析到超过上限时，消息业务事实保持已提交；Consumer 只在 `message_event_consumptions` 记录稳定 `audience_capacity_exceeded` 失败码和观察到的数量，不持久化任何用户快照、不写 Redis Stream 或 ACK，并按既定五级重试。该异常的每次受控重试及最终 Consumer DLQ 人工重放都重新计算当时受众，是“重放复用原受众快照”的唯一例外；绝不创建部分投递。全量快照封存并提交前不得写入 Redis Stream 或 ACK。它只按封存快照向每个用户写入 Redis Stream；每次写入通过 Lua 原子检查 24 小时去重键、XADD Stream 并设置去重 TTL。全部用户 Stream 成功后，Consumer 才在 MySQL 持久化事件消费记录和版本游标，再 ACK。崩溃、租约到期或 Redis 部分失败后的重投递重复使用封存快照；Lua 脚本抑制已写用户并补齐缺失用户，避免漏写或重复刷新。普通终态快照保留 24 小时；关联 Consumer DLQ 投影的完整快照保留至投影 30 天终态清理。随后通过 Redis Pub/Sub 提醒在线 Gateway。Gateway 从用户的恢复缓存读取游标事件后，仍须按当前消息可见性查询 MySQL；客户端只收到刷新提示。成员后续变化由收件箱查询和 Gateway 可见性复查处理，不补发旧事件。超过 24 小时的游标返回“需要全量刷新”，不伪造补发。

死信 Outbox 查询与重放仅对超级管理员开放，使用 `GET /api/admin/message-outboxes` 和受 `admin.messages.outbox.replay` 保护的 `POST /api/admin/message-outboxes/:id/replay`。重放必须审计且只恢复原事件身份与版本。`GET /api/health` 继续只表示进程存活；`GET /api/ready` 公开返回 HTTP 200 和 `{status, components.rabbitmq.status, components.rabbitmq.outbox_pending, components.rabbitmq.last_error_code}`。RabbitMQ 不可用时 `status` 和 RabbitMQ 组件状态为 `degraded`，但消息写入继续可用。

`/api/ready` 不承担 Consumer DLQ 告警职责，响应不得加入死信数量、失败码或最旧死信年龄。Messaging 通过 App 注入的供应商无关 `MessagingMetrics` Contract 与仅超级管理员可访问的 Consumer DLQ 查询暴露待处置总数、按 Consumer/稳定失败码的计数和最旧 `pending` 投影年龄；Contract 只定义无返回值的 `RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)` 方法，`ConsumerDLQPendingObservation` 恰含 `consumer_name`、稳定 `failure_code`、`pending_count` 和 `oldest_pending_age`。任一 `(Consumer, 稳定失败码)` 的 `pending` 数量从零变为非零且 MySQL 投影提交后，Messaging 以 best-effort 调用该方法记录立即告警观测。Adapter 自身异常只写受控运行日志，不阻止 DLQ Recorder ACK 或触发 AMQP 重试；该方法不向 Messaging 返回可传播错误。部署监控系统负责采集并执行实际告警规则；本 Change 不引入 Prometheus、OTel、Alertmanager、Webhook、SMTP 或任何通用告警发送器。观测只携带受控维度、计数和最旧年龄，不含事件载荷、用户标识或消息内容；这些待处置记录不影响应用接收 HTTP 流量的就绪判断。

Consumer DLQ 消息仅由超级管理员管理，使用 `admin.messages.dead-letter.manage` 保护的查询、逐条重放和丢弃 HTTP 入口。HTTP 从 MySQL `message_consumer_dead_letters` 分页读取受控元数据，不直接浏览 RabbitMQ 队列。重放请求以 `pending → replaying → replayed|pending` 与 30 秒租约抢占投影；Publisher Confirm 成功后标记 `replayed`，失败或租约到期恢复 `pending`。`replayed` 仅表示事件已定向返回原 Consumer，不保证会写入新的刷新事件。重放经 durable direct exchange `admin.events.replay`，使用记录的 `consumer.<consumer-name>` routing key 定向返回原 Consumer，并复用关联的完整受众快照；`audience_capacity_exceeded` 投影没有用户快照，重放时必须重新计算当前动态受众。重放事件再次经历第五次 Consumer 失败时，DLQ Recorder 必须以 `(consumer_name, event_id)` 复用同一投影，将 `replayed → pending`、递增 `replay_cycle` 并更新末次失败事实；每轮处置沿用现有 Audit Log，不创建重复管理记录。若该重放事件的 `aggregate_version` 低于持久化 Consumer 游标，Consumer 必须标记 `superseded`、不写 Redis Stream 后 ACK；更高版本已成功处理时不得让旧版本重放倒置刷新顺序。崩溃发生在 Confirm 后但状态更新前可能重复投递，Consumer 必须按事件 ID 幂等。只有最后保持 `replayed` 或 `discarded` 的投影及关联完整快照保留 30 天后物理删除；丢弃不修改消息业务事实。

### 6. 身份邮箱验证

Identity 沿用现有 Gin `email` 格式校验和数据库唯一性语义；验证模块不得重写邮箱本地部分、大小写或提供商别名。Identity 拥有 `email`、`pending_email` 和 `email_verified_at`。注册用户以未验证邮箱创建，但仍可登录；未验证邮箱不能作为外部通知渠道。当前 `email` 未验证且不存在 `pending_email` 的用户本人更换邮箱时，系统直接原子替换 `email`、保持 `email_verified_at` 为空、使旧目标邮箱的全部有效验证凭据失效，并为新 `email` 自动签发凭据及同步尝试 SMTP 投递；不得错误创建没有已验证现用邮箱支撑的 `pending_email`。用户本人更换已有已验证 `email` 时，现有已验证 `email` 保持有效，候选地址写入唯一 `pending_email`；已有 `pending_email` 时可用新的未占用候选地址原子替换，且仅在替换成功后使旧候选地址的全部有效凭据失效并为新候选地址签发凭据及同步尝试 SMTP 投递。新候选地址冲突时返回 HTTP 409，不改变原 `pending_email` 或其有效凭据。候选地址确认与替换必须通过同一用户的事务锁串行裁决：先提交的状态转换生效；替换先提交时旧 token 失效且随后确认失败，确认先提交时候选地址升格，随后更新只能基于新已验证 `email` 创建新的 `pending_email`。在候选地址确认前，外部通知继续且只投递既有已验证 `email`，不得向 `pending_email` 投递。只有目标用户完成验证，系统才在同一事务中原子提升候选地址、清空 `pending_email` 并写入 `email_verified_at`；此后外部通知渠道解析才切换到新邮箱。迁移既有用户时所有既有邮箱均标记为未验证。用户资料、注册和更新成功响应返回 `email`、`pending_email` 与 `email_verified`，不返回 token、过期时间或 SMTP 状态；确认成功同样返回更新后的安全用户资料。
邮箱验证凭据持久化记录包含用户、目标邮箱、token 摘要、签发时间、过期时间、终态和终态时间；成功、失效或过期后保留受控元数据 24 小时，再由 Identity 后台任务物理删除。原始 token 永不持久化。

验证凭据为 32 字节随机 Base64URL token，数据库只保存 SHA-256 摘要。token 绑定用户和目标邮箱，一次性使用、有效期 15 分钟；成功、过期、被新 token 替换或发送失败后均不可复用。注册、未验证当前邮箱直接替换、`pending_email` 创建/替换及认证用户显式签发/重发的每次签发尝试都计入同一用户滚动一小时最多三次的额度，SMTP 发送失败也计入次数。额度耗尽时系统在任何邮箱状态或凭据变更前返回 HTTP 429、稳定 Identity 节流错误和 `Retry-After` 秒数，且不返回邮箱、计数或 token；不得签发凭据、尝试 SMTP 投递、替换 `email` 或创建/替换 `pending_email`。用户通过已认证的 `POST /api/user/email-verifications` 签发或重发，并将验证邮件中原样提供的 token 提交至已认证的 `POST /api/user/email-verifications/confirm`；禁止 GET 链接改变验证状态或承担确认逻辑。
用户确认时若 `pending_email` 已被另一用户占用，系统返回 HTTP 409、保留 `pending_email` 并使当前 token 失效；用户必须提交新的候选邮箱。当前 `email` 已验证且不存在 `pending_email` 时，请求签发/重发返回 HTTP 409，不生成 token 或发送邮件。

注册、未验证当前邮箱直接替换或 `pending_email` 创建/替换后的自动投递失败时，用户记录保留、直接替换后的新 `email` 或既有已验证 `email` 与新的 `pending_email` 保留、刚签发 token 失效、请求返回仅含稳定错误码与安全提示的 HTTP 503 Identity 错误；不得伪造成功、暴露 `account_created`、用户 ID、邮箱或 token，也不得补偿删除账户、回滚邮箱替换或回滚候选邮箱。用户可登录并在节流允许后重发验证邮件。

管理员不得修改用户邮箱；现有管理员用户更新接口提交邮箱字段时 SHALL 拒绝。用户本人通过 `PUT /api/user/info` 提交非空 `email` 时必须同时提交 `current_password`；系统在创建/替换 `pending_email` 或直接替换未验证 `email` 前比较当前密码。缺失或不匹配时返回稳定 `IDENTITY_CURRENT_PASSWORD_INVALID` 字段错误，不改变 `email`、`pending_email` 或 `email_verified_at`，不签发凭据且不尝试 SMTP 投递。用户本人通过验证后的邮箱修改规则为：当前 `email` 已验证则创建或原子替换 `pending_email`，不覆盖既有已验证地址；当前 `email` 未验证且没有 `pending_email` 则按前述规则直接替换 `email`。SMTP 配置包含 `host`、`port`、`username_env`、`password_env`、`from_env`、`tls_mode` 与 `timeout_seconds`；`tls_mode` 仅允许 `disabled`（仅本地测试）、`starttls_required`（生产默认）和 `implicit`（SMTPS），不允许 opportunistic TLS 降级。
SMTP `timeout_seconds` 默认 10 秒且可显式覆盖；该超时约束建连、TLS、认证和发送阶段的连接 deadline。

验证邮件通过 `github.com/wneessen/go-mail` 的供应商无关 SMTP Adapter 同步发送。认证使用 SMTP 用户名加密码、应用专用密码或服务商 SMTP Token，不实现 OAuth2/XOAUTH2 Token 生命周期。SMTP 成功接收邮件前请求不得返回成功；发送失败时系统使新 token 失效、返回安全错误并允许用户在节流窗口允许后重试。Identity 不在本 Change 实现通用邮件、短信、模板、投递回执、手机号验证或用户通知偏好。

验证请求、发送成功或失败、确认成功或拒绝及节流拒绝都写入受控审计；日志和错误响应不得包含邮箱地址、token、token 摘要、SMTP 用户名、密码、服务器地址或 SMTP 原始错误。
### 7. 富文本、图片和外链

消息 Markdown 编译采用 `github.com/yuin/goldmark`，使用其默认安全渲染行为，不启用原始 HTML 的不安全渲染选项。Goldmark 负责 CommonMark/GFM 语法解析和 HTML 生成；`github.com/microcosm-cc/bluemonday` 负责对生成结果执行显式白名单清洗。系统只持久化清洗后的 HTML，不持久化原始 Markdown。标题最多 100 个 Unicode 字符，Markdown 正文最多 20,000 个 Unicode 字符；HTML 大小也必须受硬上限限制。

Goldmark 适合作为 Markdown 编译器，因为它是 Go 原生、可扩展且默认不渲染原始 HTML 和危险 URL；Bluemonday 提供 deny-by-default 的 HTML 白名单，作为第二层 XSS 防护。Markdown 清洗不替代外链代理的 SSRF 防护。

消息图片走独立消息图片用途，但复用 File Record 存储模型和安全验证能力。只允许 JPEG、PNG、WebP；单图最多 5 MiB，最长边 4,096 像素；完整解码后才写入私有对象存储。读取时必须重新检查消息当前可见性，撤销或删除后拒绝读取。

外链代理只允许 HTTPS；禁止解析到私网、环回、链路本地、云元数据地址和其他非公网地址；禁止自动跟随不受控重定向；限制连接超时、响应大小和最终 MIME；代理不永久缓存外部内容。代理请求不得把外链 URL 写入普通运行日志或审计记录。

### 8. 审计、日志和错误契约

消息发送、编辑、发布、撤销、分类变更、图片安全拒绝、外链代理拒绝和权限拒绝进入 Audit Log。只保存操作者、目标、组织、消息 ID/副本 ID、动作、结果和稳定原因码，不保存正文、HTML、图片内容、Token 或外链 URL。

HTTP 业务响应遵守现有 `code`、`error_code`、`msg`、`data` 四字段契约；需要新增稳定错误码时由 Messaging 统一拥有 `MSG_*` 前缀，并在 Route Catalog 中声明。WebSocket 升级前失败使用四字段错误信封，升级成功后保持原生 WebSocket 协议；流式连接中断不得追加 JSON 错误。

### 9. 验证策略

- Domain/Application：状态机、权限矩阵、动态受众、分类编码匹配、跨组织事务回滚。
- Repository：MySQL 约束、分页、唯一键、Outbox 抢占/并发、幂等记录。
- RabbitMQ 集成：Topic 路由、Publisher Confirm、重连补发、手动 ACK、重试和死信；使用隔离 vhost。
- HTTP：所有新增 Route Descriptor 与 Gin 路由一致，状态码、错误码和 OpenAPI 一致。
- WebSocket：ticket 一次性使用、过期拒绝、游标恢复、超过窗口全量刷新、断线不丢业务消息。
- 安全：Markdown XSS、危险协议、图片格式/尺寸/大小、SSRF 地址、重定向、MIME 和日志脱敏。
- 端到端：消息事务成功但 RabbitMQ 不可用时 Outbox 保留；RabbitMQ 恢复后只产生一次可见刷新效果。

## Risks / Trade-offs

- [RabbitMQ 不可用] → MySQL 消息事务继续成功，Outbox 保留并重试；实时推送明确显示异步延迟，增加 Broker 健康指标和死信告警。
- [至少一次投递导致重复刷新] → 事件 ID、消费者幂等记录和客户端刷新幂等；刷新提示重复不改变消息事实。
- [Topic Exchange 路由或队列声明漂移] → App 启动声明并校验固定拓扑，集成测试覆盖 Exchange、Binding、Queue 参数和死信策略。
- [动态受众查询成本增长] → 受众目标使用索引，收件箱查询先筛选组织/角色关系再分页；后续规模增长可增加只读索引或缓存，但不改变事实模型。
- [发布后编辑与已读冲突] → 每次编辑增加消息版本/更新时间；已读状态按消息副本保留，客户端收到刷新提示后重新读取详情。
- [消息图片误入普通文件接口] → 用途字段和入口双重校验，普通文件 Route 继续拒绝图片，并增加架构/HTTP 测试。
- [Markdown/外链引入 XSS 或 SSRF] → 只持久化清洗 HTML；公网 HTTPS、无不受控重定向、超时/大小/MIME 限制和安全测试。
- [WebSocket Gateway 集群事件分发不一致] → RabbitMQ 单独消费写 Redis Stream，Redis Pub/Sub 做在线提示，断线恢复统一读取 Redis Stream 并回查 MySQL 可见性。
- [RabbitMQ Cluster 运维复杂度] → 开发环境单节点，生产环境使用标准 Cluster/Quorum Queue；连接、拓扑和健康检查通过 Platform 统一装配。

## Migration Plan

1. 增加配置和依赖，但先保持新增消息路由未注册；确认 RabbitMQ vhost、凭据、Exchange/Queue 权限和本地单节点开发实例。
2. 执行数据库 AutoMigrate/迁移，创建消息、Outbox、幂等和消息图片用途字段；不修改既有消息数据（当前不存在）。
3. 部署内部消息模块、RabbitMQ 拓扑声明、Outbox Worker、WebSocket Consumer 和 Gateway。
4. 先启用私信和收件箱，再启用管理员群发、公告、分类和撤销入口；每阶段运行对应契约与集成测试。
5. 观察 Outbox backlog、发布失败、消费者重试、死信、WebSocket 在线数和恢复失败指标。
6. 回滚时停止消息写入和 Worker，保留已提交消息事实及 Outbox；撤销新增路由和消费者后，不删除已持久化消息或审计数据。RabbitMQ 队列消息只在确认不再需要重放后清理。

## Open Questions

- 生产 RabbitMQ Cluster 的节点数量、镜像/持久卷、TLS、凭据轮换和监控平台需要在部署 Change 中确定。
- 消息图片复用 File Record 时，现有 Files 模块需要提供的最小用途 Contract、对象生命周期和删除责任需要在实现前与当前文件模型核对。
- 组织成员和角色变更是否需要发布专用事件，目前选择在收件箱查询/未读查询/重连时重新计算，不把组织变更实时推送作为本 Change 的硬依赖。
- RabbitMQ 重试次数、退避上限、死信保留时间和人工重放流程需要结合运维标准确定；业务规格只固定“有界重试 + 死信 + 可观测”。
