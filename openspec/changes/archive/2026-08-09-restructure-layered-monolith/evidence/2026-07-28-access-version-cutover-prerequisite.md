# 2026-07-28 分层单体重构实施前置条件

状态：已满足

执行时间：2026-07-28 21:54:00 +08:00
性质：历史快照（2026-07-28）。本文件保留当时切换里程碑和观察期状态；当前前置结论以 `evidence/2026-08-02-access-version-cutover-current.md` 及授权版本 Change 的 2026-07-31 退出证据为准，不应再据此等待观察期或恢复旧字段能力。

依赖 Change：`migrate-access-version-storage`

## 依赖验证

- 切换里程碑：完成
- 证据：
  `openspec/changes/archive/2026-07-31-migrate-access-version-storage/evidence/2026-07-28-cutover-milestone.md`
- 所有 Token 授权版本读取已经切换到 `user_access_versions`。
- 观察期镜像写仍然保留，并继续由
  `migrate-access-version-storage` Change 跟踪。

## 本 Change 边界

- `restructure-layered-monolith` 只消费 Authorization 授权版本 Contract。
- 本 Change 不迁移 `users.token_version`。
- 本 Change 不停止观察期镜像写，不删除 User Model 旧字段，也不执行删列 DDL。
- 授权版本旧字段退出仍由 `migrate-access-version-storage` 按完整发布周期和连续
  7 天观察门槛推进。

## 结论

切换里程碑是结构重构的实施前置条件；退出里程碑不是结构重构的启动前置条件。
因此本 Change 无需等待连续 7 天观察期结束。

允许 `restructure-layered-monolith` 开始实施：是
