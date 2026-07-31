# 授权版本存储停机切换历史记录

## 状态

**已完成并退役，最后更新于 2026-07-31。**

本文只用于解释 `migrate-access-version-storage` 的历史停机切换、回滚边界和证据，
不是当前运维入口。以下过渡能力已经删除：

- 旧字段回填与固定 cutoff 迁移；
- MySQL 迁移命名锁和迁移状态；
- 新旧版本一致性观察；
- 旧字段镜像；
- 旧版本回滚预检；
- 三小时资格运行器；
- 旧列删除工具。

不得尝试调用历史文档或证据中出现的
`run-access-version-lifecycle-e2e.ps1`、
`run-access-version-rollback-preflight.ps1`、
`cmd/access-version-qualification` 或
`cmd/access-version-drop-legacy-column`；这些入口已经随退出里程碑退役。

## 当前永久状态

- Authorization 拥有 `user_access_versions`。
- Access Token、Refresh Token 和 JWT 中间件只从该表读取授权版本。
- `users.token_version` 已从运行时、User GORM Model、AutoMigrate 和隔离真实
  MySQL 中删除。
- `access_version_migration_states` 已备份后删除，启动不会重建。
- 新用户由 `EnsureVersion` 懒初始化；禁用、软删除、Kick、密码和授权关系变化
  由 `EnsureAndIncrement` 创建或提升版本。
- Seed 不创建、扫描或修复授权版本。
- 当前应用只能回滚到仍读取 `user_access_versions` 且不依赖旧列或迁移状态的版本。

## 历史切换顺序

本 Change 当时采用停机切换，而不是新旧版本滚动混跑：

```text
停止旧实例和写流量
→ 取得数据库级锁
→ 创建新表和迁移状态
→ 固定 cutoff 并幂等回填
→ 完成数量与版本一致性校验
→ 启动只读新表且原子镜像旧字段的切换版本
→ 恢复流量
```

切换后通过真实 MySQL、Docker Redis、至少 100,000 次混合操作、至少 8 个并发
工作单元、至少 3 次进程重启和周期性一致性扫描完成连续三小时加速耐久性资格验收。

## 历史退出顺序

```text
完成最终一致性扫描
→ 确认不再回滚到读取旧字段的版本
→ 发布停止镜像写的退出版本
→ 移除运行时读写、User Model 和 AutoMigrate 列声明
→ 独立 DDL 删除 users.token_version
→ 验证启动与全部认证授权链路
→ 删除迁移、观察和旧版本回滚能力
→ 备份后删除 access_version_migration_states
```

该顺序已经完成。物理删除后的旧字段恢复不属于常规应用回滚，必须作为新的独立
数据库和架构变更处理。

## 历史证据

原始记录保留在：

```text
openspec/changes/migrate-access-version-storage/evidence/
```

关键索引：

- `qualification/qualification-20260730T121335Z/`：三小时资格验收；
- `2026-07-30-final-consistency-scan.md`：退出前最终一致性扫描；
- `2026-07-31-exit-version-validation/`：退出版本验证；
- `2026-07-31-legacy-column-drop/`：旧列备份、DDL 和结构复核；
- `2026-07-31-post-drop-validation/`：删列后全量验证；
- `2026-07-31-transition-retirement/`：过渡代码和迁移状态清理。

凭据、JWT、Redis、MySQL 和对象存储密钥均不应写入证据目录。
