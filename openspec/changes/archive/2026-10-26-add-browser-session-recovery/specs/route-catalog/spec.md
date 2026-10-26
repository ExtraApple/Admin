## MODIFIED Requirements

### Requirement: 模块声明 Route Descriptor
每个业务模块 SHALL 声明自身 HTTP 路由的 Route Descriptor，而不是直接向 Gin Engine 注册路由；Descriptor SHALL 同时声明 Handler 自身可能返回的公开成功和错误契约，以及该路由实际使用的 Cookie 和必需请求／响应 Header 契约。

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
- **AND** Cookie 会话控制接口 SHALL 声明 Cookie 安全方案、必需浏览器防护／尝试 Header、Set-Cookie 与 no-store 响应事实
- **AND** 缺少必需 OpenAPI 描述或公开错误定义时 Route Catalog SHALL 在 Gin 注册前返回校验错误
- **AND** 系统 SHALL NOT 静默生成遗漏该路由、Schema、Cookie／Header 约束或错误码的 OpenAPI 文档

### Requirement: 静态访问等级确定最低认证要求
Route Descriptor SHALL 仅使用 Public、Authenticated 或 PermissionControlled 声明路由的最低访问等级。Public 只表示不经过统一 Bearer 身份中间件，不代表绕过 Handler 明确持有的 Cookie 会话或请求来源检查；系统 SHALL NOT 新增第四种访问等级。

#### Scenario: 访问 Public 路由
- **WHEN** 客户端访问标记为 Public 的路由
- **THEN** 系统 SHALL 允许匿名请求进入对应 Handler
- **AND** Cookie 会话控制 Handler SHALL 继续执行自身的凭据与请求防护契约

#### Scenario: 访问 Authenticated 路由
- **WHEN** 客户端访问标记为 Authenticated 的路由
- **THEN** 系统 SHALL 要求有效登录身份
- **AND** 系统 SHALL NOT 要求 Permission Code

#### Scenario: 访问 PermissionControlled 路由
- **WHEN** 客户端访问标记为 PermissionControlled 的路由
- **THEN** 系统 SHALL 始终要求有效登录身份
- **AND** 系统 SHALL 再根据 API Metadata 校验启用状态和 Permission Code 策略

#### Scenario: API 元数据关闭权限码检查
- **WHEN** PermissionControlled 路由对应 API Metadata 的 `need_auth = 0`
- **THEN** 系统 SHALL 跳过 Permission Code 检查
- **AND** 系统 SHALL 继续要求有效登录身份

#### Scenario: 业务路由声明最低访问等级
- **WHEN** App 收集现有业务 Route Descriptor
- **THEN** `/api/captcha`、`/api/register`、`/api/login`、`/api/refresh`、Cookie 控制入口 `POST /api/user/logout`、公开字典和公开头像 SHALL 声明为 `Public`
- **AND** 除上述精确退出入口外，`/api/user/**` SHALL 声明为 `Authenticated`
- **AND** `/api/admin/**` SHALL 声明为 `PermissionControlled`
- **AND** `/ping`、`/docs` 和 `/docs/openapi.json` SHALL 作为 App 技术路由注册，不由 API Metadata 创建

#### Scenario: API 元数据首次同步例外
- **WHEN** 已登录的 `admin` 角色用户调用 `POST /api/admin/apis/sync` 或 `POST /api/admin/apis/sync-permissions`
- **AND** 对应 API Metadata 尚未完整存在
- **THEN** 系统 SHALL 允许请求用于初始化 API Metadata 或权限
- **AND** 系统 SHALL 仍要求有效登录
- **AND** 该例外 SHALL NOT 适用于匿名请求或其他管理员路由

#### Scenario: 审计字段保持当前语义
- **WHEN** 客户端访问 `/api/*` 路由
- **THEN** Audit SHALL 按 `logging` 主规格继续异步记录请求
- **AND** `need_audit` SHALL NOT 改变静态访问等级或把受保护路由降级为匿名
- **AND** 本 Change SHALL NOT 新增按 `need_audit` 抑制审计的行为

## ADDED Requirements

### Requirement: Cookie 会话控制契约的文档快照
Gin、OpenAPI、同步和种子数据 SHALL 继续消费同一已校验 Route Catalog Snapshot。Cookie／Header 契约 SHALL 由模块 Descriptor 声明并由通用投影消费，不得在 APIDoc 维护一份按具体认证 URL 判断的第二来源。

#### Scenario: 生成会话控制 OpenAPI
- **WHEN** APIDoc 从有效 Snapshot 生成登录、续期和退出 operation
- **THEN** 文档 SHALL 表达实际 Cookie、必需 Header、请求体、成功 JSON、Set-Cookie、安全属性与四字段错误信封
- **AND** 文档 SHALL NOT 再展示 JSON Refresh Token 通道或给 Cookie 退出虚构必需 Bearer Access
- **AND** 路由公开错误 SHALL 包含实际可能出现的 CSRF 拒绝、续期被取代与安全基础设施失败，不增加仅由未挂载的 Bearer／权限中间件产生的错误

#### Scenario: 非会话路由维持身份边界
- **WHEN** 客户端访问普通当前用户或管理员路由
- **THEN** Cookie 控制元数据 SHALL NOT 降低其 Bearer 身份或 Permission Code 要求
- **AND** 文档与实际 Gin 注册 SHALL 使用相同最低访问等级

#### Scenario: 新协议描述不完整
- **WHEN** 声明的 Cookie、必需请求 Header、响应 Header 或公开错误引用不完整／不合法
- **THEN** Catalog SHALL 在 Gin 注册和文档发布前返回校验失败
- **AND** 系统 SHALL NOT 静默忽略该元数据或猜测补全路径专用描述

#### Scenario: 既有 API 配置保持
- **WHEN** 同步含 Cookie 会话控制契约的新 Snapshot
- **THEN** 系统 SHALL 保留管理员已配置的 Metadata 字段与现有投影边界
- **AND** 系统 SHALL NOT 为本 Change 回填历史 Public 路由的 NeedAuth、增加 Permission Code 或重写其他路由权限
