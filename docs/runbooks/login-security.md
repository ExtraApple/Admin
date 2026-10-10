# 登录安全策略

> 本文只保留登录链路的安全与运维要点。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/auth/spec.md`](../../openspec/specs/auth/spec.md) 为准。

## 登录链路

```text
验证码校验
→ 登录锁定检查
→ 用户存在性与启用状态检查
→ 密码校验
→ Authorization.EnsureVersion
→ 签发 Access Token 与 Refresh Token
```

任一步失败都不会签发 Token。

## 验证码和登录锁定

| Redis Key | 用途 | 过期策略 |
|---|---|---|
| `captcha:<id>` | 一次性验证码答案 | 5 分钟 |
| `fail:<username>` | 登录失败次数 | 24 小时 |
| `lock:<username>` | 用户登录锁定 | 按失败次数设置 |

- 验证码为 6 位数字，无论校验成功或失败都会被消费。
- 可计入的凭据错误累计少于 5 次时不锁定；5～9 次锁定 1 分钟，10～14 次锁定 5 分钟，15～19 次锁定 15 分钟，20 次及以上锁定 1 小时。
- 第 10 次错误和锁定到期都不会清零 `fail:<username>`；每次记录新的凭据错误都会将失败计数有效期续为 24 小时。失败计数在 24 小时内没有新错误时到期清除。
- 登录成功后清理失败次数和锁定标记。
- 错误响应不暴露用户名是否存在。

## 密码规则

注册和修改密码都要求：

- 长度不少于 6 位。
- 大写字母、小写字母、数字、特殊符号中至少包含 3 类。
- 密码使用 bcrypt 存储，不记录明文或哈希到日志。

## Token 与授权版本

- 新 Token 携带 `token_type=access` 或 `token_type=refresh`。
- Access Token 用于受保护接口，Refresh Token 只用于刷新入口。
- 存量无用途标记 Token 仅按固定的 `legacy_access_expire` 和 `legacy_refresh_expire` 兼容识别。
- 授权版本唯一读取 `user_access_versions`；登录可幂等初始化版本 1，其他认证流程缺行时拒绝 Token。
- 用户状态、密码、角色、权限、菜单和数据范围变化后，旧 Access Token 和 Refresh Token 在下一次使用时失效。
- 登出把当前 Access Token 写入 Redis 黑名单，按 `jwt.expire` 配置的固定时长保留；该 TTL 从登出时起计算，不等于 Token 当时剩余的有效期。

## 验证建议

至少验证验证码一次性消费、密码错误计数、锁定解除、登录成功清理、Access/Refresh 用途隔离、登出黑名单和授权变化后的旧 Token 失效。
