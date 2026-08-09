# 业务模块代码导航

代码按业务模块组织；入口、基础设施和路由装配集中在 `internal/app`，不要从已退役的技术目录寻找业务实现。

![模块组合关系](assets/architecture/module-composition.svg)

## 启动与装配

1. `main.go` 只调用 `app.BuildFromPath`、`Application.Run` 和 `Application.Close`。
2. `internal/app/build.go` 读取配置并通过 `internal/platform` 创建 MySQL、Redis、MinIO 和 Zap。
3. `internal/app/app.go` 构造各模块、收集 Route Descriptor、建立 Route Catalog，并启动迁移、Seed、HTTP 与后台任务。
4. `internal/app/http.go` 是唯一业务 Gin 路由注册点；`internal/app/migrate.go` 和 `internal/app/seed.go` 分别是唯一迁移与 Seed 编排入口。

## 模块索引

| 模块 | 所有权 | 建议起点 |
| --- | --- | --- |
| `internal/identity` | 用户、验证码、登录、JWT、Refresh Token、黑名单、用户资料与头像 | `application/service.go`、`adapters/http/routes.go` |
| `internal/authorization` | 角色、权限、数据范围、访问快照与授权版本 | `application/service.go`、`domain/policy.go` |
| `internal/navigation` | 菜单、角色菜单、菜单 API 绑定和可见菜单树 | `service.go`、`routes.go` |
| `internal/apimetadata` | API 元数据、运行时访问策略和 API 同步 | `application/core.go`、`adapters/http/routes.go` |
| `internal/organization` | 组织单位、成员关系、组织树和资源 Scope 查询 | `service.go`、`hierarchy.go` |
| `internal/dictionary` | 字典类型、字典条目及公开读取 | `service.go`、`http.go` |
| `internal/files` | 管理员文件记录、存取、轮转和重新验证 | `application/service.go`、`adapters/http/routes.go` |
| `internal/audit` | 请求审计、脱敏、异步记录和冷热归档 | `service.go`、`adapters/http/routes.go` |
| `internal/uploadsecurity` | 文件内容检测、重编码、摘要和安全策略 | `stage.go`、`content.go` |
| `internal/routecatalog` | 静态路由事实、Descriptor 校验和只读快照 | `catalog.go` |
| `internal/apidoc` | 基于 Route Catalog 与 API Metadata 的 OpenAPI 文档 | `service.go` |
| `internal/platform` | 配置、日志、数据库、缓存、对象存储和事务运行器 | 对应基础设施子目录 |
| `internal/app` | 唯一组合根、Gin 注册、迁移、Seed 和后台任务编排 | `build.go`、`app.go` |

## 模块内依赖

复杂模块使用 `adapters -> application -> domain` 方向；简单模块保持扁平。Application 所需的跨模块能力由调用方在 `contracts.go` 声明最小接口，再由 App 注入实现。业务模块不得导入其他模块的 Adapter、`internal/app` 或全局运行时状态。

Route Descriptor 是业务 HTTP 入口的唯一声明。新增或修改接口时，同时更新 Handler、访问等级、默认权限码、审计分类和完整 OpenAPI operation；App 从 Route Catalog Snapshot 统一执行 Gin 注册、启动 Seed、API/权限同步和文档生成。

## 验证入口

```powershell
go test ./initialize -run TestArchitecture -count=1
go test ./internal/routecatalog ./internal/app -count=1
go test ./... -count=1
```

真实 MySQL、Redis 和 MinIO 门禁位于 `testsupport/testutil/run-*-gate.ps1`；这些门禁验证 SQLite 无法覆盖的锁、并发、DDL、缓存和对象存储语义。
