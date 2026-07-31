# 数据库测试门禁

## SQLite 快速门禁

日常开发和普通 CI 执行：

```powershell
.\initialize\testutil\run-sqlite-gate.ps1
```

- 默认测试不得依赖外部 MySQL。
- 数据库测试使用 `testutil.OpenIsolatedSQLite(t)`。
- Helper 每次创建独立的临时文件数据库，并在测试清理阶段先关闭底层
  `sql.DB`，以支持 Windows 删除临时目录。
- SQLite 适用于 Model 映射、基础约束、Repository CRUD、普通事务回滚、
  顺序 Service 行为和失败注入。

## MySQL 强制门禁

合并和发布前必须执行：

```powershell
$env:ADMIN_TEST_MYSQL_DSN = "<user>:<password>@tcp(<host>:3306)/<database>?charset=utf8mb4&parseTime=True&loc=Local"
.\initialize\testutil\run-mysql-gate.ps1
```

MySQL 测试文件使用：

```go
//go:build mysql_integration
```

测试名称以 `TestMySQL` 开头。门禁未提供 DSN、数据库不可达或任一测试失败时均失败，不允许设置为可选或允许失败。

以下语义只能由真实 MySQL 验收，SQLite 结果不能替代：

- MySQL upsert 方言；
- `SELECT ... FOR UPDATE`、并发初始化和并发失效；
- 事务隔离、死锁识别和有界重试；
- 授权版本主写与用户生命周期业务更新的原子提交；
- `user_access_versions` 的 AutoMigrate、索引、外键和软删除交互；
- 启动不会声明或重建已退役的 `users.token_version` 与
  `access_version_migration_states`。

旧字段迁移锁、固定 cutoff、镜像写、一致性观察、回滚预检和独立删列工具已经随
退出里程碑退役。相关原始日志保留在 OpenSpec Change 的历史证据目录中，不再作为
当前测试入口。
