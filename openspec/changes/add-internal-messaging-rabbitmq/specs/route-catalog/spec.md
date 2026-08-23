# route-catalog Delta Specification

## ADDED Requirements

### Requirement: WebSocket 升级入口声明

系统 SHALL 将内部消息 WebSocket 升级入口作为 Route Catalog 中已校验的 Authenticated 原生协议入口声明，并由 App 统一注册。

#### Scenario: 声明消息 WebSocket 入口
- **WHEN** Messaging 提供 `GET /api/user/messages/ws` 升级入口
- **THEN** Route Descriptor SHALL 声明 Method、Path、Authenticated Access Level、Handler、API 名称、分组和 OpenAPI 原生 WebSocket 描述
- **AND** Descriptor SHALL 声明升级前可能返回的四字段 JSON 错误
- **AND** Descriptor SHALL 标明升级成功后不使用业务 JSON 信封

#### Scenario: WebSocket 入口注册
- **WHEN** Route Catalog 完成 Descriptor 校验
- **THEN** App SHALL 按唯一 Route Descriptor 注册 WebSocket 升级入口
- **AND** Messaging SHALL NOT 直接调用 Gin Engine 注册方法

#### Scenario: WebSocket 路由描述缺失
- **WHEN** WebSocket Descriptor 缺少 ticket 错误定义、升级协议描述或访问等级
- **THEN** Route Catalog SHALL 在 Gin 注册前返回校验错误
- **AND** App SHALL NOT 注册该入口

### Requirement: 消息 Route Snapshot 与 API 元数据同步

系统 SHALL 将消息 HTTP 路由和 WebSocket 升级入口纳入同一份 Route Catalog Snapshot。

#### Scenario: 同步消息路由
- **WHEN** 管理员触发 API 路由同步或权限同步
- **THEN** API Metadata 和 Authorization SHALL 消费包含消息 HTTP 路由及其访问等级的 Snapshot
- **AND** 同步 SHALL NOT 扫描 Gin Engine 或 RabbitMQ 拓扑发现消息入口
