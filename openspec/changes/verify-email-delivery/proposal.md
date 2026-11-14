## Why

邮箱验证链路已经实现并通过单元、应用层和 HTTP 契约测试，但当前证据没有覆盖真实 Mailpit、真实浏览器和真实 HTTP 的联合路径，也没有验证 QQ SMTP 的生产配置切换。只在代码层证明行为，无法发现 SMTP 认证、邮件内容、浏览器 token 接收和部署配置之间的集成偏差。

## What Changes

- 新增 `email-delivery-acceptance` 能力，定义可重复的邮箱验证验收边界、隔离环境要求和证据要求。
- 新增本地 Mailpit 验收：使用独立 MySQL、Redis、MinIO、Mailpit、Go 后端和 Vite，通过真实注册、登录和浏览器操作验证邮箱换绑、确认、重发、取消、错误路径、SMTP 故障、关闭 SMTP 和限流。
- 新增可选的 QQ 真实外发冒烟：在本地验收通过后切换 `smtp.qq.com:465`＋`implicit`，验证真实投递和确认，不改变业务代码。
- 新增验收证据文件 `docs/reviews/evidence/email-delivery-2026-11-09.md`，记录环境、命令、HTTP 状态／错误码、Mailpit message ID、脱敏截图、数据库和日志安全检查及清理结果。
- 测试通过后更新 `docs/runbooks/email-delivery.md` 的前置条件和已验证事实；未实测的 QQ 行为保持待验证表述。
- 非目标：不修改用户管理、认证、授权、路由、数据库 schema、权限码或前端业务行为；不提交真实 SMTP 凭据；不引入 Playwright 测试框架；不重复现有自动化已覆盖的并发场景；不连接生产数据库或生产对象存储。

## Capabilities

### New Capabilities

- `email-delivery-acceptance`: 邮箱验证在本地 Mailpit 与真实 SMTP 下的可重复验收边界、隔离要求、失败汇报约束和证据要求。

### Modified Capabilities

无。本次不改变任何现有产品行为或接口契约。

## Impact

- 新增：`openspec/changes/verify-email-delivery/` 下的 proposal、design、spec delta、tasks。
- 新增：`docs/reviews/evidence/email-delivery-2026-11-09.md`（验收通过后填写实际观察）。
- 修改：`docs/runbooks/email-delivery.md`（仅补充已验证的前置条件、命令和事实）。
- 运行环境：Docker（MySQL 8.0、Redis 7、MinIO、Mailpit）、Go、Node/Vite、真实 Chromium；QQ 外发为可选门禁。
- 不涉及生产代码、API、数据库迁移、Redis key、MinIO bucket 或权限码变更。
