# user-management Specification Delta

## MODIFIED Requirements

### Requirement: 当前用户资料
系统 SHALL 允许已认证用户读取和修改自己的非敏感资料字段，但头像 SHALL 只能通过专用头像接口修改。本人邮箱换绑 SHALL 复用既有 `PUT /api/user/info` 请求字段、当前密码校验和邮箱验证协议，并由个人资料邮箱编辑器接入；已验证邮箱的候选 SHALL 保留待验证语义，未验证邮箱 SHALL 保留直接替换并发送验证的语义。

#### Scenario: 查询当前用户资料
- **WHEN** 已认证用户调用 `GET /api/user/info`
- **THEN** 系统 SHALL 返回脱敏后的用户信息
- **AND** 响应 SHALL 永远不包含密码哈希
- **AND** 只有头像对象标识、归属、规范 UUID 扩展名、MIME 和验证状态均可信时，响应头像 SHALL 为 `/api/avatars/<user-id>`
- **AND** 其他情况响应头像 SHALL 为 `/api/avatars/default`

#### Scenario: 修改当前用户资料
- **WHEN** 已认证用户调用 `PUT /api/user/info` 并提交昵称或邮箱
- **THEN** 系统 SHALL 只更新允许修改的字段
- **AND** 如果邮箱已被其他用户使用，系统 SHALL 拒绝请求
- **AND** 如果没有任何可更新字段，系统 SHALL 拒绝请求
- **AND** 提交非空邮箱时，系统 SHALL 校验当前密码

#### Scenario: 资料接口提交头像字段
- **WHEN** `PUT /api/user/info` 请求包含 `avatar` 字段
- **THEN** 系统 SHALL 拒绝请求
- **AND** 系统 SHALL NOT 把外部 URL、MinIO 路径或其他客户端值写入头像字段

#### Scenario: 个人资料界面提交昵称
- **WHEN** 用户在个人资料界面保存昵称
- **THEN** 客户端 SHALL 只发送昵称字段，并要求其非空、非全空白且最多 100 个 Unicode 字符
- **AND** 客户端 SHALL NOT 自动 Trim、改变大小写或提供清空昵称操作
- **AND** 用户 ID、用户名、角色和启用状态 SHALL 保持只读；邮箱 SHALL 由独立邮箱编辑器按换绑协议提交
- **AND** 成功响应 SHALL 沿用既有脱敏用户信息结构，SHALL NOT 添加资料版本或暴露头像摘要及存储标识

#### Scenario: 保留本人邮箱修改协议
- **WHEN** 客户端通过本人资料接口提交非空 `email`
- **THEN** 系统 SHALL 保留 `current_password` 校验、邮箱唯一性检查及邮箱验证流程
- **AND** 已验证邮箱的变更 SHALL 保留待验证邮箱语义，未验证邮箱 SHALL 保留既有直接更新及验证语义
- **AND** 个人资料邮箱编辑器 SHALL 复用该协议，不得绕过密码校验或改变邮箱验证响应语义

#### Scenario: 纯资料修改不提升授权版本
- **WHEN** 用户仅修改自己的昵称、上传头像或恢复默认头像，且未发生其他身份或授权状态变化
- **THEN** 系统 SHALL NOT 创建或提升该用户的授权版本
- **AND** 仍有效的 Access Token 和 Refresh Token SHALL 按既有会话规则继续有效

#### Scenario: 并发资料写入
- **WHEN** 多个合法昵称或头像写请求先后实际提交到数据库
- **THEN** 系统 SHALL 以最后实际数据库写入的值为准
- **AND** 系统 SHALL NOT 为本轮自助维护引入 CAS、编辑锁或资料版本字段

## ADDED Requirements

### Requirement: 邮箱换绑与验证
系统 SHALL 允许已认证用户通过本人资料接口换绑自己的邮箱，并通过一次性邮箱验证凭据确认邮箱所有权。换绑 SHALL 要求当前密码；已验证当前邮箱的候选 SHALL 进入 `pending_email`，未验证当前邮箱的候选 SHALL 直接替换当前邮箱并签发验证凭据。系统 SHALL 同时拒绝与其他用户当前邮箱或待验证邮箱重复的地址，管理员 SHALL NOT 代用户换绑邮箱。

系统 SHALL 提供 `GET /api/user/email-verification-policy` 返回当前部署是否可发送验证邮件。发送器未启用时，换绑与重发 SHALL 返回 503 `IDENTITY_EMAIL_VERIFICATION_DISABLED`；确认与取消 SHALL 保持可用。验证邮件 SHALL 使用受信任前端源生成链接并包含手动 token；token SHALL 在 15 分钟内有效且只能使用一次，每用户每小时最多签发 3 次。系统 SHALL 提供 `POST /api/user/email-verifications/cancel`，按客户端提交的当前 `pending_email` 匹配取消；不匹配时返回 409 `IDENTITY_EMAIL_PENDING_STALE`。待验证邮箱 SHALL 在验证成功、显式取消或再次换绑前保留，系统 SHALL NOT 自动过期清理。

#### Scenario: 已验证邮箱换绑进入待验证
- **WHEN** 当前邮箱已验证的用户提交不同的新邮箱和正确当前密码
- **THEN** 系统 SHALL 写入 `pending_email`，保留当前已验证邮箱和通知资格
- **AND** 系统 SHALL 签发一次性验证凭据并发送验证邮件
- **AND** 响应 SHALL 返回新的待验证邮箱与当前验证状态

#### Scenario: 未验证邮箱换绑直接替换
- **WHEN** 当前邮箱未验证的用户提交不同的新邮箱和正确当前密码
- **THEN** 系统 SHALL 直接替换当前邮箱并清空待验证邮箱与验证时间
- **AND** 系统 SHALL 签发验证凭据并发送验证邮件
- **AND** 响应 SHALL 反映新的未验证当前邮箱

#### Scenario: 当前密码与唯一性
- **WHEN** 用户提交非空邮箱但当前密码错误
- **THEN** 系统 SHALL 返回 422 和 `current_password` 字段错误，不改变邮箱或待验证邮箱
- **WHEN** 用户提交的邮箱已被其他用户的当前邮箱或待验证邮箱占用
- **THEN** 系统 SHALL 返回冲突，不改变邮箱或待验证邮箱

#### Scenario: 验证邮件链接与 token
- **WHEN** 系统签发邮箱验证凭据
- **THEN** 邮件 SHALL 使用受信任前端源生成 `/profile#email_token=...` 链接，并同时包含可手动输入的 token
- **AND** token SHALL 在 15 分钟内有效、只能使用一次，且每用户每小时最多签发 3 次
- **AND** 邮件 SHALL 说明链接和 token 的用途及有效期

#### Scenario: 确认待验证邮箱
- **WHEN** 用户提交与其用户和待验证邮箱匹配的有效 token
- **THEN** 系统 SHALL 原子地把待验证邮箱提升为当前邮箱、清空待验证邮箱并记录验证时间
- **AND** 该 token SHALL 被标记为已使用
- **AND** 响应 SHALL 返回更新后的用户资料

#### Scenario: 确认当前未验证邮箱
- **WHEN** 当前邮箱未验证且用户提交与该当前邮箱匹配的有效 token
- **THEN** 系统 SHALL 记录验证时间并保留当前邮箱
- **AND** token SHALL 被标记为已使用

#### Scenario: 确认时地址已被占用
- **WHEN** 用户提交有效 token，但目标邮箱在确认前已被其他用户占用
- **THEN** 系统 SHALL 返回冲突，不提升目标邮箱
- **AND** token SHALL 被标记为已使用，待验证邮箱保持待用户取消或再次换绑

#### Scenario: 无效、过期或已使用 token
- **WHEN** 用户提交格式错误、已过期或已使用的 token
- **THEN** 系统 SHALL 返回 422 `IDENTITY_EMAIL_VERIFICATION_INVALID`
- **AND** 系统 SHALL NOT 改变当前邮箱、待验证邮箱或验证时间

#### Scenario: 重发验证邮件
- **WHEN** 用户请求重发且存在待验证邮箱
- **THEN** 系统 SHALL 向待验证邮箱签发新的验证凭据并发送邮件
- **WHEN** 当前邮箱未验证且没有待验证邮箱
- **THEN** 系统 SHALL 向当前邮箱签发新的验证凭据并发送邮件
- **AND** 当前邮箱已验证且没有待验证邮箱时 SHALL 返回冲突
- **AND** 超过每小时配额时 SHALL 返回 429 `IDENTITY_EMAIL_VERIFICATION_RATE_LIMITED`

#### Scenario: 取消待验证邮箱
- **WHEN** 用户提交与当前展示一致的 `pending_email`
- **THEN** 系统 SHALL 清空待验证邮箱并失效该候选的未使用凭据
- **AND** 当前已验证邮箱、验证状态和通知资格 SHALL 保持不变
- **AND** 取消 SHALL NOT 要求再次输入当前密码
- **WHEN** 提交的 `pending_email` 与当前值不一致或已无待验证邮箱
- **THEN** 系统 SHALL 返回 409 `IDENTITY_EMAIL_PENDING_STALE`
- **AND** 系统 SHALL NOT 清空或覆盖任何邮箱状态

#### Scenario: 待验证邮箱保留与地址占用
- **WHEN** 验证凭据过期而用户尚未确认或取消
- **THEN** `pending_email` SHALL 继续保留并继续占用该地址的唯一性
- **AND** 用户 SHALL 可以重发、验证、取消或再次换绑
- **AND** 系统 SHALL NOT 自动清空待验证邮箱

#### Scenario: SMTP 未启用
- **WHEN** 当前部署未启用验证邮件发送
- **THEN** 策略端点 SHALL 返回不可用
- **AND** 换绑与重发 SHALL 返回 503 `IDENTITY_EMAIL_VERIFICATION_DISABLED`
- **AND** 确认与取消 SHALL 保持可用，不依赖 SMTP

#### Scenario: 邮箱状态变化不提升授权版本
- **WHEN** 用户换绑、确认或取消邮箱
- **THEN** 系统 SHALL NOT 提升该用户的授权版本
- **AND** 仍有效的 Access Token 和 Refresh Token SHALL 按既有会话规则继续有效

#### Scenario: 并发换绑、确认与取消
- **WHEN** 换绑、确认或取消并发到达同一用户
- **THEN** 系统 SHALL 按用户行锁取得的先后线性化
- **AND** 任一事务 SHALL NOT 覆盖已提交的当前邮箱、丢失最新待验证邮箱或产生部分提交
