# 2026-07-29 授权版本最终 Go 门禁

状态：通过

## Go 测试结果

- `go test ./... -count=1`：通过
- `go test -race ./... -count=1`：通过
- `go test ./... -count=3`：通过
- 数据竞争：未发现
- 不稳定测试：未发现

Race Detector 使用官方临时 Linux Go 1.25.0、WSL GCC 15.2.0 和
`CGO_ENABLED=1` 执行；Windows Go 因本机没有 Windows GCC，不能直接启用
Race Detector。普通单次和三次重复门禁使用 Windows Go 执行。

## 旧字段引用审计

- 认证运行时旧字段读取：未发现
- 旧字段 fallback：未发现
- 迁移回填读取：允许
- 一致性观察扫描读取：允许
- 观察期原子镜像写：允许

认证路径按用户存在且启用校验后，从 `user_access_versions` 读取当前授权版本。
`users.token_version` 的保留运行时引用仅服务于退出里程碑前的迁移回填、
一致性观察扫描和同事务镜像写，不作为 Access Token 或 Refresh Token 的读取事实来源。

## 环境边界

SQLite 结论未替代真实 MySQL 或真实 Redis。真实 MySQL 最终门禁使用 Windows
本地隔离实例，真实 Redis 组合门禁使用 Docker 隔离实例；两项独立门禁均已有
单独证据记录。

执行时间：2026-07-29 22:48:32 CST (+0800)
