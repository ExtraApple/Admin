# 邮件发送与本地 SMTP 验收

> 本文说明工作台验证邮件在本地和真实部署中的发送链路、配置切换与排障。接口契约以运行中的 OpenAPI 为准；当前行为以 [`openspec/specs/user-management/spec.md`](../../openspec/specs/user-management/spec.md) 为准。

## 1. 发送链路

```text
工作台（邮箱卡片）
  → Admin 后端 PUT /api/user/info 或 POST /api/user/email-verifications
  → internal/identity/adapters/mail（wneessen/go-mail）
  → 配置的 SMTP 服务器
  → 本地 Mailpit 捕获 / 真实 SMTP 外发
```

- 发送库始终是 `github.com/wneessen/go-mail`；切换环境只改 SMTP 配置，不换库。
- 本地用 Mailpit 捕获邮件，不投递到真实收件人。
- 真实部署使用 QQ 邮箱、企业邮箱或事务邮件服务的 SMTP。
- Mailpit 不能用于生产。

## 2. 本地 Mailpit

### 2.1 启动

```bash
docker run --rm --name mailpit \
  -p 127.0.0.1:1025:1025 \
  -p 127.0.0.1:8025:8025 \
  axllent/mailpit \
  --smtp-auth-accept-any \
  --smtp-auth-allow-insecure
```

| 端口 | 用途 |
|---|---|
| `1025` | SMTP，后端发送验证邮件 |
| `8025` | Web UI 与 REST API |

两个认证参数是必需的：后端发送器固定使用 SMTP PLAIN 认证，Mailpit 默认不接受本地明文认证；缺少后会表现为 `IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED`。

可选持久化与容量：

```bash
docker run --rm --name mailpit \
  -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 \
  -v mailpit-data:/data \
  -e MP_DATABASE=/data/mailpit.db \
  -e MP_MAX_MESSAGES=5000 \
  axllent/mailpit --smtp-auth-accept-any --smtp-auth-allow-insecure
```

不要把 `1025` 或 `8025` 暴露到公网；上面的 `127.0.0.1` 绑定仅用于本地。

### 2.2 后端配置

```yaml
smtp:
  enabled: true
  host: 127.0.0.1
  port: 1025
  username_env: SMTP_USERNAME
  password_env: SMTP_PASSWORD
  from_env: SMTP_FROM
  timeout_seconds: 5
  tls_mode: disabled
```

```dotenv
SMTP_USERNAME=mailpit
SMTP_PASSWORD=mailpit
SMTP_FROM=no-reply@example.test
```

同时确认 `browser_session.origin` 是实际前端地址：

- 本地 Vite：`http://localhost:5173` 或 `http://localhost:15173`
- 生产：公网 HTTPS 前端地址

验证邮件中的 `/profile#email_token=...` 链接由该值生成。

### 2.3 验证流程

1. 启动 Mailpit 和后端，确认策略端点：

   ```bash
   curl -H "Authorization: Bearer <access-token>" \
     http://127.0.0.1:8080/api/user/email-verification-policy
   # data.enabled 应为 true
   ```

2. 打开工作台个人资料，编辑邮箱，提交新邮箱和当前密码。
3. 打开 `http://127.0.0.1:8025` 查看邮件；点击链接或手动粘贴 token。
4. 检查当前邮箱、验证状态、待验证邮箱和取消待验证行为。

### 2.4 自动化取信

```bash
curl -s http://127.0.0.1:8025/api/v1/messages
curl -s http://127.0.0.1:8025/api/v1/message/<ID>
```

从返回的 `Text` 或 `HTML` 提取 `#email_token=` 后的 token。token 15 分钟有效、一次性，每用户每小时最多签发 3 次。

## 3. 真实 SMTP（QQ 邮箱示例）

QQ 邮箱需要先在网页版开启 SMTP 服务并生成**授权码**；`SMTP_PASSWORD` 填授权码，不是 QQ 登录密码。

```yaml
smtp:
  enabled: true
  host: smtp.qq.com
  port: 465
  username_env: SMTP_USERNAME
  password_env: SMTP_PASSWORD
  from_env: SMTP_FROM
  timeout_seconds: 10
  tls_mode: implicit
```

```dotenv
SMTP_USERNAME=123456789@qq.com
SMTP_PASSWORD=QQ邮箱授权码
SMTP_FROM=123456789@qq.com
```

- `SMTP_FROM` 必须与认证的 QQ 邮箱一致。
- 465 使用 `tls_mode: implicit`；如果改用 587，使用 `tls_mode: starttls_required`。
- 当前发送器使用 `SMTPAuthPlain`；QQ 支持该认证。若实际返回认证失败，可将发送器认证方式改为 `SMTPAuthLogin`，不影响业务层。
- 其他 SMTP 服务按相同字段配置 host／port／TLS 模式；凭据放 `.env` 或部署 Secret，不写 YAML 明文。

## 4. 环境切换矩阵

| 环境 | `smtp.enabled` | host／port | TLS | 结果 |
|---|---|---|---|---|
| 本地 Mailpit | `true` | `127.0.0.1:1025` | `disabled` | 邮件保存在 Mailpit |
| QQ 邮箱 | `true` | `smtp.qq.com:465` | `implicit` | 真实外发 |
| QQ STARTTLS | `true` | `smtp.qq.com:587` | `starttls_required` | 真实外发 |
| 未启用邮件 | `false` | 不读取 | 不读取 | 策略 `enabled:false`，换绑／重发 503，确认／取消仍可用 |

切换环境只需修改配置并重启后端，不需要改代码或数据库。

## 5. 生产边界

- Mailpit 只用于开发和测试；默认无认证，Web UI 可查看全部邮件和验证 token。
- 生产必须使用真实 SMTP，启用 TLS，凭据放 Secret 管理器。
- 使用自有域名发件时配置 SPF、DKIM、DMARC；Mailpit 不负责这些。
- QQ 个人邮箱存在频率和反垃圾限制，适合小规模或内部使用；正式产品建议使用事务邮件服务。
- 不要把含 token 的邮件截图、Mailpit 数据库或验证链接提交到仓库。

## 6. 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 策略 `enabled:false` | `smtp.enabled: false` 或发送器未构造 | 启用 SMTP 并重启后端 |
| 换绑／重发 503 `IDENTITY_EMAIL_VERIFICATION_DISABLED` | 发送器未配置 | 检查 `smtp.enabled`、host、凭据和 `from` |
| 503 `IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED` | SMTP 连接、TLS、认证或 Mailpit 认证参数错误 | 查看后端日志；Mailpit 加 `--smtp-auth-accept-any --smtp-auth-allow-insecure` |
| 邮件链接指向错误地址 | `browser_session.origin` 不是当前前端地址 | 修改配置并重启后端 |
| 429 `IDENTITY_EMAIL_VERIFICATION_RATE_LIMITED` | 每小时超过 3 次签发 | 等待窗口结束；测试环境可清理验证凭据 |
| 409 `IDENTITY_EMAIL_PENDING_STALE` | 待验证邮箱已被其他标签页修改 | 重新读取资料后再取消 |
| 422 `IDENTITY_EMAIL_VERIFICATION_INVALID` | token 过期、已使用或格式不完整 | 重新发送验证邮件并粘贴完整 token |
| Mailpit 没有邮件 | 端口映射、SMTP 地址、TLS 模式或认证失败 | 检查 `8025`／`1025` 映射和后端日志 |

## 7. 实现说明

- 发送适配器：`internal/identity/adapters/mail/sender.go`，实现 `VerificationEmailSender`。
- 邮件正文包含验证链接和手动 token；链接使用 `browser_session.origin`，token 放 fragment，避免进入访问日志。
- 验证、重发、取消和策略端点由 `internal/identity` 提供；前端在 `web/src/pages/ProfilePage.tsx` 和 `use-profile-editor.ts` 接入。
- 历史 `GO BLOG` 项目使用 `github.com/jordan-wright/email` + `smtp.qq.com:465`；本项目的 `wneessen/go-mail` 已支持 context、超时、TLS 模式和连接复用，QQ 只是 SMTP 配置，因此不换库。

## 8. 相关文档

- [配置手册](configuration.md)
- [用户管理](../modules/user-management.md)
- [登录安全策略](login-security.md)
- [OpenAPI 文档](api-documentation.md)
