# Operation Audit Logs Design

## Data Model

所有审计数据先写入 `audit_logs` 单表。

核心字段：

- `user_id`
- `username`
- `method`
- `path`
- `query`
- `body`
- `status`
- `duration`
- `client_ip`
- `user_agent`
- `category`
- `created_at`

## Categories

| category | 规则 |
|---|---|
| `api` | 默认分类 |
| `login` | `/api/login` |
| `operation` | `POST`、`PUT`、`DELETE` |
| `permission` | 路径包含 `permissions`、`roles`、`menus` |
| `data_access` | `GET` 请求 |

权限变更比普通操作优先级更高。

## Middleware Order

审计中间件挂在 `/api` 分组上。

公开路由没有 `userID` 时记录为 0。

认证路由经过 JWT 后，审计日志可记录 `userID`。

## Sensitive Data

审计日志不会记录：

- Authorization header
- multipart 文件内容

JSON body 中以下字段会脱敏：

- `password`
- `old_password`
- `new_password`
- `confirm_password`
- `captcha_code`
- `access_token`
- `refresh_token`
- `token`

## Query APIs

管理员可查询：

- `/api/admin/audit-logs`
- `/api/admin/login-logs`
- `/api/admin/operation-logs`
- `/api/admin/permission-logs`
- `/api/admin/data-access-logs`

所有接口均需要管理员权限。
