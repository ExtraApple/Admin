# 2026-07-31 旧列删除后启动与全量回归

状态：通过

对应任务：9.8

## 环境与边界

- MySQL：Windows 本地隔离真实 MySQL
  `127.0.0.1:23306/admin_rehearsal`；
- Redis：Docker 容器 `redis`，测试地址 `127.0.0.1:6379`；
- 凭据仅从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入当前 PowerShell 测试进程；
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接或修改。

## TDD

Red：

- 先新增删列后证据契约测试；
- 因 9.8 证据目录和日志尚不存在，测试按预期失败；
- 首次真实 MySQL 连接测试又暴露结果列别名未映射问题，并在运行
  AutoMigrate 前停止，数据库未发生该测试触发的启动迁移。

Green：

- 修正真实连接结果映射；
- 通过与应用启动相同的 `migrateDatabase` 路径对隔离数据库执行
  AutoMigrate；
- 旧列在启动前不存在；
- AutoMigrate 后未重建 `users.token_version`；
- `user_access_versions` 在启动后继续存在。

原始 Red/Green 日志：

```text
tdd-red.log
tdd-red-connected-target.log
tdd-green.log
startup-schema.log
```

## 验证结果

- 启动数据库阶段与结构复核：通过；
- 关键认证授权回归：通过；
- 真实 MySQL 并发与事务门禁：通过；
- 真实 Redis 组合门禁：通过；
- 全量 Go 回归：通过。

结构结果：

```text
port=23306
database_name=admin_rehearsal
legacy_column_count=0
access_version_table_count=1
```

日志：

```text
startup-schema.log
auth-regression.log
mysql-concurrency-transaction.log
redis-combination.log
full-regression.log
schema-verification.log
```

## 结论

独立 DDL 删除旧列后，退出版本的数据库启动路径成功执行，AutoMigrate 未重建旧列；
登录、Refresh Token、JWT、授权失效长期回归、真实 MySQL 并发事务行为、Docker
Redis 组合行为和全量 Go 测试全部通过。
