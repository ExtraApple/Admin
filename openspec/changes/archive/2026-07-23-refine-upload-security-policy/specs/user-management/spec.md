## ADDED Requirements

### Requirement: 头像内容摘要
系统 SHALL 对用户头像标准化后实际写入 MinIO 的内容计算服务端 SHA-256，并与可信头像元数据一起持久化。

#### Scenario: 上传头像建立摘要
- **WHEN** JPEG、PNG 或 WebP 头像通过验证并完成标准化重新编码
- **THEN** 系统 SHALL 对重新编码后的完整 JPEG 或 PNG 字节计算 SHA-256
- **AND** 系统 SHALL 将 64 个小写十六进制字符的 `avatar_content_sha256` 与头像 object name、规范 MIME、验证状态和验证时间在同一次数据库更新中持久化
- **AND** 系统 SHALL NOT 保存原始上传图片的摘要作为可信头像摘要

#### Scenario: 恢复默认头像清除摘要
- **WHEN** 用户成功通过 `DELETE /api/user/avatar` 恢复系统默认头像
- **THEN** 系统 SHALL 清空 `avatar_content_sha256` 及其他可信自定义头像元数据

#### Scenario: 既有可信头像没有摘要
- **WHEN** 用户具有本策略上线前创建的可信头像且 `avatar_content_sha256` 为空
- **THEN** 系统 SHALL 保留现有可信头像行为
- **AND** 系统 SHALL NOT 在服务启动或头像读取时自动读取 MinIO 补算摘要
- **AND** 用户下次成功上传头像时 SHALL 建立新的摘要基线

#### Scenario: 头像摘要不对外暴露
- **WHEN** 系统返回用户资料、登录信息、初始化上下文或头像内容
- **THEN** 响应 SHALL NOT 包含 `avatar_content_sha256`
- **AND** 系统 SHALL NOT 将头像摘要写入 object key、头像 URL、普通运行日志或审计元数据

#### Scenario: 头像摘要用途受限
- **WHEN** 系统生成或保存头像摘要
- **THEN** 系统 SHALL NOT 信任客户端提供的摘要
- **AND** 系统 SHALL NOT 将摘要用于自动去重、唯一约束或恶意图片判定
