# internal-messaging Specification

## Purpose

内部消息模块负责私信、管理员群发、通知公告、组织级消息分类、动态受众、收件箱、已读状态、撤销边界和 RabbitMQ 异步刷新事件。MySQL 是事实来源；RabbitMQ 只传递最小领域事件；WebSocket 只向已认证客户端提供刷新提示。

## Requirements

### Requirement: 私信发送

系统 SHALL 允许已认证用户向同组织的启用用户发送一对一私信，并在同一 MySQL 事务中创建消息主体、唯一收件关系和 Outbox 事件。收件关系初始为未读、未删除；目标不存在、禁用、本人或无共同组织时 SHALL 返回稳定 `MSG_*` 错误且不得创建部分事实。

### Requirement: 管理员群发与公告

具备对应权限和组织数据范围的管理员 SHALL 能按组织、角色或全体用户创建群发，并能管理公告的 draft、scheduled、published、expired 和 revoked 生命周期。每个目标组织 SHALL 创建独立消息副本、分类和动态受众规则；跨组织重叠用户的副本、已读状态和 Outbox 事件不得去重。任一分类缺失或副本写入失败 SHALL 回滚整批事务。

### Requirement: 组织分类

系统 SHALL 提供组织级消息分类的创建、修改、停用和删除。编码只在同一组织内唯一；已引用分类可停用但不可删除；普通管理员受组织及子组织范围限制，超级管理员可跨组织管理；系统不得预置默认分类。

### Requirement: 动态收件箱与已读

已认证用户 SHALL 能查询当前可见的私信、群发和已发布公告、详情及按消息类型的未读数量。动态受众在收件箱和未读查询时按当前关系懒计算；公告首次可见状态根据成员 `joined_at` 与发布时间初始化；只允许删除私信收件关系，群发和公告不得使用私信删除墓碑。

### Requirement: 消息撤销

系统 SHALL 按消息类型、发送者、管理员权限和组织范围控制撤销。撤销保留消息主体、操作者、时间和审计事实；普通收件详情只返回受控 revoked/expired 状态，不返回正文。撤销、编辑和过期均须产生最小刷新事件。

### Requirement: 内容、图片和外链安全

消息只持久化通过安全策略的清洗 HTML，不持久化原始 Markdown。消息图片使用专用临时用途、15 分钟绑定期和 JPEG/PNG/WebP 完整校验，并按当前消息可见性读取；普通文件入口不得暴露消息图片。外链代理只允许 HTTPS 公网目标，拒绝私网、环回、云元数据、不受控重定向、超时、超大响应和 MIME 不匹配。

### Requirement: WebSocket 刷新提示

系统 SHALL 通过一次性、60 秒有效且只存储摘要的 WebSocket ticket 建立已认证 Gateway。刷新 payload 只包含事件 ID、游标、消息副本引用和用户引用，不得包含正文、HTML、图片、URL 或 Token。Gateway SHALL 支持 24 小时 Redis Stream 游标恢复、事件 ID 幂等和超窗全量刷新控制事件；慢连接关闭不得删除 MySQL 事实或恢复事件。

### Requirement: RabbitMQ Outbox 事件

消息事实 SHALL 与最小领域事件在同一事务写入 MySQL Outbox。Outbox Worker SHALL 使用 Publisher Confirm、30 秒租约和 `FOR UPDATE SKIP LOCKED` 的短事务；确认超时、连接失败或 Broker 拒绝按五级 `1/2/4/8/16` 秒重试，第五次失败进入死信。事件按消息副本的 `aggregate_version` 严格发布，死信 Outbox 只能由受保护的超级管理员入口按原事件身份重放。

### Requirement: 消费快照、幂等和 Consumer DLQ

竞争 Consumer SHALL 使用 `prefetch=1`、事件消费幂等记录、30 秒可续租租约和单调 `snapshot_fence`；首次处理动态受众时以不超过 500 用户的批次固化不超过 100,000 用户完整快照。快照未完成或围栏丢失时不得写 Redis Stream 或 ACK；容量超限不得写部分快照并须重新计算。Redis Stream 写入使用原子事件去重；迟到旧聚合版本 SHALL 标记 `superseded`，不得倒置刷新。

终态失败 SHALL 进入 Consumer DLQ Recorder；Recorder 先持久化唯一 `(consumer_name,event_id)` 投影再 ACK。投影重放持有 30 秒租约，Confirm 成功为 `replayed`，失败恢复 `pending`；同一事件再次第五次失败 SHALL 复用投影并递增 `replay_cycle`。超级管理员通过受保护入口查询、重放或丢弃投影；Recorder 告警队列只能由受限运维工具或 RabbitMQ Management 处置。

### Requirement: 通知投影 Contract

未来通知 Consumer SHALL 通过最小 Messaging Projection Contract 查询当前可发送且未读的投递投影。私信只在 `created` 可投递，群发和公告只在 `published` 可投递；查询动态受众时跳过已读用户。RabbitMQ 事件不得携带正文、邮箱、手机号或静态受众快照。

### Requirement: 错误、审计和就绪边界

消息 HTTP 入口 SHALL 使用四字段 JSON envelope 和稳定 `MSG_*` 错误码；Route Catalog、API Metadata、权限同步和 OpenAPI SHALL 使用同一份已校验快照。关键消息操作、权限拒绝、Outbox/Consumer/DLQ 处置 SHALL 写入受控审计或运行日志；日志不得写入正文、HTML、图片、外链 URL、Token、凭据或基础设施原始错误。`/api/ready` 只表达进程和 RabbitMQ/Outbox 就绪状态，不承担 Consumer DLQ 告警。

### Requirement: 已验证邮箱通知渠道

Identity SHALL expose a minimal verified-email notification projection to Messaging. It SHALL return only the current `email` when `email_verified_at` is set; an unverified current address or `pending_email` SHALL never be returned as a deliverable destination. A verified user changing address SHALL continue using the old verified address while `pending_email` is present, and confirmation SHALL atomically promote the pending address before the projection switches. An unverified user's direct replacement SHALL not create a deliverable old-address destination. Existing email normalization and uniqueness rules remain authoritative; phone, SMS and notification preferences are outside this Contract.
