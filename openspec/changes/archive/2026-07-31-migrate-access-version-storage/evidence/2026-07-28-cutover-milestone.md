# 2026-07-28 授权版本存储切换里程碑

状态：完成

执行时间：2026-07-28 21:45:58–21:51:23 +08:00

## 验证环境

- 应用代码与快速门禁：Windows Go 1.25 工具链。
- 数据库强制门禁：Windows 本地隔离真实 MySQL 8.0.19，
  `127.0.0.1:23306`，数据库 `admin_rehearsal`。
- 既有 Windows `MySQL80`（`127.0.0.1:3306`）未用于本次验证、
  未重启、未修改。
- Docker 未运行 MySQL；数据库凭据只从
  `%TEMP%\admin-access-version-rehearsal-20260728` 注入测试进程，
  未输出或写入仓库。

## 唯一读取事实来源

唯一读取事实来源：`user_access_versions`

- Access Token：只读取 `user_access_versions`
  - `middleware.JWTAuth` 调用 `service.IsTokenVersionValid`。
  - `service.currentUserTokenVersion` 在确认用户存在且启用后调用
    `AccessVersionRepository.CurrentVersion`。
  - `CurrentVersion` 只查询 `model.UserAccessVersion`。
- Refresh Token：只读取 `user_access_versions`
  - `service.RefreshTokens` 调用同一个 `currentUserTokenVersion`。
  - Access Token 与 Refresh Token 使用同一个授权版本读取路径。
- 认证缺行：拒绝 Token，不回退 `users.token_version`
  - `CurrentVersion` 缺行返回 `ErrAccessVersionNotFound`。
  - Access Token 与 Refresh Token 校验均把该错误作为认证失败处理。

生产代码静态审计确认，`users.token_version` 的剩余引用限于存量迁移、
观察期镜像写、User Model、Seed/超级管理员旧字段初始化和部署模式名称；
JWT `Claims.TokenVersion` 是 Token Claim，不是旧存储读取。未发现认证或授权
运行时通过 `Select`、`Pluck`、`Where`、`First` 或 `Find` 回读旧字段。

## 镜像写原子性

- 初始化与镜像：同一事务
  - `EnsureVersion` 在单个数据库事务中创建/锁定新表记录，并在提交前把最终版本
    镜像到旧字段。
- 提升与镜像：同一事务
  - `EnsureAndIncrement` 加入调用方事务，在同一事务内锁定并提升新表版本，再写入
    相同的最终镜像值。
- 镜像失败：主写与镜像全部回滚
  - 镜像目标缺失或镜像写失败时返回 `ErrAccessVersionMirrorFailed`，调用方事务不
    提交。
  - 用户状态和软删除操作的失败注入测试确认业务更新也随授权版本事务回滚。

## 门禁结果

SQLite 只读新表与原子回滚门禁：通过

```powershell
go test ./service `
  -run '^(TestAccessVersionRepositoryCurrentVersionReadsOnlyAuthorizationStorage|TestAccessVersionRepositoryEnsureVersionRollsBackWhenMirrorTargetMissing|TestAccessVersionRepositoryEnsureAndIncrementRollsBackWhenMirrorTargetMissing|TestTogglingUserStatusRollsBackWhenAccessVersionPrimaryWriteFails|TestDeletingUserRollsBackWhenAccessVersionMirrorWriteFails|TestDeletingUserSoftDeletesAndIncrementsAccessVersionInOneTransaction)$' `
  -count=1 -v
```

Windows 本地隔离真实 MySQL 原子性门禁：通过

```powershell
$env:ADMIN_TEST_MYSQL_DSN = "<从临时隔离凭据注入>"
go test -tags=mysql_integration ./service -run '^TestMySQL' -count=1 -v
```

通过的真实 MySQL 场景：

- 并发 `EnsureVersion` 只创建一行。
- 并发 `EnsureAndIncrement` 串行提升且镜像最终值一致。
- 行锁在事务提交前阻止竞争提升。
- `EnsureVersion` 与 `EnsureAndIncrement` 竞态不导致版本倒退。
- 用户软删除、授权版本提升和旧字段镜像在同一个 MySQL 事务中提交。

## 切换验收

- 7.5 停机切换演练：通过
- 7.6 授权版本生命周期 E2E：通过
- 迁移前 Access Token 和 Refresh Token 的切换兼容性：通过
- 登录、刷新、禁用、软删除、恢复、Kick 和角色授权变更：通过
- 新旧实例混跑：未发生
- 新旧版本一致性差异：0

## 结论

所有 Token 授权版本读取已经切换到 `user_access_versions`，认证缺行不会回退
旧字段；初始化、提升和旧字段镜像具备同事务提交与失败回滚证据。停机切换演练、
生命周期 E2E、SQLite 快速门禁和 Windows 本地真实 MySQL 强制门禁均通过。

切换里程碑：完成
