# 授权版本过渡能力清理证据

执行日期：2026-07-31

任务：OpenSpec `migrate-access-version-storage` 9.9。

## TDD

Red：

```powershell
go test ./initialize `
  -run '^TestAccessVersionTransitionComponentsAreRetired$' `
  -count=1 -v
```

测试逐项报告旧迁移、部署检查、资格运行器、回滚脚本、迁移状态 Model、
过渡测试、镜像/一致性指标和镜像错误仍存在。

Green：

- 删除旧字段迁移、数据库命名锁、部署混跑检查、一致性扫描、
  三小时资格运行器、删列工具和回滚预检；
- 删除迁移状态 Model 及其 AutoMigrate 注册；
- 删除只服务于迁移、观察、镜像和回滚的过渡测试与 evidence 契约测试；
- 保留授权版本 Repository、认证授权行为、MySQL/Redis 集成测试、退出运行时、
  AutoMigrate、ADR 和历史原始证据；
- 删除镜像失败与一致性差异指标，只保留长期授权版本运行指标。

退休契约随后通过。

## 隔离 MySQL 状态清理

唯一操作目标：

```text
127.0.0.1:23306/admin_rehearsal
```

正式 MySQL `127.0.0.1:3306/MySQL80` 未连接或修改。

删除前验证：

- 当前用户：`admin_rehearsal`；
- `access_version_migration_states`：1 张表、1 行；
- `user_access_versions`：存在；
- `users.token_version`：不存在。

先分别备份迁移状态表结构和数据，再计算 SHA-256：

```text
eac87ac68e4ba6abfd8c18d2771980896444095c1d34fee4e07e542357cc4159  access_version_migration_states.schema.sql
3f204460f254c7868e0238c8b57dc9d1ca3398bc4d513d2d4844240dadf667ca  access_version_migration_states.data.sql
```

随后独立执行：

```sql
DROP TABLE IF EXISTS access_version_migration_states;
```

当前源码构建的临时退出版本已运行到 `InitMysql` 完成。启动后结构：

- `access_version_migration_states`：不存在；
- `user_access_versions`：存在；
- `users.token_version`：不存在。

首次临时启动使用了旧运行目录中过期的 `.env`，在 MySQL 身份验证阶段失败，
未执行迁移或 DDL；改为使用已验证的共享凭据后启动门禁通过。

## 证据索引

- `pre-drop-verification.log`
- `access_version_migration_states.schema.sql`
- `access_version_migration_states.data.sql`
- `backup-sha256.txt`
- `schema-backup.stderr.log`
- `data-backup.stderr.log`
- `drop-and-schema-verification.log`
- `startup-schema-verification.log`
- `startup-app.log`
- `retirement-contract.log`
- `core-packages.log`
- `mysql-integration.log`
- `redis-integration.log`
- `all-packages.log`

## 验证结论

以下门禁全部通过：

```text
go test ./initialize -run ^TestAccessVersionTransitionComponentsAreRetired$ -count=1 -v
go test ./model ./service ./router ./initialize -count=1
go test -tags=mysql_integration ./model ./service ./initialize -count=1
go test -tags=redis_integration ./middleware -count=1
go test ./... -count=1
```

任务 9.9 结论：**通过。**
