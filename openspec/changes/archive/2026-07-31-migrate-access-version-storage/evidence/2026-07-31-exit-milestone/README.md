# 2026-07-31 授权版本存储退出里程碑

状态：通过

对应任务：9.10

## 里程碑结论

- `user_access_versions` 已成为授权版本唯一持久化事实来源；
- 正常启动、登录、Access Token、Refresh Token、JWT 和全部授权失效入口不再
  访问 `users.token_version`；
- `users.token_version` 已从隔离真实 MySQL 物理删除，AutoMigrate 不会重建；
- 迁移状态表、旧字段镜像、迁移观察和旧版本回滚专用过渡能力已退休；
- 文档与 ADR 已更新为退出后的永久状态；
- 本 Change 的退出里程碑已经签署，可以同步永久规格并归档。

## 证据索引

### 三小时资格验收

- `evidence/qualification/qualification-20260730T121335Z/RESULT.md`
- `evidence/qualification/qualification-20260730T121335Z/summary.json`

### 退出版本

- `evidence/2026-07-30-rollback-retirement-approval.md`
- `evidence/2026-07-30-final-consistency-scan.md`
- `evidence/2026-07-31-exit-version-validation/README.md`

### 独立删列与删列后验证

- `evidence/2026-07-31-legacy-column-drop/README.md`
- `evidence/2026-07-31-post-drop-validation/README.md`

### 过渡能力清理

- `evidence/2026-07-31-transition-retirement/README.md`

### 最终门禁

- `tdd-red.log`
- `final-validation.log`

## 签署

```text
退出里程碑状态：通过
退出版本/Commit：当前工作区，未提交
DDL 变更单：evidence/2026-07-31-legacy-column-drop/
完成时间：2026-07-31
负责人：Rog
数据库复核人：本地自动化门禁
安全/认证复核人：本地自动化门禁
Change 是否可以关闭：是
```
