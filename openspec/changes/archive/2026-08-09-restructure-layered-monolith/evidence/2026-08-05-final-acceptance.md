# 2026-08-05 最终验收与规格覆盖矩阵

## 结论

`restructure-layered-monolith` 的 Proposal、Design、6 份 Delta Spec 与 12 份主规格均已有对应实现或可执行测试。批准的可观察增强仅为菜单、API、Permission Code、角色授权和授权版本写入的事务原子性；HTTP Method、Path、访问等级、稳定错误处理、审计与文件安全行为由现有回归测试保护。

OpenSpec 校验结果：

- `openspec validate "restructure-layered-monolith" --type change --strict --no-interactive`：通过。
- `openspec validate --specs --strict --no-interactive`：12 个主规格全部通过。

## 主规格覆盖矩阵

| 主规格 | 主要可执行验收 |
| --- | --- |
| `access-version-storage` | `initialize/access_version_*_test.go`；`internal/authorization/adapters/gorm/access_version_test.go`；真实 MySQL `TestMySQLAppMigrationUsesOnlyCurrentAuthorizationSchema` 和并发版本测试 |
| `api-management` | `internal/apimetadata/application/*_test.go`；`internal/app/http_metadata_test.go`；`internal/app/default_permission_policy_test.go` |
| `auth` | `internal/identity/application/service_test.go`；`internal/identity/adapters/jwt/service_test.go`；`internal/app/identity_http_test.go`；真实 Redis 身份状态测试 |
| `dict-management` | `internal/dictionary/http_test.go`；`internal/app/dictionary_http_test.go` |
| `file-management` | `internal/files/application/service_test.go`；`internal/files/adapters/gorm/repository_test.go`；`internal/app/files_audit_test.go`；真实 MinIO 对象生命周期测试 |
| `logging` | `internal/platform/logging/*_test.go`；`internal/audit/adapters/http/routes_test.go`；`internal/app/audit_async_test.go` |
| `menu-management` | `internal/navigation/service_test.go`；`internal/navigation/repository_test.go`；`internal/app/organization_membership_http_test.go` |
| `modular-layered-architecture` | `testsupport/architecture_boundary_test.go`；`internal/app/build_test.go`；`internal/app/migrate_test.go`；`internal/app/seed_test.go` |
| `organization-management` | `internal/organization/hierarchy_test.go`；`internal/organization/scope_repository_test.go`；`internal/app/organization_http_test.go` |
| `rbac` | `internal/authorization/application/service_test.go`；`internal/app/authorization_http_test.go`；`internal/authorization/domain/policy_test.go` |
| `route-catalog` | `internal/routecatalog/catalog_test.go`；`internal/app/http_test.go`；`internal/app/http_access_policy_test.go`；`internal/app/http_metadata_test.go`；`internal/apidoc/service_test.go` |
| `user-management` | `internal/identity/application/user_service_test.go`；`internal/identity/application/context_service_test.go`；`internal/app/identity_http_test.go` |

## Delta Spec 重点覆盖

| 验收点 | 可执行证据 |
| --- | --- |
| API、权限同步和启动 Seed 统一消费 Route Catalog，不扫描 Gin Engine | `TestApplicationSynchronizersShareRouteCatalogSet`、`TestAuthorizationPermissionSyncConsumesStaticCatalog`、`TestSeedCatalogUsesValidatedSnapshotAndRemainsIdempotent`、`TestArchitectureProductionDoesNotDiscoverGinRoutes` |
| 首次同步只允许已登录 `admin`，匿名不能绕过静态访问等级 | `TestApplicationStaticAccessPrecedesMetadataBootstrap`、`TestRegisterHTTPEnforcesStaticAccessBeforeDynamicPermissionPolicy` |
| `POST /api/refresh` 元数据纠正且不改变静态 Public 路由 | `internal/apimetadata/application/sync_test.go`、`internal/app/http_metadata_test.go` |
| API Metadata Method/Path 偏离真实路由后安全拒绝并可重同步 | `TestApplicationResynchronizesRouteAfterMetadataPathDiverges` |
| 菜单/API/Permission/角色授权/授权版本同事务提交并在失败时回滚 | `TestServiceAssignsAPIsWithOnePermissionAndInvalidatesAffectedUsers`、`TestServiceRollsBackLinkedWritesWhenAuthorizationVersionFails`、`TestServiceChangesLinkedAPIPermissionAndMergesRoleGrants` |
| MySQL 外层事务死锁有界重试、失败尝试回滚、并发授权版本串行递增 | `TestMySQLTransactionRunnerRetriesDeadlockAndRollsBackAttempt`、`TestMySQLConcurrentAccessVersionTransactionsSerializeIncrements` |
| `need_audit` 不改变访问等级，审计异步且不读取 multipart 文件内容 | `TestAppRegistersFilesAuditAndPersistsAPIRequestsAsynchronously`、`TestAuditRecorderQueueReturnsBeforePersistenceCompletes`、`TestMiddlewareNeverReadsMultipartFileContent` |
| 文件安全 MIME/扩展名/内容一致性、重编码、摘要、稳定错误码 | `internal/uploadsecurity/*_test.go`、`internal/files/application/service_test.go`、真实 MinIO `TestMinIOIntegrationObjectLifecycle` |
| 固定模块、无旧技术目录、无全局状态、无跨模块 Adapter、App 唯一入口 | `testsupport/architecture_boundary_test.go` 全部 `TestArchitecture*` |

## 已执行门禁

- 快速套件：`go test ./initialize ./internal/... -count=1`，30 个包通过，10 个包无测试。
- 路由策略：`go test ./internal/routecatalog ./internal/app ./internal/apimetadata/... ./internal/apidoc -count=1`，6 个包通过，1 个包无测试。
- 强事务与授权失效：`go test ./internal/navigation ./internal/apimetadata/application ./internal/authorization/... ./internal/platform/database ./internal/app -count=1`，7 个包通过，1 个包无测试。
- 真实 MySQL：隔离临时数据库执行 `run-mysql-gate.ps1`，包含 App AutoMigrate、退役结构检查、并发授权版本和死锁重试，全部包通过；临时数据库在门禁后删除。
- 真实 Redis：临时容器执行 `run-redis-gate.ps1`，验证码一次性消费、登录失败/锁定状态和黑名单生命周期通过。
- 真实 MinIO：临时容器执行 `run-minio-gate.ps1`，Put/Open/Stat/List/Delete 生命周期通过；测试 bucket 自动清理。
- 全仓回归：`go test ./... -count=1`，31 个包通过，11 个包无测试。

## Race Detector

`go test -race ./... -count=1` 通过，所有有测试包均未报告数据竞争；Windows 使用 MSYS2 UCRT64 GCC 并通过 `CGO_ENABLED=1`、`CC=gcc` 运行。
