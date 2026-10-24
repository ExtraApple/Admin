## MODIFIED Requirements

### Requirement: API 审计日志

系统 SHALL 自动记录 `/api/*` 请求到 `audit_logs` 表，并为文件和头像上传记录不包含文件内容的结构化安全元数据。消息私信、群发、公告、编辑、发布、分类和消息图片 JSON 请求的审计 Body SHALL 使用固定 `[message omitted]` 标记；multipart 请求 SHALL 使用固定 `[multipart omitted]` 标记。

#### Scenario: 普通 API 请求被记录
- **WHEN** 用户调用 `/api/*` 接口
- **THEN** 系统 SHALL 记录 method、path、query、status、duration、client_ip、user_agent、created_at
- **AND** 如果请求上下文中存在 userID，系统 SHALL 记录 user_id

#### Scenario: 消息请求体省略
- **WHEN** 用户或管理员调用消息私信、群发、公告、编辑、发布、分类或消息图片 JSON 接口
- **THEN** Audit Body SHALL 等于固定 `[message omitted]`
- **AND** SHALL NOT 包含标题、Markdown、HTML、图片引用、正文或外链 URL

#### Scenario: 审计 Query allowlist
- **WHEN** `/api/*` 请求包含查询参数
- **THEN** Audit Query SHALL 只保留 `page`、`size`、`user_id`、`method`、`path`、`status`、`category`、`start_time` 和 `end_time`
- **AND** SHALL 丢弃 `signature`、`expires`、ticket、Token、Authorization 及其他未批准参数

#### Scenario: multipart 请求
- **WHEN** 请求 Content-Type 为 `multipart/form-data`
- **THEN** 系统 SHALL NOT 读取或记录文件内容
- **AND** 请求 Body SHALL 使用固定 `[multipart omitted]` 标记

#### Scenario: 上传验证成功被记录
- **WHEN** 普通文件或头像上传通过验证
- **THEN** 系统 SHALL 在审计 metadata 中记录上传用途、清洗后的文件名、文件大小、声明 MIME、检测 MIME、`accepted` 结果和策略版本
- **AND** 系统 SHALL NOT 记录二进制内容、预签名或签名 URL、MinIO 凭据、object key 或原始客户端路径

#### Scenario: 上传被安全策略拒绝
- **WHEN** 普通文件或头像上传因大小、类型、编码、图片或 Office 安全策略被拒绝
- **THEN** 系统 SHALL 在审计 metadata 中记录 `rejected` 结果和稳定原因码
- **AND** 系统 SHALL NOT 记录图片解析器、ZIP/XML 解析器、数据库或 MinIO 的原始内部错误

### Requirement: 敏感请求和审计数据脱敏

系统 SHALL 不在普通请求日志、审计日志或错误响应中保存 Authorization、WebSocket ticket、消息正文、图片内容、外链 URL、对象存储路径或基础设施原始错误。审计 Query SHALL 使用显式 allowlist，而不是仅依赖敏感字段 denylist；历史 Audit Log SHALL 保留，不由本 change 自动删除或回写。

#### Scenario: 敏感 HTTP 请求
- **WHEN** 请求包含 Authorization、ticket、消息正文、图片或外链参数
- **THEN** 请求日志和审计记录 SHALL 省略或替换敏感值
- **AND** HTTP 错误响应 SHALL 只返回稳定错误码和安全提示

### Requirement: 内部消息操作审计

系统 SHALL 将内部消息关键业务操作、权限拒绝及 RabbitMQ/Outbox/DLQ 处置写入现有 Audit Log 或受控运行日志，并仅保存稳定安全元数据。消息审计 SHALL 保留 action、目标消息副本引用、操作者、组织、结果和稳定原因码等允许字段，但不得保留正文、HTML、图片内容、完整外链 URL 或未批准查询参数。

#### Scenario: 消息操作和拒绝审计
- **WHEN** 用户或管理员发送、编辑、发布、撤销消息、修改分类，或因权限、组织范围、受众、分类或状态规则被拒绝
- **THEN** Audit SHALL 记录操作者、目标消息副本、组织、动作、结果、稳定原因码和时间
- **AND** Audit SHALL NOT 记录 Markdown、HTML、图片内容或完整外链 URL
