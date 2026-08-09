# 数据库迁移和 Seed 初始化

> 本文只记录当前启动行为和运维边界。开发阶段继续使用 AutoMigrate 与幂等 Seed 的决策见 [`0002-development-database-evolution-with-automigrate-and-seed.md`](../adr/0002-development-database-evolution-with-automigrate-and-seed.md)。架构事实以 [`openspec/specs/modular-layered-architecture/spec.md`](../../openspec/specs/modular-layered-architecture/spec.md) 为准。

## 当前启动顺序

```text
读取并校验配置
→ 创建 MySQL、Redis、MinIO 和 Zap
→ 构造业务模块与 Route Catalog
→ App.Migrate
→ App.SeedCatalog
→ 注册并启动 HTTP 与后台任务
```

`internal/app/migrate.go` 和 `internal/app/seed.go` 是唯一迁移与 Seed 编排入口。

## AutoMigrate 职责

当前 `App.Migrate`：

- 汇总各模块拥有的 GORM Model 并执行 `AutoMigrate`。
- 回填历史文件和头像的验证状态。
- 将不符合当前普通文件白名单的旧 `validated` 文件降级为 `legacy_unverified`。
- 不读取、删除或重写历史 MinIO 对象。

当前开发阶段没有正式 SQL migration 版本目录。生产环境切换到显式 migration 工具必须作为独立架构与部署变更处理。

## Seed 职责

`SeedCatalog` 通过事务和 Route Catalog Snapshot 幂等补齐：

- 默认角色、权限分组和字典。
- API 元数据与权限码。
- 默认菜单和菜单/API 关联。
- 超级管理员角色权限、菜单和用户。

Seed 重复执行不得插入重复记录，也不得无条件覆盖管理员维护的展示字段或删除业务数据。

## 授权版本边界

- `user_access_versions` 是唯一授权版本存储。
- 新用户首次登录通过 `EnsureVersion` 懒初始化。
- 密码、状态、Kick 和授权关系变化通过 `EnsureAndIncrement` 创建或提升版本。
- Seed 不创建、扫描或修复授权版本。
- `users.token_version`、旧迁移状态、镜像写和旧字段回滚能力已退役，不得恢复。

历史切换和退出证据见 [`access-version-storage-switch.md`](access-version-storage-switch.md)。

## 运维检查

1. 配置与必需环境变量完整。
2. MySQL、Redis、MinIO 可连接。
3. 启动日志包含迁移和 Seed 结果，且没有部分初始化错误。
4. 超级管理员可登录，API、权限和菜单元数据已补齐。
5. 重复启动不会产生重复基础数据或恢复已退役结构。
