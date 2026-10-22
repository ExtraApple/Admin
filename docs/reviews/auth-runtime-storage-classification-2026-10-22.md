# 授权运行时存储错误分类复核

## 结论

当前基线 `362ea14fc7ec21170109c57a272d703b1e0237df` 在新的独占环境中未复现先前记录的 `401 AUTHN_TOKEN_INVALID`。因此本次不修改认证生产代码；结论为**历史现象不可复现，保留证据缺口**。

## 真实结果

| 故障注入 | 故障期间 | 恢复后 |
|---|---|---|
| `user_access_versions` 表不可用 | HTTP 500、`IDENTITY_INTERNAL_ERROR`、`data=null` | `/api/user/info` HTTP 200 |
| 完整 MySQL 不可用 | HTTP 500、`IDENTITY_INTERNAL_ERROR`、`data=null` | `/api/user/info` HTTP 200 |
| 请求开始后立即停止 MySQL，20 次 | 20 次传输失败，无 HTTP 401 | MySQL 恢复后正常 |

两类 HTTP 故障请求均未签发 Token；表级故障期间未创建授权版本行。

## 代码与回归保护

源码追踪确认：

- `users.FindByID` 仅将明确 `gorm.ErrRecordNotFound` 映射为 `ErrUserNotFound`；
- `AccessVersions.Current` 仅将明确记录不存在映射为 `ErrAccessVersionNotFound`；
- 其他错误保留为内部错误；
- `Authenticate` 和认证中间件按错误身份区分认证拒绝、账号禁用和内部错误。

新增最小回归保护：

- `TestAuthenticateRejectsDisabledAndStaleAccess`
- `TestWriteIdentityErrorEmitsSafeInternalError`

现有身份、GORM 和 HTTP 测试均通过。

## 历史观察边界

先前审计记录的完整 MySQL 故障 `401 AUTHN_TOKEN_INVALID` 仍保留为历史观察，但当前无法复现，不能写成当前基线产品缺陷，也不能声称已通过修复消除。若未来出现新的可复现材料，应以新的代码基线和隔离环境重新定位。

机器证据：

- [`auth-runtime-storage-classification-2026-10-22.json`](evidence/auth-runtime-storage-classification-2026-10-22.json)

证据不包含 JWT、密码、验证码答案、Authorization header、DSN 或原始数据库错误。
