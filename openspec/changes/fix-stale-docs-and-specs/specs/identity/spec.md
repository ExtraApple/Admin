## MODIFIED Requirements

### Requirement: 已验证邮箱资格

新注册用户的 `email_verified_at` SHALL 为空但仍可登录；只有启用且 `email_verified_at` 非空的 `email` 才能通过 Identity Contract 作为业务通知渠道。`pending_email` 存在期间继续只返回当前已验证邮箱；确认后才原子切换到新邮箱。**通知渠道 Contract 响应**只返回已验证邮箱地址，不得返回 token、过期时间、SMTP 状态或持久化模型。

（说明：本要求约束的是**通知渠道 Contract**，不是 HTTP 用户资料响应。
HTTP 用户资料响应由 `user-management` 规格的「当前用户资料」要求约束，
返回 `email`、`pending_email`、`email_verified` 及头像等字段。）

#### Scenario: 仅已验证邮箱进入通知渠道
- **WHEN** 用户尚未验证邮箱，或已验证用户存在 `pending_email`
- **THEN** Identity SHALL 不返回未验证地址作为业务通知渠道

#### Scenario: 通知渠道 Contract 不返回多余字段
- **WHEN** 通知 Adapter 通过 Identity Contract 查询启用用户渠道
- **THEN** Contract SHALL 只返回已验证邮箱地址与可用性标记
- **AND** Contract SHALL NOT 返回 `pending_email`、密码、token、
  SMTP 凭据、验证状态或持久化模型

#### Scenario: HTTP 资料响应与本 Contract 相互独立
- **WHEN** 已认证用户请求 `GET /api/user/info`
- **THEN** 系统 SHALL 按 `user-management` 规格返回资料字段
- **AND** 该响应 SHALL NOT 受本要求的最小字段约束限制
- **AND** 该响应 SHALL NOT 包含邮箱验证凭据或 SMTP 状态
