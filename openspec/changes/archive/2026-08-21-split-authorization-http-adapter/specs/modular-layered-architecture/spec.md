## MODIFIED Requirements

### Requirement: 模块内部按复杂度分层
复杂模块 SHALL 在模块内部按 Domain、Application 和 Adapter 分离职责，简单模块 SHALL 能够保持扁平而不创建空层；复杂 HTTP Adapter SHALL 进一步按现有业务能力保持局部内聚，而不是把路由聚合、全部 DTO、全部 Handler 和共享传输辅助长期集中在一个混合职责文件中。

#### Scenario: 复杂模块依赖方向
- **WHEN** 架构测试检查复杂模块的包依赖
- **THEN** HTTP 和基础设施 Adapter 可以依赖 Application
- **AND** Application 可以依赖 Domain
- **AND** Domain SHALL NOT 依赖 Application 或 Adapter

#### Scenario: 简单模块保持局部性
- **WHEN** 模块没有稳定领域规则、多个入口 Adapter 或复杂跨模块依赖
- **THEN** 模块 SHALL 能够在自身目录内保持扁平
- **AND** 模块 SHALL NOT 为形式统一创建无职责的 Domain、Application 或 Repository 包

#### Scenario: Authorization HTTP Adapter 按能力内聚
- **WHEN** 架构测试检查 `internal/authorization/adapters/http`
- **THEN** `routes.go` SHALL 只负责聚合 Authorization 的 Route Descriptor
- **AND** 角色、权限和权限分组的 Handler、请求 DTO、响应 DTO 及映射逻辑 SHALL 分别保留在对应能力文件
- **AND** 角色用户和数据范围 SHALL 与角色能力放置，角色权限 SHALL 与权限能力放置
- **AND** 只有 Handler 状态、Descriptor 构造和已被多个能力实际消费的传输辅助 SHALL 放入本包共享文件

#### Scenario: 拆分不创建虚假边界
- **WHEN** Authorization HTTP Adapter 按能力拆分文件
- **THEN** 系统 SHALL 保持单一 `internal/authorization/adapters/http` 包
- **AND** 系统 SHALL NOT 为角色、权限、权限分组或关联操作创建新的一级业务模块或无独立不变量的子包

#### Scenario: 拆分保持路由契约
- **WHEN** Authorization HTTP Adapter 完成文件拆分
- **THEN** Route Descriptor SHALL 保持原有 Method、Path、Access Level、名称、分组、Permission Code、审计分类、Handler 对应关系和 OpenAPI Schema
- **AND** Descriptor 的相对顺序 SHALL 保持不变
- **AND** HTTP 请求/响应 JSON、状态码和消息文本 SHALL 保持不变
