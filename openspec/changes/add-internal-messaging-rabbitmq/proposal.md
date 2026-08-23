## Why

Admin 目前没有内部消息能力，无法支持用户私信、管理员群发、通知公告、收件箱、已读、撤销和组织级分类。消息还需要为后续邮件、短信等通知能力提供统一的异步事件出口；RabbitMQ 已确定作为事件传输层，但消息业务事实必须继续由 MySQL 维护，避免把队列误当作收件箱数据库。

## What Changes

- 新增内部消息能力，支持一对一私信、管理员群发和通知公告。
- 新增消息收件箱、分页查询、详情、未读查询、标记已读和私信收件关系删除。
- 支持组织、角色和全体用户动态受众；私信发送时校验双方属于同一组织。
- 支持组织级消息分类，使用组织内唯一编码、名称、排序和启用状态；已被消息引用的分类只能停用。
- 支持私信撤销、组织管理员范围内普通用户消息撤销，以及超级管理员跨组织撤销。
- 支持公告草稿、定时发布、有效期、发布后编辑和撤销。
- 支持 Markdown 转受限 HTML；标题、正文和消息图片执行大小、格式及安全策略。
- 新增消息图片专用用途，复用 File Record 数据模型；普通文件接口继续拒绝图片。
- 新增受消息可见性控制的图片读取和 HTTPS 外链代理，阻止 SSRF、危险重定向和不受控资源访问。
- 新增 WebSocket Gateway，通过一次性短期 ticket 鉴权，发送可恢复的“收件箱需要刷新”事件，不直接向浏览器暴露 RabbitMQ。
- 使用 MySQL Transactional Outbox 将消息事务与异步事件发布解耦；RabbitMQ 不可用时消息写入继续成功，实时推送延迟并在恢复后补发。
- 使用 RabbitMQ 持久化 Topic Exchange、独立 Durable Queue、Publisher Confirms、消费者手动 ACK、重试和 Dead Letter Exchange。
- 扩展 Identity 的邮箱验证：仅支持邮箱，使用一次性随机 token、`pending_email` 与最小 SMTP Adapter；新注册及邮箱变更后自动发送，受控重发后完成验证才可获得外部通知渠道资格。
- 预留邮件、短信消费者的事件扩展 Contract，但本 Change 除邮箱验证 SMTP 外不实现第三方邮件、短信适配器或发送逻辑。
- 增加消息操作和权限拒绝的元数据审计，不记录 Markdown、HTML、图片内容或外链 URL。
- **BREAKING**：固定一级业务模块清单、权限码、路由元数据、数据库表和 RabbitMQ 拓扑将新增内部消息相关事实；新增接口必须遵守统一四字段 JSON 响应契约，WebSocket 成功通信保持原生协议。

## Capabilities

### New Capabilities

- `internal-messaging`: 私信、管理员群发、通知公告、动态受众、收件箱、已读、撤销、分类和消息图片访问。

### Modified Capabilities

- `file-management`: 增加消息图片专用用途和受消息可见性控制的图片访问，同时保持普通文件图片上传接口拒绝规则。
- `logging`: 将内部消息发送、编辑、发布、撤销、分类变更和权限拒绝纳入元数据审计，禁止记录正文和外链。
- `modular-layered-architecture`: 增加内部消息模块及其所有权、跨模块 Contract 和 RabbitMQ/Outbox 装配边界。
- `route-catalog`: 支持新消息 HTTP 路由和 WebSocket 升级入口的静态声明、访问等级及原生协议描述。
- `api-management`: 同步新消息 API 元数据、权限码和动态权限策略。

## Impact

- **代码模块**：新增 `internal/messaging`；修改 `internal/app`、`internal/platform`、`internal/files`、`internal/audit`、`internal/routecatalog`、`internal/apimetadata` 及相关 HTTP/基础设施 Adapter。
- **数据库**：新增消息主体、分类、受众、收件箱状态、Outbox、事件幂等和 WebSocket ticket 所需的数据结构；具体迁移边界在 design 和 specs 中确定。
- **RabbitMQ**：新增连接配置、凭据管理、Topic Exchange、WebSocket Durable Queue、重试拓扑和 Dead Letter Exchange。生产环境目标为 RabbitMQ Cluster + Quorum Queue，开发环境允许单节点。
- **HTTP 接口**：新增用户收件箱和私信接口、管理员消息/公告/分类接口、消息图片接口及 WebSocket ticket 接口；具体 Method、Path、权限码在 design 和 specs 中列出。
- **协议**：业务 JSON 使用现有四字段响应信封；WebSocket 使用原生升级和事件协议；浏览器不直接连接 RabbitMQ。
- **外部依赖**：RabbitMQ Go 客户端、Markdown/HTML 清洗能力和图片验证能力；实施前必须复用或确认仓库已有依赖，避免无依据引入重复库。
- **运行与验证**：需要 RabbitMQ 集成测试、Outbox 重试与幂等测试、WebSocket 断线恢复测试、动态受众权限测试、图片/外链安全测试和消息审计测试。

### 非目标

- 本 Change 不实现邮件发送、短信发送、第三方供应商配置或模板管理。
- RabbitMQ 不保存消息正文、已读状态、撤销状态或收件箱事实。
- 不实现浏览器直连 RabbitMQ、Web STOMP 或 Web MQTT。
- 不实现聊天在线状态、Typing、群聊会话、文件附件通用能力或移动端推送。
- 不改变普通文件接口继续拒绝图片的既有安全边界。
