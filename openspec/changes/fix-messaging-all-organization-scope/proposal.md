## Why

A2 真实组合审计发现，超级管理员的 All-scope 消息管理路径返回 `500 / MSG_INTERNAL_ERROR`。`organization.ScopeReader` 已提供 `AllOrganizationIDs`，但 Messaging 组合根的组织适配器没有转发该能力，导致分类查询、群发创建和公告创建无法解析系统范围。

## What Changes

- 在 Messaging 组合根补齐 `AllOrganizationIDs` 能力转发。
- 复用现有 `organization.ScopeReader.AllOrganizationIDs` 和 `ListAllUnits` 语义。
- 让超级管理员能够通过现有真实路由读取分类、创建群发、创建并发布公告。
- 保持受限管理员的组织及子组织范围行为不变。
- 增加 All-scope 组合根和真实 HTTP 回归测试。
- 不新增路由、权限码、Redis key、MinIO bucket 或组织过滤规则。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `internal-messaging`: 明确系统范围管理员的组织集合解析必须使用既有 ScopeReader 能力；不改变既有消息路由或权限码。

## Impact

- 代码：`internal/app/messaging.go` 及 Messaging/应用组合测试。
- 规格 delta：仅用于记录现有产品契约的实现修复；本 change 归档时使用 `--skip-specs`，不同步主规格。
- 数据：无迁移、无历史数据修复。
- 运行时：现有超级管理员消息分类、群发、公告路径从内部错误恢复为既有成功/稳定业务错误语义。
- 非目标：不改变禁用组织过滤、软删除组织语义、受限管理员范围、消息模型、事件格式或组织模块接口。
