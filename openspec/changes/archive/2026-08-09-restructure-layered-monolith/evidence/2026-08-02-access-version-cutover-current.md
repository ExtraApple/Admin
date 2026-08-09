# 2026-08-02 分层单体重构授权版本前置复核

状态：已满足
复核时间：2026-08-02
依赖 Change：`migrate-access-version-storage`

## 当前结论

`migrate-access-version-storage` 已在 2026-07-31 记录退出里程碑通过。当前 `user_access_versions` 是 Access Token、Refresh Token 和授权失效操作使用的唯一长期持久化事实来源。

已观察到的退出结果：

- 正常启动、登录、Access Token、Refresh Token、JWT 校验和授权失效入口不再访问 `users.token_version`。
- `users.token_version` 已从 User Model、AutoMigrate 和隔离真实 MySQL 中移除。
- `access_version_migration_states`、旧字段镜像、迁移观察和旧版本回滚专用能力已退役。
- 后续应用版本只能依赖 `user_access_versions`，不能把旧字段作为常规回滚前提。

## 证据索引

- `openspec/changes/archive/2026-07-31-migrate-access-version-storage/evidence/2026-07-31-exit-milestone/README.md`
- `openspec/changes/archive/2026-07-31-migrate-access-version-storage/evidence/2026-07-31-exit-version-validation/README.md`
- `openspec/changes/archive/2026-07-31-migrate-access-version-storage/evidence/2026-07-31-transition-retirement/README.md`
- `docs/adr/0004-authorization-owns-access-version-storage.md`
- `openspec/specs/access-version-storage/spec.md`

## 本 Change 边界

- `restructure-layered-monolith` 只消费 Authorization 提供的授权版本 Contract。
- 本 Change 不迁移、镜像、删除或恢复 `users.token_version`。
- 本 Change 不依赖迁移状态表、观察期指标或旧版本回滚代码。
- 后续实现仍需把 `EnsureVersion`、`EnsureAndIncrement` 和当前授权版本读取封装在 Authorization 模块 Contract 后，不得把现有顶层 Service 作为长期跨模块入口。

## 前置决定

退出里程碑已经完成，因此本 Change 不需要等待旧字段观察期，也不需要为目录重构恢复旧字段双写或旧字段读取。允许开始实施：是。
