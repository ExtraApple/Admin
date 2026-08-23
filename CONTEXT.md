# Admin 管理平台

Admin 是面向后台管理场景的统一身份、授权、组织和系统资源管理平台。领域语言围绕“谁可以在什么数据范围内，通过哪些菜单和接口执行什么操作”展开。

## 身份与授权

**用户（User）**：
可以登录系统并被授予角色的人员身份。
_Avoid_: 账号、账户

**用户头像（User Avatar）**：
用户身份资料中的展示图片，属于专用上传用途，不属于文件管理中的文件记录。
_Avoid_: 头像文件记录、普通上传文件

**角色（Role）**：
一组可分配给用户的权限、菜单和数据范围。
_Avoid_: 用户组、权限组

**权限（Permission）**：
允许角色执行某类受控操作的授权定义。
_Avoid_: 菜单权限、接口权限

**权限码（Permission Code）**：
唯一标识一项权限的稳定业务编码，由角色授权、菜单可见性和 API 访问控制共同引用。
_Avoid_: 权限名称、路由名称

**超级管理员（Super Administrator）**：
受系统保护并拥有全局管理兜底能力的管理员身份，其角色编码为 `admin`。
_Avoid_: 普通管理员、Root 用户

**数据范围（Data Scope）**：
角色在组织数据上可查看或操作的边界。
_Avoid_: 数据权限、查询条件


**访问快照（Access Snapshot）**：
授权模块根据当前角色、权限、数据范围和授权版本形成的最小运行时授权事实，供身份上下文和受控操作使用。
_Avoid_: 用户模型、JWT 权限快照

**授权版本（Access Version）**：
用户授权事实的单调版本；角色、权限、菜单或数据范围变更时提升，用于使旧会话在下一次请求时失效。
_Avoid_: Token Version、会话版本

## 导航与接口

**菜单（Menu）**：
后台导航树中的目录、页面或按钮节点，可通过权限码控制对用户的可见性。
_Avoid_: 路由、权限

**按钮菜单（Button Menu）**：
表示页面操作入口的菜单节点，可与一个或多个需要相同授权的 API 关联。
_Avoid_: 按钮权限、操作权限

**API 元数据（API Metadata）**：
系统登记的接口管理记录，用于描述接口身份及其认证、审计和权限要求。
_Avoid_: Gin 路由、接口文档

## 组织

**组织单位（Organization Unit）**：
组织树中的管理节点，可承载上下级关系和用户成员关系。
_Avoid_: 部门、机构

**组织成员（Organization Member）**：
被分配到组织单位中的用户。
_Avoid_: 部门用户、组织用户

## 系统数据

**字典类型（Dictionary Type）**：
一组受管理枚举值的分类定义，以稳定编码标识。
_Avoid_: 字典、枚举表

**字典条目（Dictionary Item）**：
隶属于字典类型的单个可选值。
_Avoid_: 字典类型、配置项

**审计日志（Audit Log）**：
用于追踪 API 请求及关键管理操作的安全记录。
_Avoid_: 运行日志、调试日志

**文件记录（File Record）**：
文件管理模块维护的普通上传文件及其对象存储信息和业务元数据，不包含用户头像，也不表示文件内容已经导入业务数据。
_Avoid_: MinIO 对象、附件

**数据导入文件（Data Import File）**：
为特定数据导入流程提交并按对应业务模板解析的文件，与仅供存储和下载的文件记录相区分。
_Avoid_: 普通上传文件、文件记录

## 内部消息

**内部消息（Internal Message）**：
系统内面向用户收发的持久化内容，按消息形态、组织归属、受众和生命周期管理。
_Avoid_: 系统通知、运行日志

**私信（Private Message）**：
发送者与单个收件人之间的一对一内部消息；发送时双方必须属于同一组织。
_Avoid_: 聊天消息、即时通讯

**管理员群发（Administrative Broadcast）**：
由管理员面向组织、角色或全体用户发布的内部消息，收件人可随当前受众成员关系变化。
_Avoid_: 私信、通知公告

**通知公告（Announcement）**：
由管理员发布的可进入草稿、发布、编辑、定时、有效期和撤销生命周期的组织级内部消息。
_Avoid_: 系统日志、弹窗

**消息收件箱（Message Inbox）**：
用户查看其当前可见内部消息、已读状态和受控操作的入口。
_Avoid_: 发件箱、消息队列

**动态受众（Dynamic Audience）**：
消息可见性按当前组织成员或角色关系计算；受众关系变化会影响历史消息的可见范围。
_Avoid_: 静态收件人列表、发送快照

**消息分类（Message Category）**：
归属于组织的消息分类；超级管理员可跨组织管理，组织管理员只管理其所属组织的分类。
_Avoid_: 字典类型、权限分组

**消息撤销（Message Revocation）**：
对已发布消息保留撤销标记并使正文不可按正常消息阅读的管理动作，不等同于物理删除。
_Avoid_: 删除消息、撤回 Token

**收件箱删除（Inbox Deletion）**：
仅移除当前用户对允许删除的私信收件关系，不删除共享消息主体或其他用户的可见性。
_Avoid_: 硬删除消息、全局删除

**消息领域事件（Message Domain Event）**：
消息副本生命周期变更产生的最小异步事实，以稳定名称、事件 ID 和聚合版本标识，不携带消息正文或媒体内容。
_Avoid_: RabbitMQ 消息、收件箱内容

**消息 Outbox（Message Outbox）**：
与消息业务事实在同一事务中持久化的待发布消息领域事件记录，是 RabbitMQ 不可用时的可靠事实来源。
_Avoid_: 收件箱、RabbitMQ 队列

**死信 Outbox（Dead Message Outbox）**：
达到发布重试上限但尚未由 RabbitMQ 接收的消息 Outbox 终态；只能通过受审计的事件重放恢复。
_Avoid_: RabbitMQ Dead Letter Exchange、已撤销消息

**消息事件重放（Message Event Replay）**：
将死信 Outbox 按原事件身份和聚合版本恢复为待发布的受控操作，不创建新的业务消息事实。
_Avoid_: 重发消息、创建新通知

**消息重试队列（Message Retry Queue）**：
为单个消息事件 Consumer 提供固定延迟后重新投递的专用队列，不等同于主消费队列或死信队列。
_Avoid_: 直接重投递、Outbox

**通知投影（Notification Projection）**：
由 Messaging 按当前消息状态和动态受众计算、供 App 注入的通知 Consumer 读取的最小投递数据。
_Avoid_: RabbitMQ 事件载荷、静态收件人快照、网络 Projection API

**Outbox 租约（Outbox Lease）**：
Worker 对待发布消息 Outbox 的有限期独占处理权；租约到期后可由其他 Worker 安全接管。
_Avoid_: 永久状态锁、全局单 Worker

**消息事件消费游标（Message Event Consumption Cursor）**：
Consumer 对每个消息副本已处理的最大聚合版本的持久化事实，用于将迟到事件（包括在更高版本后到达的 Consumer 死信重放）标记为 `superseded`，不重复或倒置产生刷新副作用。
_Avoid_: RabbitMQ 队列位置、临时内存计数

**受控重试次数（Controlled Retry Attempt）**：
由应用验证和递增的事件消费重试次数，用于决定何时进入消息重试队列或死信队列。
_Avoid_: 不受信任的消息头、无限重投递

**Consumer 死信消息（Consumer Dead Letter Message）**：
超过受控处理重试上限后进入特定 Consumer 隔离死信队列的事件；与尚未被 Broker 接收的死信 Outbox 相区分。
_Avoid_: 死信 Outbox、已撤销内部消息

**消息事件受众快照（Message Event Audience Snapshot）**：
Consumer 首次处理消息领域事件时固化的不超过 100,000 名当前可见用户集合，仅用于该事件的可恢复刷新投递，不改变消息动态受众定义。关联 Consumer 死信投影时保留并供重放复用。
_Avoid_: 消息静态收件人、组织成员快照

**受众快照容量超限（Audience Snapshot Capacity Overflow）**：
首次消费解析到超过 100,000 名当前可见用户时的受控 Consumer 失败；不建立部分用户快照或刷新投递，重试和 Consumer 死信重放均重新计算当前受众。
_Avoid_: 截断收件人、原快照重放

**Consumer 死信投影（Consumer Dead Letter Projection）**：
由 DLQ Recorder 持久化的 Consumer 死信受控元数据，是管理员查询、重放和丢弃的事实来源，不直接浏览 RabbitMQ 队列。每个 Consumer 和事件只有一个投影；重放后再次死信时递增 `replay_cycle` 并重新打开原投影，全部处置历史保留在 Audit Log。
_Avoid_: RabbitMQ Management API、死信 Outbox、每轮重复管理记录

**消息运行观测 Contract（Messaging Operational Observability Contract）**：
App 注入 Messaging 的供应商无关且 best-effort 窄 Contract，仅定义无返回值的 `RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)`；Observation 只传递 Consumer、稳定失败码、计数和最旧年龄，用于 Consumer DLQ 投影提交后首条 `pending` 告警观测。Adapter 异常只记录受控运行日志，不影响 DLQ Recorder ACK 或触发 AMQP 重试；部署监控系统决定采集和告警。
_Avoid_: 通用指标名称或标签 map、Prometheus/OTel SDK、Webhook 发送器、SMTP 告警实现、观测失败阻塞死信记录

**消息 Replay Exchange（Message Replay Exchange）**：
专门将管理员重放的 Consumer 死信事件定向返回原 Consumer 的 Exchange，不向其他 Consumer 广播。
_Avoid_: 公共事件 Exchange、Outbox 重放

**DLQ Recorder 告警队列（DLQ Recorder Alert Queue）**：
DLQ Recorder 无法持久化 Consumer 死信投影后进入的不可自动丢弃队列，仅供受限运维工具或 Broker 管理界面处置。
_Avoid_: Consumer 死信投影、自动清理队列

**消息事件快照租约（Message Event Snapshot Lease）**：
保存在消息事件消费记录中的有限期独占处理权，使用单调 `snapshot_fence` 防止失租 Consumer 提交或继续副作用。抢占和续期采用独立短 MySQL 事务，长快照事务不锁定该消费记录；只有持有当前围栏并续期租约的 Consumer 能完成同一事件快照，失租后由新围栏 Consumer 接管。
_Avoid_: 无主快照构建、独立租约表、Redis 锁、全局单 Consumer、过期所有者继续投递


**已验证邮箱（Verified Email）**：
已由用户通过一次性邮箱验证凭据确认所有权的邮箱地址，才具备外部通知投递资格。
_Avoid_: 登录邮箱、任意用户资料邮箱

**待验证邮箱（Pending Email）**：
用户请求变更但尚未完成所有权验证的候选邮箱；在验证成功前不替代既有已验证邮箱，也不得接收业务通知。业务通知继续仅投递当前已验证邮箱；验证成功原子提升后才切换渠道。
_Avoid_: 已验证邮箱、临时登录名、并行投递地址

**邮箱验证凭据（Email Verification Credential）**：
绑定用户与目标邮箱、仅保存摘要且只能使用一次的短期随机凭据，用于确认邮箱所有权。
_Avoid_: 邮箱密码、登录 Token

