## MODIFIED Requirements

### Requirement: API 审计日志
系统 SHALL 自动记录 `/api/*` 请求到 `audit_logs` 表，并为文件和头像上传记录不包含文件内容的结构化安全元数据。

#### Scenario: 普通 API 请求被记录
- **WHEN** 用户调用 `/api/*` 接口
- **THEN** 系统 SHALL 记录 method、path、query、status、duration、client_ip、user_agent、created_at
- **AND** 如果请求上下文中存在 userID，系统 SHALL 记录 user_id

#### Scenario: 请求体脱敏
- **WHEN** 请求体为 JSON 且包含密码、token 或验证码字段
- **THEN** 系统 SHALL 将敏感字段值记录为 `***`

#### Scenario: multipart 请求
- **WHEN** 请求 Content-Type 为 `multipart/form-data`
- **THEN** 系统 SHALL NOT 读取或记录文件内容
- **AND** 请求 Body SHALL 使用固定省略标记

#### Scenario: 上传验证成功被记录
- **WHEN** 普通文件或头像上传通过验证
- **THEN** 系统 SHALL 在审计 metadata 中记录上传用途、清洗后的文件名、文件大小、声明 MIME、检测 MIME、`accepted` 结果和策略版本
- **AND** 系统 SHALL NOT 记录二进制内容、预签名或签名 URL、MinIO 凭据、object key 或原始客户端路径

#### Scenario: 上传被安全策略拒绝
- **WHEN** 普通文件或头像上传因大小、类型、编码、图片或 Office 安全策略被拒绝
- **THEN** 系统 SHALL 在审计 metadata 中记录 `rejected` 结果和稳定原因码
- **AND** 系统 SHALL NOT 记录图片解析器、ZIP/XML 解析器、数据库或 MinIO 的原始内部错误

#### Scenario: 在文件名不可用前拒绝
- **WHEN** 请求在取得并清洗有效文件名之前因 multipart 或请求体错误被拒绝
- **THEN** 系统 SHALL 记录上传用途和稳定原因码
- **AND** 文件名 metadata SHALL 为空或使用安全占位值，不得记录未清洗路径

### Requirement: 审计日志查询
系统 SHALL 为管理员提供审计日志查询接口，并返回已持久化的上传安全 metadata。

#### Scenario: 查询全部 API 日志
- **WHEN** 管理员请求 `GET /api/admin/audit-logs`
- **THEN** 系统 SHALL 分页返回审计日志
- **AND** 存在结构化 metadata 的日志 SHALL 返回该字段

#### Scenario: 查询登录日志
- **WHEN** 管理员请求 `GET /api/admin/login-logs`
- **THEN** 系统 SHALL 返回 category 为 `login` 的审计日志

#### Scenario: 查询操作日志
- **WHEN** 管理员请求 `GET /api/admin/operation-logs`
- **THEN** 系统 SHALL 返回 category 为 `operation` 或 `permission` 的审计日志

#### Scenario: 查询权限变更日志
- **WHEN** 管理员请求 `GET /api/admin/permission-logs`
- **THEN** 系统 SHALL 返回 category 为 `permission` 的审计日志
- **AND** GET 查询类角色、权限、菜单接口 SHALL NOT 归类为 `permission`
- **AND** 只有权限、菜单、角色绑定关系相关写操作 SHALL 归类为 `permission`

#### Scenario: 查询数据访问日志
- **WHEN** 管理员请求 `GET /api/admin/data-access-logs`
- **THEN** 系统 SHALL 返回 category 为 `data_access` 的审计日志

### Requirement: 审计日志冷热归档
系统 SHALL 支持将超过保留天数的审计日志及其上传安全 metadata 从热表归档到冷表。

#### Scenario: 归档任务未启用
- **WHEN** `audit_log_archive.enabled` 为 false
- **THEN** 系统 SHALL NOT 启动审计日志归档任务

#### Scenario: 归档过期日志
- **WHEN** `audit_log_archive.enabled` 为 true
- **AND** `audit_logs` 中存在早于 `retention_days` 的记录
- **THEN** 系统 SHALL 按 `batch_size` 批量复制记录及 metadata 到 `audit_log_archives`
- **AND** 只有复制成功后才删除 `audit_logs` 中对应记录
