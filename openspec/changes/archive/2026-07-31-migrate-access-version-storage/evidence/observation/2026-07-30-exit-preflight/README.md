# 2026-07-30 旧字段退出任务前置门禁

状态：通过

执行时间：2026-07-30 23:16 +08:00

触发策略：事件触发，不设置固定执行时间

触发原因：用户在 23:15 要求继续本 Change

本记录满足任务 8.7 对当次剩余任务开始前门禁的要求。本次门禁不能替代任务 8.6；
任务 8.6 已由独立连续三小时资格运行验证。

## 验证环境

- 数据库：Windows 本地隔离真实 MySQL，
  `127.0.0.1:23306/admin_rehearsal`。
- Redis：Docker Redis 容器 `redis`，测试连接 `127.0.0.1:6379`。
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接、停止、重启或修改。
- MySQL 和 Redis 凭据只从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入当前 PowerShell 测试进程，未输出到日志或写入仓库。

## 门禁结果

### 关键认证授权回归

```powershell
go test ./service ./router -count=1
```

结果：

- `admin/service`：通过；
- `admin/router`：通过；
- 关键认证授权回归：通过。

原始日志：`auth-regression.log`

### 真实 MySQL 并发与事务门禁

```powershell
go test -tags=mysql_integration ./service `
  -run '^TestMySQL(AccessVersionRepository|DeletingUser)' `
  -count=1 -v
```

结果：8 个场景全部通过，覆盖：

- 并发首次初始化；
- 并发版本提升；
- 行锁等待；
- 初始化与提升竞态；
- 真实死锁识别和有界重试；
- 镜像写失败原子回滚；
- 用户软删除和版本提升原子提交；
- 用户软删除业务更新在镜像失败时回滚。

真实 MySQL 并发与事务门禁：通过。

原始日志：`mysql-concurrency-transaction.log`

### 真实 Redis 组合门禁

```powershell
go test -tags=redis_integration ./middleware `
  -run '^TestRealRedisAccessRefreshAndBlacklistCombination$' `
  -count=1 -v
```

结果：Access Token、Refresh Token 及两类黑名单组合行为全部通过。

真实 Redis 组合门禁：通过。

原始日志：`redis-combination.log`

### 只读一致性扫描

```powershell
go test -tags='mysql_integration,rollback_preflight' ./service `
  -run '^TestMySQLAccessVersionRollbackPreflightConsistencyScan$' `
  -count=1 -v
```

结果：

```text
cutoff_user_id=1
consistency_differences=0
legal_post_cutoff_missing=0
```

一致性扫描差异：0。

原始日志：`consistency-scan.log`

## 结论

- 关键认证授权回归：通过；
- 真实 MySQL 并发与事务门禁：通过；
- 真实 Redis 组合门禁：通过；
- 一致性扫描差异：0；
- 前置门禁未发现阻止旧字段退出任务继续实施的问题。
