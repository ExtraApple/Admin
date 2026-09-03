# identity Specification

## Purpose

Identity 负责用户身份、资料、认证和邮箱所有权验证。邮箱验证只确认外部通知渠道资格，不扩展手机号、短信或通知偏好能力。

## Requirements

### Requirement: 已验证邮箱资格

新注册用户的 `email_verified_at` SHALL 为空但仍可登录；只有启用且 `email_verified_at` 非空的 `email` 才能通过 Identity Contract 作为业务通知渠道。`pending_email` 存在期间继续只返回当前已验证邮箱；确认后才原子切换到新邮箱。对外响应只返回 `email`、`pending_email` 和 `email_verified`，不得返回 token、过期时间或 SMTP 状态。

#### Scenario: 仅已验证邮箱进入通知渠道
- **WHEN** 用户尚未验证邮箱，或已验证用户存在 `pending_email`
- **THEN** Identity SHALL 不返回未验证地址作为业务通知渠道

### Requirement: 待验证邮箱变更

已认证用户通过 `PUT /api/user/info` 修改邮箱时 SHALL 提交并重新验证 `current_password`。已验证当前邮箱创建或替换 `pending_email`；未验证且无 pending 的当前邮箱可原子直接替换但仍保持未验证。候选地址冲突返回 409；候选确认和替换按用户事务锁串行化；提交 SMTP 失败不得回滚已提交邮箱状态，当前用户仍可登录并在节流允许后重发。

#### Scenario: 邮箱状态变更串行化
- **WHEN** 邮箱候选确认与候选替换并发执行
- **THEN** Identity SHALL 按同一用户事务锁串行化，并以先提交的状态为准

### Requirement: 一次性验证凭据

注册、邮箱替换和显式重发 SHALL 生成 32 字节随机 Base64URL token，只保存 SHA-256 摘要、用户、目标邮箱和 15 分钟过期时间；同一用户和目标邮箱的新凭据使旧凭据失效。确认入口只允许已认证用户提交自身未过期、未使用凭据，并在同一事务中提升 pending、写入 `email_verified_at`、清空 pending 和使凭据不可复用。凭据终态元数据保留 24 小时后清理。

#### Scenario: 验证凭据一次性确认
- **WHEN** 已认证用户提交自身未过期且未使用的验证凭据
- **THEN** Identity SHALL 原子完成邮箱确认，并使凭据不可复用

### Requirement: 验证邮件节流

自动和显式签发 SHALL 共用每用户滚动一小时三次额度，失败投递也计数。额度耗尽 SHALL 返回 429、稳定错误码和 `Retry-After`，不得改变邮箱状态、签发凭据或发送邮件；已验证且无 pending 用户重发 SHALL 返回 409。

#### Scenario: 验证邮件额度耗尽
- **WHEN** 用户在滚动一小时内第四次请求验证邮件
- **THEN** Identity SHALL 返回 429 和 `Retry-After`，且不签发凭据

### Requirement: 最小同步 SMTP 投递

配置 SHALL 包含 `host`、`port`、账号/密码/发件人环境变量引用、`tls_mode` 和超时。TLS 模式只允许 `disabled`、`starttls_required`、`implicit`；生产默认要求 STARTTLS，不得 opportunistic 降级；超时默认 10 秒并覆盖建连、TLS、认证和发送。SMTP 投递同步执行；失败、超时或拒绝使刚签发凭据失效并返回安全错误，不伪造发送成功。

#### Scenario: 验证邮件投递失败
- **WHEN** SMTP 投递失败、超时或被拒绝
- **THEN** Identity SHALL 使刚签发凭据失效并返回安全错误

### Requirement: 邮箱验证安全边界

邮箱地址、token、token 摘要、SMTP 用户名、密码、服务器地址和 SMTP 原始错误不得出现在 HTTP 响应、审计日志或运行日志。管理员用户更新接口 SHALL 拒绝邮箱字段，不得改变邮箱、pending 或验证状态。既有邮箱迁移为未验证并保持登录能力，但不自动获得外部通知资格。

#### Scenario: 敏感邮箱验证数据脱敏
- **WHEN** 邮箱验证请求或 SMTP 投递失败
- **THEN** HTTP 响应、审计日志和运行日志 SHALL 不包含邮箱凭据或原始 SMTP 错误

#### Scenario: 新注册邮箱未验证但可登录
- **WHEN** 用户注册并提交有效邮箱
- **THEN** Identity SHALL 创建用户且 `email_verified_at` 为空并允许登录
- **AND** Messaging 或通知 Consumer SHALL NOT 将该邮箱作为可投递渠道

#### Scenario: 注册验证邮件投递失败
- **WHEN** 新用户已创建且注册验证邮件 SMTP 投递失败
- **THEN** Identity SHALL 保留用户、使刚签发凭据失效并返回 HTTP 503 稳定 Identity 错误
- **AND** 用户 SHALL 能登录并在节流允许后请求重发
- **AND** 响应 data SHALL NOT 包含 `account_created`、用户 ID、邮箱或 token

#### Scenario: 已验证邮箱通知渠道查询
- **WHEN** 通知 Adapter 通过 Identity Contract 查询启用用户渠道
- **THEN** Identity SHALL 仅返回 `email_verified_at` 非空的当前邮箱
- **AND** Contract SHALL NOT 返回 pending_email、密码、token、SMTP 凭据或持久化模型

#### Scenario: 待验证邮箱仍使用旧渠道
- **WHEN** 启用用户存在已验证当前 `email` 和未确认 `pending_email`
- **THEN** Contract SHALL 仅返回当前已验证邮箱
- **WHEN** 用户确认候选地址并原子提升为 `email`
- **THEN** 后续查询 SHALL 仅返回新的已验证邮箱

#### Scenario: 邮箱规范化与唯一性
- **WHEN** Identity 接收注册邮箱或候选 `pending_email`
- **THEN** 系统 SHALL 沿用现有格式校验和数据库唯一性语义
- **AND** 不得改写本地部分、大小写或提供商别名

#### Scenario: 邮箱更新密码校验失败不改变状态
- **WHEN** `PUT /api/user/info` 提交非空 email 且 current_password 缺失或不匹配
- **THEN** 系统 SHALL 返回 HTTP 422、`IDENTITY_CURRENT_PASSWORD_INVALID` 及 `current_password` 字段
- **AND** SHALL NOT 修改 email、pending_email、email_verified_at、凭据或发送邮件

#### Scenario: 待验证邮箱替换冲突保留原状态
- **WHEN** 已验证用户替换已有 pending_email 且候选地址已被占用
- **THEN** Identity SHALL 返回 HTTP 409 并保留原 pending_email 及其有效凭据

#### Scenario: 未验证当前邮箱直接替换
- **WHEN** 当前邮箱未验证且无 pending_email，提交未占用候选地址
- **THEN** Identity SHALL 原子替换 email、保持未验证并使旧邮箱凭据失效
- **AND** SHALL NOT 创建 pending_email 或将旧邮箱作为通知渠道

#### Scenario: 邮箱验证确认返回安全资料
- **WHEN** 已认证用户成功确认验证凭据
- **THEN** 确认接口 SHALL 返回包含 email、pending_email、email_verified 的更新后资料
- **AND** GET 请求 SHALL NOT 改变验证状态

#### Scenario: 已验证且无候选邮箱无需重发
- **WHEN** 当前 email 已验证且不存在 pending_email 的用户请求重发
- **THEN** 系统 SHALL 返回 HTTP 409 且不生成 token 或发送邮件

#### Scenario: 邮箱验证节流
- **WHEN** 用户在滚动一小时内第四次请求验证邮件
- **THEN** 系统 SHALL 返回 HTTP 429、稳定节流错误和 `Retry-After`
- **AND** SHALL NOT 生成 token 或发送邮件

#### Scenario: SMTP 配置边界
- **WHEN** 应用加载 SMTP 配置
- **THEN** 配置 SHALL 包含 host、port、username_env、password_env、from_env、tls_mode 和 timeout_seconds
- **AND** tls_mode 仅允许 disabled、starttls_required 或 implicit，默认超时为 10 秒且覆盖各阶段 deadline
- **AND** 系统 SHALL NOT 使用 opportunistic TLS 降级或 OAuth2/XOAUTH2 生命周期

#### Scenario: 验证失败安全边界
- **WHEN** token 无效、过期、已使用、SMTP 失败或请求被节流
- **THEN** HTTP 响应、审计和运行日志 SHALL NOT 回显邮箱、token、token 摘要、SMTP 凭据、服务器地址或原始错误
- **THEN** HTTP 响应、审计日志和运行日志 SHALL 不包含邮箱凭据或原始 SMTP 错误
