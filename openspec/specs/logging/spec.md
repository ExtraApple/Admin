# logging Specification

## Purpose

日志模块定义应用运行日志和计划中的审计日志边界。运行日志用于帮助定位服务行为，审计日志用于记录安全相关的业务操作。

## Requirements

### Requirement: Zap 运行日志
系统 SHALL 使用 Zap 作为运行日志核心，为服务启动、定时任务、HTTP 请求和运行错误提供运行日志。

#### Scenario: 服务启动时初始化日志
- **WHEN** 应用启动并读取配置成功
- **THEN** 系统初始化全局 Zap logger
- **AND** 后续初始化流程可以通过全局 logger 写入运行日志

#### Scenario: 初始化失败
- **WHEN** MySQL、Redis 或 MinIO 初始化失败
- **THEN** 系统写入 error 或 fatal 级别运行日志
- **AND** 失败日志 SHALL 包含错误对象

#### Scenario: Gin 请求日志
- **WHEN** HTTP 请求经过 Gin 路由
- **THEN** 系统写入请求运行日志
- **AND** 日志 SHALL 包含 method、path、status、latency、client_ip、user_agent
- **AND** 日志 SHALL NOT 记录请求体、Authorization header、密码、验证码或 token

#### Scenario: 文件轮转任务运行
- **WHEN** 文件轮转任务启动、跳过、移动文件或遇到错误
- **THEN** 系统通过 Zap 写入描述执行结果的运行日志

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
- **AND** GET 查询类角色、权限、菜单接口 SHALL NOT 归类为 `permission`
- **AND** 只有权限、菜单、角色绑定关系相关写操作 SHALL 归类为 `permission`

#### Scenario: 查询数据访问日志
- **WHEN** 管理员请求 `GET /api/admin/data-access-logs`
- **THEN** 系统返回 category 为 `data_access` 的审计日志

### Requirement: 审计日志冷热归档
系统 SHALL 支持将超过保留天数的审计日志从热表归档到冷表。

#### Scenario: 归档任务未启用
- **WHEN** `audit_log_archive.enabled` 为 false
- **THEN** 系统 SHALL NOT 启动审计日志归档任务

#### Scenario: 归档过期日志
- **WHEN** `audit_log_archive.enabled` 为 true
- **AND** `audit_logs` 中存在早于 `retention_days` 的记录
- **THEN** 系统按 `batch_size` 批量复制记录到 `audit_log_archives`
- **AND** 只有复制成功后才删除 `audit_logs` 中对应记录


