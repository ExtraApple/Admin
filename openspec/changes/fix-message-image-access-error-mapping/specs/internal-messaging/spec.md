## MODIFIED Requirements

### Requirement: 内容、图片和外链安全

系统 SHALL 只持久化通过安全策略的清洗 HTML，不持久化原始 Markdown。消息图片使用专用临时用途、15 分钟绑定期和 JPEG/PNG/WebP 完整校验，并按当前消息可见性读取；普通文件入口不得暴露消息图片。对已通过消息可见性检查但图片未找到、未绑定、已登记清理或访问不合法的读取请求，系统 SHALL 返回稳定 `MSG_NOT_FOUND`；对象存储不可用、数据库失败或未知基础设施失败 SHALL 返回稳定 `MSG_INTERNAL_ERROR`，且不得返回 Files 原始错误或存储定位。外链代理只允许 HTTPS 公网目标，拒绝私网、环回、云元数据、不受控重定向、超时、超大响应和 MIME 不匹配。

#### Scenario: 消息内容安全
- **WHEN** 用户提交消息正文、图片或外链
- **THEN** 系统 SHALL 只持久化或返回通过安全策略验证的内容

#### Scenario: 消息图片不可访问
- **WHEN** 当前消息对用户可见但图片未找到、未绑定、已登记清理或访问状态不合法
- **THEN** 系统 SHALL 返回 HTTP 404 和 `MSG_NOT_FOUND`
- **AND** SHALL NOT 返回图片内容、Files 错误、对象路径或存储状态

#### Scenario: 消息图片存储失败
- **WHEN** 当前消息对用户可见且图片读取遇到存储、数据库或未知基础设施失败
- **THEN** 系统 SHALL 返回 HTTP 500 和 `MSG_INTERNAL_ERROR`
- **AND** SHALL NOT 返回原始基础设施错误或对象定位

### Requirement: 错误、审计和就绪边界

消息 HTTP 入口 SHALL 使用四字段 JSON envelope 和稳定 `MSG_*` 错误码；Route Catalog、API Metadata、权限同步和 OpenAPI SHALL 使用同一份已校验快照。关键消息操作、权限拒绝、Outbox/Consumer/DLQ 处置 SHALL 写入受控审计或运行日志；日志不得写入正文、HTML、图片、外链 URL、Token、凭据或基础设施原始错误。消息图片的 Files-owned 错误 SHALL 在 App 组合根翻译为 Messaging-owned 稳定错误，不跨模块暴露 Files 实现。`/api/ready` 只表达进程和 RabbitMQ/Outbox 就绪状态，不承担 Consumer DLQ 告警。

#### Scenario: 消息错误与审计
- **WHEN** 消息 HTTP 操作成功或被拒绝
- **THEN** 系统 SHALL 返回稳定错误契约并记录受控审计或运行元数据
