# identity Specification

## Purpose

Identity 负责用户身份、资料、认证和邮箱所有权验证。邮箱验证只确认外部通知渠道资格，不扩展手机号、短信或通知偏好能力。

## Requirements

### Requirement: 已验证邮箱资格

新注册用户的 `email_verified_at` SHALL 为空但仍可登录；只有启用且 `email_verified_at` 非空的 `email` 才能通过 Identity Contract 作为业务通知渠道。`pending_email` 存在期间继续只返回当前已验证邮箱；确认后才原子切换到新邮箱。对外响应只返回 `email`、`pending_email` 和 `email_verified`，不得返回 token、过期时间或 SMTP 状态。

### Requirement: 待验证邮箱变更

已认证用户通过 `PUT /api/user/info` 修改邮箱时 SHALL 提交并重新验证 `current_password`。已验证当前邮箱创建或替换 `pending_email`；未验证且无 pending 的当前邮箱可原子直接替换但仍保持未验证。候选地址冲突返回 409；候选确认和替换按用户事务锁串行化；提交 SMTP 失败不得回滚已提交邮箱状态，当前用户仍可登录并在节流允许后重发。

### Requirement: 一次性验证凭据

注册、邮箱替换和显式重发 SHALL 生成 32 字节随机 Base64URL token，只保存 SHA-256 摘要、用户、目标邮箱和 15 分钟过期时间；同一用户和目标邮箱的新凭据使旧凭据失效。确认入口只允许已认证用户提交自身未过期、未使用凭据，并在同一事务中提升 pending、写入 `email_verified_at`、清空 pending 和使凭据不可复用。凭据终态元数据保留 24 小时后清理。

### Requirement: 验证邮件节流

自动和显式签发共用每用户滚动一小时三次额度，失败投递也计数。额度耗尽返回 429、稳定错误码和 `Retry-After`，不得改变邮箱状态、签发凭据或发送邮件；已验证且无 pending 用户重发返回 409。

### Requirement: 最小同步 SMTP 投递

配置 SHALL 包含 `host`、`port`、账号/密码/发件人环境变量引用、`tls_mode` 和超时。TLS 模式只允许 `disabled`、`starttls_required`、`implicit`；生产默认要求 STARTTLS，不得 opportunistic 降级；超时默认 10 秒并覆盖建连、TLS、认证和发送。SMTP 投递同步执行；失败、超时或拒绝使刚签发凭据失效并返回安全错误，不伪造发送成功。

### Requirement: 邮箱验证安全边界

邮箱地址、token、token 摘要、SMTP 用户名、密码、服务器地址和 SMTP 原始错误不得出现在 HTTP 响应、审计日志或运行日志。管理员用户更新接口 SHALL 拒绝邮箱字段，不得改变邮箱、pending 或验证状态。既有邮箱迁移为未验证并保持登录能力，但不自动获得外部通知资格。
