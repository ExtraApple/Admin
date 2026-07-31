# 2026-07-29 授权版本 Delta Spec 行为一致性

状态：通过

执行时间：2026-07-29（Asia/Shanghai）

## 验证环境

- 仓库：`D:\work\go\admin`
- Go：Windows 本地 Go 工具链
- MySQL：Windows 本地隔离实例
  `127.0.0.1:23306/admin_rehearsal`
- 凭据仅在执行期间从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared`
  注入，未写入证据文件
- 未连接、停止或重启 `127.0.0.1:3306/MySQL80`

## OpenSpec

OpenSpec 校验：通过

执行：

```text
openspec validate migrate-access-version-storage --strict
```

结果：

```text
Change 'migrate-access-version-storage' is valid
```

## Delta Spec 到行为的映射

### `specs/access-version-storage/spec.md`

- 登录与 Token 签发：通过
  - `TestLoginInitializesMissingAccessVersionAfterCredentialsSucceed`
- Refresh Token：通过
  - `TestLoginRefreshTokenUsesAndValidatesAuthorizationAccessVersion`
- 真实 MySQL 迁移行为：通过
  - `TestMySQLMigrateDatabaseCompletesAccessVersionMigrationAndIsRestartSafe`
  - 验证固定 cutoff、NULL 归一化、正版本保留、completed 状态和重启不扩大存量集合

### `specs/auth/spec.md`

- 登录与 Token 签发：通过
  - `TestLoginInitializesMissingAccessVersionAfterCredentialsSucceed`
- Refresh Token：通过
  - `TestLoginRefreshTokenUsesAndValidatesAuthorizationAccessVersion`
- JWT 校验顺序与缺行拒绝：通过
  - `TestJWTAuthReadsAuthorizationVersionAfterUserStatusCheck`
  - 验证先拒绝禁用用户，再访问 Authorization 授权版本存储

### `specs/user-management/spec.md`

- 用户生命周期与授权失效：通过
  - `TestDeletingUserSoftDeletesAndIncrementsAccessVersionInOneTransaction`
  - `TestRestoringSoftDeletedUserKeepsDeletionVersionAndRejectsPreDeleteTokens`
  - 验证软删除和授权版本提升原子提交、版本记录保留以及恢复后旧 Token 仍失效

## 执行结果

```text
ok  admin/service
ok  admin/router
ok  admin/initialize
```

以上验证覆盖三个 delta spec 的登录、Access Token、Refresh Token、JWT
校验顺序、授权版本缺行拒绝、用户软删除/恢复和真实 MySQL 迁移行为。
