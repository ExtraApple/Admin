## MODIFIED Requirements

### Requirement: 自动化 API 文档
系统 SHALL 根据 Route Catalog 的已校验路由描述、公开错误定义和必要的 API Metadata Snapshot 生成 OpenAPI 文档，而不是扫描 Gin Engine。

#### Scenario: 访问 Swagger UI 页面
- **WHEN** `api_docs.enabled = true`
- **AND** 用户访问 `GET /docs`
- **THEN** 系统返回 Swagger UI 页面
- **AND** 成功 HTML SHALL NOT 使用业务 JSON 信封包装

#### Scenario: 获取 OpenAPI JSON
- **WHEN** `api_docs.enabled = true`
- **AND** 用户访问 `GET /docs/openapi.json`
- **THEN** 系统返回未包装的 OpenAPI 3.0 JSON
- **AND** 文档包含 `/api/` 前缀下的业务接口
- **AND** 文档结合 Route Catalog 与 `apis` 表元数据生成接口标题、分组、描述和认证要求
- **AND** 对已配置 DTO 映射的 JSON 请求接口生成请求体 Schema
- **AND** 上传接口生成 `multipart/form-data` 文件字段
- **AND** JSON 成功响应 SHALL 描述固定四字段成功信封
- **AND** JSON 错误响应 SHALL 按状态描述固定四字段错误信封
- **AND** 每个错误状态 SHALL 枚举该路由允许的顶层 `error_code`
- **AND** HTTP 422 字段校验响应 SHALL 枚举允许的字段级 `error_code` 并描述 `data.fields`
- **AND** 文档 SHALL 包含 App 根据 Access Level 组合的认证和权限中间件错误
- **AND** 二进制、头像和其他原生成功响应 SHALL 保留对应 Content-Type 和原生 Schema
- **AND** API Doc SHALL NOT 扫描 Gin Engine 或查询全局数据库状态

#### Scenario: OpenAPI 生成失败
- **WHEN** `GET /docs/openapi.json` 在成功文档提交前无法生成文档
- **THEN** 系统 SHALL 返回 HTTP 500 四字段 JSON 错误信封
- **AND** `error_code` SHALL 为稳定技术错误码
- **AND** 响应 SHALL NOT 包含内部 Metadata、数据库或生成器错误

#### Scenario: 关闭自动化 API 文档
- **WHEN** `api_docs.enabled = false`
- **THEN** 系统不注册 `/docs` 和 `/docs/openapi.json` 路由
