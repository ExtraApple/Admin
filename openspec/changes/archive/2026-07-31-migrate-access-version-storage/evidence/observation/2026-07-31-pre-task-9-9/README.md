# 任务 9.9 前置观察门禁

执行日期：2026-07-31

任务：删除旧字段镜像、迁移观察和旧版本回滚的过渡能力。

## 环境边界

- MySQL：Windows 本地隔离真实 MySQL
  `127.0.0.1:23306/admin_rehearsal`；
- Redis：Docker Redis，测试地址 `127.0.0.1:6379`；
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接、停止、重启或修改；
- 凭据只从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入子进程，未写入日志。

## 结果

- `auth-regression.log`：Service 与 Router 关键认证授权回归通过；
- `mysql-concurrency-transaction.log`：真实 MySQL 并发初始化、并发提升、
  行锁、死锁重试和事务回滚门禁通过；
- `post-drop-schema.log`：删列后启动路径通过，旧列未重建，
  `user_access_versions` 存在；
- `redis-combination.log`：真实 Redis 下 Access Token、Refresh Token 和
  黑名单组合门禁通过。

前置门禁结论：**通过，可以开始任务 9.9。**
