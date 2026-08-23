# identity Delta Specification

## ADDED Requirements

### Requirement: 已验证邮箱资格

系统 SHALL 仅将已验证邮箱作为外部通知的邮箱投递资格；本 Change 不支持手机号、短信验证或用户通知偏好。

#### Scenario: 新注册邮箱未验证
- **WHEN** 用户注册并提交有效邮箱
- **THEN** Identity SHALL 创建用户且 `email_verified_at` 为空
- **AND** 用户 SHALL 仍可正常登录
- **AND** Messaging 或未来通知 Consumer SHALL NOT 将该邮箱视为可投递渠道

#### Scenario: 注册验证邮件投递失败
- **WHEN** 新用户已创建且注册自动验证邮件的 SMTP 投递失败
- **THEN** Identity SHALL 保留该用户且 `email_verified_at` 保持为空
- **AND** 系统 SHALL 使刚签发的验证凭据失效
- **AND** 注册接口 SHALL 返回稳定 HTTP 503 Identity 错误而不得伪造注册成功或补偿删除用户
- **AND** 用户 SHALL 能登录并在节流允许后请求重发

#### Scenario: 注册投递失败的安全响应
- **WHEN** 注册验证邮件 SMTP 投递失败
- **THEN** HTTP 503 错误 SHALL 只包含稳定 Identity 错误码和安全提示
- **AND** 错误 `data` SHALL NOT 包含 `account_created`、用户 ID、邮箱或 token

#### Scenario: 已验证邮箱可用于通知渠道查询
- **WHEN** App 注入的通知 Adapter 通过 Identity Contract 查询启用用户的邮箱渠道
- **THEN** Identity SHALL 仅返回已启用且 `email_verified_at` 非空的邮箱
- **AND** Contract SHALL NOT 返回密码、验证 token、SMTP 凭据或持久化模型

#### Scenario: 待验证邮箱期间解析业务通知渠道
- **WHEN** 启用用户具有 `email_verified_at` 非空的当前 `email` 和未确认的 `pending_email`
- **THEN** Identity Contract SHALL 仅返回当前已验证 `email` 作为业务通知渠道
- **AND** Contract SHALL NOT 返回或允许 Adapter 使用 `pending_email`
- **WHEN** 用户确认候选地址并在同一事务中提升为 `email`
- **THEN** 后续渠道查询 SHALL 仅返回新的已验证 `email`

#### Scenario: 返回安全验证状态
- **WHEN** 用户查询资料、注册成功或用户资料更新成功
- **THEN** Identity SHALL 返回 `email`、`pending_email` 和 `email_verified`
- **AND** 响应 SHALL NOT 返回 token、过期时间或 SMTP 投递状态

#### Scenario: 沿用现有邮箱规范化
- **WHEN** Identity 接收注册邮箱或候选 `pending_email`
- **THEN** 系统 SHALL 沿用现有 Gin `email` 格式校验和数据库唯一性语义
- **AND** 邮箱验证 SHALL NOT 改写本地部分、大小写或提供商别名

### Requirement: 待验证邮箱变更

系统 SHALL 在确认候选邮箱所有权前保留既有已验证邮箱。

#### Scenario: 邮箱变更要求当前密码
- **WHEN** 已认证用户通过 `PUT /api/user/info` 提交非空 `email`
- **THEN** 请求 SHALL 同时包含 `current_password`
- **AND** Identity SHALL 在任何邮箱状态或验证凭据变更前比较该密码与当前密码摘要
- **WHEN** `current_password` 缺失或不匹配
- **THEN** Identity SHALL 返回带 `current_password` 字段和稳定 `IDENTITY_CURRENT_PASSWORD_INVALID` 错误码的 HTTP 422 错误
- **AND** 系统 SHALL NOT 修改 `email`、`pending_email` 或 `email_verified_at`，也不得签发凭据或尝试 SMTP 投递

- **WHEN** 用户本人当前 `email_verified_at` 非空，且请求将邮箱改为未被其他用户占用的候选地址
- **THEN** Identity SHALL 写入新的 `pending_email`
- **AND** 既有 `email` 与 `email_verified_at` SHALL 保持不变
- **AND** 系统 SHALL 为候选邮箱签发新的验证凭据并同步尝试 SMTP 投递

#### Scenario: 待验证邮箱创建或替换后的同步投递失败
- **WHEN** 已验证用户的 `pending_email` 已创建或替换，且自动验证邮件 SMTP 投递失败、超时或被拒绝
- **THEN** Identity SHALL 保留当前已验证 `email`、`email_verified_at` 与新的 `pending_email`，并使刚签发的凭据失效
- **AND** 用户资料更新接口 SHALL 返回稳定 HTTP 503 Identity 错误，不得回滚已提交的候选邮箱或报告验证邮件已发送
- **AND** 外部通知渠道 SHALL 继续仅解析当前已验证 `email`
- **AND** 用户 SHALL 能在节流允许后为当前 `pending_email` 请求重发

#### Scenario: 替换已有待验证邮箱
- **WHEN** 已验证用户已有 `pending_email`，且提交另一个未被其他用户占用的候选邮箱
- **THEN** Identity SHALL 在同一事务中将新候选地址替换为 `pending_email`
- **AND** 系统 SHALL 仅在替换提交后使旧候选地址的全部有效验证凭据失效，并为新 `pending_email` 签发验证凭据及同步尝试 SMTP 投递
- **AND** 既有 `email` 与 `email_verified_at` SHALL 保持不变

#### Scenario: 替换待验证邮箱时的候选地址冲突
- **WHEN** 已验证用户已有 `pending_email`，且提交的替代候选邮箱已被其他用户占用
- **THEN** Identity SHALL 返回 HTTP 409
- **AND** 系统 SHALL 保留原 `pending_email` 及其有效验证凭据

#### Scenario: 候选确认与替换并发
- **WHEN** 同一用户对当前 `pending_email` 的有效确认与替换该候选邮箱的请求并发执行
- **THEN** Identity SHALL 使用同一用户的事务锁串行化状态转换，且先成功提交的转换 SHALL 成为当前状态
- **WHEN** 候选替换先提交
- **THEN** 系统 SHALL 使旧候选地址 token 失效，后到的确认 SHALL 返回稳定的无效验证凭据错误
- **WHEN** 候选确认先提交
- **THEN** 系统 SHALL 原子提升已确认候选地址，后到的邮箱更新 SHALL 只可基于该新已验证 `email` 创建新的 `pending_email`
- **AND** 系统 SHALL NOT 静默覆盖已提交的邮箱状态或确认结果

#### Scenario: 未验证当前邮箱更换
- **WHEN** 用户本人当前 `email_verified_at` 为空、`pending_email` 为空，且提交未被其他用户占用的候选邮箱
- **THEN** Identity SHALL 在同一事务中直接将候选地址替换为 `email`、保持 `email_verified_at` 为空，并为新 `email` 签发验证凭据及同步尝试 SMTP 投递
- **AND** 系统 SHALL 使该用户针对旧 `email` 的全部有效验证凭据失效
- **AND** 系统 SHALL NOT 创建 `pending_email` 或将旧未验证 `email` 用作业务通知渠道

#### Scenario: 未验证邮箱替换后的同步投递失败
- **WHEN** 未验证当前邮箱的直接替换已提交，且自动验证邮件 SMTP 投递失败、超时或被拒绝
- **THEN** Identity SHALL 保留新 `email`、保持 `email_verified_at` 为空并使刚签发的凭据失效
- **AND** 用户资料更新接口 SHALL 返回稳定 HTTP 503 Identity 错误，不得回滚已提交的邮箱替换或报告验证邮件已发送
- **AND** 用户 SHALL 能登录并在节流允许后为当前新 `email` 请求重发

#### Scenario: 确认待验证邮箱
- **WHEN** 已认证用户提交匹配其身份和 `pending_email` 的有效验证 token
- **THEN** Identity SHALL 在同一事务中将 `pending_email` 提升为 `email`
- **AND** Identity SHALL 清空 `pending_email`、写入 `email_verified_at` 并使凭据不可复用

#### Scenario: 确认时候选邮箱冲突
- **WHEN** 有效 `pending_email` token 确认时该候选邮箱已被另一用户占用
- **THEN** Identity SHALL 返回 HTTP 409
- **AND** 系统 SHALL 保留 `pending_email` 并使当前 token 失效
- **AND** 用户 SHALL 提交新的候选邮箱才能继续

#### Scenario: 迁移既有邮箱
- **WHEN** 系统迁移已有 `users.email` 记录
- **THEN** 系统 SHALL 将所有既有 `email_verified_at` 保持为空
- **AND** 既有用户 SHALL 保持登录能力但没有外部通知邮箱资格

#### Scenario: 管理员修改邮箱被拒绝
- **WHEN** 管理员通过既有用户更新接口提交邮箱字段
- **THEN** Identity SHALL 拒绝该字段
- **AND** 系统 SHALL NOT 修改用户的 `email`、`pending_email` 或 `email_verified_at`

#### Scenario: 凭据终态清理
- **WHEN** 验证凭据成功、失效或过期
- **THEN** 系统 SHALL 仅保留其受控元数据 24 小时
- **AND** Identity 后台任务 SHALL 在保留期后物理删除该凭据
- **AND** 系统 SHALL NOT 持久化原始 token

### Requirement: 一次性邮箱验证凭据

系统 SHALL 使用受控、短期且一次性的验证凭据确认邮箱所有权。

#### Scenario: 签发验证凭据
- **WHEN** 注册成功、未验证当前邮箱直接替换、`pending_email` 创建/替换或用户请求重发验证邮件
- **THEN** 系统 SHALL 生成 32 字节随机 Base64URL token
- **AND** 系统 SHALL 仅保存 token 的 SHA-256 摘要、用户 ID、目标邮箱和 15 分钟过期时间
- **AND** 新凭据 SHALL 使同一用户和目标邮箱的旧凭据失效

#### Scenario: 确认验证凭据
- **WHEN** 已认证用户通过 `POST /api/user/email-verifications/confirm` 提交未过期、未使用且绑定自身目标邮箱的 token
- **THEN** 系统 SHALL 完成对应邮箱验证
- **AND** GET 请求 SHALL NOT 改变验证状态

#### Scenario: 确认返回更新后资料
- **WHEN** 已认证用户成功确认邮箱验证 token
- **THEN** `POST /api/user/email-verifications/confirm` SHALL 返回更新后的安全用户资料
- **AND** 响应 SHALL 包含 `email`、`pending_email` 与 `email_verified`

#### Scenario: 验证邮件携带确认凭据
- **WHEN** 系统发送验证邮件
- **THEN** 邮件 SHALL 包含供用户复制的完整随机 token
- **AND** 邮件 SHALL NOT 包含会改变验证状态的 GET 链接

#### Scenario: 已认证签发或重发
- **WHEN** 用户调用 `POST /api/user/email-verifications`
- **THEN** 系统 SHALL 为当前用户未验证 `email` 或 `pending_email` 签发凭据并同步尝试 SMTP 投递
- **AND** 新凭据或 SMTP 发送失败 SHALL 使同一用户和目标邮箱的旧凭据失效
- **AND** 每次签发尝试 SHALL 计入与自动签发共用的滚动一小时三次限制，即使 SMTP 投递失败

#### Scenario: 已验证邮箱无需重发
- **WHEN** 当前 `email` 已验证且不存在 `pending_email` 的用户调用 `POST /api/user/email-verifications`
- **THEN** 系统 SHALL 返回 HTTP 409
- **AND** 系统 SHALL NOT 生成 token 或发送验证邮件

#### Scenario: 验证邮件请求节流
- **WHEN** 用户在滚动一小时内已完成三次自动或显式验证邮件签发尝试
- **THEN** 系统 SHALL 拒绝新的签发或重发请求

#### Scenario: 邮箱变更自动签发被节流
- **WHEN** 用户提交会触发自动验证邮件的邮箱直接替换、`pending_email` 创建或替换，且其滚动一小时签发额度已耗尽
- **THEN** Identity SHALL 返回 HTTP 429、稳定 Identity 节流错误和 `Retry-After`
- **AND** 系统 SHALL NOT 修改 `email`、`pending_email` 或 `email_verified_at`，也不得签发凭据或尝试 SMTP 投递

#### Scenario: 节流响应
- **WHEN** 系统拒绝滚动一小时内第四次验证邮件请求
- **THEN** 系统 SHALL 返回 HTTP 429、稳定 Identity 节流错误和 `Retry-After` 秒数
- **AND** 响应 SHALL NOT 包含邮箱、当前计数或 token
- **AND** 系统 SHALL NOT 生成新的 token 或发送邮件

### Requirement: 最小 SMTP 验证邮件投递

系统 SHALL 使用供应商无关 SMTP 发送邮箱验证邮件；此能力不构成通用邮件发送功能。

#### Scenario: SMTP 配置
- **WHEN** 应用加载 SMTP 配置
- **THEN** 配置 SHALL 包含 `host`、`port`、`username_env`、`password_env`、`from_env`、`tls_mode` 与 `timeout_seconds`
- **AND** `tls_mode` SHALL 仅允许 `disabled`、`starttls_required` 或 `implicit`
- **AND** `disabled` SHALL 仅用于本地测试 SMTP 服务，`starttls_required` SHALL 为生产默认，`implicit` SHALL 用于 SMTPS
- **AND** 系统 SHALL NOT 使用 opportunistic TLS 降级
- **AND** 系统 SHALL 使用 SMTP 用户名加密码、应用专用密码或服务商 SMTP Token 认证
- **AND** 系统 SHALL NOT 实现 OAuth2/XOAUTH2 Token 生命周期
- **AND** `timeout_seconds` 默认 SHALL 为 10 秒且可显式覆盖建连、TLS、认证和发送阶段的连接 deadline

#### Scenario: 同步投递成功
- **WHEN** 系统签发新的验证凭据
- **AND** SMTP Server 成功接收验证邮件
- **THEN** 系统 SHALL 返回成功且凭据可供确认

#### Scenario: 同步投递失败
- **WHEN** SMTP 投递失败、超时或拒绝
- **THEN** 系统 SHALL 使刚签发的凭据失效并返回安全错误
- **AND** 系统 SHALL NOT 报告验证邮件已发送

### Requirement: 邮箱验证安全边界

系统 SHALL 保护邮箱验证的凭据和 SMTP 细节。

#### Scenario: 验证失败或节流
- **WHEN** 验证 token 无效、过期、已使用或请求被节流
- **THEN** 系统 SHALL 返回稳定 Identity 错误码的四字段错误信封
- **AND** 响应、审计和运行日志 SHALL NOT 回显邮箱地址、token、token 摘要、SMTP 凭据、服务器地址或原始 SMTP 错误
