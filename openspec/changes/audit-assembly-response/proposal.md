## Why

授权运行时和资源生命周期审计已经分别固定边界，但 Route Catalog、Gin 实际注册、API Metadata／权限同步、App 组合根、OpenAPI 和统一响应之间仍缺少一套真实组合证据。当前系统最需要排除的是“已声明但未生效”“数据库配置改变静态安全边界”以及“文档／原生响应与真实入口漂移”，因此现在进行有限范围 A3 审计，而不是继续扩大业务功能。

## What Changes

- 建立一个有限范围的 A3 装配与响应契约审计，覆盖 Route Catalog／Gin／OpenAPI 集合、Access Level／API Metadata／Permission、App 依赖接线以及业务 JSON／原生响应／技术路由四个章节。
- 使用真实 App 组合根、真实 HTTP／JWT 和结构化快照比较，验证 Public、Authenticated、PermissionControlled、技术路由、原生响应、未匹配路径和错误 Method 的代表性行为。
- 核对同一 Route Catalog Snapshot 是否被 Gin 注册、API Metadata 同步、RBAC／Permission 同步、审计分类和 OpenAPI 共同消费；验证 required／optional 依赖的实际装配和启动失败边界。
- 核对 HTTP 状态、四字段 JSON envelope、稳定错误码、错误 owner、OpenAPI Schema、原生 Content-Type、`/ping`、`/api/ready`、`/docs` 和 `/docs/openapi.json` 的契约。
- 记录规范化路由集合、访问矩阵、依赖矩阵、响应矩阵和去敏机器证据；将结果分类为契约一致、实现偏差、证据不足或语义／文档待裁决。
- 发现明确产品偏差时只保存证据，不在本 change 内修改代码、规格、路由、权限或错误码；后续修复通过独立 change 处理。
- 不进行全端点验收、完整 API Management 事务审计、前端联合验收、生产部署切换、压力测试或未来路由设计。

## Capabilities

### New Capabilities

- `assembly-response-audit`: 固定 A3 装配与响应契约的快照、代表性 HTTP 证据、错误 owner、去敏字段和停止边界；这是审计工件契约，不是运行时产品能力。

### Modified Capabilities

无。`route-catalog`、`modular-layered-architecture`、`api-management` 和 `api-response-contract` 的当前 Requirement 作为审计判据，不在本 change 内修改。

## Impact

- 审计代码入口：`internal/app`、`internal/routecatalog`、`internal/apidoc`、`internal/apimetadata`、`internal/authorization` 及各业务 HTTP Adapter。
- 目标事实：Route Descriptor、Catalog Snapshot、Gin 实际路由、API Metadata／Permission 同步、App 依赖装配、OpenAPI paths、JSON／原生响应和技术路由。
- 证据产出：`docs/reviews/contract-readiness-a3-assembly-response.md`、`docs/reviews/evidence/a3-assembly-response.json` 和本 change 的 `specs/assembly-response-audit/spec.md`；内容去敏且不保存凭据、Token、DSN、对象路径或原始内部错误。
- 不新增路由、权限码、数据库表、Redis key、MinIO bucket、第三方依赖或产品运行时能力；审计型 delta 后续归档时必须跳过主规格同步。
