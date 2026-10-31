## Why

身份模块已经实现邮箱换绑、一次性验证凭据、重发和确认协议，但默认 `smtp.enabled: false` 且个人资料页只读展示邮箱，用户无法完成换绑或验证；同时缺少邮件内链接、SMTP 可用性信号和取消待验证邮箱能力，待验证地址会被长期占用。现有 `user-management` 规格还明确禁止本轮个人资料界面提供邮箱换绑入口，必须由新 change 正式修改。

## What Changes

- 新增 `GET /api/user/email-verification-policy`，返回当前部署是否启用邮件验证发送；个人资料邮箱卡片据此禁用换绑／重发，但保留验证码确认和取消待验证。
- 邮件发送器未配置时，换绑和重发返回稳定 503 `IDENTITY_EMAIL_VERIFICATION_DISABLED`，不再返回通用 500。
- 中文验证邮件同时包含 `${browser_session.origin}/profile#email_token=...` 链接和手动 token；链接 token 放在 fragment，避免进入服务端访问日志。
- 新增 `POST /api/user/email-verifications/cancel`：按当前展示的 `pending_email` 匹配取消；不匹配返回 409 `IDENTITY_EMAIL_PENDING_STALE`；取消不需要再次输入密码。
- 待验证邮箱在验证成功、显式取消或再次换绑前保留，不自动过期清理；UI 明确提示地址仍被占用并提供取消。
- 个人资料页新增邮箱编辑器：当前邮箱、验证状态、待验证邮箱、换绑、验证当前邮箱、验证待验证邮箱、重发、取消待验证；邮箱与昵称分开提交，只 Trim 首尾空白并保留大小写。
- 链接自动提交一次确认并立即清除 fragment；匿名打开时沿用现有登录 `from`（含 hash）回跳。
- 换绑、确认、取消均不提升访问版本、不使现有会话失效；不改变 `PUT /api/user/info` 既有密码校验、唯一性和 pending／verified 语义。
- 个人资料由三个独立编辑区扩展为四个（昵称、头像、邮箱、密码），仍同一时间只展开一个。
- 非目标：多邮箱、邮箱登录、管理员代改邮箱、待验证自动过期、会话失效、异步／Outbox 邮件、注册页未发送提示、精确限流倒计时、短信或手机验证；仓库默认不启用 SMTP。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `user-management`: MODIFIED `当前用户资料`（邮箱界面从只读改为可用，保留既有协议）；ADDED `邮箱换绑与验证`（换绑、验证、重发、取消、策略、SMTP 关闭、冲突、版本不变）。
- `admin-workbench`: MODIFIED `运行中续期失败保留编辑`（邮箱候选草稿与当前密码清理）；MODIFIED `个人资料三个独立编辑区`（扩展为四个）；MODIFIED `昵称编辑与身份字段只读`（邮箱可编辑，其他身份字段仍只读）；MODIFIED `个人资料当前标签同步与并发边界`（邮箱状态同步）。

## Impact

- 后端路由：新增 `GET /api/user/email-verification-policy`、`POST /api/user/email-verifications/cancel`；均为已认证接口，不新增权限码。
- 后端错误码：新增 `IDENTITY_EMAIL_VERIFICATION_DISABLED`（503）、`IDENTITY_EMAIL_PENDING_STALE`（409）；复用既有邮箱验证冲突、凭据无效和限流错误。
- 后端代码：`internal/identity` 的 DTO、UserService、EmailVerificationService、HTTP handler、mail sender、错误映射和路由描述；组合根把 `browser_session.origin` 与 `smtp.enabled` 传入身份模块。
- 前端：个人资料邮箱编辑器与策略读取、链接 token 自动确认、取消待验证、错误码映射、会话上下文刷新和测试；不新增路由或 Web Storage。
- 数据库：无迁移；复用 `users.email`、`pending_email`、`email_verified_at` 与既有验证凭据表。
- 配置与验收：新增 SMTP／Mailpit 验收说明；仓库默认 `smtp.enabled: false`，隔离验收环境临时启用 Mailpit（SMTP 1025／HTTP 8025）。
- 规格与文档：`user-management`、`admin-workbench` 增量规格；`CONTEXT.md` 术语已更新；`docs/modules/user-management.md`、`docs/runbooks/configuration.md`。
- 不涉及 Redis key、MinIO bucket、权限码或数据库列变更。
