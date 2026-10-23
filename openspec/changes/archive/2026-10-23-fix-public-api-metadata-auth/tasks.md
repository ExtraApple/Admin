## 1. 基线与实现范围

- [x] 1.1 固定当前代码基线、`api-management` 主规格、A3 偏差证据和现有 API Metadata Seed／GORM 模型行为。
- [x] 1.2 确认本 change 只处理 Catalog 新建 Metadata 的 Public 零值持久化，不修改既有 Metadata、手工 CRUD、模型默认值或历史数据。

## 2. Catalog Seed 修复

- [x] 2.1 在 `internal/app/seed.go` 的新 API Metadata 分支中保留 Descriptor Access 到 `needAuth` 的现有计算和 Permission Code 语义。
- [x] 2.2 对每条新 Metadata 使用记录级事务创建记录；当 `needAuth == 0` 时显式更新 `need_auth=0`，确保 GORM 默认值不覆盖 Public 结果。
- [x] 2.3 让创建或显式零值更新任一步失败时回滚该条 Metadata，并返回既有同步错误；不扩大为整个 Catalog 的大事务。
- [x] 2.4 验证既有 Metadata、软删除恢复记录、`POST /api/refresh` 兼容分支和 PermissionControlled 默认权限码行为未被改变。

## 3. 回归与真实数据库验证

- [x] 3.1 新增 SQLite focused Seed 回归，覆盖 Public、Authenticated、PermissionControlled 新记录的 `need_auth`、Permission Code 和既有字段保留。
- [x] 3.2 新增 `//go:build mysql_integration` MySQL 门禁测试，直接验证真实 MySQL 中 Public 新 Metadata 最终为 `need_auth=0`，并清理唯一临时记录。
- [x] 3.3 运行现有 Route Catalog、API Metadata 同步、静态 Access Level 和响应契约测试，确认匿名 Public 与受保护路由边界不变。
- [x] 3.4 通过真实隔离 App 或等价 HTTP smoke 验证 API sync 后 Public Metadata 不生成 Permission Code，PermissionControlled Metadata 仍要求认证。

## 4. 规格与交付验证

- [x] 4.1 复核 `api-management` delta 与实现一致，明确已有 Metadata 保留和历史数据修复不在本 change 范围。
- [x] 4.2 运行格式化、focused tests、MySQL gate 和 OpenSpec 严格校验；不修改 A3 归档工件。
- [x] 4.3 复核变更不新增路由、权限码、数据库表、Redis key、MinIO bucket 或第三方依赖，并记录后续历史数据修复分流。
