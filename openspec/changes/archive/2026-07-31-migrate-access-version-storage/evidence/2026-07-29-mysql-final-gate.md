# 2026-07-29 授权版本真实 MySQL 最终门禁

状态：通过

执行时间：2026-07-29 22:22:25 +08:00

数据库：Windows 本地隔离 MySQL

- 地址：`127.0.0.1:23306/admin_rehearsal`
- 测试凭据在 PowerShell 进程内从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared` 注入。
- 未输出或写入密码。
- 未连接、停止或重启 `127.0.0.1:3306/MySQL80`。

## 执行命令

```powershell
go test -tags=mysql_integration ./... -run "^TestMySQL" -count=1 -v
```

## 门禁结果

### 迁移

- 迁移锁及释放：通过
  - `TestMySQLAccessVersionMigrationLockSerializesExecutorsAndReleasesAfterSuccess`
  - `TestMySQLAccessVersionMigrationLockReleasesAfterOperationFailure`
  - `TestMySQLAccessVersionMigrationProductionLockSerializesExecutors`
- 固定 cutoff、回填、失败重启与启动阻断：通过
  - `TestMySQLStartAccessVersionMigrationRecordsFixedCutoffAndRunningState`
  - `TestMySQLBackfillAccessVersionsPreservesPositiveValuesAndNormalizesNonPositiveValues`
  - `TestMySQLAccessVersionMigrationRetryReusesCutoffAndPreservesHigherVersion`
  - `TestMySQLAccessVersionMigrationFailureRestartUsesOriginalCutoff`
  - `TestMySQLMigrateDatabaseBlocksStartupWhenCompletionStateCannotPersist`
  - `TestMySQLMigrateDatabaseBlocksStartupWhenBackfillValidationFails`
  - `TestMySQLMigrateDatabaseCompletesAccessVersionMigrationAndIsRestartSafe`

### 并发、锁和重试

- 并发 `EnsureVersion`：通过
  - `TestMySQLAccessVersionRepositoryConcurrentEnsureVersionCreatesOneRow`
- 并发提升与镜像一致：通过
  - `TestMySQLAccessVersionRepositoryConcurrentIncrementsSerializeWithoutLosingVersions`
  - `TestMySQLAccessVersionRepositoryConcurrentEnsureAndIncrementNeverRegressesVersion`
- `SELECT ... FOR UPDATE` 行锁：通过
  - `TestMySQLAccessVersionRepositoryIncrementWaitsForLockedVersionRow`
- 真实死锁识别及有界重试：通过
  - `TestMySQLAccessVersionRepositoryEnsureVersionRetriesAfterRealDeadlock`
  - 测试在两个真实 MySQL 事务中按相反顺序锁定用户行和授权版本行，
    形成 InnoDB 死锁。
  - InnoDB 以错误 1213 中止第一次初始化事务；生产 Repository 将其识别为
    可重试错误，指标记录一次事务重试和一次死锁重试，第二次尝试成功。

### 原子事务和生命周期

- 主写与旧字段镜像事务：通过
  - `TestMySQLAccessVersionRepositoryConcurrentIncrementsSerializeWithoutLosingVersions`
  - `TestMySQLAccessVersionRepositoryMirrorFailureRollsBackBothStores`
  - 后者通过真实 MySQL `CHECK` 约束强制镜像 UPDATE 失败，并验证
    `user_access_versions.version` 与 `users.token_version` 同时回滚。
- 软删除与版本提升事务/回滚：通过
  - `TestMySQLDeletingUserSoftDeletesAndIncrementsAccessVersionInOneTransaction`
  - `TestMySQLDeletingUserRollsBackWhenAccessVersionMirrorWriteFails`
  - 失败用例验证镜像失败时用户保持未删除，两个版本存储保持原值。

### Schema

- MySQL AutoMigrate、索引、外键和软删除：通过
  - `TestMySQLAccessVersionAutoMigrateSchemaAndSoftDelete`
  - 验证主键、非级联外键约束、用户软删除和授权版本记录保留。

SQLite 结果未用于替代上述 MySQL 命名锁、行锁、死锁、并发、事务和
AutoMigrate 结论。
