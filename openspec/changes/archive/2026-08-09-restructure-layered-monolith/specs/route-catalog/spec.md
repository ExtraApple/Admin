## ADDED Requirements

### Requirement: 模块声明 Route Descriptor
每个业务模块 SHALL 声明自身 HTTP 路由的 Route Descriptor，而不是直接向 Gin Engine 注册路由。

#### Scenario: 模块提供路由
- **WHEN** App 构造一个包含 HTTP 入口的业务模块
- **THEN** 该模块 SHALL 返回包含 HTTP Method、Path、Access Level、Handler、API 名称、分组、默认 Permission Code、默认审计分类及 OpenAPI 描述的 Route Descriptor
- **AND** 模块 SHALL NOT 调用 Gin 的 `GET`、`POST`、`PUT`、`PATCH`、`DELETE` 或其他注册方法

#### Scenario: OpenAPI 描述完整
- **WHEN** 模块提供包含 HTTP 入口的 Route Descriptor
- **THEN** Descriptor SHALL 为该路由提供完整的 OpenAPI operation 描述
- **AND** 需要请求体或响应体的接口 SHALL 提供对应 Schema
- **AND** 上传接口 SHALL 提供 `multipart/form-data` 和文件字段描述
- **AND** 缺少必需 OpenAPI 描述时 Route Catalog SHALL 在 Gin 注册前返回校验错误
- **AND** 系统 SHALL NOT 静默生成遗漏该路由或 Schema 的 OpenAPI 文档

### Requirement: Route Catalog 启动前校验路由
Route Catalog SHALL 在 Gin 注册前收集和校验全部 Route Descriptor。

#### Scenario: 路由集合有效
- **WHEN** 所有 Descriptor 的规范化 `method + path` 唯一且字段组合有效
- **THEN** Route Catalog SHALL 生成稳定的只读路由快照
- **AND** App SHALL 能够继续注册路由

#### Scenario: 路由重复
- **WHEN** 两个 Descriptor 使用相同的规范化 HTTP Method 和 Path
- **THEN** Route Catalog SHALL 返回启动错误
- **AND** App SHALL NOT 启动 HTTP 服务

#### Scenario: 路由描述无效
- **WHEN** Descriptor 缺少 Method、Path、Handler 或使用未知 Access Level
- **THEN** Route Catalog SHALL 返回校验错误
- **AND** App SHALL NOT 注册该路由

### Requirement: App 是唯一 Gin 路由注册点
`internal/app` SHALL 是唯一允许修改 Gin Engine 路由集合的位置。

#### Scenario: 注册有效路由集合
- **WHEN** Route Catalog 校验完成
- **THEN** App SHALL 按 Descriptor 的 Access Level 挂载中间件
- **AND** App SHALL 将全部 Descriptor 注册到 Gin Engine

#### Scenario: 校验注册完整性
- **WHEN** HTTP 兼容测试比较 Route Catalog 与 Gin 实际业务路由
- **THEN** 两个路由集合 SHALL 在 Method 和 Path 上一致
- **AND** 系统 SHALL NOT 出现遗漏、重复或绕过 Catalog 注册的业务路由

### Requirement: 静态访问等级确定最低认证要求
Route Descriptor SHALL 使用 Public、Authenticated 或 PermissionControlled 声明路由的最低访问等级。

#### Scenario: 访问 Public 路由
- **WHEN** 客户端访问标记为 Public 的路由
- **THEN** 系统 SHALL 允许匿名请求进入对应 Handler

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
- **THEN** `/api/captcha`、`/api/register`、`/api/login`、`/api/refresh`、公开字典和公开头像 SHALL 声明为 `Public`
- **AND** `/api/user/**` SHALL 声明为 `Authenticated`
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

### Requirement: 数据库元数据不得动态改变代码路由
API Metadata SHALL 只能配置已声明路由的运行时策略，不能创建、替换或提升代码路由的匿名访问能力。

#### Scenario: 数据库存在未声明 API
- **WHEN** `apis` 表存在 Route Catalog 中不存在的 Method 和 Path
- **THEN** 系统 SHALL NOT 因该记录向 Gin 注册新路由

#### Scenario: 数据库策略与静态等级冲突
- **WHEN** API Metadata 试图使 Authenticated 或 PermissionControlled 路由匿名
- **THEN** 系统 SHALL 保持 Descriptor 声明的最低认证要求

### Requirement: 静态路由事实与运行时元数据分离
Route Catalog SHALL 拥有代码静态路由事实，API Metadata SHALL 拥有管理员可配置的展示和运行时策略事实。

#### Scenario: 读取静态路由事实
- **WHEN** 系统需要 Handler、真实 Method/Path 或静态 Access Level
- **THEN** 系统 SHALL 使用 Route Catalog
- **AND** API Metadata SHALL NOT 替换这些代码事实

#### Scenario: 读取运行时策略
- **WHEN** PermissionControlled 路由执行动态策略校验
- **THEN** 系统 SHALL 使用 API Metadata 的 `status`、`need_auth` 和 `permission_code`
- **AND** `need_audit` SHALL 保持本 Change 前的运行时语义

#### Scenario: 同步创建 API 元数据
- **WHEN** Route Catalog 中的路由在 API Metadata 中不存在
- **THEN** 同步流程 SHALL 使用 Catalog 的名称、分组、默认 Permission Code 和默认审计分类创建记录
- **AND** 已存在记录 SHALL 按既有兼容规则保留管理员配置

### Requirement: Route Catalog 提供同步和文档快照
Route Catalog SHALL 向 API Metadata 同步、RBAC 路由权限同步、API Doc 和启动 Seed 提供同一份已校验路由快照。

#### Scenario: API Metadata 同步路由
- **WHEN** 管理员触发 API 路由同步
- **THEN** API Metadata SHALL 消费 Route Catalog 快照
- **AND** 同步流程 SHALL NOT 通过扫描 Gin Engine 发现业务路由

#### Scenario: 启动 Seed 和 RBAC 路由权限同步
- **WHEN** App 启动执行 Seed，或管理员调用 `POST /api/admin/permissions/sync`
- **THEN** 对应用例 SHALL 消费 Route Catalog Snapshot
- **AND** 系统 SHALL NOT 通过扫描 Gin Engine 发现业务路由
- **AND** Route Catalog 收集和 Gin 注册 SHALL NOT 隐式创建 Permission 记录

#### Scenario: 生成 OpenAPI 文档
- **WHEN** API Doc 生成 OpenAPI 文档
- **THEN** API Doc SHALL 合并 Route Catalog 描述和必要的 API Metadata 快照
- **AND** API Doc SHALL NOT 直接扫描 Gin Engine 或查询全局数据库状态
