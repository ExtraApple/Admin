# internal-messaging Specification

## Purpose

内部消息模块负责私信、管理员群发、通知公告、组织级消息分类、动态受众、用户收件箱、已读/撤销状态和 RabbitMQ 异步事件。MySQL 是消息事实来源，RabbitMQ 只负责后端事件传输，WebSocket 只向已认证客户端提供收件箱刷新提示。

## ADDED Requirements

### Requirement: 私信发送

系统 SHALL 允许已认证用户向同组织的启用用户发送一对一私信。

#### Scenario: 发送私信成功
- **WHEN** 已认证用户调用 `POST /api/user/messages/private`
- **AND** 收件人存在、已启用且与发送者至少共享一个组织
- **AND** 标题和 Markdown 正文通过内容安全校验
- **THEN** 系统 SHALL 创建私信消息主体和唯一收件关系
- **AND** 收件关系 SHALL 初始为未读、未删除
- **AND** 系统 SHALL 在同一 MySQL 事务中写入消息事实和 Outbox 事件

#### Scenario: 私信收件人不属于同组织
- **WHEN** 发送者与收件人没有共同组织
- **THEN** 系统 SHALL 拒绝请求
- **AND** 系统 SHALL NOT 创建消息、收件关系或 Outbox 事件

#### Scenario: 私信目标不可用
- **WHEN** 收件人不存在、被禁用或为发送者本人
- **THEN** 系统 SHALL 返回稳定 `MSG_*` 错误
- **AND** 系统 SHALL NOT 暴露目标用户是否存在的额外敏感信息

### Requirement: 管理员群发

系统 SHALL 允许具备 `admin.messages.broadcast.manage` 权限的管理员向组织、角色或全体用户发布群发消息。

#### Scenario: 按组织群发
- **WHEN** 管理员提交一个或多个可管理组织及分类编码
- **AND** 管理员的数据范围覆盖全部目标组织及其子组织
- **THEN** 系统 SHALL 为每个目标组织创建独立消息副本
- **AND** 每个副本 SHALL 保存组织、分类和动态受众规则
- **AND** 收件人 SHALL 按当前组织成员关系计算，而不是永久展开为静态收件人列表

#### Scenario: 按角色群发
- **WHEN** 管理员选择角色受众
- **AND** 目标角色属于管理员可管理的组织范围
- **THEN** 系统 SHALL 保存角色受众规则
- **AND** 当前拥有该角色且属于目标组织的启用用户 SHALL 能看到该消息

#### Scenario: 全体用户群发
- **WHEN** 超级管理员选择全体用户受众
- **THEN** 系统 SHALL 按组织创建独立副本
- **AND** 每个组织内当前启用成员 SHALL 能看到对应副本
- **AND** 普通管理员 SHALL NOT 使用全体用户受众

#### Scenario: 跨目标组织的重叠收件人
- **WHEN** 同一用户同时属于一次群发或公告的多个目标组织
- **THEN** 系统 SHALL 为该用户显示每个当前可见组织副本的独立收件箱项
- **AND** 每个副本 SHALL 独立计算已读、未读、WebSocket 刷新和 Outbox 事件
- **AND** 系统 SHALL NOT 按逻辑消息去重这些组织副本
- **AND** 同一组织副本内命中多个受众规则的用户 SHALL 只获得一个用户状态和一次该副本事件投递

#### Scenario: 群发副本事务失败
- **WHEN** 任一目标组织分类缺失或任一副本写入失败
- **THEN** 系统 SHALL 回滚本次群发的全部消息副本和 Outbox 事件
- **AND** 系统 SHALL NOT 返回部分成功结果

### Requirement: 通知公告生命周期

系统 SHALL 支持组织级通知公告的草稿、定时发布、发布、有效期、发布后编辑和撤销。

#### Scenario: 创建公告草稿
- **WHEN** 具备 `admin.messages.announcement.manage` 权限的管理员提交有效公告内容和受众
- **THEN** 系统 SHALL 创建 `draft` 状态的组织消息副本
- **AND** 草稿 SHALL NOT 出现在普通用户收件箱
- **AND** 草稿 SHALL 只能由有权管理对应组织的管理员查询

#### Scenario: 定时发布公告
- **WHEN** 管理员提交未来的 RFC 3339 UTC 发布时间
- **THEN** 系统 SHALL 将公告置为 `scheduled`
- **AND** 到达发布时间后系统 SHALL 原子地转为 `published`
- **AND** 系统 SHALL 为每个已发布副本写入收件箱刷新 Outbox 事件

#### Scenario: 立即发布公告
- **WHEN** 管理员发布有效草稿且发布时间为当前时间或未设置
- **THEN** 系统 SHALL 将公告置为 `published`
- **AND** 当前符合受众规则的用户 SHALL 能在收件箱看到公告

#### Scenario: 公告首次可见的已读状态
- **WHEN** 用户首次进入公告当前受众且尚无该公告用户状态
- **THEN** 系统 SHALL 依据当前组织成员关系的加入时间初始化状态
- **AND** 成员加入时间晚于公告发布时间的历史公告 SHALL 初始化为已读
- **AND** 其他首次可见公告 SHALL 初始化为未读

#### Scenario: 发布后编辑公告
- **WHEN** 管理员修改未撤销且未过期的公告
- **THEN** 系统 SHALL 更新标题、清洗后的正文、受众、分类或有效期
- **AND** 系统 SHALL 保留已读状态
- **AND** 系统 SHALL 写入刷新事件使在线用户重新读取公告

#### Scenario: 公告过期
- **WHEN** 当前 UTC 时间达到公告 `expires_at`
- **THEN** 系统 SHALL 将公告视为过期并从普通收件箱隐藏
- **AND** 系统 SHALL 保留公告管理查询和审计记录

#### Scenario: 公告撤销
- **WHEN** 具备公告管理权限的发送者或超级管理员撤销已发布公告
- **THEN** 系统 SHALL 保留消息主体并记录撤销者和撤销时间
- **AND** 收件箱详情 SHALL 返回已撤销标记但不得返回可阅读正文
- **AND** 系统 SHALL 写入刷新 Outbox 事件

### Requirement: 组织级消息分类

系统 SHALL 支持组织级消息分类管理。

#### Scenario: 创建分类
- **WHEN** 具备 `admin.messages.category.manage` 权限的管理员提交组织内唯一编码、名称、排序和启用状态
- **THEN** 系统 SHALL 创建归属于目标组织的分类
- **AND** 普通管理员 SHALL 只能管理其所属组织及子组织范围内的分类
- **AND** 超级管理员 SHALL 能跨组织管理分类

#### Scenario: 修改分类
- **WHEN** 管理员修改未被保护字段的分类
- **THEN** 系统 SHALL 应用名称、排序或启用状态变更
- **AND** 已被消息引用的分类 SHALL 仍可停用

#### Scenario: 删除分类
- **WHEN** 管理员删除一个未被任何消息引用的分类
- **THEN** 系统 SHALL 删除该分类
- **AND** 已被消息引用的分类删除请求 SHALL 被拒绝并保留原记录

#### Scenario: 分类编码冲突
- **WHEN** 分类编码已被同一组织的其他分类使用
- **THEN** 系统 SHALL 拒绝创建或修改
- **AND** 不得修改已有分类或消息引用

#### Scenario: 分类不预置
- **WHEN** 新组织尚未创建消息分类
- **THEN** 系统 SHALL NOT 自动创建默认消息分类
- **AND** 该组织 SHALL 不能以不存在的分类编码创建或发布消息


### Requirement: 动态收件箱

系统 SHALL 提供已认证用户的消息收件箱、详情和未读查询。

#### Scenario: 查询收件箱
- **WHEN** 用户调用 `GET /api/user/messages`
- **THEN** 系统 SHALL 返回当前用户当前可见的私信、群发和已发布公告
- **AND** 系统 SHALL 默认按发布时间倒序分页
- **AND** SHALL 支持按分类、消息类型、已读状态和关键词筛选
- **AND** 已撤销或已过期消息 SHALL 保留受控状态信息但不得按正常正文显示

#### Scenario: 查询消息详情
- **WHEN** 用户调用 `GET /api/user/messages/:id`
- **AND** 消息当前对该用户可见
- **THEN** 系统 SHALL 返回消息元数据、清洗后的 HTML、已读状态和撤销/过期状态
- **AND** 系统 SHALL NOT 返回原始 Markdown、内部对象存储路径或 RabbitMQ 载荷

#### Scenario: 查询不可见消息
- **WHEN** 用户请求不存在、已删除、已撤销不可读或当前不符合受众的消息
- **THEN** 系统 SHALL 返回统一的资源不可用错误
- **AND** 系统 SHALL NOT 暴露消息存在性、受众规则或组织状态

#### Scenario: 查询未读数量
- **WHEN** 用户调用 `GET /api/user/messages/unread-count`
- **THEN** 系统 SHALL 按私信、群发、公告和总数返回未读数量
- **AND** 动态受众的未读状态 SHALL 按当前可见性懒计算并持久化

#### Scenario: 成员或角色关系变化
- **WHEN** 用户的组织成员或角色关系发生变化
- **THEN** 系统 SHALL 在该用户下一次收件箱查询、未读查询或 WebSocket 游标恢复时重新计算群发和公告可见性
- **AND** 系统 SHALL NOT 仅因成员或角色关系变化补发历史消息刷新事件

### Requirement: 已读与收件箱删除

系统 SHALL 按用户维护消息已读状态，并只允许删除私信收件关系。

#### Scenario: 标记已读
- **WHEN** 用户调用 `PUT /api/user/messages/:id/read`
- **AND** 消息当前对用户可见
- **THEN** 系统 SHALL 幂等地记录该用户的已读时间
- **AND** 重复调用 SHALL NOT 产生错误或重复状态

#### Scenario: 删除私信收件关系
- **WHEN** 用户调用 `DELETE /api/user/messages/:id/inbox`
- **AND** 消息是该用户的私信收件关系
- **THEN** 系统 SHALL 写入当前用户删除墓碑并从其收件箱隐藏
- **AND** 系统 SHALL NOT 删除消息主体、发送者记录或其他用户状态

#### Scenario: 删除群发或公告
- **WHEN** 用户尝试删除管理员群发或通知公告
- **THEN** 系统 SHALL 拒绝请求
- **AND** 系统 SHALL 保留该消息的已读状态和可见性

#### Scenario: 动态受众删除后重新出现
- **WHEN** 用户属于群发或公告的当前受众
- **AND** 用户曾经查询、已读或退出后再次符合受众
- **THEN** 系统 SHALL 按消息类型重新计算可见性
- **AND** 群发和公告 SHALL NOT 使用私信删除墓碑语义

### Requirement: 消息撤销授权

系统 SHALL 按消息类型、发送者身份和组织数据范围控制撤销。

#### Scenario: 普通用户撤销自己的私信
- **WHEN** 普通用户撤销自己发送的私信
- **THEN** 系统 SHALL 标记消息已撤销
- **AND** 收件人 SHALL 只能看到已撤销标记而不能读取正文

#### Scenario: 组织管理员撤销普通用户消息
- **WHEN** 组织管理员撤销其所属组织及子组织范围内普通用户发送的消息
- **THEN** 系统 SHALL 允许撤销私信、群发或公告
- **AND** 系统 SHALL 记录操作者、组织范围和撤销时间

#### Scenario: 组织管理员撤销管理员消息
- **WHEN** 组织管理员尝试撤销管理员或超级管理员发送的消息
- **THEN** 系统 SHALL 拒绝请求
- **AND** 系统 SHALL NOT 修改消息状态

#### Scenario: 公告发布者撤销自己的公告
- **WHEN** 发布者撤销自己发布的公告
- **THEN** 系统 SHALL 允许撤销该公告副本并记录撤销时间和审计元数据
- **AND** 组织管理员 SHALL NOT 仅凭组织范围撤销其他管理员发布的公告

#### Scenario: 超级管理员跨组织撤销
- **WHEN** 超级管理员撤销任意组织副本的消息
- **THEN** 系统 SHALL 允许操作并记录审计元数据
- **AND** 系统 SHALL 只修改目标消息副本及其关联状态

### Requirement: Markdown 和消息媒体安全

系统 SHALL 在持久化或代理返回前验证并清洗消息内容、图片和外链。

#### Scenario: Markdown 转换成功
- **WHEN** 请求提交标题不超过 100 个 Unicode 字符且 Markdown 正文不超过 20,000 个 Unicode 字符
- **AND** 清洗后的 HTML 不超过 128 KiB UTF-8 字节
- **AND** 内容不包含被禁止的脚本、事件属性或危险协议
- **THEN** 系统 SHALL 转换为受限 HTML 并只持久化清洗后的 HTML
- **AND** 系统 SHALL NOT 持久化原始 Markdown

#### Scenario: 恶意 Markdown 被拒绝
- **WHEN** 内容包含脚本、事件属性、JavaScript/Data 等危险协议或超过长度上限
- **THEN** 系统 SHALL 返回稳定 `MSG_*` 校验错误
- **AND** 系统 SHALL NOT 写入消息、File Record 或 Outbox

#### Scenario: 消息图片上传
- **WHEN** 用户通过消息图片专用入口上传 JPEG、PNG 或 WebP
- **AND** 文件不超过 5 MiB、最长边不超过 4,096 像素且能被完整解码
- **THEN** 系统 SHALL 复用 File Record 模型保存带上传者引用和 15 分钟绑定时限的临时消息图片用途记录
- **AND** 系统 SHALL 在创建、编辑或发布消息的同一事务中，把由当前操作者持有且未过期的临时图片绑定到逻辑消息 ID
- **AND** 普通文件列表和普通文件下载入口 SHALL NOT 暴露该消息图片

#### Scenario: 图片访问服从消息可见性
- **WHEN** 用户读取消息图片
- **AND** 用户当前有权查看所关联消息
- **THEN** 系统 SHALL 返回经过验证的图片
- **AND** 消息被撤销、私信关系被删除或受众不再匹配后 SHALL 拒绝图片读取

#### Scenario: 外链代理安全
- **WHEN** 用户请求 Markdown 中的外链或外部图片
- **THEN** 系统 SHALL 只允许 HTTPS 公网目标
- **AND** 系统 SHALL 拒绝私网、环回、链路本地、云元数据地址、不受控重定向、超时、超大响应或不匹配 MIME
- **AND** 系统 SHALL NOT 永久缓存外部内容或记录完整外链 URL 到日志

### Requirement: WebSocket ticket 和可恢复刷新事件

系统 SHALL 通过应用 WebSocket Gateway 为已认证用户提供可恢复的收件箱刷新提示。

#### Scenario: 签发 WebSocket ticket
- **WHEN** 已认证用户调用 `POST /api/user/messages/ws-ticket`
- **THEN** 系统 SHALL 返回一次性 ticket
- **AND** ticket SHALL 只存储摘要并在 60 秒后过期
- **AND** ticket SHALL 只能用于一次 WebSocket 升级

#### Scenario: WebSocket 升级
- **WHEN** 客户端调用 `GET /api/user/messages/ws` 并提交有效未使用 ticket
- **THEN** 系统 SHALL 建立应用 WebSocket 连接
- **AND** 连接 SHALL 绑定签发 ticket 的用户身份
- **AND** 浏览器 SHALL NOT 连接 RabbitMQ Web STOMP、Web MQTT 或其他 Broker WebSocket 入口

#### Scenario: 新消息或状态变化提示
- **WHEN** 消息发布、编辑、撤销或过期导致用户收件箱可能变化
- **THEN** 系统 SHALL 发布最小事件到 RabbitMQ
- **AND** WebSocket 客户端 SHALL 收到不包含正文的“收件箱需要刷新”事件
- **AND** 事件 SHALL 包含事件 ID、游标和消息副本引用

#### Scenario: WebSocket 断线恢复
- **WHEN** 客户端使用最近 24 小时内的游标重连
- **THEN** 系统 SHALL 补发该用户仍可能可见的刷新提示事件
- **AND** 事件补发 SHALL 按事件 ID 幂等

#### Scenario: 游标超出保留窗口
- **WHEN** 客户端游标早于 24 小时恢复缓存
- **THEN** 系统 SHALL 返回需要全量刷新收件箱的控制事件
- **AND** 系统 SHALL NOT 伪造完整历史事件


#### Scenario: 慢连接关闭并恢复
- **WHEN** WebSocket 客户端写入队列已满或单次写入超过 5 秒
- **THEN** Gateway SHALL 关闭该连接
- **AND** 系统 SHALL 保留 MySQL 消息事实和 Redis 恢复事件
- **AND** 客户端 SHALL 能使用游标重连恢复，不得因慢连接阻塞其他连接

### Requirement: RabbitMQ Outbox 事件

系统 SHALL 使用 MySQL Outbox 和 RabbitMQ 传输消息领域事件。

#### Scenario: 消息事务写入有序语义 Outbox
- **WHEN** 消息创建、发布、编辑、撤销或过期状态变更成功
- **THEN** 系统 SHALL 在同一 MySQL 事务写入唯一事件 ID、稳定生命周期事件名称和 Routing Key `messaging.message.<action>.v1` 的 Outbox 记录
- **AND** 同一消息副本的 `aggregate_version` SHALL 单调递增
- **AND** 前一版本未发布或已进入 `dead` 时 SHALL NOT 正常发布后一版本
- **AND** RabbitMQ 暂时不可用 SHALL NOT 回滚已提交的消息事实

#### Scenario: 并发 Worker 抢占和租约恢复
- **WHEN** 多个 Outbox Worker 同时请求可发布事件
- **THEN** 系统 SHALL 在短 MySQL 事务中使用 `FOR UPDATE SKIP LOCKED` 仅抢占每个消息副本的下一 `aggregate_version`
- **AND** 系统 SHALL 写入 Worker 身份和有限租约
- **AND** Worker 崩溃或租约到期后其他 Worker SHALL 能安全接管未发布事件

#### Scenario: Outbox 连接、Confirm 和租约时限
- **WHEN** Worker 发布已抢占的 Outbox
- **THEN** 系统 SHALL 使用 5 秒 AMQP 连接超时、10 秒 Publisher Confirm 超时和 30 秒 Worker 租约
- **AND** 等待 Confirm 时 SHALL 续租，超时或崩溃后 SHALL 允许其他 Worker 在租约到期后接管

#### Scenario: Publisher Confirm 成功
- **WHEN** Outbox Worker 向持久化 Topic Exchange 发布事件并收到 Publisher Confirm
- **THEN** 系统 SHALL 标记 Outbox 已发布
- **AND** 同一事件 SHALL NOT 被正常重发

#### Scenario: Publisher Confirm 失败达到终态
- **WHEN** RabbitMQ 连接失败、Confirm 超时或 Broker 拒绝事件
- **THEN** 系统 SHALL 按 `1s`、`2s`、`4s`、`8s`、`16s` 有界重试并保留未发布 Outbox
- **AND** 第五次失败后 SHALL 将 Outbox 标记为 `dead` 并记录稳定错误码
- **AND** 系统 SHALL NOT 尝试将无法发布的事件直接写入 RabbitMQ Dead Letter Exchange

#### Scenario: 超级管理员重放死信 Outbox
- **WHEN** 超级管理员通过 `POST /api/admin/message-outboxes/:id/replay` 重放 `dead` Outbox
- **THEN** 系统 SHALL 以原事件 ID、事件名称、消息副本和聚合版本恢复待发布状态
- **AND** 系统 SHALL 记录重放操作者、时间和结果
- **AND** 非超级管理员 SHALL NOT 查询或重放死信 Outbox

#### Scenario: 并发 Consumer 幂等 ACK 与用户恢复事件
- **WHEN** 多个应用实例以竞争 Consumer 和 `prefetch=1` 接收 WebSocket 事件
- **THEN** 系统 SHALL 在 `message_event_consumptions` 中以 `snapshotting` 状态、30 秒可续租租约和单调递增 `snapshot_fence` 只允许一个 Consumer 构建该事件快照
- **AND** 抢占和续租 SHALL 使用独立短 MySQL 事务；快照构建的长 `REPEATABLE READ` 事务 SHALL NOT 锁定该消费记录
- **AND** 租约持有者 SHALL 在单个 `REPEATABLE READ` MySQL 事务中固定首次消费时的读视图，并以每批最多 500 用户持久化按 Consumer、事件 ID 和用户唯一的动态受众快照
- **AND** 快照状态变更和完成提交 SHALL 同时匹配当前 `snapshot_fence` 与未过期租约
- **AND** Consumer SHALL 在全量快照封存并提交前不写 Redis Stream 或 ACK
- **AND** Consumer SHALL 仅在事件未过时的首次处理时创建该快照
- **AND** Consumer SHALL 通过 Redis Lua 脚本对每个快照用户原子检查 24 小时事件去重键、写入 Redis Stream 并设置去重 TTL
- **AND** Consumer SHALL 在所有用户 Stream 写入成功后持久化按 Consumer 名称和消息副本唯一的最大聚合版本游标及事件消费记录，再 ACK
- **WHEN** Consumer 在构建快照时无法续租或发现 `snapshot_fence` 不匹配
- **THEN** Consumer SHALL 回滚未完成快照事务，且不得写 Redis Stream 或 ACK
- **AND** 重投递 Consumer SHALL 取得新的 `snapshot_fence` 后接管
- **WHEN** 已完整提交快照的 Consumer 在 Redis Stream 写入或 ACK 前失去租约或围栏
- **THEN** 它 SHALL 停止新的 Redis 写入和 ACK
- **AND** 持有新围栏的 Consumer SHALL 复用完整快照，并通过 Lua 幂等写入补齐任何缺失用户
- **WHEN** Consumer 在 Stream 写入期间崩溃或部分失败后重投递
- **THEN** Consumer SHALL 接管或复用封存快照，抑制已写事件并补齐缺失用户
- **WHEN** 事件的 `aggregate_version` 低于持久化 Consumer 游标（包括更高版本成功处理后的 Consumer DLQ 旧版本重放），或同一事件再次投递
- **THEN** Consumer SHALL 标记为 `superseded` 或幂等完成后安全 ACK，且不得产生新的用户恢复事件或倒置刷新顺序

#### Scenario: 动态受众快照容量上限与清理
- **WHEN** 一个事件在首次消费时解析到超过 100,000 个当前可见用户
- **THEN** 消息业务事实 SHALL 保持已提交
- **AND** Consumer SHALL 只记录稳定 `audience_capacity_exceeded` 失败码和本次观察到的受众数量
- **AND** Consumer SHALL NOT 持久化任何用户 `message_event_deliveries` 行、写入 Redis Stream 或 ACK，不得产生部分刷新投递
- **AND** Consumer SHALL 按既定五级重试后进入 Consumer DLQ
- **AND** 每次受控重试及该 Consumer DLQ 记录的人工重放 SHALL 重新计算当时动态受众；受众容量超限是“重放复用原受众快照”的唯一例外
- **WHEN** 消费成功完成或事件过时后安全 ACK
- **THEN** 系统 SHALL 保留普通终态完整快照 24 小时
- **WHEN** 完整受众快照关联 Consumer DLQ 投影
- **THEN** 系统 SHALL 保留该完整快照至投影 30 天终态清理
- **AND** Messaging 后台任务 SHALL 在相应保留期后删除完整快照

#### Scenario: Consumer 延迟重试和死信
- **WHEN** Consumer 处理事件失败
- **THEN** 系统 SHALL 验证并递增受控 `x-retry-attempt` 头，依次通过独立 durable quorum TTL 重试队列按 `1s`、`2s`、`4s`、`8s`、`16s` 延迟后重新投递
- **AND** 无效或越界重试头 SHALL NOT 绕过第五次死信规则
- **AND** 系统 SHALL NOT 使用无延迟的重复 `nack(requeue=true)` 形成热循环
- **AND** 第五次处理失败后 SHALL 路由到按 Consumer 和事件类型隔离、保留 7 天的 Dead Letter Queue

#### Scenario: DLQ Recorder 持久化死信
- **WHEN** DLQ Recorder 收到 Consumer Dead Letter Message
- **THEN** Recorder SHALL 以 `(consumer_name, event_id)` 在 MySQL `message_consumer_dead_letters` 创建或更新唯一投影，持久化 Consumer、原队列、最小事件载荷、受控重试头、末次稳定失败码、观察到的受众数量和关联的完整受众快照（如有）后 ACK
- **WHEN** 该唯一投影处于 `replayed` 且同一事件在重放后再次第五次失败
- **THEN** Recorder SHALL 将投影重开为 `pending`、递增 `replay_cycle` 并更新末次失败事实，不创建重复投影
- **AND** 每轮重放与再次死信 SHALL 写入现有 Audit Log
- **WHEN** Recorder 无法持久化记录
- **THEN** Recorder SHALL 不 ACK，并通过自身独立 TTL 重试队列按同一五次策略重试
- **AND** 第五次失败后 SHALL 进入不可自动丢弃、仅由受限运维 CLI 或 RabbitMQ 管理界面处置的运维告警队列
- **AND** 受限运维重放 SHALL 重置受控重试头并定向返回原 DLQ Recorder

#### Scenario: 超级管理员管理 Consumer 死信
- **WHEN** 超级管理员通过 `GET /api/admin/message-dead-letters` 查询 Consumer DLQ 消息
- **THEN** 系统 SHALL 从 MySQL `message_consumer_dead_letters` 分页返回受控元数据
- **AND** 系统 SHALL NOT 使用 RabbitMQ Management API 或直接浏览 AMQP 队列
- **WHEN** 超级管理员通过 `POST /api/admin/message-dead-letters/:id/replay` 重放死信消息
- **THEN** 系统 SHALL 以 `pending → replaying → replayed|pending` 和 30 秒租约抢占记录
- **AND** 系统 SHALL 在 Publisher Confirm 成功后经 durable direct exchange `admin.events.replay`、使用 `consumer.<consumer-name>` routing key 定向返回原 Consumer；`replayed` 仅表示该定向发布成功
- **AND** 具有关联完整受众快照的记录 SHALL 复用该快照；`audience_capacity_exceeded` 记录 SHALL 没有用户快照并重新计算当前动态受众
- **AND** 若重放事件再次第五次失败，DLQ Recorder SHALL 复用同一投影、将 `replayed → pending` 并递增 `replay_cycle`
- **AND** 若重放事件的 `aggregate_version` 低于 Consumer 游标，Consumer SHALL 标记 `superseded`、不写 Redis Stream 后 ACK
- **AND** Confirm 后崩溃导致的重复投递 SHALL 由 Consumer 事件 ID 幂等处理
- **WHEN** 超级管理员通过 `DELETE /api/admin/message-dead-letters/:id` 丢弃死信消息
- **THEN** 系统 SHALL 标记记录为 `discarded` 而不修改消息业务事实
- **AND** 最终保持 `replayed` 与 `discarded` 的记录及关联完整受众快照 SHALL 保留 30 天后物理删除，`pending` 与 `replaying` SHALL NOT 按时间自动删除
- **AND** 查询、重放和丢弃 SHALL 使用 `admin.messages.dead-letter.manage` 并写入审计

#### Scenario: Consumer 死信重放循环审计
- **WHEN** 同一 Consumer 事件在任意 `replay_cycle` 再次进入 Consumer DLQ
- **THEN** 管理查询 SHALL 在唯一投影中返回当前 `replay_cycle` 与末次失败受控元数据
- **AND** 系统 SHALL 通过现有 Audit Log 保留每轮重放、再次死信和最终丢弃或成功重放的处置历史

#### Scenario: Consumer 死信不改变应用就绪
- **WHEN** MySQL 存在 `pending` 或 `replaying` Consumer DLQ 投影
- **THEN** 系统 SHALL NOT 因这些记录改变 `GET /api/ready` 的 HTTP 状态或 `status`
- **AND** 仅具备 `admin.messages.dead-letter.manage` 的超级管理员查询和受控监控指标 SHALL 暴露待处置总数、按 Consumer 与稳定失败码计数及最旧 `pending` 投影年龄
- **AND** 任一 `(Consumer, 稳定失败码)` 的 `pending` 计数从零变为非零且 MySQL 投影提交后，Messaging SHALL 以 best-effort 调用 App 注入的供应商无关 `MessagingMetrics.RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)`；Observation SHALL 仅包含 `consumer_name`、稳定 `failure_code`、`pending_count` 和 `oldest_pending_age`，且方法 SHALL NOT 向 Messaging 返回错误。部署监控系统 SHALL 负责实际告警规则
- **AND** Contract Adapter 异常 SHALL 只写受控运行日志，不得阻止 DLQ Recorder ACK 或触发 AMQP 重试；公开就绪响应和 Observation SHALL NOT 返回事件载荷、用户标识、消息内容、凭据或告警提供商实现细节

### Requirement: 未来通知投影

系统 SHALL 允许未来邮件、短信等进程内 Consumer 以最小消息领域事件引用调用 Messaging Projection Contract 查询当前可发送且未读的投递投影，而不向 RabbitMQ 事件加入正文、邮箱、手机号或静态受众快照。

#### Scenario: Consumer 查询当前投递投影
- **WHEN** App 注入的通知 Consumer 使用消息副本 ID 和事件身份调用 Messaging Projection Contract
- **THEN** Messaging SHALL 仅返回当前可投递且未读用户的用户 ID、显示名、标题、清洗 HTML、服务端派生纯文本、消息副本 ID 和事件版本
- **AND** 通知 Adapter SHALL 通过 Identity Contract 以用户 ID 解析当前渠道地址
- **AND** 已撤销、过期、当前不可见或已读的消息 SHALL NOT 返回可投递内容
- **AND** Projection Contract SHALL NOT 暴露 GORM Model、存储凭据、RabbitMQ Channel、网络 API 或消息原始 Markdown

#### Scenario: 跨组织副本的外部通知不去重
- **WHEN** 同一用户同时符合一个逻辑群发或公告的多个组织副本，并且每个副本均满足当前可投递且未读条件
- **THEN** Projection Contract SHALL 对每个 `message_copy_id` 独立返回该用户
- **AND** 通知 Consumer MAY 为每个副本独立发起外部通知
- **AND** 同一组织副本内命中多个受众规则的用户 SHALL 只返回一次

#### Scenario: 待验证邮箱不接收业务通知
- **WHEN** Identity 用户同时具有当前已验证 `email` 和未确认 `pending_email`
- **THEN** 通知 Adapter SHALL 仅通过 Identity Contract 解析并向当前已验证 `email` 发起业务通知
- **AND** Adapter SHALL NOT 接收或使用 `pending_email`
- **WHEN** 用户确认 `pending_email` 并由 Identity 原子提升为 `email`
- **THEN** 后续渠道解析 SHALL 仅返回新的已验证 `email`

#### Scenario: 延迟外部通知跳过已读用户
- **WHEN** 通知 Consumer 因 RabbitMQ 延迟、重试或重放而查询 `created` 或 `published` 事件，且收件人已经在站内读取对应消息
- **THEN** Projection Contract SHALL 不返回该收件人
- **AND** Consumer SHALL NOT 为该收件人发起邮件或短信投递

#### Scenario: 查询后的已读不撤回在途投递
- **WHEN** Projection Contract 已返回未读收件人，而该用户在外部 Consumer 发起 SMTP 或短信投递前后才将消息标记为已读
- **THEN** Consumer MAY 完成该已在途投递
- **AND** 本 Change SHALL NOT 引入外部投递预约、发送状态或取消机制

#### Scenario: 外部通知允许的生命周期事件
- **WHEN** 通知 Consumer 收到私信 `created` 或管理员群发/公告 `published` 事件
- **THEN** Consumer MAY 查询投递投影并发起外部通知
- **WHEN** Consumer 收到 `edited`、`revoked` 或 `expired` 事件
- **THEN** Consumer SHALL NOT 主动发送邮件或短信

### Requirement: 消息接口错误和审计

系统 SHALL 为消息 HTTP 接口提供统一错误契约和受控审计。

#### Scenario: 消息领域错误
- **WHEN** 请求违反消息权限、状态、受众、分类、媒体或资源可见性规则
- **THEN** 系统 SHALL 返回四字段错误信封
- **AND** 顶层 `error_code` SHALL 使用由 Messaging 拥有的稳定 `MSG_*` 编码
- **AND** 响应 SHALL NOT 包含数据库、RabbitMQ、MinIO、解析器或外链代理原始错误

#### Scenario: 消息操作审计
- **WHEN** 用户或管理员发送、编辑、发布、撤销、变更分类或被拒绝执行消息操作
- **THEN** Audit SHALL 记录操作者、目标、组织、消息副本 ID、动作、结果和稳定原因码
- **AND** Audit SHALL NOT 记录 Markdown、HTML、图片内容、Token 或外链 URL
