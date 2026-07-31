# 2026-07-28 授权版本生命周期端到端测试

状态：通过

环境：Docker Desktop（真实 Redis）+ Windows 本地隔离真实 MySQL 8.0.19

执行时间：2026-07-28 21:39:45–21:39:46 +08:00

## 测试边界

- 应用：停机切换后的新制品，地址 `http://127.0.0.1:28082`。
- MySQL：Windows 本地隔离实例 `127.0.0.1:23306`，数据库 `admin_rehearsal`。
- Redis：Docker Desktop 隔离容器 `admin-av-redis-20260728`。
- 既有 Windows `MySQL80`（`127.0.0.1:3306`）未用于测试、未重启、未修改。
- E2E 用户、密码、验证码、Access Token 和 Refresh Token 均只存在于测试进程或 `%TEMP%`，未写入仓库。
- 测试结束后已物理清理临时用户、角色及其关联；残留临时用户数和角色数均为 `0`。

## 执行方式

使用可重复的 PowerShell E2E 脚本：

```powershell
.\initialize\testutil\run-access-version-lifecycle-e2e.ps1
```

脚本通过公开 HTTP 接口执行注册、登录、刷新、管理员状态切换、软删除、Kick 和角色分配。项目当前没有用户恢复 HTTP 路由，因此恢复步骤只在隔离演练库中执行受控的 `deleted_at = NULL` 维护操作，再通过公开登录和认证接口验证恢复语义。

## 结果

- 注册：通过；注册后、首次登录前的 `user_access_versions` 行数为 `0`。
- 登录：通过
  - 首次登录幂等创建授权版本 `1`。
  - `/api/user/info` 返回 HTTP 200。
- 刷新：通过
  - `POST /api/refresh` 返回 HTTP 200 和新的 Access Token、Refresh Token。
- 禁用：旧 Access Token 和 Refresh Token 均被拒绝
  - 用户状态切换为禁用。
  - 授权版本从 `1` 提升为 `2`。
- 重新启用：通过
  - 用户状态恢复启用。
  - 授权版本从 `2` 提升为 `3`。
  - 用户可重新登录。
- 软删除：旧 Access Token 和 Refresh Token 均被拒绝，授权版本记录保留
  - 授权版本从 `3` 提升为 `4`。
  - 用户软删除后 `user_access_versions` 记录仍存在。
- 恢复：删除前 Token 仍被拒绝，重新登录成功
  - 恢复过程未重置授权版本，版本保持 `4`。
  - 删除前签发的 Access Token 和 Refresh Token 在恢复后仍返回 HTTP 401。
- Kick：旧 Access Token 和 Refresh Token 均被拒绝
  - 授权版本从 `4` 提升为 `5`。
- 授权变更：角色分配后旧 Access Token 和 Refresh Token 均被拒绝
  - 通过管理员角色分配接口改变目标用户授权。
  - 授权版本从 `5` 提升为 `6`。

## 版本与清理证明

| 检查点 | `user_access_versions.version` | `users.token_version` | 结果 |
| --- | ---: | ---: | --- |
| 首次登录 | 1 | 1 | 一致 |
| 禁用 | 2 | 2 | 一致 |
| 重新启用 | 3 | 3 | 一致 |
| 软删除 | 4 | 4 | 一致且版本记录保留 |
| 恢复 | 4 | 4 | 一致且版本未重置 |
| Kick | 5 | 5 | 一致 |
| 角色分配 | 6 | 6 | 一致 |

新旧版本一致性差异：0

清理后再次查询：

- `av_e2e_` 临时用户：`0`
- `av_e2e_role_` 临时角色：`0`
- 全库已有新表记录的新旧版本差异：`0`

## 结论

登录、Refresh Token、账号禁用、软删除、恢复、Kick 和角色授权变更均符合授权版本迁移规格。所有失效操作都使旧 Access Token 和 Refresh Token 在下一次使用时被拒绝，恢复不会使删除前 Token 重新生效，并且观察期镜像值始终与新表主值一致。
