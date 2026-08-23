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

- Messaging → Identity：当前用户 Principal、启用状态和用户基础信息。
- Messaging → Organization：组织成员、组织树、组织及子组织范围、成员关系变化查询。
- Messaging → Authorization：当前用户的角色关系、权限校验和消息管理范围。
- Messaging → Files：消息图片专用上传、对象元数据和受消息可见性约束的读取能力。
- Messaging → Audit：写入受控消息操作元数据。
- App → Messaging：注入 Repository、TransactionRunner、RabbitMQ Publisher、Redis 和 Logger。

Contract 不暴露 GORM Model、Gin Context、SQL、Redis Client、RabbitMQ Channel 或 MinIO Client。

### 2. 消息存储与动态受众

建议新增以下 MySQL 表，最终字段以 Model 和迁移任务为准：

- `message_categories`：`organization_id`、编码、名称、排序、启用状态、时间；组织内编码唯一。
- `messages`：逻辑消息 ID、组织副本 ID、类型、发送者、分类、标题、清洗后的 HTML、草稿/发布/撤销/过期状态、发布时间和有效期。
- `message_audiences`：消息副本与组织、角色、全体受众的关系；受众类型和目标 ID 组合唯一。
- `message_recipients`：私信实际收件人关系，保存已读和当前用户删除墓碑；只用于私信。
- `message_user_states`：动态群发/公告的用户查看和已读状态；按消息副本、用户唯一。
- `message_outboxes`：事件 ID、事件类型/版本、聚合类型和 ID、组织/消息副本引用、最小事件载荷、重试次数、下次重试时间、发布时间和最后安全错误码。
- `message_event_consumptions`：本模块消费者的事件 ID 幂等记录，按消费者名称和事件 ID 唯一。

跨组织发送在一个最外层 MySQL 事务内为每个目标组织创建独立消息副本；按分类编码匹配。任一组织分类缺失或任一副本写入失败，整批回滚。

私信发送时校验双方共享至少一个组织；发送后组织关系变化不删除既有私信。群发和公告按当前组织成员/角色关系判断可见性；成员关系变化在下一次收件箱查询、未读查询或 WebSocket 恢复时生效。

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

用户收件箱和私信发送使用 `Authenticated`，管理员管理入口使用 `PermissionControlled`。超级管理员遵守现有 `admin` 角色兜底，但仍不得绕过消息状态、组织副本和审计约束。

### 4. HTTP 和 WebSocket 入口

初始 HTTP 路由：

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

WebSocket 升级入口：

- `GET /api/user/messages/ws?ticket=<short-lived-ticket>&cursor=<optional-cursor>`

WebSocket ticket 只允许使用一次，有效期 60 秒，服务端只存 ticket 摘要到 Redis，TTL 60 秒；访问日志、审计日志和错误日志必须对 `ticket` 查询参数脱敏。升级成功后，连接绑定用户身份，不接受 RabbitMQ 凭据或长期 JWT。

### 5. RabbitMQ 拓扑和 Outbox

RabbitMQ 拓扑：

- Topic Exchange：`admin.events.v1`，durable。
- WebSocket Queue：`admin.messaging.websocket`，durable quorum queue，绑定 `messaging.inbox.refresh`。
- Dead Letter Exchange：`admin.events.dlx`，持久化；死信队列按消费者和事件类型隔离。
- 邮件、短信队列本次不声明具体消费者；后续 Change 按同一 Exchange 和独立 Durable Queue 接入。

事件最小字段：`event_id`、`event_name`、`event_version`、`message_copy_id`、`organization_id`、`occurred_at`、`aggregate_version`。不得携带 Markdown、HTML、图片二进制、长期 Token 或外链 URL。

Outbox Worker 使用 Publisher Confirms；确认成功后标记 Outbox 已发布。RabbitMQ 不可用、Confirm 超时或连接中断时保留 Outbox，按有界指数退避重试。应用启动不因 RabbitMQ 暂时不可用而失败，但必须产生受控运行日志和健康状态。

WebSocket Consumer 完成本地幂等记录后 ACK，将事件写入 Redis Stream 作为 24 小时恢复缓存，并通过 Redis Pub/Sub 提醒所有在线 Gateway 节点。Gateway 从恢复缓存读取游标事件后，再按当前消息可见性查询 MySQL；客户端只收到刷新提示。超过 24 小时的游标返回“需要全量刷新”，不伪造补发。

### 6. 富文本、图片和外链

消息 Markdown 编译采用 `github.com/yuin/goldmark`，使用其默认安全渲染行为，不启用原始 HTML 的不安全渲染选项。Goldmark 负责 CommonMark/GFM 语法解析和 HTML 生成；`github.com/microcosm-cc/bluemonday` 负责对生成结果执行显式白名单清洗。系统只持久化清洗后的 HTML，不持久化原始 Markdown。标题最多 100 个 Unicode 字符，Markdown 正文最多 20,000 个 Unicode 字符；HTML 大小也必须受硬上限限制。

Goldmark 适合作为 Markdown 编译器，因为它是 Go 原生、可扩展且默认不渲染原始 HTML 和危险 URL；Bluemonday 提供 deny-by-default 的 HTML 白名单，作为第二层 XSS 防护。Markdown 清洗不替代外链代理的 SSRF 防护。

消息图片走独立消息图片用途，但复用 File Record 存储模型和安全验证能力。只允许 JPEG、PNG、WebP；单图最多 5 MiB，最长边 4,096 像素；完整解码后才写入私有对象存储。读取时必须重新检查消息当前可见性，撤销或删除后拒绝读取。

外链代理只允许 HTTPS；禁止解析到私网、环回、链路本地、云元数据地址和其他非公网地址；禁止自动跟随不受控重定向；限制连接超时、响应大小和最终 MIME；代理不永久缓存外部内容。代理请求不得把外链 URL 写入普通运行日志或审计记录。

### 7. 审计、日志和错误契约

消息发送、编辑、发布、撤销、分类变更、图片安全拒绝、外链代理拒绝和权限拒绝进入 Audit Log。只保存操作者、目标、组织、消息 ID/副本 ID、动作、结果和稳定原因码，不保存正文、HTML、图片内容、Token 或外链 URL。

HTTP 业务响应遵守现有 `code`、`error_code`、`msg`、`data` 四字段契约；需要新增稳定错误码时由 Messaging 统一拥有 `MSG_*` 前缀，并在 Route Catalog 中声明。WebSocket 升级前失败使用四字段错误信封，升级成功后保持原生 WebSocket 协议；流式连接中断不得追加 JSON 错误。

### 8. 验证策略

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
