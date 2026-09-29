# internal-messaging Specification

## Purpose

内部消息模块负责私信、管理员群发、通知公告、组织级消息分类、动态受众、收件箱、已读状态、撤销边界和 RabbitMQ 异步刷新事件。MySQL 是事实来源；RabbitMQ 只传递最小领域事件；WebSocket 只向已认证客户端提供刷新提示。
## Requirements
### Requirement: 私信发送

系统 SHALL 允许已认证用户向同组织的启用用户发送一对一私信，并在同一 MySQL 事务中创建消息主体、唯一收件关系和 Outbox 事件。收件关系初始为未读、未删除；目标不存在、禁用、本人或无共同组织时 SHALL 返回稳定 `MSG_*` 错误且不得创建部分事实。

#### Scenario: 私信事务发送
- **WHEN** 已认证用户向同组织的启用用户发送有效私信
- **THEN** 系统 SHALL 在同一事务中创建消息、收件关系和 Outbox 事件

### Requirement: 管理员群发与公告

具备对应权限和组织数据范围的管理员 SHALL 能按组织、角色或全体用户创建群发，并能管理公告的 draft、scheduled、published、expired 和 revoked 生命周期。每个目标组织 SHALL 创建独立消息副本、分类和动态受众规则；跨组织重叠用户的副本、已读状态和 Outbox 事件不得去重。任一分类缺失或副本写入失败 SHALL 回滚整批事务。

#### Scenario: 群发副本原子创建
- **WHEN** 管理员向多个组织群发消息
- **THEN** 系统 SHALL 为每个目标组织创建独立副本，并在任一副本失败时回滚全部事实

### Requirement: 组织分类

系统 SHALL 提供组织级消息分类的创建、修改、停用和删除。编码只在同一组织内唯一；已引用分类可停用但不可删除；普通管理员受组织及子组织范围限制，超级管理员可跨组织管理；系统不得预置默认分类。

#### Scenario: 消息分类约束
- **WHEN** 管理员创建、修改或删除消息分类
- **THEN** 系统 SHALL 执行组织范围、编码唯一和已引用分类保护规则

### Requirement: 动态收件箱与已读

已认证用户 SHALL 能查询当前可见的私信、群发和已发布公告、详情及按消息类型的未读数量。动态受众在收件箱和未读查询时按当前关系懒计算；公告首次可见状态根据成员 `joined_at` 与发布时间初始化；只允许删除私信收件关系，群发和公告不得使用私信删除墓碑。

#### Scenario: 动态收件箱查询
- **WHEN** 已认证用户查询收件箱或未读数量
- **THEN** 系统 SHALL 按当前受众关系计算可见消息和已读状态

### Requirement: 消息撤销

系统 SHALL 按消息类型、发送者、管理员权限和组织范围控制撤销。撤销保留消息主体、操作者、时间和审计事实；普通收件详情只返回受控 revoked/expired 状态，不返回正文。撤销、编辑和过期均须产生最小刷新事件。

#### Scenario: 消息撤销
- **WHEN** 具备权限的操作者撤销消息
- **THEN** 系统 SHALL 保留消息事实和审计信息，并隐藏可读正文

### Requirement: 内容、图片和外链安全

系统 SHALL 只持久化通过安全策略的清洗 HTML，不持久化原始 Markdown。消息图片使用专用临时用途、15 分钟绑定期和 JPEG/PNG/WebP 完整校验，并按当前消息可见性读取；普通文件入口不得暴露消息图片。外链代理只允许 HTTPS 公网目标，拒绝私网、环回、云元数据、不受控重定向、超时、超大响应和 MIME 不匹配。

#### Scenario: 消息内容安全
- **WHEN** 用户提交消息正文、图片或外链
- **THEN** 系统 SHALL 只持久化或返回通过安全策略验证的内容

### Requirement: WebSocket 刷新提示

系统 SHALL 通过一次性、60 秒有效且只存储摘要的 WebSocket ticket 建立已认证 Gateway。刷新 payload 只包含事件 ID、游标、消息副本引用和用户引用，不得包含正文、HTML、图片、URL 或 Token。Gateway SHALL 支持 24 小时 Redis Stream 游标恢复、事件 ID 幂等和超窗全量刷新控制事件；慢连接关闭不得删除 MySQL 事实或恢复事件。

#### Scenario: 收件箱刷新提示
- **WHEN** 消息状态变化导致收件箱可能变化
- **THEN** 系统 SHALL 向已认证客户端提供不含正文的可恢复刷新提示

### Requirement: RabbitMQ Outbox 事件

消息事实 SHALL 与最小领域事件在同一事务写入 MySQL Outbox。Outbox Worker SHALL 使用 Publisher Confirm、30 秒租约和 `FOR UPDATE SKIP LOCKED` 的短事务；确认超时、连接失败或 Broker 拒绝按五级 `1/2/4/8/16` 秒重试，第五次失败进入死信。事件按消息副本的 `aggregate_version` 严格发布，死信 Outbox 只能由受保护的超级管理员入口按原事件身份重放。

#### Scenario: Outbox 事件可靠发布
- **WHEN** 消息事实提交后投递系统暂时不可用
- **THEN** 系统 SHALL 保留 Outbox 事实并按受控次数重试，最终失败进入死信

### Requirement: 消费快照、幂等和 Consumer DLQ

竞争 Consumer SHALL 使用 `prefetch=1`、事件消费幂等记录、30 秒可续租租约和单调 `snapshot_fence`；首次处理动态受众时以不超过 500 用户的批次固化不超过 100,000 用户完整快照。快照未完成或围栏丢失时不得写 Redis Stream 或 ACK；容量超限不得写部分快照并须重新计算。Redis Stream 写入使用原子事件去重；迟到旧聚合版本 SHALL 标记 `superseded`，不得倒置刷新。

终态失败 SHALL 进入 Consumer DLQ Recorder；Recorder 先持久化唯一 `(consumer_name,event_id)` 投影再 ACK。投影重放持有 30 秒租约，Confirm 成功为 `replayed`，失败恢复 `pending`；同一事件再次第五次失败 SHALL 复用投影并递增 `replay_cycle`。超级管理员通过受保护入口查询、重放或丢弃投影；Recorder 告警队列只能由受限运维工具或 RabbitMQ Management 处置。

#### Scenario: Consumer 死信处置
- **WHEN** 事件处理超过受控重试上限
- **THEN** 系统 SHALL 先持久化唯一死信投影再确认原消息，并支持受保护重放

### Requirement: 通知投影 Contract

未来通知 Consumer SHALL 通过最小 Messaging Projection Contract 查询当前可发送且未读的投递投影。私信只在 `created` 可投递，群发和公告只在 `published` 可投递；查询动态受众时跳过已读用户。RabbitMQ 事件不得携带正文、邮箱、手机号或静态受众快照。

#### Scenario: 通知投影查询
- **WHEN** 未来通知 Consumer 请求可发送消息
- **THEN** 系统 SHALL 通过最小 Contract 返回当前可发送且未读的投递投影

### Requirement: 错误、审计和就绪边界

消息 HTTP 入口 SHALL 使用四字段 JSON envelope 和稳定 `MSG_*` 错误码；Route Catalog、API Metadata、权限同步和 OpenAPI SHALL 使用同一份已校验快照。关键消息操作、权限拒绝、Outbox/Consumer/DLQ 处置 SHALL 写入受控审计或运行日志；日志不得写入正文、HTML、图片、外链 URL、Token、凭据或基础设施原始错误。`/api/ready` 只表达进程和 RabbitMQ/Outbox 就绪状态，不承担 Consumer DLQ 告警。

#### Scenario: 消息错误与审计
- **WHEN** 消息 HTTP 操作成功或被拒绝
- **THEN** 系统 SHALL 返回稳定错误契约并记录受控审计或运行元数据

### Requirement: 已验证邮箱通知渠道

Identity SHALL expose a minimal verified-email notification projection to Messaging. It SHALL return only the current `email` when `email_verified_at` is set; an unverified current address or `pending_email` SHALL never be returned as a deliverable destination. A verified user changing address SHALL continue using the old verified address while `pending_email` is present, and confirmation SHALL atomically promote the pending address before the projection switches. An unverified user's direct replacement SHALL not create a deliverable old-address destination. Existing email normalization and uniqueness rules remain authoritative; phone, SMS and notification preferences are outside this Contract.

#### Scenario: 已验证邮箱通知渠道
- **WHEN** Messaging 查询用户外部通知渠道
- **THEN** Identity SHALL 仅提供当前已验证邮箱，不提供未验证或待验证地址

### Requirement: 消息长度上限来自配置

系统 SHALL 从 `messaging.max_title_runes` 与 `messaging.max_body_runes`
读取消息标题与正文的长度上限，SHALL NOT 使用与配置无关的硬编码值。

具体地：

- 标题 SHALL 按 Unicode 字符数计数，上限取自 `max_title_runes`。
- Markdown 正文 SHALL 按 Unicode 字符数计数，上限取自 `max_body_runes`。
- 默认值 SHALL 为标题 100、正文 20,000，与既往行为一致。
- 配置值为 `0` SHALL 视为"未配置"，取上述默认值；
  配置值为负数或高于上限 SHALL 在配置加载阶段失败。
- 校验 SHALL 在领域层执行，且领域层 SHALL NOT 依赖配置模块；
  上限 SHALL 以值的形式由应用层传入。
- 超出上限时系统 SHALL 返回稳定错误码 `MSG_TITLE_TOO_LONG`
  或 `MSG_BODY_TOO_LONG`。
- 清洗后 HTML 的 128 KiB 上限 SHALL 保持独立且不可通过配置调整。

#### Scenario: 默认配置下的长度边界

- **WHEN** 应用以默认配置启动（`max_title_runes = 100`、`max_body_runes = 20000`）
- **AND** 提交恰好 100 个 Unicode 字符的标题与恰好 20,000 个 Unicode 字符的正文
- **THEN** 系统 SHALL 接受并完成编译

#### Scenario: 默认配置下超出标题上限

- **WHEN** 应用以默认配置启动
- **AND** 提交 101 个 Unicode 字符的标题
- **THEN** 系统 SHALL 拒绝并返回 `MSG_TITLE_TOO_LONG`

#### Scenario: 默认配置下超出正文上限

- **WHEN** 应用以默认配置启动
- **AND** 提交 20,001 个 Unicode 字符的 Markdown 正文
- **THEN** 系统 SHALL 拒绝并返回 `MSG_BODY_TOO_LONG`

#### Scenario: 自定义上限生效

- **WHEN** 配置 `max_title_runes` 为 120 且 `max_body_runes` 为 25000
- **AND** 提交 120 个 Unicode 字符的标题与 25,000 个 Unicode 字符的正文
- **THEN** 系统 SHALL 接受并完成编译

#### Scenario: 长度按 Unicode 字符而非字节计数

- **WHEN** 提交由多字节字符组成的标题或正文
- **THEN** 系统 SHALL 按 Unicode 字符数判定是否超限
- **AND** 系统 SHALL NOT 按字节长度判定

#### Scenario: 清洗后 HTML 上限不可配置

- **WHEN** 消息正文通过长度校验但清洗后 HTML 超过 128 KiB
- **THEN** 系统 SHALL 拒绝并返回 `MSG_HTML_TOO_LARGE`
- **AND** 该上限 SHALL NOT 受 `max_body_runes` 配置影响

#### Scenario: 非法上限配置被拒绝

- **WHEN** 配置的 `max_title_runes` 或 `max_body_runes` 为负数或高于允许上限
- **THEN** 系统 SHALL 在配置加载阶段失败
- **AND** 系统 SHALL NOT 以静默默认值继续启动

#### Scenario: 未配置的上限取默认值

- **WHEN** 配置的 `max_title_runes` 或 `max_body_runes` 为 `0` 或该键缺失
- **THEN** 系统 SHALL 使用默认值（标题 100、正文 20,000）
- **AND** 系统 SHALL NOT 在配置加载阶段失败


### Requirement: 消息周期维护调度

App SHALL 在迁移成功后注册唯一 `messaging-cleanup` 后台任务，启动立即执行一轮，整轮完成后等待一小时再运行下一轮；同一进程 SHALL NOT 重叠执行。每轮 SHALL 使用统一 UTC 资格判断时间，在15分钟父超时内先运行既有受众快照清理和终态 Consumer 死信清理，再顺序运行图片清理、公告发布、公告过期。既有两项各最多30秒，C5三项各最多5分钟，均受父取消约束，不保证后项获得完整时间预算。

系统 SHALL 保留既有快照到期及终态死信30天清理规则和各500条上限；子项失败不阻止后项，父取消后 SHALL 停止启动新子项。迁移本身 SHALL NOT 执行存储清理。

#### Scenario: 启动处理到期存量
- **WHEN** 迁移成功且应用后台任务开始运行
- **THEN** 系统 SHALL 立即执行一轮维护并允许处理历史到期数据
- **AND** 系统 SHALL NOT 以启动时间水位排除历史数据
- **AND** 一轮结束后 SHALL 等待一小时，不承诺整点执行

#### Scenario: 子任务失败隔离
- **WHEN** 图片清理失败或其子 context 超时但父 context 仍有效
- **THEN** 系统 SHALL 记录图片任务结果并继续公告任务
- **AND** 已提交的数据库变更 SHALL 保留

#### Scenario: 整轮取消
- **WHEN** 父 context 到期或应用关闭
- **THEN** 系统 SHALL 取消当前工作并将尚未启动的任务记录为 skipped
- **AND** 系统 SHALL NOT 启动重叠轮次或无取消的后台补偿

#### Scenario: 原有清理继续执行
- **WHEN** C5 接入维护任务，包括启动时未配置 RabbitMQ 的部署
- **THEN** 系统 SHALL 保留并执行既有两项数据库保留期清理
- **AND** 系统 SHALL NOT 将 pending Outbox 或 pending Consumer 死信当作终态历史数据删除

### Requirement: 消息维护批量配置与推进

系统 SHALL 在配置加载阶段读取 `messaging.cleanup_batch_size`，缺省或0取100，1–1000有效，负数或超界启动失败。每次查询及每类C5任务单轮尝试数量 SHALL 不超过该值；图片新登记与到期重试 SHALL 共享图片额度，发布与过期额度相互独立。冲突和失败同样消耗额度，不以成功数量决定是否继续扫描。

候选 SHALL 在数据库内过滤资格、按ID升序有界查询，不改变管理列表排序、分页或total。系统 SHALL 为各候选类别跨轮保存进程内游标与固定扫描上界，已尝试的成功/失败/冲突均推进游标，到扫描尾部再回绕。重启 SHALL 允许重置游标，不承诺跨任意重启的持久公平性。

#### Scenario: 批量默认与非法配置
- **WHEN** 配置缺失或为0
- **THEN** 每类C5任务 SHALL 使用100条额度
- **AND** 负数或大于1000 SHALL 在Load时失败，不以默认值掩盖非法输入

#### Scenario: 有界且独立的任务额度
- **WHEN** 三类任务均有超过N条到期记录且cleanup_batch_size为N
- **THEN** 每类任务 SHALL 最多尝试N个对象或组织副本
- **AND** 图片失败 SHALL NOT 挤占公告发布或公告过期额度
- **AND** 图片首次登记与重试总尝试 SHALL 不超过N

#### Scenario: 坏记录不持续占满前页
- **WHEN** 低ID候选持续失败且进程持续运行
- **THEN** 系统 SHALL 在后续轮次从已尝试ID之后继续至固定扫描上界，再回绕
- **AND** 后续合格记录 SHALL 获得尝试机会，不因每轮从头只取失败前页而永久饥饿

#### Scenario: 新登记与重试均获机会
- **WHEN** 图片新候选与到期重试同时积压
- **THEN** 系统 SHALL 在同一N额度内给两者处理机会；N为1时跨轮交替优先
- **AND** 一侧为空时另一侧 SHALL 能使用剩余额度
- **AND** 同一图片首次尝试后 SHALL NOT 当轮再次作为重试处理

### Requirement: 到期公告副本原子转换

系统 SHALL 按组织副本独立事务推进公告，保持状态、aggregate_version和既有Outbox事件的原子性。发布只选择scheduled且publish_at<=now、expires_at为空或>now的公告；过期选择expires_at<=now的published公告，以及publish_at<=now且已到期的scheduled公告。已错过有效期的scheduled SHALL 直接变为expired，仅产生MessageExpired，不补MessagePublished。

每个成功副本 SHALL 递增一次聚合版本并生成对应最小事件；系统 SHALL NOT 聚合多个组织副本回滚、合并跨组织事件或只写状态不写事件。CAS冲突 SHALL 作为本次未提交跳过，其他单条错误记录并继续，不建立公告失败表。

#### Scenario: 到期公告正常发布
- **WHEN** scheduled公告到达publish_at且尚未达到expires_at
- **THEN** 系统 SHALL 在单副本事务中写入published、新聚合版本和MessagePublished Outbox
- **AND** 系统 SHALL 沿用既有投递链路，不等待Broker确认

#### Scenario: 停机后已错过有效期
- **WHEN** scheduled公告的publish_at和expires_at均不晚于本轮now
- **THEN** 过期任务 SHALL 将该副本直接转为expired并写MessageExpired
- **AND** 发布任务 SHALL NOT 为其产生MessagePublished

#### Scenario: 已发布公告到期
- **WHEN** published公告的expires_at<=now
- **THEN** 过期任务 SHALL 在同一副本事务中写expired状态、版本及MessageExpired

#### Scenario: 并发修改或事件插入失败
- **WHEN** 当前副本CAS冲突或Outbox插入失败
- **THEN** 本次尝试 SHALL NOT 留下无对应事件的新状态
- **AND** CAS冲突 SHALL NOT 被统计为成功发布
- **AND** 其他组织副本 SHALL 继续处理，已成功副本不回滚

### Requirement: 自动公告发布的 Broker 配置边界

自动发布 SHALL 仅受启动时RabbitMQ是否配置的门控，不受瞬时连接健康门控。未配置时 SHALL 跳过自动发布并记录rabbitmq_not_configured，不额外查询待发布数量；图片和公告过期 SHALL 仍运行。配置变更 SHALL 经重启生效，不热创建publisher。

已配置但连接失败时，C5 SHALL 继续提交公告与Outbox，沿用既有有限重试和dead规则；Outbox dead SHALL NOT 回滚公告或自动重新发布。系统 SHALL 复用现有受保护Outbox重放入口和原事件身份，不新增恢复控制面。

#### Scenario: 启动未配置 RabbitMQ
- **WHEN** 应用启动未配置RabbitMQ
- **THEN** 自动发布 SHALL 每轮跳过并记录一次配置原因，计数为0
- **AND** 图片清理与公告过期 SHALL 继续运行，过期事件允许保留在pending Outbox

#### Scenario: 已配置但临时断连
- **WHEN** RabbitMQ已配置但连接不可用
- **THEN** 公告状态转换 SHALL 不等待连接恢复
- **AND** 事件 SHALL 交给既有Outbox有限重试，dead不会因网络恢复自动重放

#### Scenario: 人工重放原 Outbox
- **WHEN** 事件已dead且具备既有超级管理员范围和重放权限的操作者请求恢复
- **THEN** 系统 SHALL 通过既有 `/api/admin/message-outboxes/:id/replay` 重置原Outbox
- **AND** C5 SHALL NOT 回滚公告、创建第二个发布事件或新增API
