# docs 使用说明

`docs/` 保存实现导航、业务边界摘要、运维手册、架构决策和历史背景。接口契约不在 Markdown 中重复维护。

## 事实来源

| 内容 | 入口 |
|---|---|
| 领域术语 | [`CONTEXT.md`](../CONTEXT.md) |
| 当前系统行为 | [`openspec/specs/`](../openspec/specs/) |
| 计划中的行为变化 | [`openspec/changes/`](../openspec/changes/) |
| API 路径、请求和响应 Schema | 本地 `http://localhost:8080/docs`、`http://localhost:8080/docs/openapi.json` |
| 长期架构决策 | [`adr/`](adr/) |
| 代码导航 | [`module-navigation.md`](module-navigation.md) |

发生冲突时，当前行为以 OpenSpec 为准；运行代码用于核验实现一致性。

工作台及响应契约当前处于跨 change 实施／联合验收流程：[`build-admin-workbench`](../openspec/changes/build-admin-workbench/) 与 [`standardize-api-response-contract`](../openspec/changes/standardize-api-response-contract/) 追踪新增行为和发布门禁。此处导航不将 delta 自动同步为长期规格，也不表示已归档或验收完成。

## 目录职责

| 目录 | 内容 |
|---|---|
| `modules/` | 业务模块边界摘要和非 API 规则 |
| `runbooks/` | 配置、部署、切换、排障和维护步骤 |
| `assets/architecture/` | Mermaid 源文件及 SVG、PNG 导出文件 |
| `adr/` | 已接受的长期架构决策 |
| `modify/` | 架构演进、路线图和历史背景 |
| `agents/` | 文档维护规则 |

## 业务模块摘要

| 文档 | 领域 |
|---|---|
| [`user-management.md`](modules/user-management.md) | 用户、会话和头像边界 |
| [`role-management.md`](modules/role-management.md) | 角色和角色关联 |
| [`permission-management.md`](modules/permission-management.md) | 权限码和权限同步 |
| [`data-scope.md`](modules/data-scope.md) | 数据范围和资源过滤 |
| [`menu-management.md`](modules/menu-management.md) | 菜单树和菜单维护 |
| [`menu-api-integration.md`](modules/menu-api-integration.md) | 菜单与 API 权限码联动 |
| [`api-metadata-management.md`](modules/api-metadata-management.md) | API 元数据和运行时策略 |
| [`organization-management.md`](modules/organization-management.md) | 组织单位和组织成员 |
| [`dictionary-management.md`](modules/dictionary-management.md) | 字典类型和字典条目 |

## 运维手册

| 文档 | 主题 |
|---|---|
| [`configuration.md`](runbooks/configuration.md) | 配置来源、Secret 和部署 |
| [`login-security.md`](runbooks/login-security.md) | 验证码、登录锁定和 Token |
| [`database-migration-and-seeding.md`](runbooks/database-migration-and-seeding.md) | AutoMigrate、Seed 和启动初始化 |
| [`file-management.md`](runbooks/file-management.md) | 文件安全、状态和存储 |
| [`runtime-logging.md`](runbooks/runtime-logging.md) | Zap 运行日志 |
| [`messaging.md`](runbooks/messaging.md) | RabbitMQ、Outbox 和 DLQ Recorder 处置 |
| [`audit-logging.md`](runbooks/audit-logging.md) | 审计日志和冷热归档 |
| [`api-documentation.md`](runbooks/api-documentation.md) | Swagger UI 和 OpenAPI 维护 |
| [`access-version-storage-switch.md`](runbooks/access-version-storage-switch.md) | 授权版本迁移历史 |

工作台 npm 命令、开发代理、同源生产部署、内存 Token 与 API Client 原生入口见 [根 README](../README.md#管理工作台)。隔离 MySQL／Redis／MinIO 与真实数据准备边界见 [配置手册](runbooks/configuration.md#工作台隔离联调环境)，用户定向编辑及 409 处理见 [用户管理](modules/user-management.md#工作台集成实施中的-change)；架构取舍见 [ADR 0011](adr/0011-react-frontend-and-customizable-ui-stack.md) 和 [ADR 0012](adr/0012-user-oriented-membership-management.md)。

## 维护规则

- Route Descriptor 和 OpenAPI 生成链路维护接口契约。
- 当前行为先更新 `openspec/specs/`；未来变化先创建 `openspec/changes/`。
- 模块摘要只记录 Swagger 不适合表达的边界、背景和验证重点。
- 可执行部署、切换和排障步骤进入 `runbooks/`；长期架构取舍进入 `adr/`。
- 图表源文件与导出文件放在 `assets/architecture/`，引用文档使用相对路径。
- 不保留临时调试日志、过期目录结构、伪代码实现步骤或未批准的未来方案。
