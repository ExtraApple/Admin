# api-response-contract Specification

## Purpose

定义业务 JSON 响应的统一四字段信封、HTTP 失败分类、安全错误详情、原生成功协议例外，以及后端与前端分阶段发布门禁。

## Requirements

### Requirement: 业务 JSON 响应使用固定信封
系统 SHALL 对业务 JSON 响应使用且只使用 `code`、`error_code`、`msg` 和 `data` 四个顶层字段。

#### Scenario: 返回 JSON 成功响应
- **WHEN** `/api/*` 业务路由或 `/ping` 成功返回 JSON
- **THEN** `code` SHALL 等于实际 HTTP 状态码
- **AND** `error_code` SHALL 为空字符串
- **AND** `msg` SHALL 固定为 `success`
- **AND** `data` SHALL 包含该接口原有业务载荷而不额外改变分页或列表结构

#### Scenario: 返回 JSON 错误响应
- **WHEN** 业务路由、中间件、技术路由或恢复逻辑在提交原生成功响应前返回 JSON 错误
- **THEN** `code` SHALL 等于实际 HTTP 状态码
- **AND** `error_code` SHALL 为非空稳定机器码
- **AND** `msg` SHALL 为不包含内部原因的安全英文 fallback
- **AND** `data` SHALL 默认为 `null`
- **AND** 响应 SHALL NOT 增加第五个顶层字段

#### Scenario: 返回无载荷成功响应
- **WHEN** 成功操作没有业务返回值
- **THEN** 系统 SHALL 返回 `data: null`
- **AND** 系统 SHALL NOT 省略 `data`

#### Scenario: 返回集合
- **WHEN** 成功响应的业务载荷是集合且结果为空
- **THEN** 对应集合 SHALL 序列化为 `[]`
- **AND** 对应集合 SHALL NOT 序列化为 `null`

### Requirement: HTTP 状态表达失败类别
系统 SHALL 使用 HTTP 状态表达协议和业务失败类别，而不是把所有 Application 错误转换为 HTTP 400。

#### Scenario: 请求无法按协议解析
- **WHEN** JSON 语法损坏、JSON 类型不兼容、path/query 语法无效或传输请求格式损坏
- **THEN** 系统 SHALL 返回 HTTP 400 和 `HTTP_*` 稳定错误码

#### Scenario: 认证失败
- **WHEN** 登录凭据错误，或 Access/Refresh Token 缺失、无效、过期或用途错误
- **THEN** 系统 SHALL 返回 HTTP 401 和对应 `AUTHN_*` 错误码

#### Scenario: 授权失败
- **WHEN** 请求已经建立身份但账号状态、API 策略或 Permission Code 不允许访问
- **THEN** 系统 SHALL 返回 HTTP 403 和对应模块错误码

#### Scenario: 资源或方法不存在
- **WHEN** 业务资源不存在
- **THEN** 系统 SHALL 返回 HTTP 404 和资源所有模块的稳定错误码
- **AND** 未匹配的 `/api` 路由 SHALL 返回信封化 HTTP 404
- **AND** 已存在 `/api` 路由使用不支持的方法 SHALL 返回信封化 HTTP 405

#### Scenario: 冲突和领域验证失败
- **WHEN** 请求违反唯一性、保护资源或状态转换约束
- **THEN** 系统 SHALL 返回 HTTP 409
- **AND** 当请求语法正确但验证码、字段或领域规则不满足时，系统 SHALL 返回 HTTP 422

#### Scenario: 大小、媒体和登录限制
- **WHEN** 请求体或文件超过限制
- **THEN** 系统 SHALL 返回 HTTP 413
- **AND** 不支持或不匹配的媒体类型 SHALL 返回 HTTP 415
- **AND** 渐进式登录锁定 SHALL 返回 HTTP 429

#### Scenario: 服务端失败
- **WHEN** 系统遇到未分类内部失败
- **THEN** 系统 SHALL 返回 HTTP 500 和 `HTTP_INTERNAL_ERROR`
- **AND** 必需数据库、Redis、MinIO 或其他依赖不可用 SHALL 返回 HTTP 503 或现有稳定存储错误映射

### Requirement: 稳定错误码具有唯一所有权
所有公开错误码 SHALL 具有全局唯一的语义所有者，并与单一状态、消息和安全数据 Schema 绑定。

#### Scenario: 定义新模块错误码
- **WHEN** 模块增加公开错误码
- **THEN** Authentication SHALL 使用 `AUTHN_*`
- **AND** Authorization SHALL 使用 `AUTHZ_*`
- **AND** Identity SHALL 使用 `IDENTITY_*`
- **AND** Navigation、API Metadata、Organization、Dictionary 和 Audit SHALL 分别使用 `NAV_*`、`API_META_*`、`ORG_*`、`DICT_*` 和 `AUDIT_*`
- **AND** 通用 HTTP 协议和恢复错误 SHALL 使用 `HTTP_*`

#### Scenario: 保留现有上传和文件错误码
- **WHEN** 系统迁移 Upload Security 或 Files 错误响应
- **THEN** `REQUEST_INVALID`、`AVATAR_FIELD_NOT_WRITABLE`、`UPLOAD_*`、`FILE_*`、`IMAGE_*`、`STORAGE_*`、`PERSISTENCE_FAILED` 和 `INTERNAL_ERROR` 等既有公开值 SHALL 保持不变
- **AND** 其他所有者 SHALL NOT 重新声明这些值

#### Scenario: 未分类错误
- **WHEN** HTTP Adapter 收到没有公开分类的内部错误
- **THEN** 系统 SHALL 返回模块批准的内部错误定义或 `HTTP_INTERNAL_ERROR`
- **AND** 系统 SHALL NOT 把 `err.Error()`、数据库错误、堆栈或基础设施路径序列化给客户端

### Requirement: 校验错误提供安全字段详情
HTTP 422 字段校验失败 SHALL 通过白名单结构返回可本地化详情。

#### Scenario: 返回字段校验失败
- **WHEN** 一个或多个语法正确的 JSON 字段不满足字段或领域规则
- **THEN** 顶层 `error_code` SHALL 为该模块的稳定校验失败码
- **AND** `data.fields` SHALL 为稳定排序的数组
- **AND** 每项 SHALL 包含 JSON 字段名 `field`、稳定字段级 `error_code` 和安全英文 `message`
- **AND** 同一字段 SHALL 能够包含多个错误项

#### Scenario: 不泄漏拒绝值
- **WHEN** 系统构造字段校验详情
- **THEN** 响应 SHALL NOT 包含被拒绝的值、请求体、密码、Token、验证码或框架内部校验文本
- **AND** 跨字段规则 SHALL 归属最可操作的 JSON 字段

#### Scenario: 无法归属字段的领域错误
- **WHEN** HTTP 422 领域失败不能安全归属单一字段
- **THEN** 系统 SHALL 使用专用顶层错误码
- **AND** `data` SHALL 为 `null`

#### Scenario: 请求解析失败
- **WHEN** 请求在 JSON、path 或 query 解析阶段以 HTTP 400 失败
- **THEN** 系统 SHALL NOT 伪造 `data.fields`

### Requirement: 只有批准的错误携带安全数据
错误响应的 `data` SHALL 默认为 `null`，只有规格明确声明的数据 DTO 可以返回。

#### Scenario: 登录凭据错误返回剩余次数
- **WHEN** 用户名或密码错误且账号尚未锁定
- **THEN** 系统 MAY 在 `data.remaining_attempts` 返回剩余尝试次数
- **AND** 该详情 SHALL NOT 暴露用户名是否存在

#### Scenario: 登录锁定返回等待时间
- **WHEN** 登录受到渐进式锁定
- **THEN** 系统 SHALL 在 `data.retry_after_seconds` 返回剩余秒数
- **AND** 系统 SHALL 返回相同语义的标准 `Retry-After` Header

#### Scenario: 内部错误不携带详情
- **WHEN** 系统返回 HTTP 500 或 503
- **THEN** `data` SHALL 为 `null`
- **AND** 内部 cause SHALL 只保留在服务端错误链和受控日志中

### Requirement: 原生成功协议保持不包装
成功的 HTML、OpenAPI 文档和二进制响应 SHALL 保持其原生协议。

#### Scenario: 返回文档和二进制成功响应
- **WHEN** `/docs` 成功返回 Swagger HTML
- **OR** `/docs/openapi.json` 成功返回 OpenAPI 文档
- **OR** 文件下载成功返回字节流
- **OR** 头像读取成功返回 JPEG 或 PNG
- **THEN** 系统 SHALL NOT 使用四字段 JSON 信封包装该内容

#### Scenario: 返回 CORS 预检成功
- **WHEN** CORS OPTIONS 预检成功
- **THEN** 系统 SHALL 保持无业务 JSON 信封的预检响应

#### Scenario: 原生端点提交前失败
- **WHEN** 文件下载或 OpenAPI 生成在成功响应提交前失败
- **THEN** 系统 SHALL 返回四字段 JSON 错误信封

#### Scenario: 流式响应提交后失败
- **WHEN** 文件字节或其他原生响应已经提交后发生错误
- **THEN** 系统 SHALL 记录服务端错误并终止响应
- **AND** 系统 SHALL NOT 向已经提交的内容追加 JSON 信封

### Requirement: 前后端分阶段实施和统一发布
后端和前端 SHALL 共享同一响应契约，并 SHALL 在一次正式发布切换中移除旧响应模式；实施和验收可以分阶段进行。

#### Scenario: 没有 web 时实施后端
- **WHEN** 根目录 `web/` 尚不存在
- **THEN** 系统 SHALL 允许实施 Backend Part
- **AND** 系统 SHALL 允许执行 Backend Acceptance
- **AND** 系统 SHALL 阻塞 Frontend Part、Frontend Acceptance 和 Integration Acceptance

#### Scenario: 后端验收通过但前端未完成
- **WHEN** Backend Part 已通过 Backend Acceptance
- **AND** Frontend Part 或 Integration Acceptance 尚未通过
- **THEN** 系统 SHALL 将状态记录为 `backend-accepted / release-blocked`
- **AND** 系统 SHALL NOT 将 Change 标记为完成或归档
- **AND** 系统 SHALL NOT 将新响应契约作为独立生产发布物部署

#### Scenario: 前端实施前置条件
- **WHEN** 开始 Frontend Part
- **THEN** 根目录 `web/` SHALL 已存在
- **AND** 前端框架、包管理器、集中 API Client、错误本地化接缝和原生响应入口 SHALL 已确认

#### Scenario: 前端消费成功响应
- **WHEN** 前端 API Client 收到成功信封
- **THEN** Client SHALL 校验 HTTP 状态与 `code` 一致并返回 `data`

#### Scenario: 前端消费错误响应
- **WHEN** 前端 API Client 收到错误信封
- **THEN** Client SHALL 暴露顶层和字段级 `error_code` 及批准的安全详情
- **AND** 前端 SHALL 按 `error_code` 本地化
- **AND** 英文 `msg` SHALL 只作为未知错误码的 fallback

#### Scenario: 客户端访问原生端点
- **WHEN** 前端访问下载、头像或原始 OpenAPI 端点
- **THEN** Client SHALL 通过显式原生响应路径处理
- **AND** Client SHALL NOT 把成功字节流或 OpenAPI 文档当作四字段信封解析

#### Scenario: Backend Acceptance
- **WHEN** 后端完成统一响应、错误分类、Route Catalog、OpenAPI、日志恢复和后端契约测试
- **THEN** 系统 SHALL 验证后端运行时响应、OpenAPI、日志和安全边界
- **AND** Backend Acceptance SHALL NOT 代替前端或联合验收

#### Scenario: Frontend Acceptance
- **WHEN** 前端完成 API Client、错误本地化、字段错误展示和原生响应处理
- **THEN** 系统 SHALL 验证前端单元测试、类型检查和构建
- **AND** Frontend Acceptance SHALL NOT 代替真实后端联合验收

#### Scenario: Integration Acceptance
- **WHEN** Backend Acceptance 和 Frontend Acceptance 均已通过
- **AND** 真实后端与真实 `web/` 已启动
- **THEN** 系统 SHALL 使用浏览器验证登录、锁定、权限、字段校验、空集合、文件、头像、OpenAPI 和未知错误码 fallback
- **AND** 只有 Integration Acceptance 通过后，Change 才具备完成和统一发布条件

#### Scenario: 完成迁移
- **WHEN** Backend Acceptance、Frontend Acceptance 和 Integration Acceptance 全部通过
- **THEN** 系统 SHALL 删除旧响应 DTO、旧辅助函数和直接序列化内部错误的路径
- **AND** 系统 SHALL NOT 保留双信封、版本 Header、旧字段别名或兼容端点
