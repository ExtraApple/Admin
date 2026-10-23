## Context

A3 真实隔离运行发现，`internal/app/seed.go` 的 `syncCatalogAPIs` 对新 Descriptor 计算了正确的 `needAuth=0`，但 `internal/apimetadata/adapters/gorm.API.NeedAuth` 带有 GORM `default:1`，`Create(&api)` 会把零值 Public 配置持久化为 `1`。当前 App 的 Gin 注册仍以 Route Catalog 的静态 Access Level 为准，所以未观察到 Public 路由被认证中间件阻断；偏差位于 API Metadata 投影。

当前 API Management 规则要求 Public 路由不需要 Permission Code 检查，已有 Metadata 的展示和运行时策略字段按兼容规则保留。修复必须只处理新建 Catalog Metadata 的零值持久化，不把历史数据迁移或手工 CRUD 策略裁决混入本 change。

## Goals / Non-Goals

**Goals:**

- 保证新建 Public Route Descriptor 对应的 `apis.need_auth` 最终持久化为 `0`。
- 保证新建 Authenticated 和 PermissionControlled Metadata 的 `need_auth=1`、Permission Code 和其他默认字段保持现有语义。
- 将单条 API Metadata 的创建与 Public 零值显式修正放在同一数据库事务内，失败不留下半成品。
- 通过 SQLite focused 回归和 MySQL 集成门禁覆盖 ORM 默认值与真实数据库行为。
- 明确已有 Metadata、软删除恢复记录和 API Metadata CRUD 不在本 change 内自动修复或重新裁决。

**Non-Goals:**

- 不修改 `API.NeedAuth` 的全局模型默认值，不引入指针字段或新的持久化模型。
- 不自动修复已经存在的 `Public + need_auth=1` 数据；另建数据修复 change。
- 不禁止管理员通过 API Metadata CRUD 修改既有策略字段。
- 不改变 Gin 路由注册、Access Level、Permission Code、审计分类、Redis key、MinIO bucket、数据库表或第三方依赖。
- 不扩大为全量 API Management 事务审计或历史兼容策略重设计。

## Decisions

### 1. 只修复 Catalog 新记录路径

`syncCatalogAPIs` 继续以 Descriptor Access 为静态事实。新记录计算 `needAuth` 后：

- `needAuth == 1` 沿现有 `Create` 语义写入。
- `needAuth == 0` 创建后在同一记录级事务内使用显式列更新，强制数据库最终值为 `0`。

不修改 existing-record 分支，除现有 `/api/refresh` 兼容规则外继续保留管理员字段；软删除记录恢复也保持现有字段。

备选方案：修改 GORM 模型默认值或改用指针字段。放弃原因：影响所有 API Metadata 创建调用方，范围超过本 bug。

### 2. 记录级事务，不扩大同步事务

对每个新 Descriptor 使用 `db.WithContext(ctx).Transaction` 包裹创建与 Public 零值更新。任一步失败返回现有同步错误并回滚该条记录；不把整个 Catalog 同步包成一个大事务，避免扩大锁范围和回滚范围。

### 3. 用 focused seed 测试固定真实边界

SQLite 测试直接调用 `syncCatalogAPIs`，使用最小 Public、Authenticated、PermissionControlled Descriptor，并查询数据库最终值。测试同时验证既有 Metadata 的管理员字段不被新逻辑触碰。

### 4. MySQL 集成测试沿用现有门禁

新增 `//go:build mysql_integration` 测试，使用 `ADMIN_TEST_MYSQL_DSN` 和现有 `run-mysql-gate.ps1` 约定，直接验证 MySQL 中 Public `need_auth=0` 的最终值。测试使用唯一临时路由并清理记录，不要求普通 `go test ./...` 连接外部 MySQL。

### 5. 主规格修改只澄清新建结果

`api-management` 的 API 路由同步 Requirement 增加 Public 新记录的持久化结果：新建 Metadata 必须保存 `need_auth=0` 且不生成 Permission Code；已有 Metadata 继续按兼容规则保留。归档时同步该 delta；历史数据修复另立 change。

## Risks / Trade-offs

- **两条 SQL 写入**：Public 新记录需要 Create 加显式列更新；记录级事务保证不会留下中间结果，代价是多一次 UPDATE。
- **历史数据仍可能偏差**：本 change 不改既有记录，报告和后续数据修复 change 必须明确该边界。
- **手工 Metadata 策略仍可配置**：这是当前 API Management 兼容规则的既有行为，不在本修复中禁止或重写。
- **SQLite 与 MySQL 差异**：SQLite 回归捕获业务逻辑，MySQL gate 捕获真实 ORM 默认值和数据库写入行为；两者缺一不可。
