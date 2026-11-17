# email-delivery-acceptance Specification

## Purpose

定义邮箱投递在隔离本地环境与可选真实 SMTP 下的验收边界、状态覆盖、证据要求和失败处理规则。

## Requirements
### Requirement: Isolated local email delivery acceptance

邮箱投递验收 SHALL 使用独立的 MySQL、Redis、MinIO、Mailpit、后端和前端资源，并使用临时配置与临时凭据。验收 SHALL NOT 读取仓库 `.env`、生产数据库、生产对象存储或复用旧验收数据。

#### Scenario: Local acceptance starts

- **WHEN** 验收人员启动本地邮箱投递验收
- **THEN** 后端、前端和依赖均使用专用端口、专用卷和临时配置
- **AND** `GET /api/user/email-verification-policy` 返回 `enabled:true`
- **AND** 仓库默认 `config.yaml` 与 `.env` 保持不变

### Requirement: Email verification acceptance coverage

本地验收 SHALL 覆盖已验证邮箱换绑进入待验证、未验证邮箱直接替换、重发与 token 轮换、取消待验证、错误输入、SMTP 投递失败、关闭 SMTP、限流以及链接和 token 安全。

#### Scenario: User-visible email states

- **WHEN** 本地验收执行邮箱换绑与确认流程
- **THEN** 已验证当前邮箱的候选进入 `pending_email`，当前邮箱保持有效
- **AND** 未验证当前邮箱的候选直接替换当前邮箱并触发验证
- **AND** 重发使旧 token 失效，取消清空待验证邮箱且不改变当前邮箱
- **AND** 无效、过期或已使用的 token 不改变邮箱状态

#### Scenario: Delivery and policy failures

- **WHEN** SMTP 不可用或 `smtp.enabled:false`
- **THEN** 换绑和重发返回稳定的 503 错误码，待验证状态按既有语义保留
- **AND** 关闭 SMTP 时策略端点为 `enabled:false`，确认与取消仍可用
- **AND** 同一用户每小时第 4 次签发返回 429

#### Scenario: Link and token safety

- **WHEN** 验收检查验证邮件和浏览器链接
- **THEN** 邮件链接使用受信任的 `browser_session.origin`
- **AND** token 只出现在 fragment 和确认请求体中，确认后 fragment 被清除
- **AND** 数据库只保存 token 哈希，日志、截图和证据不包含明文 token

### Requirement: Opt-in real SMTP acceptance

真实 SMTP 验收 SHALL 仅在本地验收通过且操作者提供有效凭据后执行。切换 SHALL 只修改临时 SMTP 配置并重启同一后端实例，不修改业务代码。

#### Scenario: Real SMTP delivery

- **WHEN** 操作者提供真实 SMTP 凭据并启用真实外发
- **THEN** 使用 `smtp.qq.com:465` 与 `tls_mode:implicit` 发送并完成确认
- **AND** `SMTP_FROM` 与认证账号一致
- **AND** 授权码、Cookie、密码和 token 不进入证据、日志或截图

#### Scenario: Real SMTP unavailable

- **WHEN** 没有可用真实 SMTP 凭据或外发失败
- **THEN** 该项标记为未执行或外部阻塞
- **AND** 已通过的本地验收证据仍然有效

### Requirement: Evidence and failure reporting

验收 SHALL 记录可复核的环境、命令、实际结果和清理事实。任何失败 SHALL 先停止并汇报，等待操作者查看后再修复。

#### Scenario: Acceptance evidence

- **WHEN** 验收结束
- **THEN** 证据文件包含环境与端口、自动化门禁结果、场景结果、HTTP 状态／错误码、Mailpit message ID、脱敏截图和清理结果
- **AND** 未执行的场景明确标记，不写成通过

#### Scenario: Failure before remediation

- **WHEN** 任一场景失败或出现非预期状态
- **THEN** 验收立即暂停，不自动修改代码或配置
- **AND** 汇报预期、实际、证据和未执行范围
- **AND** 只有操作者查看并确认修复方向后才继续

