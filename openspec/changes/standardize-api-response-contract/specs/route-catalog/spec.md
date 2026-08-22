## MODIFIED Requirements

### Requirement: 模块声明 Route Descriptor
每个业务模块 SHALL 声明自身 HTTP 路由的 Route Descriptor，而不是直接向 Gin Engine 注册路由；Descriptor SHALL 同时声明 Handler 自身可能返回的公开成功和错误契约。

#### Scenario: 模块提供路由
- **WHEN** App 构造一个包含 HTTP 入口的业务模块
- **THEN** 该模块 SHALL 返回包含 HTTP Method、Path、Access Level、Handler、API 名称、分组、默认 Permission Code、默认审计分类及 OpenAPI 描述的 Route Descriptor
- **AND** 模块 SHALL NOT 调用 Gin 的 `GET`、`POST`、`PUT`、`PATCH`、`DELETE` 或其他注册方法

#### Scenario: OpenAPI 描述完整
- **WHEN** 模块提供包含 HTTP 入口的 Route Descriptor
- **THEN** Descriptor SHALL 为该路由提供完整的 OpenAPI operation 描述
- **AND** 需要请求体或响应体的接口 SHALL 提供对应 Schema
- **AND** 上传接口 SHALL 提供 `multipart/form-data` 和文件字段描述
- **AND** 每个 JSON 错误响应 SHALL 引用该路由可能返回的顶层公开错误定义
- **AND** HTTP 422 字段校验响应 SHALL 同时声明可能的字段级错误定义
- **AND** 缺少必需 OpenAPI 描述或公开错误定义时 Route Catalog SHALL 在 Gin 注册前返回校验错误
- **AND** 系统 SHALL NOT 静默生成遗漏该路由、Schema 或错误码的 OpenAPI 文档

### Requirement: Route Catalog 启动前校验路由
Route Catalog SHALL 在 Gin 注册前收集和校验全部 Route Descriptor、响应 Schema 和公开错误定义。

#### Scenario: 路由集合有效
- **WHEN** 所有 Descriptor 的规范化 `method + path` 唯一、字段组合有效且错误定义一致
- **THEN** Route Catalog SHALL 生成稳定的只读路由快照
- **AND** App SHALL 能够继续注册路由

#### Scenario: 路由重复
- **WHEN** 两个 Descriptor 使用相同的规范化 HTTP Method 和 Path
- **THEN** Route Catalog SHALL 返回启动错误
- **AND** App SHALL NOT 启动 HTTP 服务

#### Scenario: 路由描述无效
- **WHEN** Descriptor 缺少 Method、Path、Handler、使用未知 Access Level、缺少必需响应 Schema 或声明无效错误定义
- **THEN** Route Catalog SHALL 返回校验错误
- **AND** App SHALL NOT 注册该路由

#### Scenario: 错误码所有权冲突
- **WHEN** 两个不同所有者声明相同公开错误码
- **OR** 同一错误码被声明为不同状态、英文消息或安全数据 Schema
- **THEN** Route Catalog SHALL 返回启动错误
- **AND** App SHALL NOT 注册任何业务路由

#### Scenario: 重复引用同一错误定义
- **WHEN** 多条路由引用同一所有者的同一公开错误定义
- **THEN** Route Catalog SHALL 接受这些引用
- **AND** 只读快照中的定义 SHALL 保持相同语义且不能被消费者修改

## ADDED Requirements

### Requirement: App 组合中间件错误契约
App SHALL 在 Route Catalog 校验前，把实际包围 Handler 的认证和权限中间件错误定义加入对应路由。

#### Scenario: 组合 Public 路由
- **WHEN** App 组合 Public Route Descriptor
- **THEN** App SHALL NOT 增加仅由认证或 Permission Code 中间件产生的 401/403 错误

#### Scenario: 组合 Authenticated 路由
- **WHEN** App 组合 Authenticated Route Descriptor
- **THEN** App SHALL 加入认证中间件可能返回的稳定 401/403 错误定义

#### Scenario: 组合 PermissionControlled 路由
- **WHEN** App 组合 PermissionControlled Route Descriptor
- **THEN** App SHALL 加入认证和 API Metadata 权限中间件可能返回的稳定错误定义
- **AND** Handler 所属模块 SHALL NOT 为文档目的导入 Identity 或 API Metadata 的 HTTP Adapter

### Requirement: Route Catalog 约束错误响应 Schema
Route Catalog SHALL 确保业务 JSON 错误响应使用统一信封，并允许原生成功协议保持自身 Schema。

#### Scenario: 校验 JSON 错误响应
- **WHEN** Descriptor 声明 HTTP 4xx 或 5xx JSON 响应
- **THEN** Schema SHALL 包含 `code`、`error_code`、`msg` 和 `data`
- **AND** 每个错误定义状态 SHALL 与所在响应状态一致

#### Scenario: 校验原生成功响应
- **WHEN** Descriptor 声明 HTML、原始 OpenAPI JSON 或二进制成功响应
- **THEN** Route Catalog SHALL 保留对应 Content-Type 和原生 Schema
- **AND** Route Catalog SHALL NOT 要求成功内容使用业务 JSON 信封
