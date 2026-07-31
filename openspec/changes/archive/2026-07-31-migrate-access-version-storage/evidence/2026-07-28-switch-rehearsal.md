# 2026-07-28 授权版本存储停机切换演练

状态：通过

环境：Docker Desktop（仅 Redis/MinIO）+ Windows 本地隔离真实 MySQL 8.0.19

执行时间：2026-07-28 21:15:40–21:26:18 +08:00

## 隔离范围

- 既有 Windows `MySQL80`（`127.0.0.1:3306`）未用于演练、未重启、未修改。
- 演练数据库为 Windows 本地隔离 MySQL，地址 `127.0.0.1:23306`，数据库 `admin_rehearsal`。
- Docker Desktop 仅运行演练所需的 Redis 和 MinIO，不运行 MySQL。
- 旧实例监听 `28081`，新实例监听 `28082`。
- 密码、JWT 密钥及 Token 仅保存在 `%TEMP%\admin-access-version-rehearsal-20260728`，未写入仓库证据。

## 制品

- 旧制品 SHA-256：`46e83629ff120a1dd488a5f8c0c8622783294467d505367f13bb4f54731984b3`
- 制品 SHA-256：`89452a357141aa779515520c6732d3f4c502b6abaece3bf04bb04271569c893d`
- 新制品同时包含迁移、`EnsureVersion`、`EnsureAndIncrement`、只读新表和观察期镜像写。

## 停机切换时间线

1. `2026-07-28 21:15:40 +08:00`：旧实例正常响应，新实例未提供流量。  
   旧实例承载流量：是
2. `2026-07-28 21:16:18 +08:00`：旧实例已经停止，新实例尚未启动。  
   旧实例承载流量：否；新实例承载流量：否
3. `2026-07-28 21:16:33 +08:00`：新实例完成数据库初始化、Seed 检查后开始监听。
4. `2026-07-28 21:22:36 +08:00`：旧实例端口关闭，新实例健康检查返回 HTTP 200。  
   旧实例承载流量：否；新实例承载流量：是

混跑检查：0 个时间点

## 迁移与一致性

- 迁移状态：completed
- `cutoff_user_id`：`1`
- cutoff 范围缺行：`0`
- 一致性差异：0
- 版本倒退：`0`
- `completed_at`：已记录

上述结果直接查询 Windows 本地隔离真实 MySQL `127.0.0.1:23306` 获得。

## Token 兼容性

- 迁移前 Access Token：切换后有效
  - 切换前在旧实例调用 `/api/user/info` 返回 HTTP 200。
  - 切换后使用同一 Token 在新实例调用 `/api/user/info` 返回 HTTP 200。
- 迁移前 Refresh Token：切换后刷新成功
  - 切换后调用 `POST /api/refresh` 返回 HTTP 200、业务 `code=200`。
  - 响应包含新的 Access Token 和 Refresh Token。

## 强制门禁

在 Windows 本地隔离真实 MySQL 上执行：

```powershell
go test -tags=mysql_integration ./... -run '^TestMySQL' -count=1
```

结果：通过。

## 结论

本次演练按“旧实例提供流量 → 全部实例停止 → 仅新实例提供流量”的顺序完成；没有观察到新旧实例混跑。迁移状态、cutoff 回填、一致性、迁移前 Token 兼容性和真实 MySQL 强制门禁均通过。
