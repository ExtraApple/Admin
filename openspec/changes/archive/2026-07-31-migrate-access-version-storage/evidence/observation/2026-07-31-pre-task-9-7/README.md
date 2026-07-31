# 2026-07-31 任务 9.7 前置门禁

状态：通过

执行日期：2026-07-31（Asia/Shanghai）

触发策略：事件触发，不设置固定执行时间

触发原因：开始独立 DDL 物理删除 `users.token_version`

## 验证环境

- 数据库：Windows 本地隔离真实 MySQL，
  `127.0.0.1:23306/admin_rehearsal`；
- Redis：Docker 容器 `redis`，测试地址 `127.0.0.1:6379`；
- 旧证据记录的容器名 `admin-av-redis-20260728` 已不存在，本次通过
  `docker ps` 和 TCP 探测确认实际 Redis；
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接、停止、重启或修改；
- 凭据仅从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入当前 PowerShell 测试进程，未写入日志或仓库。

## 门禁结果

- 关键认证授权回归：通过；
- 真实 MySQL 并发与事务门禁：8 个场景通过；
- 真实 Redis 组合门禁：通过；
- 最后一次可执行的新旧字段一致性扫描差异：0；
- cutoff 后合法缺行：0。

原始日志：

```text
auth-regression.log
mysql-concurrency-transaction.log
redis-combination.log
consistency-scan.log
```

本次门禁通过后允许开始任务 9.7。物理删列完成后，新旧字段一致性扫描不再适用；
后续门禁改为新表结构、认证授权和事务行为验证。
