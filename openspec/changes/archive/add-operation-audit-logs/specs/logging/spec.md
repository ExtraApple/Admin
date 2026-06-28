## ADDED Requirements

### Requirement: API 审计日志
系统 SHALL 自动记录 `/api/*` 请求到 `audit_logs` 表。

#### Scenario: 普通 API 请求被记录
- **WHEN** 用户调用 `/api/*` 接口
- **THEN** 系统记录 method、path、query、status、duration、client_ip、user_agent、created_at
- **AND** 如果请求上下文中存在 userID，系统记录 user_id

#### Scenario: 请求体脱敏
- **WHEN** 请求体为 JSON 且包含密码、token 或验证码字段
- **THEN** 系统将敏感字段值记录为 `***`

#### Scenario: multipart 请求
- **WHEN** 请求 Content-Type 为 `multipart/form-data`
- **THEN** 系统 SHALL NOT 读取或记录文件内容

### Requirement: 审计日志查询
系统 SHALL 为管理员提供审计日志查询接口。

#### Scenario: 查询全部 API 日志
- **WHEN** 管理员请求 `GET /api/admin/audit-logs`
- **THEN** 系统分页返回审计日志

#### Scenario: 查询登录日志
- **WHEN** 管理员请求 `GET /api/admin/login-logs`
- **THEN** 系统返回 category 为 `login` 的审计日志

#### Scenario: 查询操作日志
- **WHEN** 管理员请求 `GET /api/admin/operation-logs`
- **THEN** 系统返回 category 为 `operation` 或 `permission` 的审计日志

#### Scenario: 查询权限变更日志
- **WHEN** 管理员请求 `GET /api/admin/permission-logs`
- **THEN** 系统返回 category 为 `permission` 的审计日志

#### Scenario: 查询数据访问日志
- **WHEN** 管理员请求 `GET /api/admin/data-access-logs`
- **THEN** 系统返回 category 为 `data_access` 的审计日志
