## Why

Admin 的最终目标不是只有后端接口，而是形成可运行、可验证的完整前后端链路。当前响应契约 Change 同时涉及后端 HTTP 协议、OpenAPI、前端 API Client、错误码本地化和真实页面行为；如果把后端和前端混成一个不可分阶段的实施步骤，在 `web/` 尚未创建时就无法推进后端工作。

本 Change 调整为两个实施部分：Backend Part 和 Frontend Part。后端部分可以先实现并通过后端验收，但由于这是破坏性协议切换，后端不得独立发布；前端部分完成后，必须使用真实后端和真实前端进行前后端联合验收，整个 Change 才算完成。

## What Changes

- **BREAKING(HTTP response contract)**：业务 JSON 统一使用 `code`、`error_code`、`msg` 和 `data` 四字段信封。
- 将实施工作明确分为 **Backend Part** 和 **Frontend Part**，两部分共享同一套响应、错误码和 OpenAPI 契约。
- Backend Part 允许在没有 `web/` 的情况下实施和进行后端验收。
- Backend Acceptance 只证明后端实现正确，不代表后端可以独立部署或发布。
- Frontend Part 等根目录 `web/` 创建并确认技术栈、集中 API Client 和本地化接缝后实施。
- Frontend Acceptance 覆盖前端 Client、错误码本地化、字段错误展示和原生响应处理。
- 增加 Frontend/Backend Integration Acceptance，使用真实后端和真实前端验证登录、锁定、权限、校验、空集合、文件和未知错误码 fallback。
- 保持一次发布切换，不保留双信封、旧字段别名、兼容 Header、版本协商或兼容端点。
- 保留 Swagger HTML、成功 OpenAPI JSON、文件下载、头像读取和成功 CORS 预检的原生协议。
- 继续复用 Upload Security 的既有稳定错误码，不重命名已有公开值。

## Capabilities

### New Capabilities

- `api-response-contract`: 定义后端响应信封、前端消费契约、错误码、原生协议例外以及分阶段验收边界。

### Modified Capabilities

- `route-catalog`: 在 Route Descriptor 中记录并验证逐路由顶层和字段级公开错误定义。
- `api-management`: 根据经过验证的 Route Catalog 错误定义生成 OpenAPI 响应，不包装成功的原始 OpenAPI 文档。
- `auth`: 统一登录、验证码、锁定和 Token 错误，并暴露安全的结构化详情。
- `logging`: 记录分类服务端错误及内部 Cause，同时不记录敏感信息或把预期 4xx 当作服务端错误。

## Impact

- Backend Part 影响 Platform HTTP Response、Route Catalog、API Doc、App Middleware、所有后端 HTTP Adapter 和后端契约测试。
- Frontend Part 影响未来根目录 `web/` 的 API Client、错误类型、本地化、表单状态和原生响应调用。
- Frontend/Backend Integration Acceptance 影响真实后端、真实前端和浏览器端到端验证。
- HTTP Method、Path、Access Level、Permission Code、数据库 Schema、Redis Key、MinIO Bucket、对象命名和业务事务行为不变。
- 后端验收通过后，代码可以进入等待前端的开发分支，但不得作为独立生产发布物。
- 只有 Backend Acceptance、Frontend Acceptance 和 Frontend/Backend Integration Acceptance 全部通过，Change 才能完成或归档。
- 不存在前端时，后端部分可以完成；不存在前端时，Change 整体不能完成。
