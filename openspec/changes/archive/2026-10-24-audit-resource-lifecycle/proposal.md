## Why

授权运行时审计已经闭合其有限范围，但文件、消息可见性和审计日志仍缺少一套按当前已发布契约整理的真实组合证据。现有单元测试、局部 C5 验收和静态代码观察不能单独证明跨 HTTP、JWT、MySQL、Redis、MinIO 及后台任务边界的一致性；现在需要在创建任何修复 change 前，先区分契约一致、实现偏差、证据不足和语义待裁决。

## What Changes

- 建立一个有限范围的 A2 资源与生命周期证据审计，内部覆盖三个章节：消息当前可见性与撤销边界、文件／对象／元数据失败一致性、审计日志脱敏与范围查询。
- 使用真实 App 组合根、HTTP／JWT、隔离 MySQL／Redis／MinIO 和确定性交错，验证普通文件、消息图片、私信／群发／公告可见性、清理队列、轮转及后台日志的当前行为。
- 为正常、拒绝、关键故障和并发场景保存“前置事实 → 操作 → HTTP／错误结果 → 数据库投影 → 对象存储投影 → 日志投影”的去敏证据。
- 将每个场景分类为契约一致、实现偏差、证据不足或语义待裁决；发现明确产品偏差时只记录证据，不在本 change 内修复，后续另建修复 change。
- 只新增审计报告和机器可读证据；不修改产品代码、当前主规格、永久测试、路由、权限码、数据模型、Redis key、MinIO bucket 或第三方依赖。
- 不把完整 RabbitMQ／Consumer／DLQ、头像、历史文件批量重新验证、未来通知 Consumer、全消息功能、生产破坏性操作或 A3 装配与响应审计纳入本 change。

## Capabilities

### New Capabilities

- `resource-lifecycle-audit`: 固定本 change 的资源与生命周期审计证据、Case 分类、去敏字段和停止边界；这是审计工件契约，不是运行时产品能力。

### Modified Capabilities

无。`file-management`、`internal-messaging` 和 `logging` 的当前 Requirement 继续作为审计判据，不在本 change 内修改。

## Impact

- 审计代码入口：`internal/app`、`internal/files`、`internal/messaging`、`internal/audit` 及其 HTTP／GORM／对象存储适配器。
- 目标行为边界：消息列表／详情／图片读取、普通文件上传／下载／删除／轮转、消息图片绑定与清理、清理重试／dead／终结、Outbox／Consumer／DLQ 与运行日志交界。
- 证据产出：`docs/reviews/contract-readiness-a2-resource-lifecycle.md`、`docs/reviews/evidence/a2-resource-lifecycle.json` 和本 change 的 `specs/resource-lifecycle-audit/spec.md`；内容去敏且不保存密码、Token、Authorization header、DSN、对象路径或原始基础设施错误。审计型 delta 不同步到产品主规格。
- 审计只使用独立临时 schema、Redis namespace、MinIO 对象和临时 Runner；完成后清理本 change 拥有的资源，不触碰共享业务库或生产数据。
