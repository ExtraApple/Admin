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
| [`audit-logging.md`](runbooks/audit-logging.md) | 审计日志和冷热归档 |
| [`api-documentation.md`](runbooks/api-documentation.md) | Swagger UI 和 OpenAPI 维护 |
| [`access-version-storage-switch.md`](runbooks/access-version-storage-switch.md) | 授权版本迁移历史 |

## 维护规则

- Route Descriptor 和 OpenAPI 生成链路维护接口契约。
- 当前行为先更新 `openspec/specs/`；未来变化先创建 `openspec/changes/`。
- 模块摘要只记录 Swagger 不适合表达的边界、背景和验证重点。
- 可执行部署、切换和排障步骤进入 `runbooks/`；长期架构取舍进入 `adr/`。
- 图表源文件与导出文件放在 `assets/architecture/`，引用文档使用相对路径。
- 不保留临时调试日志、过期目录结构、伪代码实现步骤或未批准的未来方案。
