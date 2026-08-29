# 内部消息采用动态受众与消息主体复制

## 状态

已接受（需求层决策；HTTP、WebSocket 和数据结构由后续 OpenSpec Change 细化）。

## 背景

Admin 现有 OpenSpec 没有内部消息规格，`TODO.md` 只列出发送、收件箱、分类、已读、撤销和收件箱删除。组织成员和数据范围已经存在，消息需求同时包含一对一私信、管理员群发、通知公告、组织级分类和实时推送。

如果把收件人永久展开为静态列表，组织成员变化会导致历史消息与当前受众不一致；如果把跨组织消息只保存为一个共享主体，分类、撤销、组织权限和查询范围会互相污染。消息正文还需要富文本、图片和外链安全策略，不能直接复用普通文件或原始 HTML 语义。

## 决策

### 1. 内部消息作为独立业务能力

内部消息拥有消息主体、消息分类、受众、生命周期、收件箱状态和推送事件的业务所有权。Identity 只提供当前用户身份，Organization 提供组织成员事实，Authorization 提供权限与资源范围，Audit 只记录受控操作元数据。

内部消息不进入用户、组织或审计模块的模型和服务；跨模块调用使用调用方定义的最小 Contract。

### 2. 私信发送时校验同组织，群发和公告使用动态受众

私信发送时要求发送者与收件人属于同一组织。群发和公告的受众可以是组织、角色或全体用户，收件箱可见性按当前组织成员和角色关系计算。

受众变化会影响历史消息可见范围。消息内容本身不因成员关系变化而复制成新的消息主体。

### 3. 跨组织发送创建独立副本

超级管理员跨组织发送时，每个目标组织创建独立消息副本。副本分别绑定组织级分类，并按分类编码匹配；任一目标组织缺少分类或写入失败时，整批操作回滚，不产生部分副本。

独立副本使组织级查询、分类管理、撤销和数据范围保持局部一致，也避免跨组织消息共享一个可变的组织归属。

### 4. 生命周期区分私信、群发和公告

私信与管理员群发发送后不可编辑。通知公告支持草稿、发布、定时发布、有效期、发布后编辑和撤销。撤销保留消息记录与撤销标记，但正文不再按正常消息阅读。

收件人只允许删除私信的当前收件关系；管理员群发和公告只能标记已读，不能由收件人删除共享消息主体。私信的实际收件人计为未读，管理员群发对新进入受众的用户计为未读。组织成员关系保存并保持稳定的加入时间：公告首次可见时，成员在公告发布后才加入组织则初始化为已读；其他公告初始化为未读。组织成员或角色关系变更后，群发和公告在用户下一次收件箱、未读查询或 WebSocket 恢复时重新计算可见性；成员变化本身不补发历史消息刷新事件。

组织管理员的管理范围为其所属组织及子组织；组织管理员可以撤销这些范围内普通用户发送的全部消息类型，但不能撤销管理员或超级管理员发送的消息。普通用户可以撤销自己发送的私信。公告只能由其发布者撤销自己的公告，或由超级管理员跨组织撤销；组织管理员不得仅凭组织范围撤销其他管理员发布的公告。

消息分类包含组织内唯一编码、名称、排序和启用状态；已被消息引用的分类只能停用，不能删除。跨组织副本按分类编码匹配。组织不预置默认消息分类，必须显式创建分类后才能创建或发布引用该分类的消息。

公告创建始终先进入 `draft`；立即发布和定时发布都通过显式发布动作完成，避免创建请求意外广播。撤销消息保留受控占位及状态元数据，但收件人不得读取正文、图片或外链。

消息操作使用按能力拆分的权限码，至少区分私信发送、组织消息管理、公告管理、分类管理和全局撤销；超级管理员继续沿用既有 `admin` 角色兜底语义。

### 5. WebSocket 只作为可恢复的收件箱刷新通道

实时推送采用 WebSocket。连接通过一次性短期 ticket 鉴权，不把长期 Bearer JWT 放入 URL。ticket 只允许使用一次，有效期为 60 秒。消息和收件箱持久化事实不依赖连接是否在线。

事件使用游标支持断线恢复；事件游标保留 24 小时，重连客户端携带最后游标，服务端按当前用户可见范围补发“收件箱需要刷新”事件，不在事件中重复发送富文本正文。超出恢复窗口后客户端必须重新拉取收件箱。事件补发必须幂等，不能绕过消息撤销、收件箱删除和权限校验。

Gateway 对每条连接使用有界单写入队列；队列满或单次写入超过 5 秒即关闭连接。客户端通过游标重连恢复或全量查询收件箱，慢连接不得无限占用内存或阻塞同一用户的其他连接。

### 6. 富文本先转换和清洗，再持久化或展示

消息输入采用 Markdown，服务端转换为受限 HTML。标题最多 100 个 Unicode 字符，Markdown 正文最多 20,000 个 Unicode 字符，清洗后的 HTML 最多 128 KiB UTF-8 字节；脚本、事件属性和危险协议必须被拒绝或清洗。消息图片只允许 JPEG、PNG 和 WebP，单图不超过 5 MiB，最长边不超过 4,096 像素，服务端必须完整解码并保存验证结果。

消息图片复用 File Record 数据模型，但使用独立的消息图片上传用途；普通文件接口继续拒绝图片。图片上传先创建由上传者持有、15 分钟内必须绑定的临时媒体记录；创建、编辑或发布消息时，在共享事务中将图片绑定到 `Message.LogicalID`，过期未绑定记录及对象由 Files 清理。图片访问必须服从消息当前可见性。

外链和图片由服务端按请求代理，不永久缓存外部内容。代理只允许 HTTPS，解析后的目标必须是公网地址；禁止私网、环回、链路本地和云元数据地址，禁止不受控重定向，并限制连接时间、响应大小和响应 MIME。

当前 `file-management` 规格明确拒绝普通文件图片，因此复用记录模型需要在 OpenSpec Change 中同步修改文件用途、图片验证、代理访问、权限联动、审计和清理边界；不得直接绕过现有文件安全策略。

消息发送、编辑、发布、撤销、分类变更和权限拒绝进入现有 Audit Log，但只记录操作者、目标、组织、消息 ID、结果和稳定原因码。审计不得记录 Markdown、HTML、图片内容或外链 URL。

### 7. RabbitMQ 作为跨能力事件传输层

系统选用 RabbitMQ 作为内部异步事件传输层，为后续邮件、短信、站内消息、定时任务和其他通知消费者提供统一投递入口。RabbitMQ SHALL NOT 作为消息收件箱或业务状态数据库；消息正文、受众、已读、撤销、分类和审计事实仍由 MySQL 持久化。

消息写入与事件发布采用 Transactional Outbox：业务事务在 MySQL 中同时写入消息事实和 Outbox 记录；独立 Publisher Worker 使用 RabbitMQ Publisher Confirms。AMQP 连接超时为 5 秒、Confirm 超时为 10 秒、Worker 租约为 30 秒；Worker 在短 MySQL 事务中以 `FOR UPDATE SKIP LOCKED` 仅抢占同一消息副本的下一聚合版本，并记录可在等待 Confirm 时续期的有限租约。崩溃或租约到期后其他 Worker 可以接管。确认成功后标记 Outbox 完成。Outbox 按 `1s`、`2s`、`4s`、`8s`、`16s` 重试；第五次失败后进入 MySQL `dead` 状态，只有受审计的人工重放才能恢复待发布。Broker 不可用时不能可靠写入 RabbitMQ DLX，因此 Publisher 失败不得伪装为 Broker 死信。

RabbitMQ Consumer 使用手动确认。应用实例作为竞争 Consumer，以 `prefetch=1` 消费；Consumer 在 MySQL 中只在首次处理未过时事件时持久化动态受众快照。它通过 Redis Lua 原子脚本按用户/事件 ID 检查 24 小时去重键、写入 Stream 并设置 TTL；全部写入成功后才持久化消费记录和最大版本游标，再 ACK。崩溃或部分 Redis 写入后的重投递复用该快照并安全补齐缺失用户。处理失败时，应用验证并递增受控 `x-retry-attempt` 头，经各 Consumer 独立的 durable quorum TTL 重试队列按 `1s`、`2s`、`4s`、`8s`、`16s` 延迟重新投递；无效或越界计数不得绕过第五次死信规则。第五次失败进入按 Consumer 与事件类型隔离、保留 7 天的 Dead Letter Exchange 队列。竞争 DLQ Recorder 只有在将最小事件载荷与受控头持久化为 MySQL Consumer DLQ 投影后才 ACK；Recorder 写失败不 ACK，并经独立五级重试后进入不可自动丢弃的运维告警队列。Consumer DLQ 的 HTTP 管理只读取 MySQL 投影，不使用 RabbitMQ Management API 浏览队列；超级管理员使用 `admin.messages.dead-letter.manage` 查询、逐条重放或丢弃，并审计全部处置。邮件、短信和 WebSocket 推送消费者必须使用业务事件 ID 做幂等去重，不能把至少一次投递误当成恰好一次处理。

Consumer DLQ 的管理员 HTTP 管理事实来自 MySQL 投影：DLQ Recorder 写入成功才 ACK；MySQL 故障时不 ACK，经自身五级重试后进入不可自动丢弃、只由受限运维 CLI 或 RabbitMQ 管理界面处置的告警队列。HTTP 重放通过 durable direct `admin.events.replay` 和 `consumer.<consumer-name>` routing key 定向回原 Consumer，禁止重新发布至公共 Topic Exchange。投影以 `pending → replaying → replayed|pending` 和 30 秒租约抢占；Confirm 后崩溃产生的重复投递由 Consumer 事件 ID 幂等吸收。`replayed` 与 `discarded` 投影保留 30 天，未终态投影不按时间自动清理。动态受众在单个 `REPEATABLE READ` 事务中以每批最多 500 用户封存；Redis Stream 写入完成或事件过时安全 ACK 后，未关联 Consumer DLQ 投影的完整快照保留 24 小时再清理。该短期快照仅保证事件恢复一致性，不改变消息动态受众语义。

为防止长时恢复改变事件的原受众，关联 Consumer DLQ 投影的完整受众快照随投影保留 30 天并供人工重放复用；普通终态完整快照仅保留 24 小时。快照构建以 `snapshotting`、30 秒可续租租约和单调 `snapshot_fence` 抢占，使用单个 `REPEATABLE READ` 读视图、每批 500 用户，单事件最多 100,000 用户。所有快照状态变更和完成提交必须匹配当前围栏与未过期租约；构建时失租或围栏不匹配必须回滚且不得写 Redis/ACK，完整快照提交后的失租必须停止新写入和 ACK，由新围栏 Consumer 复用快照并借助 Lua 补齐。首次读视图超过上限时，消息事务保持成功，Consumer 只记录 `audience_capacity_exceeded` 和观察到的数量，不持久化任何用户快照或部分 Redis Stream 投递，并按五级重试后进入 DLQ；每次重试及该 DLQ 投影的人工重放都重新计算当前受众。这是原受众快照保留与重放复用的唯一例外。DLQ Recorder 告警队列重放只定向返回原 Recorder，禁止改投公共 Topic Exchange。

快照租约与 `snapshot_fence` 是 `message_event_consumptions` 的事实字段，不增加独立租约表或 Redis 锁。抢占及续租使用独立短 MySQL 事务；长 `REPEATABLE READ` 快照事务不锁该消费记录，最终完成时用当前围栏和未过期租约作条件更新。这样续租不会被最多 100,000 用户的快照事务阻塞，同时仍由 MySQL 原子裁决唯一所有者。

Consumer 版本游标高于 Consumer DLQ 重放事件的聚合版本时，旧事件已经被更高版本覆盖。重放仍可由 `admin.events.replay` 成功定向到原 Consumer，投影也可标记为 `replayed`，但该状态只表示重放发布成功；原 Consumer 必须将旧事件标记为 `superseded`、不写 Redis Stream 后 ACK。该规则保持刷新顺序，且不允许旧事件在较新编辑或撤销后重新触发刷新。

浏览器 SHALL NOT 直接连接 RabbitMQ Web STOMP、Web MQTT 或其他 Broker WebSocket 入口。应用自身的 WebSocket Gateway 负责短期 ticket 鉴权、当前用户可见性校验和“收件箱需要刷新”事件推送；RabbitMQ 只连接后端消费者。

事件契约 SHALL 使用稳定的细粒度生命周期事件名称 `messaging.message.created|published|edited|revoked|expired.v1`、事件 ID、消息副本 ID、组织 ID、发生时间和最小业务引用，不携带 Markdown、清洗后 HTML、图片二进制、长期 Token 或外链 URL。同一消息副本按单调聚合版本严格发布，前一版本未发布或死信时不得越过。各消费者按事件类型订阅自己的队列，新增邮件或短信能力不得修改内部消息核心事务。

未来邮件和短信 Consumer 是 App 注入的进程内 Adapter，使用 Messaging 定义的最小 Projection Contract，以事件引用查询当前允许投递且未读的用户 ID、显示名、标题、清洗 HTML、服务端派生纯文本、消息副本 ID 和事件版本；Adapter 通过 Identity Contract 解析当前渠道地址。不得将正文、邮箱、手机号或静态受众快照加入 RabbitMQ 载荷，亦不得增加网络 Projection API。私信 `created` 和管理员群发/公告 `published` 是唯一允许触发外部通知的生命周期事件；编辑、撤销和过期只更新收件箱可见性。一个用户同时属于多个目标组织时，每个组织消息副本都是独立收件箱和外部通知投递单位，不按逻辑消息去重；同一副本内受众规则重叠仍只返回一次。Consumer 因延迟、重试或重放查询投影时，已在站内读取消息的收件人不再返回，Consumer 不得再向其发送外部通知；查询完成后才发生的已读不撤回已在途发送。本 Change 不定义外部投递预约、发送状态或取消机制。

WebSocket Consumer 在事件发生时解析当前动态受众，为每个当前可见用户写入独立的 24 小时 Redis Stream 恢复事件，再通过 Redis Pub/Sub 提醒在线 Gateway。Gateway 返回事件前仍以当前 MySQL 可见性为准；成员后续变化由收件箱查询与该复查处理，不补发旧事件。

死信 Outbox 查询与重放仅通过超级管理员的 `GET /api/admin/message-outboxes` 和 `POST /api/admin/message-outboxes/:id/replay` 完成；重放接口受 `admin.messages.outbox.replay` 保护并写入审计。`GET /api/health` 继续只报告进程存活；公开 `GET /api/ready` 始终以 HTTP 200 返回 `{status, components.rabbitmq.status, components.rabbitmq.outbox_pending, components.rabbitmq.last_error_code}`，RabbitMQ 不可用时总体和组件状态为 `degraded`，且不泄露基础设施细节。待处置 Consumer DLQ 是运行告警而非接流量故障，不改变 `/api/ready` 的 HTTP 状态或 `status`；仅受保护的死信管理查询和监控指标暴露待处置总数、按 Consumer/稳定失败码计数和最旧 `pending` 年龄。

Consumer DLQ 在经历五次失败后必须由人工处置，因此任一 `(Consumer, 稳定失败码)` 的 `pending` 投影从零变为非零即触发一次立即运维告警。告警只含 Consumer、稳定失败码、待处置计数和最旧 `pending` 年龄；不含事件 ID、事件载荷、用户标识或消息内容。

Messaging 在 Consumer DLQ MySQL 投影提交后，通过 App 注入的供应商无关 `MessagingMetrics.RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)` best-effort 记录首条 `pending` 告警观测。该 Contract 只有这一无返回值的方法，Observation 只含 Consumer、稳定失败码、待处置计数和最旧 `pending` 年龄；Messaging 不管理指标名称、通用标签 map 或告警供应商。部署监控系统负责采集与实际告警规则。Contract 不向 Messaging 传播错误：Adapter 异常只写受控运行日志，不阻止 DLQ Recorder ACK 或触发 AMQP 重试。本 Change 不新增 Prometheus、OTel、Alertmanager、Webhook、SMTP 或通用告警发送能力，也不让 Messaging 核心导入任何告警提供商 SDK。

该选型增加 RabbitMQ 服务、连接配置、拓扑声明、监控、重试、死信清理和集成测试要求；OpenSpec Change 必须明确 Exchange、Queue、Routing Key、重试上限、Dead Letter Exchange、消息持久化和 RabbitMQ 不可用时的降级行为。

Consumer DLQ 投影按 `(consumer_name, event_id)` 唯一。人工重放后同一事件再次经历五次失败时，DLQ Recorder 重新打开原投影 `replayed → pending`、递增 `replay_cycle` 并更新末次失败事实，不创建重复管理记录；现有 Audit Log 保留每一轮重放、再次死信和最终处置历史。30 天清理只从最终保持 `replayed` 或 `discarded` 的终态开始计算。

### 8. RabbitMQ 生产拓扑与故障策略

生产环境 SHALL 使用 RabbitMQ Cluster 和 Quorum Queue；开发环境可以使用单节点实例。应用拓扑采用持久化 Topic Exchange，按稳定事件类型使用 Routing Key；WebSocket、邮件和短信消费者未来分别绑定独立 Durable Queue，消费者之间不得共享同一工作队列。

本次只实现站内消息和 WebSocket 消费者。邮件、短信不实现具体适配器、发送逻辑或第三方配置；只保留稳定的事件发布 Contract 和后续可接入的消费者扩展点，不创建空操作或伪实现。

RabbitMQ 不可用时，消息写入接口 SHALL 继续在 MySQL 事务中保存消息事实和 Outbox，并将实时推送标记为异步延迟；Broker 恢复后由 Outbox Worker 补发。系统 SHALL NOT 因实时推送暂时不可用回滚已经成功提交的消息事务，也 SHALL NOT 向客户端宣称实时事件已经送达。

RabbitMQ 的 Publisher Confirm、消费者手动 ACK、重试上限、Dead Letter Exchange、队列持久化和拓扑声明属于后续 OpenSpec Change 的可观察实现契约。

### 9. 邮箱验证只为外部通知资格提供最小 Identity 能力

本 Change 只实现邮箱验证，不实现手机号验证、短信验证、用户渠道偏好或通用邮件能力。Identity 沿用现有 Gin `email` 格式校验和数据库唯一性语义，不改写本地部分、大小写或提供商别名。Identity 使用 `pending_email` 和 `email_verified_at` 保持已验证邮箱在候选邮箱验证前不变；未验证用户仍可登录，但其邮箱不得作为外部通知渠道。未验证当前 `email` 且没有 `pending_email` 的用户自行更换邮箱时，系统直接原子替换 `email`、保持未验证状态、使针对旧目标的有效凭据失效并为新地址签发凭据及同步投递验证邮件，不创建 `pending_email`。该投递失败时保留新 `email`、使新凭据失效并返回安全 HTTP 503，不回滚邮箱替换；用户可登录后重发。已验证用户变更邮箱时系统保留当前 `email` 和 `email_verified_at`，写入 `pending_email` 并同步投递验证邮件；已存在 `pending_email` 时，用户可用新的未占用候选地址在同一事务中替换它，旧候选地址的凭据只在替换成功后失效。替换目标冲突则返回 HTTP 409、保留原 `pending_email` 和其有效凭据。候选确认与替换通过同一用户的事务锁串行裁决：先提交的状态转换生效；替换先提交使旧 token 失效，确认先提交则提升候选邮箱，随后更新基于新的已验证邮箱创建新的 `pending_email`，不允许静默覆盖。候选邮箱创建或替换后的投递失败时，保留既有已验证邮箱和新的候选邮箱、使新凭据失效并返回安全 HTTP 503，不回滚候选邮箱。用户通过 `PUT /api/user/info` 发起任何非空 `email` 变更时必须提交 `current_password`；缺失或不匹配返回含 `current_password` 字段的 HTTP 422 `IDENTITY_CURRENT_PASSWORD_INVALID` 错误，并且不改变邮箱状态、凭据或发送邮件。注册、邮箱直接替换、候选邮箱创建/替换和显式重发的每次签发尝试都消耗同一用户滚动一小时三次额度，SMTP 失败也计数；额度耗尽时，在修改任何邮箱状态前返回 HTTP 429 和 `Retry-After`，不得签发凭据或发送邮件。候选邮箱未确认期间，未来业务通知继续只投递当前已验证 `email`，不得发送到 `pending_email`；确认后原子提升，后续渠道查询才切换到新邮箱。管理员不得修改用户邮箱。用户资料与验证确认成功响应只返回 `email`、`pending_email` 和布尔 `email_verified`。验证凭据是只保存 SHA-256 摘要的 32 字节随机 Base64URL token，绑定用户和目标邮箱，有效期 15 分钟；新凭据、成功、过期或发送失败会使旧凭据失效。验证邮件仅提供由用户复制并通过已认证 POST 确认接口提交的 token；GET 链接不得改变状态。

验证邮件是本 Change 对“邮件不实现”非目标的唯一例外：使用 `go-mail` 通过 SMTP 用户名/密码或服务商应用密码同步发送。TLS 模式只允许 `disabled`（本地测试）、`starttls_required`（生产默认）和 `implicit`（SMTPS），禁止 opportunistic 降级；生产不实现 OAuth2/XOAUTH2 Token 生命周期。注册的 SMTP 投递失败保留账户但使 token 失效并仅返回安全 HTTP 503 错误码；不暴露账户已创建、用户 ID、邮箱或 token，用户可登录后重发。SMTP 未成功接收邮件时不得伪造成功或补偿删除账户。所有验证行为审计但不记录邮箱、token、SMTP 凭据、服务器地址或原始 SMTP 错误。

验证凭据终态后仅保留 24 小时受控元数据，再由 Identity 后台任务物理删除，原始 token 永不持久化。候选邮箱在确认时发生唯一性冲突则返回 HTTP 409、保留 `pending_email` 并使当前 token 失效；已验证且没有候选邮箱时重发同样返回 HTTP 409。SMTP 默认超时为 10 秒且可显式覆盖，覆盖建连、TLS、认证与发送 deadline。


## 后果

- 需要新增内部消息业务模块，并同步扩展固定一级模块清单、Route Catalog、API Metadata、权限码和 OpenAPI 规格。
- 组织成员变化会改变历史群发和公告的可见性，未读计算必须按消息形态定义，不能假设所有收件人都有静态关系记录。
- WebSocket 需要 ticket 签发、游标事件、重连补发和幂等处理；连接断开不能影响消息写入或收件箱事实。
- 图片复用文件记录会改变当前“普通文件拒绝图片”的既有边界，必须单独声明消息图片用途，不能让管理员普通文件接口意外接受图片。
- 服务端代理外链和图片会引入 SSRF、超时、大小、MIME、缓存和审计风险，后续设计必须把这些约束写成可测试行为。
- 这不是立即实现任务。下一步应先创建独立 OpenSpec change，补充 proposal、design、tasks 和涉及模块的 delta specs，再开始代码变更。
