# file-management Delta Specification

## ADDED Requirements

### Requirement: 消息图片专用用途

系统 SHALL 在不放宽普通文件接口图片拒绝规则的前提下，支持由内部消息模块调用的专用消息图片用途。

#### Scenario: 通过消息图片入口上传
- **WHEN** 已认证用户或具备消息管理权限的管理员通过消息图片专用入口上传 JPEG、PNG 或 WebP
- **AND** 图片通过完整解码、大小、格式和尺寸验证
- **THEN** Files SHALL 创建带有消息图片用途和消息所有者引用的 File Record
- **AND** 普通管理员文件上传接口 SHALL NOT 接受该图片

#### Scenario: 普通文件入口上传图片
- **WHEN** 客户端通过 `POST /api/admin/files` 上传 JPEG、PNG、WebP、SVG 或其他图片
- **THEN** 系统 SHALL 保持现有 HTTP 415 拒绝行为
- **AND** 系统 SHALL NOT 因消息图片用途而写入普通文件记录

### Requirement: 消息图片受消息可见性控制

系统 SHALL 允许消息模块按当前消息可见性读取消息图片，并禁止脱离消息授权直接访问。

#### Scenario: 读取可见消息图片
- **WHEN** 用户当前有权查看引用该图片的消息
- **THEN** Files SHALL 返回经过验证的图片内容和规范 MIME
- **AND** 响应 SHALL NOT 暴露 MinIO bucket、object key 或存储凭据

#### Scenario: 读取不可见消息图片
- **WHEN** 消息已撤销、私信关系已删除、消息已过期或用户不再符合动态受众
- **THEN** Files SHALL 拒绝图片读取
- **AND** 系统 SHALL NOT 返回图片内容或存储状态

### Requirement: 消息图片安全审计边界

系统 SHALL 对消息图片上传和拒绝记录受控安全元数据。

#### Scenario: 消息图片验证成功
- **WHEN** 消息图片通过验证并保存
- **THEN** Audit metadata SHALL 记录消息图片用途、清洗文件名、大小、声明 MIME、检测 MIME、`accepted` 结果和策略版本
- **AND** metadata SHALL NOT 记录图片内容、object key、签名 URL 或 MinIO 凭据

#### Scenario: 消息图片验证失败
- **WHEN** 消息图片因大小、格式、解码或尺寸策略被拒绝
- **THEN** Audit metadata SHALL 记录 `rejected` 结果和稳定原因码
- **AND** metadata SHALL NOT 记录解析器原始错误或图片字节
