## 1. 后端邮箱契约

- [x] 1.1 在 `internal/identity/dto.go` 增加 `EmailVerificationPolicy{Enabled bool}` 和 `CancelEmailVerificationRequest{PendingEmail string}`，保持既有资料、重发和确认请求结构不变。
- [x] 1.2 在 `internal/identity/application/errors.go` 增加 `CodeEmailVerificationDisabled`（503）和 `CodeEmailPendingStale`（409），并在 `internal/identity/adapters/http/errors.go` 增加稳定 HTTP 错误定义。
- [x] 1.3 在 `internal/identity/application/email_verification.go` 增加 `Cancel(ctx, userID, expectedPendingEmail)`：按用户行锁比较当前 `pending_email`，匹配则清空并失效该候选的未使用凭据；不匹配或为空返回 `CodeEmailPendingStale`；不要求密码。
- [x] 1.4 在 `UserService.UpdateSelf` 和 `EmailVerificationService.Send/Resend` 中，当发送器未配置时返回 `CodeEmailVerificationDisabled`，替换通用内部错误；确认和取消不受 SMTP 状态影响。
- [x] 1.5 修改 `internal/identity/adapters/mail/sender.go`：接收受信任前端源，中文正文同时包含 `${origin}/profile#email_token=<token>` 链接和手动 token，并注明 15 分钟有效；保持 `SendVerification(ctx, email, token)` 接口。
- [x] 1.6 在 `internal/app/identity.go` 把 `config.SMTP.Enabled` 与 `config.BrowserSession.Origin` 传入身份组合根；仅在启用时构造 sender，并让策略端点读取同一事实。
- [x] 1.7 在 `internal/identity/adapters/http/routes.go` 注册 `GET /api/user/email-verification-policy` 和 `POST /api/user/email-verifications/cancel`，保持 authenticated、无新权限码，并更新路由描述。
- [x] 1.8 增加 Go 测试：策略启用／关闭、SMTP 关闭 503、取消匹配／stale、确认与取消并发、链接与中文邮件正文、邮箱变化不提升访问版本。
- [x] 1.9 运行 `go test ./...`，验证新增路由行为与既有身份测试。

## 2. 前端数据层

- [x] 2.1 在 `web/src/lib/models.ts` 增加邮箱验证策略、取消请求和必要的邮箱验证响应类型。
- [x] 2.2 在 `web/src/lib/api.ts` 增加策略读取、重发、确认和取消方法，复用统一认证、信封、字段错误和网络错误映射。
- [x] 2.3 在 `web/src/lib/errors.ts` 补齐邮箱验证相关错误码的中文文案：禁用、stale、无效、投递失败、限流、当前密码错误和邮箱占用。
- [x] 2.4 增加 `web/src/lib/email-verification.ts`（或等价模块），解析并清除 `/profile#email_token=...`，只保留一次自动确认所需的 token。
- [x] 2.5 增加请求层、错误映射和 hash token 测试，覆盖 422／409／429／503 与 fragment 解析。

## 3. 个人资料邮箱界面

- [x] 3.1 扩展 `web/src/pages/use-profile-editor.ts`：邮箱候选、当前密码、验证 token、策略状态、重发／确认／取消动作、独立 dirty 状态和 503 后重读资料。
- [x] 3.2 在 `web/src/pages/ProfilePage.tsx` 邮箱卡片内实现换绑表单、当前／待验证状态、验证当前／待验证邮箱、重发、取消待验证和策略不可用提示。
- [x] 3.3 实现链接自动确认：登录后读取 fragment、自动提交一次、立即清除 hash；失败保留 token；匿名登录回跳保留 hash。
- [x] 3.4 将个人资料扩为昵称、头像、邮箱、密码四个独立编辑区，保持同一时间只展开一个和既有未保存修改保护。
- [x] 3.5 在 `web/src/management.css` 或个人资料样式处增加邮箱状态、待验证、验证码、取消和窄屏样式，复用现有状态语义。
- [x] 3.6 会话挂起或失效时清空邮箱当前密码和验证 token；邮箱候选草稿只允许同一 userID／SID 恢复。
- [x] 3.7 增加组件测试：策略关闭、已验证／未验证换绑、链接自动确认、取消 409、429／503、四编辑区互斥、密码与 token 草稿清理。
- [x] 3.8 运行 `npm run typecheck`、`npm test -- --run`、`npm run build`。

## 4. 文档、验收与交付

- [x] 4.1 更新 `docs/modules/user-management.md`（邮箱换绑／验证／pending／取消／SMTP 关闭）和 `docs/runbooks/configuration.md`（SMTP 启用、Mailpit 本地验收）。
- [x] 4.2 运行 `go test ./...` 与前端完整检查，记录结果。
- [x] 4.3 使用 Mailpit（SMTP 1025／HTTP 8025）和隔离 MySQL／Redis 做浏览器验收：已验证换绑、未验证直接替换、登录／匿名链接确认、重发、取消、策略关闭、429、亮暗主题和 375／850／1440 宽度。
- [x] 4.4 更新 `TODO.md` 的“邮箱更换与验证”条目，记录验证命令与结果。
- [x] 4.5 按 OpenSpec 流程同步 `user-management`／`admin-workbench` 主规格，验收完成后归档 change。
