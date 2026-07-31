# 2026-07-30 旧字段退出审批后前置门禁

状态：通过

执行时间：2026-07-30 23:39 +08:00

触发策略：事件触发，不设置固定执行时间

触发原因：发布负责人确认弃用旧代码版本回滚

## 验证环境

- 数据库：Windows 本地隔离真实 MySQL，
  `127.0.0.1:23306/admin_rehearsal`；
- Redis：Docker Redis 容器 `redis`，测试地址 `127.0.0.1:6379`；
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接、停止、重启或修改；
- 凭据仅从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入当前 PowerShell 测试进程，未写入日志或仓库。

## 门禁结果

- 关键认证授权回归：通过；
- 真实 MySQL 并发与事务门禁：通过；
- 真实 Redis 组合门禁：通过；
- 一致性扫描差异：0；
- cutoff 后合法缺行：0。

原始日志：

```text
auth-regression.log
mysql-concurrency-transaction.log
redis-combination.log
consistency-scan.log
```

本次一致性扫描发生在任务 9.2 的回滚弃用确认之后，可作为停止旧字段镜像写之前
的最终只读扫描。
