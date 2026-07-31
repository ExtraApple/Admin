# 2026-07-29 授权版本 SQLite 快速门禁

状态：通过

执行时间：2026-07-29 21:55:24 +08:00

数据库：隔离 SQLite

## 执行命令

```powershell
go test ./model -run '^(TestModelsAutoMigrateAccessVersionStorage|TestUserAccessVersionSQLiteDefaultsAndTransactions)$' -count=1 -v

go test ./service -run '^(TestAccessVersionRepositoryCurrentVersionReadsOnlyAuthorizationStorage|TestAccessVersionRepositoryEnsureVersionCreatesMissingVersionOne|TestAccessVersionRepositoryEnsureAndIncrementJoinsCallerTransaction|TestAccessVersionRepositoryEnsureVersionMirrorsInitializedFinalVersion|TestAccessVersionRepositoryEnsureAndIncrementMirrorsFinalVersion|TestAccessVersionRepositoryEnsureVersionRollsBackWhenMirrorTargetMissing|TestAccessVersionRepositoryEnsureAndIncrementRollsBackWhenMirrorTargetMissing|TestLoginInitializesMissingAccessVersionAfterCredentialsSucceed|TestLoginReturnsNoTokensWhenAccessVersionInitializationFails|TestLoginRollsBackInitializationAndReturnsNoTokensWhenVersionRereadFails|TestDeletingUserRollsBackWhenAccessVersionMirrorWriteFails)$' -count=1 -v
```

## 门禁结果

- Repository：通过
  - `TestAccessVersionRepositoryCurrentVersionReadsOnlyAuthorizationStorage`
  - `TestAccessVersionRepositoryEnsureVersionCreatesMissingVersionOne`
  - `TestAccessVersionRepositoryEnsureAndIncrementJoinsCallerTransaction`
  - 初始化和提升后的旧字段事务镜像测试通过。
- Service 数据库 Adapter：通过
  - `TestLoginInitializesMissingAccessVersionAfterCredentialsSucceed`
  - 登录通过真实 GORM SQLite Adapter 完成授权版本初始化后再签发 Token。
- 顺序初始化：通过
  - `TestModelsAutoMigrateAccessVersionStorage`
  - `TestUserAccessVersionSQLiteDefaultsAndTransactions`
  - `TestAccessVersionRepositoryEnsureVersionCreatesMissingVersionOne`
  - `TestAccessVersionRepositoryEnsureAndIncrementJoinsCallerTransaction`
- 失败注入：通过
  - `TestLoginReturnsNoTokensWhenAccessVersionInitializationFails`
  - `TestLoginRollsBackInitializationAndReturnsNoTokensWhenVersionRereadFails`
  - `TestAccessVersionRepositoryEnsureVersionRollsBackWhenMirrorTargetMissing`
  - `TestAccessVersionRepositoryEnsureAndIncrementRollsBackWhenMirrorTargetMissing`
  - `TestDeletingUserRollsBackWhenAccessVersionMirrorWriteFails`

测试输出中的 SQLite `record not found` 日志来自预期的缺行初始化或回滚断言，相关测试均通过。
本门禁只验证方言无关的快速反馈语义，不替代真实 MySQL 的命名锁、行锁、并发和死锁验收。
