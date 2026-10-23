## 1. 审计基线与真实组合环境

- [ ] 1.1 固定当前代码基线、Go／服务构建信息、`route-catalog`、`modular-layered-architecture`、`api-management`、`api-response-contract` 规格和相关 runbook 版本。
- [ ] 1.2 收集所有 Route Descriptor、已校验 Route Catalog Snapshot、App 技术路由和公开错误定义，记录规范化规则与代码基线。
- [ ] 1.3 启动当前真实 App 组合根，验证真实 HTTP、JWT、路由注册、API Metadata／Permission 读取和 OpenAPI 入口；按需准备隔离 MySQL／Redis／MinIO。
- [ ] 1.4 创建去敏 Runner、代表性 Public／Authenticated／PermissionControlled 身份、范围内／范围外管理员、超级管理员和技术路由夹具；准备可清理的错误状态。
- [ ] 1.5 确认本 change 不修改产品代码、当前主规格、路由、权限码、模型、Redis key、MinIO bucket 或第三方依赖。

## 2. Route Catalog、Gin 与 OpenAPI 集合

- [ ] 2.1 规范化并比较 Route Catalog Snapshot 与 Gin 实际业务路由，验证无遗漏、重复、旁路注册和数据库未声明路由注册。
- [ ] 2.2 验证 Descriptor 的 Method、Path、Handler、Access Level、默认 Permission Code、审计分类、公开错误定义、请求／响应 Schema 和原生协议描述完整。
- [ ] 2.3 通过受控校验入口验证重复 Method + Path、缺字段、错误 owner、缺 Schema 或错误定义冲突在 Gin 注册前阻断，并记录启动结果。
- [ ] 2.4 比较 Route Catalog Snapshot 与 API Metadata／Permission 同步输入和输出，确认同步不扫描 Gin、不新增未声明路由且保留批准的首次同步例外。
- [ ] 2.5 对 OpenAPI `paths`、Method、Schema、错误码、认证要求和原生协议标记执行语义比较；验证成功文档未被业务 JSON 信封包装。
- [ ] 2.6 触发或注入 `/docs/openapi.json` 成功提交前的生成失败，验证 HTTP 500、四字段错误信封、`HTTP_INTERNAL_ERROR` 和无内部错误泄露。

## 3. Access Level、Metadata 与 Permission 矩阵

- [ ] 3.1 为每类代表性路由执行匿名、无效／过期 Token、有效身份无权限、范围外、有权限且范围内请求，记录状态、稳定错误码、响应 envelope 和策略事实。
- [ ] 3.2 验证 `need_auth=0` 对 PermissionControlled 路由只跳过 Permission Code 检查，仍要求有效登录身份，且不能把静态 Access Level 降级为 Public。
- [ ] 3.3 验证 API Metadata 缺失、停用、路径偏离和权限码错误时的既有安全语义；确认数据库记录不能替换 Handler 或创建新 Gin 路由。
- [ ] 3.4 验证超级管理员、首次 API／Permission 同步例外、普通管理员和范围外管理员的实际访问差异；不将超级管理员放行推导为普通权限通过。
- [ ] 3.5 验证消息、Outbox／Consumer／DLQ、邮箱和 WebSocket 代表性入口的默认 Permission Code、Access Level、审计分类和原生协议标记。

## 4. 组合根、技术路由与依赖接线

- [ ] 4.1 建立 App required／optional Dependencies 接线矩阵，记录字段、提供方、实际赋值位置、fallback／禁用语义和消费路径。
- [ ] 4.2 运行架构护栏和真实启动验证，确认 required 缺失不会静默放行，optional 依赖行为符合声明，业务模块不自行创建基础设施客户端。
- [ ] 4.3 验证 App 是唯一 Gin 注册点，业务模块只返回 Route Descriptor，后台任务和 Messaging／API Doc／Audit 适配器均由组合根接线。
- [ ] 4.4 分别验证 `/ping`、`/api/ready`、`/docs`、`/docs/openapi.json` 的归属、Access Level、RabbitMQ degraded 语义、Metadata 消费边界和公开响应内容。
- [ ] 4.5 验证未匹配 `/api` 路径和已存在路径的错误 Method 返回信封化 404／405，且生产 SPA 回退不会吞掉 API 错误。

## 5. 响应、错误 owner 与原生协议

- [ ] 5.1 对代表性业务 JSON 成功响应验证 HTTP 状态、`code`、空 `error_code`、`msg=success`、`data` 形状、空集合 `[]` 和无载荷 `null`。
- [ ] 5.2 对代表性 400／401／403／404／405／409／422／429／500／503 验证状态、稳定错误码、安全英文 fallback、字段详情和 `data` 形状。
- [ ] 5.3 为每条代表性 Descriptor 核对 Handler、认证中间件、Permission 中间件和恢复路径的公开错误 owner；确认同一错误码不发生状态、消息或安全 Schema 冲突。
- [ ] 5.4 验证 Swagger HTML、原始 OpenAPI JSON、文件下载、头像、Blob、WebSocket upgrade 和 CORS 预检的原生成功 Content-Type／Schema；成功流提交后错误不得追加 JSON。
- [ ] 5.5 验证原生端点提交前失败使用安全 JSON 错误契约，提交后失败只记录并终止；不把原生成功内容当作四字段信封解析。

## 6. 证据、报告与清理

- [ ] 6.1 为每个 Case 保存“静态声明 → 装配结果 → HTTP 操作 → 响应／错误 → Snapshot／Metadata／Permission／OpenAPI 投影 → 日志结果”的去敏记录。
- [ ] 6.2 写入 `docs/reviews/evidence/a3-assembly-response.json`，保存规范化集合差异、访问矩阵、依赖摘要、响应摘要、错误 owner 和 OpenAPI 语义比较结果。
- [ ] 6.3 写入 `docs/reviews/contract-readiness-a3-assembly-response.md`，按契约一致、实现偏差、证据不足、语义／文档待裁决分组，列出未覆盖边界。
- [ ] 6.4 若发现明确装配、权限或响应偏差，保存原始去敏证据并停止本 change 内修复；后续产品、规格或文档处理通过独立 change 分流。
- [ ] 6.5 复核报告和 JSON 不含凭据、Token、Authorization header、DSN、对象路径、原始内部错误或未脱敏用户数据。
- [ ] 6.6 清理本 change 创建的服务进程、schema、Redis 数据、临时对象、Runner、配置、凭据和日志；确认共享业务库未写入。
- [ ] 6.7 运行 OpenSpec 严格校验，复核 proposal、design、审计型 delta 和 tasks 边界一致；归档时使用 `--skip-specs`，不将审计型 delta 同步到产品主规格。
