# 2026-07-31 独立 DDL 物理删除旧列

状态：通过

对应任务：9.7

## 目标和边界

- 唯一目标：Windows 本地隔离 MySQL
  `127.0.0.1:23306/admin_rehearsal`；
- 物理变更：`ALTER TABLE users DROP COLUMN token_version`；
- 正式 MySQL `127.0.0.1:3306/MySQL80` 未连接或修改；
- 退出版本 9.6 通过证据是工具执行的强制前置条件；
- 凭据只通过 `ADMIN_ACCESS_VERSION_EXIT_DSN` 环境变量注入，不进入参数、
  日志或仓库。

## TDD

Red：

- 工具入口、目标守卫、审批短语和证据校验不存在，单元测试编译失败；
- 首次实际预检暴露 `CURRENT_USER()` 别名兼容问题；数据库在 DDL 前未发生变更，
  随后补充真实 MySQL Red 测试稳定复现。

Green：

- 只接受 `admin_rehearsal@tcp(127.0.0.1:23306)/admin_rehearsal`；
- 明确拒绝 3306、其他主机、其他数据库、其他用户和非 TCP 连接；
- 要求精确审批短语及任务 9.6 通过证据；
- 真实连接再次校验 `@@port`、`DATABASE()` 和 `CURRENT_USER()`；
- 真实 MySQL 临时表测试证明备份、删列、重复执行幂等及 AutoMigrate 不重建旧列。

日志：

```text
tdd-red.log
tdd-red-connected-target.log
tdd-green-integration.log
tdd-green-connected-target.log
```

## 备份和恢复

DDL 前保存：

```text
database-change/pre-users-show-create.sql
database-change/legacy-token-version-values.tsv
database-change/restore-token-version.sql
database-change/target.txt
database-change/artifact-sha256.txt
tool-sha256.txt
```

`restore-token-version.sql` 仅用于独立紧急数据库恢复，不属于常规应用回滚。
`hash-verification.log` 证明四个数据库备份/目标文件的 SHA-256 与清单一致。

## 执行结果

首次执行：

```text
result=dropped
target=127.0.0.1:23306/admin_rehearsal
table=users
column=token_version
completed_at=2026-07-31T18:18:57+08:00
```

重复执行：

```text
result=already_absent
```

独立 `cmd.exe` + MySQL 结构复核：

```text
port=23306
database_name=admin_rehearsal
legacy_column_count=0
```

原始证据：

```text
ddl-execution.log
ddl-idempotent-rerun.log
schema-verification.log
hash-verification.log
database-change/result.json
idempotent-rerun/result.json
```

## 结论

`users.token_version` 已从隔离真实 MySQL 物理删除；备份、恢复 SQL、完整性哈希、
目标复核、删列结果和幂等复核均已归档。
