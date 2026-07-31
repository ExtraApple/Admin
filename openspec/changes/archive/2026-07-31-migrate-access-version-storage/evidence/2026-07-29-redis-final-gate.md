# 2026-07-29 授权版本真实 Redis 最终门禁

状态：通过

执行时间：2026-07-29 22:34:11 +08:00

Redis：Docker Desktop 隔离容器

- 容器：`admin-av-redis-final-gate-20260729`
- 地址：`127.0.0.1:26379`
- 数据库：`15`
- 容器仅绑定 Windows 回环地址，未使用宿主机其他 Redis 端口。
- 密码配置从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared` 注入测试进程；
  未输出或写入密码。
- MySQL 仍使用 Windows 本地隔离实例，本门禁没有连接、停止或重启 MySQL。

## 执行命令

```powershell
go test -tags=redis_integration ./middleware `
  -run "^TestRealRedisAccessRefreshAndBlacklistCombination$" `
  -count=1 -v
```

## 门禁结果

- 真实 Redis 连接：通过
- Access Token 初始认证：通过
- Refresh Token 初始刷新：通过
- Access Token 黑名单：通过
  - `service.Logout` 写入真实 `blacklist:<access-token>`。
  - JWT 中间件随后拒绝该 Access Token。
  - key 值为 `1`，TTL：正数，且不超过 Access Token 配置有效期。
- Access Token 黑名单不影响 Refresh Token：通过
  - Access Token 被拉黑后，原 Refresh Token 仍可成功刷新。
- Refresh Token 黑名单：通过
  - `service.Logout` 写入真实 `blacklist:<refresh-token>`。
  - `service.RefreshTokens` 随后以稳定
    `ErrRefreshTokenInvalid` 拒绝该 Refresh Token。
  - key 值为 `1`，TTL：正数，且不超过 Refresh Token 配置有效期。

测试：`TestRealRedisAccessRefreshAndBlacklistCombination`

测试使用隔离 SQLite 保存用户及 `user_access_versions`，仅用于提供认证所需的
持久化状态；Redis 黑名单读写、key 和 TTL 均由 Docker 中的真实 Redis 执行。
未使用 Redis Hook、Mock 或内存替身。

测试清理后 Redis DB 15 的 `DBSIZE` 为 `0`。
