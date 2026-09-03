## 1. 基础设施与模块骨架

- [x] 1.1 使用 Goldmark 编译 Markdown、Bluemonday 按白名单清洗 HTML，并确认图片处理依赖，避免重复引入
- [x] 1.2 增加 RabbitMQ 连接、vhost、TLS、重试、死信、动态受众 100,000 上限和消息内容上限配置
- [x] 1.3 创建 `internal/messaging` 模块及 Domain、Application、Adapter 分层骨架
- [x] 1.4 在 App 组合根装配 Messaging、RabbitMQ Publisher、Redis、Outbox Worker、竞争 Consumer、供应商无关且 best-effort 的 `MessagingMetrics` Adapter 和仅 RabbitMQ 可降级的 `/api/ready` 状态；Consumer DLQ 只经受保护观测与 Metrics Contract 上报，不影响就绪或 DLQ Recorder ACK
- [x] 1.5 扩展架构边界测试，允许 `messaging` 一级模块并禁止跨模块 Adapter 依赖
- [x] 1.6 实现并验证 RabbitMQ 客户端与本地 Broker 的真实联通；不声明拓扑、不发布事件

## 2. 消息领域模型与持久化

- [x] 2.1 定义私信、管理员群发、通知公告、草稿、定时、发布、过期和撤销领域状态
- [x] 2.2 定义消息主体、组织副本、分类、动态受众和私信收件关系模型
- [x] 2.3 定义用户消息状态、未读计算和私信删除墓碑模型
- [x] 2.4 定义消息 Outbox、事件消费幂等、持久化 Consumer 版本游标、由 `message_event_consumptions` 以独立短事务承载单调 `snapshot_fence` 和 30 秒可续租租约的首次消费动态受众快照、`audience_capacity_exceeded` 无部分用户快照/记录观察数量并重算受众的例外、以 `(consumer_name, event_id)` 唯一并可递增 `replay_cycle` 重新打开的 Consumer DLQ 投影和并发抢占模型
- [x] 2.5 增加 MySQL 索引、唯一约束和 AutoMigrate/迁移注册
- [x] 2.6 验证跨组织副本和分类编码缺失时的事务全回滚

## 3. 跨模块 Contract 与 Repository

- [x] 3.1 定义 Messaging 调用 Identity 的用户身份、启用状态和通知渠道地址解析 Contract
- [x] 3.2 定义 Messaging 调用 Organization 的组织成员、子组织和角色受众 Contract
- [x] 3.2a 保存并在成员覆盖时保持组织成员加入时间，供公告首次可见已读判定
- [x] 3.3 定义 Messaging 调用 Authorization 的权限与消息组织范围 Contract
- [x] 3.4 定义 Messaging 调用 Files 的消息图片用途和受可见性控制读取 Contract
- [x] 3.5 定义 Messaging 调用 Audit 的受控元数据写入 Contract
- [x] 3.6 实现消息、分类、受众、收件关系、用户状态、带 Worker 租约的 Outbox、事件消费幂等/版本游标、`REPEATABLE READ` 500 用户分批/100,000 上限、由消费记录短事务续租且以 `snapshot_fence` 保护完成条件的动态受众完整快照、容量超限无用户快照及重算当前受众、以 Consumer/事件唯一并可从 `replayed` 重新打开的 Consumer DLQ 投影 Repository
- [x] 3.7 增加 Repository 分页、筛选、唯一约束、动态受众查询、跳锁抢占、Outbox/消费记录快照租约短事务续期与 `snapshot_fence` 围栏、Consumer 游标并发、完整受众快照/关联 DLQ 30 天清理、容量超限观察数量及重算、DLQ 投影重放状态与 `replay_cycle` 重新打开验证

- [x] 3.8 定义 App 注入的供应商无关 `MessagingMetrics.RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)` 窄 Contract：仅含 Consumer、稳定失败码、待处置计数与最旧年龄且不返回错误；不增加 Prometheus、OTel、Alertmanager、Webhook、SMTP 或告警发送器依赖

## 4. 内容安全与消息图片

- [x] 4.1 实现标题和 Markdown 正文长度校验
- [x] 4.2 实现 Markdown 到受限 HTML 的转换和 XSS 清洗
- [x] 4.3 实现危险协议、脚本、事件属性和不允许标签的拒绝测试
- [x] 4.4 增加消息图片专用用途并复用 Files 安全验证链路
- [x] 4.5 实现 JPEG、PNG、WebP、5 MiB 和 4,096 像素边界验证
- [x] 4.6 实现消息图片读取时的当前消息可见性校验
- [x] 4.7 实现 HTTPS 公网外链代理、SSRF 防护、重定向限制、超时和 MIME 限制
- [x] 4.8 验证普通文件接口仍拒绝图片且不暴露消息图片对象

## 5. 用户私信与收件箱

- [x] 4.12 为 Application 层增加单元测试：私信同组织校验、私信可追溯、动态受众生成/上限/无匹配、公告编辑窗口、分类管理、删除语义、幂等事件、过期发布/撤回/过期 WebSocket 拒绝、本人删除/管理员撤销、External proxy 覆盖状态映射、受控 Audit 元数据、受保护 Admin Permission Matrix、消息图本归属可见性和供应商无关 Metrics Contract
- [x] 5.2 实现收件箱分页、详情、关键词、分类、类型和已读筛选
- [x] 5.3 实现私信已读、未读总数和按消息类型统计
- [x] 5.4 实现私信收件关系删除墓碑并保持消息主体不变
- [x] 5.5 实现普通用户撤销自己发送的私信
- [x] 5.6 实现动态群发/公告可见性和懒创建用户消息状态
- [x] 5.7 验证受众变化、撤销、过期和删除后的可见性边界

## 6. 管理员群发、公告与分类

- [x] 6.1 实现组织、角色和全体用户动态受众解析；同一组织副本内去重用户，跨目标组织副本保持独立收件箱、未读和事件投递
- [x] 6.2 实现普通管理员所属组织及子组织范围校验
- [x] 6.3 实现超级管理员跨组织复制和分类编码匹配
- [x] 6.4 实现管理员群发创建、管理查询和发送后不可编辑
- [x] 6.5 实现公告草稿、定时发布、立即发布和有效期任务
- [x] 6.6 实现公告发布后编辑、已读保留和撤销终态
- [x] 6.7 实现组织级分类 CRUD、停用和已引用分类删除保护
- [x] 6.8 实现组织管理员普通用户消息撤销、公告发布者撤销自己的公告、超级管理员全局撤销，并验证三方不能逾越对方边界
- [x] 6.9 验证权限码、消息类型、发送者身份和组织范围组合矩阵

## 7. RabbitMQ Outbox 与消费者

- [x] 7.1 实现持久化 Topic Exchange、全 Quorum WebSocket/TTL 重试/DLX 队列、durable direct `admin.events.replay` 与绑定声明
- [x] 7.2 实现 5 秒连接、10 秒 Confirm、30 秒可续租租约的 Outbox Worker 跳锁批量抢占、Publisher Confirm 和发布状态更新
- [x] 7.3 实现 RabbitMQ 连接失败、Confirm 超时和有界退避重试
- [x] 7.4 实现竞争 Consumer、`prefetch=1`、`message_event_consumptions` 独立短事务续租且以单调 `snapshot_fence` 围栏的快照租约、`REPEATABLE READ` 500 用户/100,000 上限封存完整动态受众快照、失租停止/新围栏接管、容量超限无部分投递/五级重试/重算当前受众、低于版本游标的 Consumer DLQ 旧版本重放 `superseded`、Redis Lua Stream 去重、24 小时或关联 DLQ 30 天完整快照清理、持久化版本游标/事件幂等和重复事件安全处理
- [x] 7.5 实现受控 `x-retry-attempt`、独立 Quorum TTL 重试队列、第五次失败后的隔离 Dead Letter Queue、含最终容量错误与观察数量的 MySQL DLQ 投影、DLQ Recorder 告警队列及其原 Recorder 重放、`admin.events.replay`、30 秒重放租约、完整快照复用与容量超限重算例外、以 Consumer/事件唯一且 `replay_cycle` 重新打开的超级管理员 Consumer DLQ 处置、通过 best-effort `MessagingMetrics` 上报待处置总数/按 Consumer 和失败码/最旧年龄、每个 Consumer/失败码首条 `pending` 的立即告警观测及受控运行日志
- [x] 7.6 验证 RabbitMQ 不可用时消息事务成功、Outbox 保留并恢复后补发
- [x] 7.7 定义 App 注入的进程内邮件/短信 Consumer Messaging Projection Contract、允许通知的生命周期事件、跨目标组织副本不去重及延迟/重试/重放时跳过查询时已读收件人、已查询后的在途投递不撤回的规则，不创建空操作消费者、外部投递预约或第三方适配器

## 8. WebSocket Gateway 与恢复

- [x] 8.1 实现一次性 60 秒 WebSocket ticket 签发和 Redis 摘要存储
- [x] 8.2 实现 ticket 重放、过期、用户绑定和错误脱敏拒绝
- [x] 8.3 实现 WebSocket 升级入口和连接生命周期管理
- [x] 8.4 实现 RabbitMQ 事件写入 24 小时 Redis Stream 恢复缓存
- [x] 8.5 实现 Redis Pub/Sub 到在线 Gateway 节点的刷新提示分发
- [x] 8.6 实现游标补发、幂等和超出窗口全量刷新控制事件
- [x] 8.7 验证 WebSocket 事件不携带正文、Token、图片或外链 URL
- [x] 8.8 验证断线、重连、撤销、编辑、过期和动态受众变化行为

## 9. HTTP Adapter、路由与 API 元数据

- [x] 9.1 定义 Messaging DTO、稳定 `MSG_*` 错误和四字段 HTTP 映射
- [x] 9.2 实现用户私信、收件箱、已读、删除、撤销和图片 HTTP Handler
- [x] 9.3 实现 WebSocket ticket 和升级 Handler
- [x] 9.4 实现管理员群发、公告、撤销、分类、超级管理员死信 Outbox 查询/重放和 Consumer DLQ 查询/重放/丢弃 HTTP Handler
- [x] 9.5 为全部 HTTP、WebSocket 和 `/api/ready` 入口创建完整 Route Descriptor
- [x] 9.6 增加消息路由 API Metadata 默认 Permission Code 和审计分类
- [x] 9.7 更新 API 权限同步、OpenAPI Schema 和原生 WebSocket 描述
- [x] 9.8 验证 Route Catalog、Gin 路由、API Metadata 和权限表集合一致

## 10. 审计、运行日志与安全脱敏

- [x] 10.1 记录消息发送、编辑、发布、撤销、分类变更和权限拒绝审计元数据
- [x] 10.2 记录 Outbox、RabbitMQ 发布、消费、重试、死信和 Consumer DLQ 处置运行日志，并通过无返回值的 best-effort `MessagingMetrics.RecordConsumerDLQPending` 暴露受控 Consumer/失败码/待处置计数/最旧 `pending` 年龄；每个 Consumer/失败码从零到首条 `pending` 时立即记录不含事件载荷的告警观测，Adapter 失败不阻塞 ACK 或触发 AMQP 重试
- [x] 10.3 脱敏 WebSocket ticket、Authorization、消息正文、图片内容和外链 URL
- [x] 10.4 验证审计和运行日志不泄露 Markdown、HTML、对象存储路径或基础设施错误

## 11. 契约与集成验证

- [x] 11.1 增加消息 Domain/Application 状态机和权限矩阵测试
- [x] 11.2 增加消息 HTTP 四字段响应、错误码和 OpenAPI 契约测试
- [x] 11.3 增加真实 RabbitMQ 连接/Confirm 时限、Quorum TTL 重试、竞争 Consumer 一致读 500 用户/100,000 上限完整受众快照、`snapshot_fence` 失租接管与 Lua Stream 幂等、容量超限无部分投递/重算、处理更高版本后旧 Consumer DLQ 重放 `superseded` 且不倒置刷新、DLQ Recorder/告警队列及重放、`admin.events.replay`、30 秒重放租约、Consumer DLQ 处置与同一投影 `replay_cycle` 再次死信、仅受控 Observation 的无返回值 best-effort `MessagingMetrics.RecordConsumerDLQPending` 与 Adapter 失败不阻塞 ACK、死信不降级 `/api/ready` 和超级管理员人工重放集成测试
- [x] 11.4 增加 Outbox 跳锁抢占、续租/租约恢复、重启补发、事件幂等、消费记录短事务续租不阻塞 `REPEATABLE READ` 快照及围栏丢失/接管恢复、容量上限例外、Consumer DLQ `replay_cycle` 重新打开及终态清理测试
- [x] 11.5 增加 WebSocket ticket、游标恢复和超窗全量刷新测试
- [x] 11.6 增加 Markdown、图片、SSRF、重定向、MIME 和日志脱敏安全测试
- [x] 11.7 增加跨组织事务、目标组织重叠用户的独立副本/未读/事件投递、动态受众、未读语义和撤销边界测试
- [x] 11.8 运行受影响模块测试和完整 Go 测试套件
- [x] 11.9 更新 CONTEXT、模块导航、OpenSpec 主规格、部署运行说明和 DLQ Recorder 告警队列 CLI/管理界面处置手册

## 12. Identity 邮箱验证

- [x] 12.1 增加 SMTP host、port、账号、发件人、10 秒默认且可覆盖的超时与 `disabled`/`starttls_required`/`implicit` TLS 模式配置，并引入 go-mail
- [x] 12.2 增加 `pending_email`、`email_verified_at` 与一次性邮箱验证凭据模型及迁移
- [x] 12.3 实现 32 字节 token 摘要、15 分钟有效期、一次性使用、请求替换、自动/显式签发共用且失败也计数的每用户每小时三次节流、额度耗尽时不变更邮箱状态、终态 24 小时清理与 429 `Retry-After`
- [x] 12.4 实现注册、未验证当前邮箱直接替换及待验证邮箱创建/替换后的同步 SMTP 发送、仅安全错误码的账户/新邮箱/候选邮箱保留 503、发送失败 token 失效和受控重发
- [x] 12.5 实现 `PUT /api/user/info` 邮箱变更的 `current_password` 再认证、候选邮箱冲突 409、未验证当前邮箱的原子直接替换与旧凭据失效、确认/替换的事务串行化，以及已验证邮箱的原子 `pending_email` 创建/替换与提升；返回更新后的安全用户资料且不改变未验证用户登录能力
- [x] 12.6 定义 Messaging 调用 Identity 的已验证邮箱通知渠道 Contract：未验证当前邮箱更换不产生可投递旧地址；`pending_email` 期间继续仅返回当前已验证 `email`，确认原子提升后才切换；沿用现有邮箱校验/唯一性，保持手机号、短信和用户偏好为非目标
- [x] 12.7 增加安全邮箱验证状态 DTO、已认证签发/确认 HTTP 路由、邮箱变更 `current_password` 请求/422 字段错误、管理员邮箱字段拒绝、四字段错误、审计/日志脱敏和 SMTP 配置验证
- [x] 12.8 增加邮箱验证状态机、邮箱变更再认证、自动/显式签发共用节流及额度耗尽不变更状态、SMTP 失败后账户/新邮箱/候选邮箱保留、24 小时清理、迁移、未验证当前邮箱的原子替换与旧凭据失效、`pending_email` 原子替换/冲突/提升及确认并发、旧已验证邮箱业务通知与确认后渠道切换、脱敏测试

## 13. Code Review 整改

- [x] 13.1 修复公告 draft-first 生命周期：创建始终为 `draft`，定时发布和立即发布均通过显式发布动作完成，并补充回归测试
- [x] 13.2 接通消息图片 `image_ids`：在消息创建、编辑或发布事务中绑定当前操作者持有且未过期的临时图片，并补充端到端测试
- [x] 13.4 增加邮箱确认与邮箱变更的同一用户事务锁，保证先提交状态生效并补充交叉并发测试
- [x] 13.3 修复动态受众快照的原子性：使用覆盖受众解析与完整快照写入的单一 `REPEATABLE READ` 事务；租约或 `snapshot_fence` 失效时不得保留部分投递行、写入 Redis Stream 或 ACK，并补充故障注入与 MySQL 并发测试
- [x] 13.5 将邮箱签发额度检查与凭据写入合并为原子操作，保证自动签发和显式重发共享每小时三次上限
- [x] 13.6 移除 Messaging Application 对 WebSocket SDK 的直接依赖，将升级和协议状态码转换下沉到 Adapter
- [x] 13.7 按项目约定将 Messaging 跨模块 Contract 整理到单一 `contracts.go`
- [x] 13.8 将 App 后台 Messaging 日志统一接入受控 `RuntimeLogger` Contract，并限制为允许字段
- [ ] 13.10 按 OpenSpec 流程处理主规格同步：完成实现和验证后 archive 当前 Change，再合并 delta 到主规格
- [x] 13.9 清理 `CONTEXT.md` 中的 RabbitMQ、WebSocket、DLQ 和运行时实现细节
- [x] 13.11 评估 RabbitMQ Publisher 的重复发布流程，抽取共享实现或记录明确保留重复的理由
- [x] 13.12 运行整改回归测试、完整 Go 测试和严格 OpenSpec 校验
- [x] 13.13 处理非法 RabbitMQ 事件：按受控五级重试进入 Consumer DLQ Projection；仅保存安全元数据和稳定 fingerprint，不保存原始 payload；非法事件 Projection 仅允许查询和丢弃，不允许重放，并补充 RabbitMQ、Repository 和 HTTP 契约测试
- [x] 13.14 在隔离 RabbitMQ vhost 和 MySQL 数据库中执行 `rabbitmq_integration`、`mysql_integration` 门禁，并记录命令、环境前置条件和通过结果
