# 2026-07-30 三小时资格验收启动前置门禁

状态：通过

执行时间：2026-07-30 20:04:16 +08:00

触发策略：事件触发，不设置固定执行时间

触发原因：用户要求继续实施并准备启动三小时加速耐久性资格验收

本记录是任务 8.7 要求的当次任务开始前门禁证据。它不能替代任务 8.6 的连续
三小时资格验收，也不能单独用于确认任务 9.1。

## 验证环境

- 数据库：Windows 本地隔离真实 MySQL，
  `127.0.0.1:23306/admin_rehearsal`。
- 正式 Windows 服务 `MySQL80` 保持 Running；未连接、停止、重启或修改
  `127.0.0.1:3306` 上的正式实例。
- Redis：Docker 容器 `redis`，测试连接 `127.0.0.1:6379`。
- 数据库密码只从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入当前 PowerShell 测试进程，未输出或写入仓库。

## 关键认证授权回归

```powershell
go test ./service ./router -count=1
```

结果：

```text
ok  	admin/service
ok  	admin/router
```

## 资格运行器与后台恢复回归

```powershell
go test ./cmd/access-version-qualification -count=1
```

结果：

```text
ok  	admin/cmd/access-version-qualification
```

后台恢复测试验证独立 Windows 进程可以持久化 PID、状态、退出码、日志和最终
summary，并验证证据文件不包含 MySQL DSN、Redis 密码、工作用户密码或 JWT
Secret。

## 真实 MySQL 并发与事务门禁

```powershell
$env:ADMIN_TEST_MYSQL_DSN = "<从临时隔离凭据注入>"
go test -tags=mysql_integration ./service `
  -run '^TestMySQL(AccessVersionRepository|DeletingUser)' `
  -count=1 -v
```

结果：8 个场景全部通过，覆盖并发首次初始化、并发提升、行锁等待、初始化与提升
竞态、真实死锁重试、镜像写失败回滚、软删除原子提交和镜像失败业务回滚。

## 真实 Redis 组合门禁

```powershell
$env:ADMIN_TEST_REDIS_ADDR = "127.0.0.1:6379"
$env:ADMIN_TEST_REDIS_DB = "15"
go test -tags=redis_integration ./middleware `
  -run '^TestRealRedisAccessRefreshAndBlacklistCombination$' `
  -count=1 -v
```

结果：通过。覆盖 Access Token 初始认证、Refresh Token 初始刷新、Access Token
黑名单、Refresh Token 黑名单和两类黑名单的独立语义。

## 只读一致性扫描

```powershell
$env:ADMIN_TEST_MYSQL_DSN = "<从临时隔离凭据注入>"
go test -tags='mysql_integration,rollback_preflight' ./service `
  -run '^TestMySQLAccessVersionRollbackPreflightConsistencyScan$' `
  -count=1 -v
```

最终结果：

```text
rollback preflight passed: cutoff_user_id=1 consistency_differences=0 legal_post_cutoff_missing=0
```

扫描为只读，未修改隔离演练数据库。

## 门禁结论

- 关键认证授权回归：通过
- 资格运行器与后台恢复回归：通过
- 真实 MySQL 并发与事务门禁：通过
- 真实 Redis 组合门禁：通过
- 一致性扫描差异：0
- cutoff 后合法缺行：0
- 允许继续执行短时父子进程 smoke：是
- 允许把本记录视为三小时资格验收：否
