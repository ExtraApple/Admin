# api-management Delta Specification

## ADDED Requirements

### Requirement: 消息 API 元数据同步

系统 SHALL 为内部消息 HTTP 路由同步 API 元数据和默认 Permission Code，并保留管理员可配置的启停策略。

#### Scenario: 创建消息 API 元数据
- **WHEN** Route Catalog Snapshot 包含内部消息 HTTP 路由且 `apis` 表不存在对应 Method + Path
- **THEN** API Metadata SHALL 创建包含名称、分组、Access Level 对应认证标记、默认审计标记和默认 Permission Code 的记录
- **AND** `method + path` SHALL 保持唯一

#### Scenario: 同步消息权限码
- **WHEN** 管理员调用 API 权限同步
- **THEN** 系统 SHALL 为消息 Permission Code 创建缺失的权限记录
- **AND** 已存在权限码 SHALL NOT 重复创建
- **AND** WebSocket 成功帧和 RabbitMQ Exchange/Queue SHALL NOT 被错误创建为 API Permission Code

### Requirement: 消息接口动态授权
系统 SHALL 在 API Metadata 启用且权限和组织范围均满足时，才允许调用消息及死信管理入口。

#### Scenario: 消息接口动态授权
- **WHEN** 非超级管理员请求 `/api/admin/messages`、`/api/admin/announcements`、`/api/admin/message-categories` 或死信 Outbox 重放入口
- **AND** API Metadata 已启用
- **AND** 用户拥有对应消息 Permission Code
- **THEN** 系统 SHALL 允许请求进入 Messaging Handler
- **AND** 用户缺少对应权限或超出组织数据范围时 SHALL 拒绝请求

#### Scenario: 死信 Outbox 重放权限
- **WHEN** 请求 `GET /api/admin/message-outboxes` 或 `POST /api/admin/message-outboxes/:id/replay`
- **THEN** API Metadata SHALL 将接口限制为超级管理员
- **AND** 重放接口 SHALL 声明默认 Permission Code `admin.messages.outbox.replay`
- **AND** 系统 SHALL 仅接受 `dead` 状态的 Outbox

#### Scenario: Consumer 死信管理权限
- **WHEN** 请求 Consumer DLQ 查询、重放或丢弃入口
- **THEN** API Metadata SHALL 将接口限制为超级管理员
- **AND** 接口 SHALL 声明默认 Permission Code `admin.messages.dead-letter.manage`

### Requirement: 邮箱验证接口元数据

系统 SHALL 将邮箱验证接口纳入 Route Catalog Snapshot、API Metadata 与 Identity 错误契约。

#### Scenario: 声明邮箱验证接口
- **WHEN** Identity 提供 `POST /api/user/email-verifications` 或 `POST /api/user/email-verifications/confirm`
- **THEN** Route Descriptor SHALL 将接口标记为 Authenticated
- **AND** API Metadata SHALL 使用 Identity 默认审计分类
- **AND** 确认接口 SHALL 使用四字段 JSON 错误响应而非 GET 链接状态变更

#### Scenario: 邮箱验证状态响应
- **WHEN** Identity 返回注册、当前用户资料或用户资料更新成功响应
- **THEN** OpenAPI Schema SHALL 包含 `email`、`pending_email` 和 `email_verified`
- **AND** Schema SHALL NOT 包含验证 token、过期时间或 SMTP 投递状态

#### Scenario: 邮箱变更的当前密码字段
- **WHEN** 用户资料更新请求包含非空 `email`
- **THEN** OpenAPI Request Schema SHALL 要求 `current_password`
- **WHEN** 当前密码缺失或不匹配
- **THEN** 用户资料更新 Route Descriptor SHALL 声明 HTTP 422 与 `current_password` 字段的稳定 `IDENTITY_CURRENT_PASSWORD_INVALID` 错误

#### Scenario: 注册验证邮件不可用
- **WHEN** 新用户注册后 SMTP 无法接收自动验证邮件
- **THEN** 注册 Route Descriptor SHALL 声明稳定 HTTP 503 Identity 错误
- **AND** 错误响应 SHALL NOT 暴露邮箱、SMTP 连接信息或原始失败原因

#### Scenario: 未验证邮箱替换验证邮件不可用
- **WHEN** 未验证用户的邮箱直接替换已提交后，SMTP 无法接收自动验证邮件
- **THEN** 用户资料更新 Route Descriptor SHALL 声明稳定 HTTP 503 Identity 错误
- **AND** 错误响应 SHALL NOT 暴露邮箱、SMTP 连接信息、原始失败原因或已提交状态

#### Scenario: 待验证邮箱创建或替换验证邮件不可用
- **WHEN** 已验证用户的 `pending_email` 已创建或替换后，SMTP 无法接收自动验证邮件
- **THEN** 用户资料更新 Route Descriptor SHALL 声明稳定 HTTP 503 Identity 错误
- **AND** 错误响应 SHALL NOT 暴露邮箱、SMTP 连接信息、原始失败原因或已提交状态

#### Scenario: 替换待验证邮箱的候选地址冲突
- **WHEN** 已验证用户替换已有 `pending_email` 时提交已被占用的候选邮箱
- **THEN** 用户资料更新 Route Descriptor SHALL 声明稳定 HTTP 409 Identity 错误
- **AND** 错误响应 SHALL NOT 暴露占用用户、候选邮箱或原 `pending_email` 的提交状态

#### Scenario: 验证确认与节流响应
- **WHEN** 邮箱验证确认成功
- **THEN** OpenAPI Schema SHALL 声明更新后的安全用户资料响应
- **WHEN** 用户在滚动一小时内第四次自动或显式请求验证邮件
- **THEN** 相关 Route Descriptor SHALL 声明 HTTP 429、稳定 Identity 错误和 `Retry-After` Header
- **AND** 注册投递失败的 HTTP 503 错误 data Schema SHALL NOT 包含 `account_created`、用户 ID、邮箱或 token

#### Scenario: 邮箱验证冲突响应
- **WHEN** 确认时 `pending_email` 已被占用，或已验证且没有候选邮箱的用户请求重发
- **THEN** Route Descriptor SHALL 声明稳定 HTTP 409 Identity 错误
- **AND** 错误响应 SHALL NOT 暴露占用用户、候选邮箱或 token
