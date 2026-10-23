## Why

A3 审计发现：Route Catalog 中的 Public Descriptor 在首次创建 API Metadata 时计算出的 `need_auth=0`，经过 GORM 默认值写入 MySQL 后实际变成了 `1`。这使 API Metadata 投影偏离已发布的 Public 路由同步契约；当前 Gin 静态 Public 边界仍有效，但需要在新记录写入时修复该持久化偏差。

## What Changes

- 修复 Catalog Seed 创建新 API Metadata 时的零值持久化，确保 Public Descriptor 最终保存 `need_auth=0`。
- 将新 Metadata 的创建与显式零值更新放入同一条记录级数据库事务；更新失败时不保留半成品记录。
- 保持 Authenticated 和 PermissionControlled 新 Metadata 的 `need_auth=1`、默认 Permission Code、`status`、`need_audit` 和展示字段语义不变。
- 保持既有 Metadata（包括软删除记录恢复）的管理员字段，不在本 change 中自动修复历史错误记录。
- 增加 SQLite focused 回归测试、`mysql_integration` MySQL 门禁和既有 HTTP 静态访问验证。
- 更新 `api-management` 规格，明确从 Route Catalog 新建 Public Metadata 时必须持久化 `need_auth=0`，既有记录按兼容规则保留。

## Capabilities

### New Capabilities

无。本 change 修复现有 API Metadata 同步行为，不新增运行时能力。

### Modified Capabilities

- `api-management`: 明确 Route Catalog 新建 Public API Metadata 的 `need_auth` 持久化结果和既有记录保留边界。

## Impact

- 代码：`internal/app/seed.go` 的 Catalog API Metadata 新记录写入路径。
- 测试：新增 focused Seed 回归和 MySQL 集成门禁；不改变默认测试对外部 MySQL 的依赖。
- 规格：修改 `openspec/specs/api-management/spec.md` 对应 API 路由同步 Requirement。
- 不新增路由、权限码、数据库表、Redis key、MinIO bucket 或第三方依赖。
- 不包含历史 Metadata 数据修复；历史 `Public + need_auth=1` 记录另建独立数据修复 change。
