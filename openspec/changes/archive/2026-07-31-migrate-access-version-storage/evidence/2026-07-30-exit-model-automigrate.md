# 2026-07-30 旧字段 Model 与 AutoMigrate 退出证据

状态：通过

对应任务：9.5

本任务只移除应用层 GORM Model 和 AutoMigrate 对旧列的声明，不执行
`users.token_version` 物理删列；独立 DDL 仍属于任务 9.7。

## TDD Red

9.5 的行为测试先证明退出版仍声明旧字段：

```text
TestExitVersionUserModelDoesNotDeclareLegacyTokenVersion
→ exit-version User model still declares TokenVersion

TestExitVersionAutoMigrateDoesNotDeclareLegacyTokenVersionColumn
→ exit-version AutoMigrate still declares users.token_version
```

移除正式 Model 字段后，标签编译进一步暴露过渡测试仍依赖
`model.User.TokenVersion`：

```text
go test -tags=mysql_integration ./... -run '^$' -count=1
→ initialize/access_version_migration_mysql_integration_test.go 编译失败

go test -tags=redis_integration ./... -run '^$' -count=1
→ middleware/access_version_redis_integration_test.go 编译失败
```

真实 MySQL 运行还证明 4 个过渡测试仍错误要求退出版 `migrateDatabase` 执行旧字段
迁移，与已完成的 9.4 冲突。

## Green 实现

- 从 `model.User` 移除 `TokenVersion`，JWT Claim 中的同名字段保持不变。
- `model.Models` 继续迁移 `User`、`UserAccessVersion` 和迁移状态，但正式 User
  schema 不再声明 `token_version`。
- 旧迁移测试使用测试专用 `legacyAccessVersionMigrationTestUser` 显式声明旧列；
  fixture 通过 `schema.Namer` 保留每个真实 MySQL 测试的隔离表前缀。
- 旧迁移启动语义改由测试专用 helper 显式调用，退出版 `migrateDatabase` 不重新
  引入旧字段运行时访问。
- Redis 组合测试使用独立的授权版本常量，不再从 `model.User` 读取旧字段。

## Green 验证

定向 Model 与 AutoMigrate 测试：

```powershell
go test ./model ./initialize `
  -run 'TestExitVersion(UserModelDoesNotDeclareLegacyTokenVersion|AutoMigrateDoesNotDeclareLegacyTokenVersionColumn)$' `
  -count=1 -v
```

结果：2 个 9.5 行为测试全部通过。

相关包与全仓回归：

```powershell
go test ./model ./initialize -count=1
go test ./... -count=1
```

结果：全部通过。

标签编译门禁：

```powershell
go test -tags=mysql_integration ./... -run '^$' -count=1
go test -tags=redis_integration ./... -run '^$' -count=1
```

结果：全部通过。

真实外部依赖门禁：

```powershell
go test -tags=mysql_integration ./initialize -count=1 -v
go test -tags=redis_integration ./middleware `
  -run '^TestRealRedisAccessRefreshAndBlacklistCombination$' `
  -count=1 -v
```

结果：

- Windows 本地隔离 MySQL
  `127.0.0.1:23306/admin_rehearsal`：initialize 全套通过；
- Docker Redis `127.0.0.1:6379`，DB 15：组合测试通过；
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接、停止、重启或修改；
- 凭据仅从 `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared` 注入测试
  子进程，未输出或写入仓库。

静态检查：

```text
git diff --check
gofmt -d <9.5 相关 Go 文件>
```

结果：通过，无格式差异；临时外部门禁脚本已删除。

## 结论

退出版的 User GORM Model 和 AutoMigrate 已不再声明 `token_version`，启动迁移不会
因为正式 Model 而重新创建旧列。物理旧列仍保留，等待任务 9.7 的独立数据库变更。
