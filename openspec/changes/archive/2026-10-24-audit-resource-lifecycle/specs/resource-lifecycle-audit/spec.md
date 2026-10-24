## ADDED Requirements

### Requirement: 资源与生命周期审计证据
本 change SHALL 以真实组合路径和去敏机器证据核对资源与生命周期边界，并 SHALL 将消息可见性、文件／对象／元数据一致性以及审计日志范围作为同一 A2 审计的独立章节记录。

#### Scenario: 真实组合路径与投影证据
- **WHEN** 审计执行代表性正常、拒绝、失败或确定性交错 Case
- **THEN** 审计记录 SHALL 同时保留前置事实、实际操作、HTTP／错误结果、数据库投影、对象存储投影和日志投影
- **AND** Case SHALL 标记其真实基础设施或测试专用故障注入层级

#### Scenario: 证据分类与停止边界
- **WHEN** Case 已完成或因环境、入口或语义原因无法完成
- **THEN** 审计 SHALL 将结果分类为契约一致、实现偏差、证据不足或语义待裁决
- **AND** 未执行路径 SHALL NOT 被标记为通过
- **AND** 明确产品偏差 SHALL 只保留去敏证据，不在本 change 内修复

#### Scenario: 敏感证据去除
- **WHEN** 审计写入报告或机器可读证据
- **THEN** 证据 SHALL NOT 包含密码、Token、Authorization header、DSN、对象路径、正文、HTML、图片内容或原始基础设施错误
- **AND** 证据 SHALL 保留足以复核状态码、稳定错误码、关系摘要、队列／重试摘要、对象存在性分类和日志字段结论的信息

### Requirement: 审计型规格不改变产品运行契约
本 change 的 `resource-lifecycle-audit` delta SHALL 只约束审计工件和证据，不得改变 `file-management`、`internal-messaging` 或 `logging` 的运行时 Requirement、路由、权限码、模型、Redis key 或 MinIO bucket。

#### Scenario: 审计发现产品偏差
- **WHEN** A2 Case 证明当前实现偏离已发布产品规格
- **THEN** 本 change SHALL 记录偏差和适用证据
- **AND** 后续产品修复或规格裁决 SHALL 通过独立 change 处理

#### Scenario: 审计 change 归档
- **WHEN** 本 change 的审计任务完成并归档
- **THEN** 归档 SHALL 使用跳过产品规格同步的流程
- **AND** `resource-lifecycle-audit` SHALL NOT 被合并进产品主规格作为运行时能力
