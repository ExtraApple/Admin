## MODIFIED Requirements

### Requirement: WebSocket 与 Messaging 入口声明
系统 SHALL 将内部消息 WebSocket ticket/upgrade、消息 HTTP 入口和 `/api/ready` 纳入同一份已校验 Route Catalog Snapshot。

#### Scenario: 原生 WebSocket upgrade
- **WHEN** Catalog 声明 `GET /api/user/messages/ws`
- **THEN** 该路由 SHALL 为 Authenticated、声明无请求体和 101 NoBody 成功响应，并标记原生 `websocket` 协议
- **AND** 升级前错误 SHALL 使用统一四字段错误信封

#### Scenario: RabbitMQ 就绪
- **WHEN** App 声明 `/api/ready`
- **THEN** 该入口 SHALL 与 `/ping` 的进程存活语义分离
- **AND** Broker 不可用时只报告受控 RabbitMQ 状态，不以 Consumer DLQ 告警改变 HTTP 路由或响应集合

#### Scenario: 消息 WebSocket 入口完整声明
- **WHEN** Messaging 提供 `GET /api/user/messages/ws` 升级入口
- **THEN** Route Descriptor SHALL 声明 Authenticated Access Level、Handler、API 名称、分组及原生 WebSocket OpenAPI 描述
- **AND** Descriptor SHALL 声明升级前四字段错误和升级成功不使用业务 JSON 信封

#### Scenario: WebSocket 入口由 App 注册
- **WHEN** Route Catalog 完成 Descriptor 校验
- **THEN** App SHALL 按唯一 Descriptor 注册 WebSocket 升级入口
- **AND** Messaging SHALL NOT 直接调用 Gin Engine 注册

#### Scenario: WebSocket Descriptor 缺失字段
- **WHEN** WebSocket Descriptor 缺少 ticket 错误定义、升级协议描述或访问等级
- **THEN** Route Catalog SHALL 在 Gin 注册前返回校验错误
- **AND** App SHALL NOT 注册该入口

#### Scenario: RabbitMQ 降级就绪入口
- **WHEN** App 提供 `GET /api/ready`
- **THEN** Route Descriptor SHALL 声明 Public、HTTP 200、`system` 分组和受控 RabbitMQ 状态 JSON 响应
- **AND** RabbitMQ `degraded` SHALL 使用 HTTP 200；待处置 Consumer DLQ SHALL NOT 改变入口状态
- **AND** 响应 SHALL NOT 暴露 Broker 地址、凭据、时间戳、Worker 身份或原始连接错误

#### Scenario: 消息路由快照同步
- **WHEN** 管理员触发 API 路由同步或权限同步
- **THEN** API Metadata 和 Authorization SHALL 消费包含消息 HTTP 路由及访问等级的 Route Catalog Snapshot
- **AND** 同步 SHALL NOT 扫描 Gin Engine 或 RabbitMQ 拓扑发现消息入口
