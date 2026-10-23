## Context

本 change 是授权运行时和资源生命周期之后的第三个顶层审计切片：装配与响应契约。当前 `route-catalog`、`modular-layered-architecture`、`api-management` 和 `api-response-contract` 已规定 Route Descriptor、Catalog Snapshot、App 唯一注册、Access Level、API Metadata、OpenAPI、错误 owner、统一 JSON 信封和原生响应边界；本 change 不重新设计这些契约。

A3 需要同时观察代码声明、App 组合根、Gin 实际路由、API Metadata／Permission 同步、OpenAPI paths 和真实 HTTP 响应。静态路由矩阵或单个成功接口不能证明“声明即生效”，也不能证明数据库策略没有改变最低认证要求。

审计按一份报告交付，包含四个章节：

1. Route Catalog／Gin／OpenAPI 集合；
2. Access Level／API Metadata／Permission；
3. App 组合根与依赖接线；
4. 业务 JSON、原生响应和技术路由。

## Goals / Non-Goals

**Goals:**

- 固定当前代码基线、已发布规格、运行配置、路由快照和去敏环境边界。
- 使用真实 App 组合根、真实 HTTP／JWT 和结构化快照比较；必要时使用隔离 MySQL／Redis／MinIO 支撑代表性入口，但不扩大为资源生命周期审计。
- 对 Public、Authenticated、PermissionControlled、技术路由、原生响应、未匹配路径和错误 Method 建立代表性矩阵。
- 比较规范化后的 Route Catalog、Gin 业务路由、API Metadata／Permission 同步结果和 OpenAPI paths；核对同一 Snapshot 的消费者是否漂移。
- 验证 required／optional 依赖的真实接线、启动前 Catalog 阻断、运行时 OpenAPI 失败和中间件错误 owner。
- 核对四字段业务 JSON envelope、稳定错误码、`data` 形状、原生 Content-Type、`/ping`、`/api/ready`、`/docs` 和 `/docs/openapi.json`。
- 将结果分类为契约一致、实现偏差、证据不足或语义／文档待裁决，并保存去敏报告和机器证据。

**Non-Goals:**

- 不修改产品代码、当前主规格、永久测试、路由、权限码、数据模型、Redis key、MinIO bucket 或第三方依赖。
- 不创建新的业务路由、API Metadata、Permission Code、错误码或响应 DTO。
- 不逐端点验收全部 `/api`，不重新审计 API 权限同步的完整事务、菜单联动、用户 Token 影响或 A2 文件／消息生命周期。
- 不进行前端联合验收、生产部署切换、随机压力测试、容量基准或未来路由设计。
- 不把展示字段漂移、文档漂移和运行时安全偏差混为一个结论；不以静态声明替代真实 HTTP 证据。

## Decisions

### 1. 使用当前规格作为判据，审计型 delta 只记录证据契约

本 change 读取当前 `route-catalog`、`modular-layered-architecture`、`api-management` 和 `api-response-contract` 的 Requirement／Scenario。新增的 `assembly-response-audit` delta 只固定本次审计的证据格式、分类和停止边界，不改变产品运行契约。

归档时必须使用 `--skip-specs`，避免把审计流程同步进产品主规格。若发现产品偏差，保留证据并另建修复或规格裁决 change。

### 2. Route Catalog Snapshot 是代码事实源

Route Descriptor 和已校验 Snapshot 是 Method、Path、Handler、Access Level、默认 Permission Code、审计分类、公开错误定义和 OpenAPI operation 的代码事实源。API Metadata 只能配置已声明路由允许的运行时策略，不能新增路由、替换 Handler 或把 Authenticated／PermissionControlled 降级为匿名。

比较路由集合前规范化 Method、Path 参数和技术／原生协议标签；不使用字节顺序比较，也不将数据库中的未声明 API 视为实际路由。

### 3. 使用四类结构矩阵和代表性 HTTP

集合矩阵比较 Catalog、Gin、同步结果和 OpenAPI；访问矩阵覆盖匿名、无效／过期 Token、无权限、范围外、范围内、Metadata 缺失／停用／`need_auth=0`；接线矩阵覆盖 required／optional 依赖、App 构造、后台任务和技术入口；响应矩阵覆盖业务 JSON、原生协议、技术路由、未匹配路径和错误 Method。

每个风险类别选择代表性真实入口，不对全部业务 Endpoint 做笛卡尔积。

### 4. 区分启动前校验和请求级失败

重复或无效 Descriptor、错误 owner 冲突、缺少 Schema／公开错误定义和集合注册绕过属于启动前 Catalog 阻断，App 不应注册部分业务路由。`/docs/openapi.json` 在成功响应提交前生成失败属于请求级 HTTP 500 四字段错误；原生响应提交后失败不能追加 JSON。

### 5. 统一响应只约束业务 JSON 成功／错误路径

业务 JSON 路由严格比较 HTTP 状态、`code`、`error_code`、`msg` 和 `data`；错误 owner、状态、字段错误 Schema 和安全详情必须与 Descriptor／OpenAPI 一致。Swagger HTML、OpenAPI JSON、文件／头像流、WebSocket upgrade 和 CORS 预检保留原生成功协议，升级前或提交前错误仍使用适用的安全错误契约。

### 6. 技术路由单独核对语义和 Snapshot 边界

`/ping`、`/docs` 和 `/docs/openapi.json` 作为 App 技术路由单独验证；`/api/ready` 按当前消息／Route Catalog 约定核对其 Public、HTTP 200、受控 RabbitMQ 状态和 Snapshot 消费边界。若不同规格对归属存在冲突，分类为语义待裁决，不在审计中猜测改动。

### 7. 真实装配优先，静态分析辅助

真实 App 启动、真实 HTTP／JWT、实际 Gin 路由和最终 OpenAPI 是一级证据。源码、LSP、架构护栏和已有测试用于建立矩阵、解释失败和补充 required／optional 接线证据，但不能单独证明实际可达性或响应行为。

## Risks / Trade-offs

- **全量路由数量大**：代表性矩阵可能遗漏单个 Endpoint 特殊声明；报告必须列出入口选择和未覆盖集合，不能宣称全端点通过。
- **Metadata 与静态 Catalog 存在双层事实**：错误的优先级判断可能掩盖匿名降级；报告同时保存 Descriptor、Metadata 和实际 HTTP 结果。
- **OpenAPI 生成可能因环境配置失败**：区分 Catalog 启动校验、文档请求级失败和环境证据不足，不用 fallback 把错误改成成功。
- **技术路由规格归属可能不一致**：`/api/ready` 的 Snapshot／Metadata边界若冲突，保留语义待裁决，不擅自修改代码。
- **组合根启动成功不等于所有依赖已生效**：同时使用 required 接线矩阵和代表性运行请求；optional fallback 的结果必须单独记录。
- **文档漂移容易被误报为产品缺陷**：区分运行时集合／权限／响应偏差和只影响说明文本的文档问题。
