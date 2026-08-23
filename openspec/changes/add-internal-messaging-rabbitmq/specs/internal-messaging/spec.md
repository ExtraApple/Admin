# internal-messaging Specification

## Purpose

内部消息模块负责私信、管理员群发、通知公告、组织级消息分类、动态受众、用户收件箱、已读/撤销状态和 RabbitMQ 异步事件。MySQL 是消息事实来源，RabbitMQ 只负责后端事件传输，WebSocket 只向已认证客户端提供收件箱刷新提示。

## Requirements

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

#### Scenario: 超级管理员跨组织撤销
- **WHEN** 超级管理员撤销任意组织副本的消息
- **THEN** 系统 SHALL 允许操作并记录审计元数据
- **AND** 系统 SHALL 只修改目标消息副本及其关联状态

### Requirement: Markdown 和消息媒体安全

系统 SHALL 在持久化或代理返回前验证并清洗消息内容、图片和外链。

#### Scenario: Markdown 转换成功
- **WHEN** 请求提交标题不超过 100 个 Unicode 字符且 Markdown 正文不超过 20,000 个 Unicode 字符
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
- **THEN** 系统 SHALL 复用 File Record 模型保存消息图片用途记录
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

### Requirement: RabbitMQ Outbox 事件

系统 SHALL 使用 MySQL Outbox 和 RabbitMQ 传输消息领域事件。

#### Scenario: 消息事务写入 Outbox
- **WHEN** 消息创建、发布、编辑、撤销或过期状态变更成功
- **THEN** 系统 SHALL 在同一 MySQL 事务写入唯一事件 ID 的 Outbox 记录
- **AND** RabbitMQ 暂时不可用 SHALL NOT 回滚已提交的消息事实

#### Scenario: Publisher Confirm 成功
- **WHEN** Outbox Worker 向持久化 Topic Exchange 发布事件并收到 Publisher Confirm
- **THEN** 系统 SHALL 标记 Outbox 已发布
- **AND** 同一事件 SHALL NOT 被正常重发

#### Scenario: Publisher Confirm 失败
- **WHEN** RabbitMQ 连接失败、Confirm 超时或 Broker 拒绝事件
- **THEN** 系统 SHALL 保留未发布 Outbox
- **AND** 系统 SHALL 按有界重试策略再次发布
- **AND** 超过重试上限 SHALL 进入 Dead Letter Exchange 并记录受控错误

#### Scenario: 消费者幂等 ACK
- **WHEN** WebSocket Consumer 收到事件
- **AND** 事件 ID 尚未被该消费者处理
- **THEN** 消费者 SHALL 记录幂等状态、写入 24 小时恢复缓存并 ACK
- **WHEN** 同一事件再次投递
- **THEN** 消费者 SHALL 不重复产生业务状态变化但仍可安全 ACK

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
