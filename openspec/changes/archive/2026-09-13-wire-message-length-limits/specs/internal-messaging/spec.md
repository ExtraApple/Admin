## ADDED Requirements

### Requirement: 消息长度上限来自配置

系统 SHALL 从 `messaging.max_title_runes` 与 `messaging.max_body_runes`
读取消息标题与正文的长度上限，SHALL NOT 使用与配置无关的硬编码值。

具体地：

- 标题 SHALL 按 Unicode 字符数计数，上限取自 `max_title_runes`。
- Markdown 正文 SHALL 按 Unicode 字符数计数，上限取自 `max_body_runes`。
- 默认值 SHALL 为标题 100、正文 20,000，与既往行为一致。
- 配置值为 `0` SHALL 视为"未配置"，取上述默认值；
  配置值为负数或高于上限 SHALL 在配置加载阶段失败。
- 校验 SHALL 在领域层执行，且领域层 SHALL NOT 依赖配置模块；
  上限 SHALL 以值的形式由应用层传入。
- 超出上限时系统 SHALL 返回稳定错误码 `MSG_TITLE_TOO_LONG`
  或 `MSG_BODY_TOO_LONG`。
- 清洗后 HTML 的 128 KiB 上限 SHALL 保持独立且不可通过配置调整。

#### Scenario: 默认配置下的长度边界

- **WHEN** 应用以默认配置启动（`max_title_runes = 100`、`max_body_runes = 20000`）
- **AND** 提交恰好 100 个 Unicode 字符的标题与恰好 20,000 个 Unicode 字符的正文
- **THEN** 系统 SHALL 接受并完成编译

#### Scenario: 默认配置下超出标题上限

- **WHEN** 应用以默认配置启动
- **AND** 提交 101 个 Unicode 字符的标题
- **THEN** 系统 SHALL 拒绝并返回 `MSG_TITLE_TOO_LONG`

#### Scenario: 默认配置下超出正文上限

- **WHEN** 应用以默认配置启动
- **AND** 提交 20,001 个 Unicode 字符的 Markdown 正文
- **THEN** 系统 SHALL 拒绝并返回 `MSG_BODY_TOO_LONG`

#### Scenario: 自定义上限生效

- **WHEN** 配置 `max_title_runes` 为 120 且 `max_body_runes` 为 25000
- **AND** 提交 120 个 Unicode 字符的标题与 25,000 个 Unicode 字符的正文
- **THEN** 系统 SHALL 接受并完成编译

#### Scenario: 长度按 Unicode 字符而非字节计数

- **WHEN** 提交由多字节字符组成的标题或正文
- **THEN** 系统 SHALL 按 Unicode 字符数判定是否超限
- **AND** 系统 SHALL NOT 按字节长度判定

#### Scenario: 清洗后 HTML 上限不可配置

- **WHEN** 消息正文通过长度校验但清洗后 HTML 超过 128 KiB
- **THEN** 系统 SHALL 拒绝并返回 `MSG_HTML_TOO_LARGE`
- **AND** 该上限 SHALL NOT 受 `max_body_runes` 配置影响

#### Scenario: 非法上限配置被拒绝

- **WHEN** 配置的 `max_title_runes` 或 `max_body_runes` 为负数或高于允许上限
- **THEN** 系统 SHALL 在配置加载阶段失败
- **AND** 系统 SHALL NOT 以静默默认值继续启动

#### Scenario: 未配置的上限取默认值

- **WHEN** 配置的 `max_title_runes` 或 `max_body_runes` 为 `0` 或该键缺失
- **THEN** 系统 SHALL 使用默认值（标题 100、正文 20,000）
- **AND** 系统 SHALL NOT 在配置加载阶段失败
