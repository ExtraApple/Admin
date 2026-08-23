# api-management Delta Specification

## ADDED Requirements

### Requirement: 消息 API 元数据同步

系统 SHALL 为内部消息 HTTP 路由同步 API 元数据和默认 Permission Code，并保留管理员可配置的启停策略。

#### Scenario: 创建消息 API 元数据
- **WHEN** Route Catalog Snapshot 包含内部消息 HTTP 路由且 `apis` 表不存在对应 Method + Path
- **THEN** API Metadata SHALL 创建包含名称、分组、Access Level 对应认证标记、默认审计标记和默认 Permission Code 的记录
- **AND** `method + path` SHALL 保持唯一

#### Scenario: 同步消息权限码
- **WHEN** 管理员调用 API 权限同步
- **THEN** 系统 SHALL 为消息 Permission Code 创建缺失的权限记录
- **AND** 已存在权限码 SHALL NOT 重复创建
- **AND** WebSocket 成功帧和 RabbitMQ Exchange/Queue SHALL NOT 被错误创建为 API Permission Code

#### Scenario: 消息接口动态授权
- **WHEN** 非超级管理员请求 `/api/admin/messages`、`/api/admin/announcements` 或 `/api/admin/message-categories` 下的接口
- **AND** API Metadata 已启用
- **AND** 用户拥有对应消息 Permission Code
- **THEN** 系统 SHALL 允许请求进入 Messaging Handler
- **AND** 用户缺少对应权限或超出组织数据范围时 SHALL 拒绝请求
