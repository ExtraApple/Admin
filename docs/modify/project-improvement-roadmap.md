# 项目优化路线图

## 修改时间

2026-07-24

## 文档定位

本文只维护项目优化的长期阶段划分、前置依赖、完成标准和精确执行顺序。

项目能力现状见[mature-admin-system-comparison-overview](mature-admin-system-comparison-overview.md)，当前目录和模块重构的详细设计见[layered-monolith-restructuring](layered-monolith-restructuring.md)，文件安全历史和后续边界见[file-upload-security-recommendations](file-upload-security-recommendations.md)。

## 排序原则

优先级不只按功能价值排序，还要考虑安全风险、前置依赖和返工成本：

```text
先封堵安全风险
  → 再建立回归测试保护
  → 再补可观测能力
  → 再实施模块化分层重构
  → 再扩展业务模块
  → 最后固化数据库迁移和多数据库能力
```

## 阶段 0：已经完成，不再重复开发

1. 自动化 API 文档基础能力。
2. Go Seed 初始化。
3. SQLite 内存数据库 Seed 测试。

以上能力进入维护状态。新增路由、权限、菜单、字典或默认数据时，应同步补充元数据和回归测试，不再单独立项重做。

## 阶段 1：封堵文件上传安全风险（已完成）

### 1. 文件上传安全 V1

文件上传安全 V1 已完成代码实现、自动化验证、OpenSpec 主规格同步和 change 归档，现已进入维护状态。

已完成目标：

- 同时加固普通文件和用户头像上传。
- 在写入 MinIO 前完成请求体、大小、类型、文件名和图片内容校验。
- 管理员普通文件只允许 PDF、UTF-8 TXT、UTF-8 CSV，并始终使用附件下载语义。
- JPEG、PNG、WebP 只进入用户头像标准化链路；管理员预览兼容路由统一返回 HTTP 409。
- 对实际写入对象存储的管理员普通文件和标准化头像建立服务端 SHA-256 完整性基线。
- 处理历史未验证文件和历史头像的兼容边界。
- 补齐安全审计和自动化测试。

完整约束、V1/V2 边界及已确认决策见[file-upload-security-recommendations](file-upload-security-recommendations.md)。

完成结果：

- 普通文件和头像上传均使用统一的底层安全校验能力。
- 不支持的文件在写入 MinIO 前被拒绝。
- 摘要不用于 object key、自动去重、病毒检测或唯一约束。
- 历史文件和头像具有明确、可测试的过渡行为。
- 文件安全测试可以脱离真实 MinIO 稳定运行。
- OpenSpec、模块文档和自动化测试与最终行为一致。

## 阶段 2：建立核心回归测试保护

当前不立即开发首页或消息模块，先按风险从高到低补测试。

### 2. 测试基础设施

- 统一测试数据库初始化和清理。
- 提供测试路由、登录、Token、用户、角色和权限夹具。
- 避免每个模块重复搭建测试环境。

### 3. 登录、JWT 和 Token 生命周期测试

- 登录成功和失败。
- 禁用用户拒绝登录。
- Token 缺失、过期、伪造和版本失效。
- 修改密码、权限变更和强制下线后旧 Token 失效。

### 4. RBAC、菜单/API 联动和数据范围测试

- API 动态权限允许和拒绝。
- 角色绑定、解绑权限。
- 菜单绑定 API、权限码同步和删除清理。
- 本人、本组织、本组织及子组织、全部数据范围。

### 5. 审计日志和文件生命周期测试

- 关键管理操作生成审计日志。
- multipart 内容和敏感字段不进入审计正文。
- 文件上传失败回滚、删除双删和轮转失败回滚。

完成标准：

- 登录、权限、菜单、组织和文件关键链路均有自动化测试。
- 后续权限重构可以依靠测试判断行为是否发生回归。
- `go test ./... -count=1` 可以稳定重复通过。

## 阶段 3：补齐基础可观测能力

### 6. `request_id` 请求追踪

- 接收合法的上游 request ID，缺失时自动生成。
- 响应头、Gin Context、Zap 日志和审计日志使用同一个 request ID。
- 错误响应可返回 request ID，方便定位日志。

### 7. 健康检查

- 提供不依赖外部服务的存活检查。
- 提供 MySQL、Redis、MinIO 就绪检查。
- 区分进程存活和依赖可用，避免单一接口语义混乱。
- 为每个依赖设置短超时，禁止健康检查长时间阻塞。

### 8. 基础运行指标

- 请求总数、状态码和耗时。
- 慢请求记录。
- 异常请求统计。
- MySQL、Redis、MinIO 检查结果。

完成标准：

- 任意错误请求可以通过 request ID 关联访问日志、运行日志和审计日志。
- 运维系统可以分别判断服务是否存活、是否可以正常接收流量。
- 首页仪表盘后续可以复用稳定的统计数据来源。

## 阶段 4：在测试保护下实施模块化分层重构

详细目录、分层职责、模块边界和迁移阶段见[layered-monolith-restructuring](layered-monolith-restructuring.md)。

### 9. 建立应用装配和基础设施层（已完成）

- 建立 `internal/app` 作为唯一组合根；入口为仓库根目录的 `main.go`
  （不是 `cmd/admin`，仓库中没有 `cmd/` 目录）。
- 将配置、数据库、Redis、MinIO、RabbitMQ、HTTP 响应信封和日志创建迁移到
  `internal/platform`（现有子目录：`config`、`database`、`cache`、
  `objectstorage`、`rabbitmq`、`httpresponse`、`logging`）。
- 使用 constructor 显式注入依赖：每个模块以 `Dependencies` 结构体接收依赖，
  由 `internal/app` 逐字段装配，并由架构测试
  `TestArchitectureDeclaredDependenciesAreWired` 强制校验。
- 已停止 `global` 访问：仓库中不存在 `global` 包，
  `TestArchitectureProductionDoesNotUseLegacyGlobal` 持续守卫。

### 10. 按业务模块迁移（已完成）

- 先迁移字典模块验证模板：`internal/dictionary` 为扁平模块。
- 再迁移文件、授权、身份、组织、审计和 OpenAPI：对应
  `internal/files`、`internal/authorization`、`internal/identity`、
  `internal/organization`、`internal/audit`、`internal/apidoc` 均已就位。
- 复杂模块采用 Domain、Application、Adapter 分层。
- 简单模块保持扁平，避免形式化分层膨胀。
- 权限检查、数据范围和 Token 失效在 Authorization module 内统一收敛
  （见 ADR 0004、ADR 0005）。

### 11. 自动化 API 文档增强

- 分离纯 OpenAPI 生成核心和数据库 adapter。
- 补充响应 DTO Schema。
- 统一错误响应和错误码枚举。
- 补充字段示例和更细的接口描述。

完成标准：

- App 成为唯一 composition root。
- 业务 module 不再直接访问全局基础设施。
- 阅读单一业务能力主要停留在一个 module 目录。
- 权限重构前后的核心测试结果一致。
- Apifox 可以从 OpenAPI 文档获得主要请求和响应结构。

## 阶段 5：扩展后台业务能力

底盘稳定后，再按依赖关系增加业务模块。

### 12. 首页仪表盘

优先复用已有用户、登录、审计、文件和运行指标，不单独复制统计口径。

### 13. 数据导入导出

先实现需求明确、风险较低的导出，再实现带校验、错误报告和幂等策略的导入。

### 14. 岗位管理

在组织数据范围测试稳定后实施，明确岗位与用户、组织、角色之间的边界。

### 15. 内部消息 / 通知公告

在用户、组织、岗位模型稳定后实施，避免收件人范围和数据权限重复返工。

### 16. 配置管理增强

优先支持业务开关、脱敏展示和变更日志；数据库密码、JWT 密钥等基础设施密钥继续由环境变量或密钥管理系统维护。

## 阶段 6：数据库和开发效率工程化

### 17. 数据库 migrations 和 migration 工具化

只有满足以下条件后才开始：

- 主要基础模块已经完成。
- 核心表结构进入稳定期。
- MySQL 关键链路测试已经具备。
- 已明确迁移失败、回滚和 Seed 版本策略。

### 18. 内部代码生成器

在 Domain、Application、Adapter、路由、权限码、文档和测试的模块模板稳定后再实现，避免把尚未稳定的模式固化进生成器。

## 阶段 7：长期多数据库扩展

### 19. PostgreSQL 数据库适配

先抽象数据库配置和清理 MySQL 方言绑定，再增加 PostgreSQL 驱动和集成测试。

### 20. `psql` 迁移执行和集成验证

仅在 PostgreSQL 已成为正式支持目标后实施，用于迁移执行、表结构检查和集成验证。

## 精确执行顺序

如果没有新的高优先级线上缺陷或明确业务截止时间，按以下顺序逐项推进：

```text
文件上传安全 V1（已完成并归档）
→ 测试基础设施（当前）
→ 登录/JWT/Token 测试
→ RBAC/菜单/API/数据范围测试
→ 审计日志和文件生命周期测试
→ request_id
→ 存活与就绪检查
→ 基础 metrics、慢请求和异常统计
→ 建立 App 和 Platform
→ 迁移 Dictionary 验证模块模板
→ 迁移 Files 和 Upload Security
→ 迁移 Authorization 和 Route Catalog
→ 迁移 Identity、Organization、Audit 和 API Doc
→ 删除旧横向目录和 global service locator
→ OpenAPI 响应与错误模型增强
→ 首页仪表盘
→ 数据导入导出
→ 岗位管理
→ 内部消息/通知公告
→ 配置管理增强
→ migrations
→ 代码生成器
→ PostgreSQL
→ psql 验证
```

每一项开始前先创建对应 OpenSpec change，并以其规格、设计和任务作为实现依据。前一阶段的“完成标准”未满足时，原则上不进入下一阶段。
