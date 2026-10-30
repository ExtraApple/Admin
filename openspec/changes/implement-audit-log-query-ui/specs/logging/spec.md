# logging Specification Delta

## MODIFIED Requirements

### Requirement: 审计日志查询
系统 SHALL 为管理员提供审计日志查询接口，支持分页与筛选，并返回已持久化的上传安全 metadata。五个查询接口 SHALL 接受 `page`、`size`、`user_id`、`username`、`method`、`path`、`status`、`category`、`start_time`、`end_time` 查询参数；`username` 与 `path` SHALL 使用包含匹配，`user_id`、`method`、`status`、`category` SHALL 使用精确匹配。`size` 超过 50 时 SHALL 按 50 处理。`start_time` 与 `end_time` SHALL 使用 `YYYY-MM-DD HH:mm:ss` 格式并按服务器本地时区解析；任一参数非空但无法解析，或开始时间晚于结束时间时，接口 SHALL 返回 422 `AUDIT_VALIDATION_INVALID`，不得忽略筛选条件。

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

#### Scenario: 按用户名筛选
- **WHEN** 管理员在任一查询接口提供非空 `username`
- **THEN** 系统 SHALL 只返回 username 包含该值的审计日志
- **AND** 结果 SHALL 继续遵守分页与其余筛选条件

#### Scenario: 分页大小上限
- **WHEN** 管理员请求的 `size` 大于 50
- **THEN** 系统 SHALL 最多返回 50 条记录
- **AND** 响应中的 `size` SHALL 为 50

#### Scenario: 非法时间范围
- **WHEN** `start_time` 或 `end_time` 非空但格式不是 `YYYY-MM-DD HH:mm:ss`，或 `start_time` 晚于 `end_time`
- **THEN** 系统 SHALL 返回 422 `AUDIT_VALIDATION_INVALID`
- **AND** 系统 SHALL NOT 执行该次查询

#### Scenario: 本地时间筛选
- **WHEN** 管理员使用服务器本地时区的 `start_time` 与 `end_time` 发起查询
- **THEN** 系统 SHALL 按同一本地时区比较 `created_at`
- **AND** 筛选结果 SHALL NOT 因 UTC 与本地时区转换产生时间偏移
