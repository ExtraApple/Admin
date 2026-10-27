# 登录安全策略

> 本文只保留登录链路的安全与运维要点。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/auth/spec.md`](../../openspec/specs/auth/spec.md) 为准。

## 登录链路

```text
验证码校验
→ 登录锁定检查
→ 用户存在性与启用状态检查
→ 密码校验
→ Authorization.EnsureVersion
→ 建立 Redis 登录会话并签发绑定 SID 的 Access／Refresh
→ 提交成功后返回 Access，并设置 HttpOnly Refresh Cookie
```

任一步失败都不会返回成功登录或替换原有效 Cookie 会话；显式登录成功只替换该浏览器原有效 Cookie 绑定的会话，不撤销用户其他独立登录。

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

- 至少 6 个 Unicode 字符。
- UTF-8 编码后不超过 72 字节；该限制在 bcrypt 之前检查，不截断、不换算法。
- 大写字母、小写字母、数字、特殊符号中至少包含 3 类。
- 密码使用 bcrypt 存储，不记录明文或哈希到日志。
- 72 字节限制只约束注册和新密码设置，不收紧登录或旧密码比较；服务端验证优先于前端提示。
## Token、会话与授权版本

- Access 必须携带 `token_type=access` 与 SID；Refresh 必须携带 `token_type=refresh`、SID、jti 和代号。不推断旧 Token 用途，所有旧 Token 均需重新登录。
- Access 仅驻页面内存，业务接口继续使用 Bearer，并实际校验用户状态、`user_access_versions` 和活动 SID。Cookie 不能直接授权业务请求，`session_id` 也不是凭据。
- Identity 持有 Redis `auth:session:<sid>` 活动事实；Authorization 仍唯一持有 MySQL `user_access_versions`。登录可幂等初始化版本 1，其他认证流程缺行拒绝 Token。
- 用户状态、密码、角色、权限、菜单和数据范围变化仍使旧凭据在下一次使用时失效。普通退出只撤销当前 SID，不提升授权版本、不使用 Access 黑名单、不影响其他独立登录。
- Refresh 为最小签名会话绑定，不重复携带角色／权限快照；Access 的授权语义不变。不得通过扩大 Cookie 上限或把 Refresh 转存 JavaScript 来处理浏览器 Cookie 大小限制。

## 浏览器控制协议

| 请求 | JSON body | 成功 data |
|---|---|---|
| `POST /api/login` | `username`、`password`、`captcha_id`、`captcha_code` | `access_token / session_id / user` |
| `POST /api/refresh` | 精确空对象 `{}` | `access_token / session_id` |
| `POST /api/user/logout` | 精确空对象 `{}` | `null` |

三者要求 `Content-Type: application/json`、唯一且精确匹配配置的 `Origin`、`X-Admin-Request: 1`；Refresh 额外要求不可预测的 `X-Refresh-Attempt-ID`（16～128 字符，base64url 或 UUID）。浏览器使用同源请求与 `credentials: 'same-origin'`；Origin 由浏览器发送，不在 JavaScript 中伪造。不增加跨域凭据 CORS。Refresh／Logout 不需要有效 Access，不接受 JSON `refresh_token` 或其他未知字段。响应统一 `Cache-Control: no-store`。

`admin_refresh` 是 Host-only、`Path=/api`、HttpOnly、SameSite=Strict 的会话 Cookie；HTTPS 必须 Secure，不设置 Domain 或长期 Expires／Max-Age，删除时使用同范围属性。唯一 HTTP 例外见[配置手册](configuration.md#浏览器会话与反向代理)。Refresh 不进入 JSON、JavaScript、Web Storage、日志或截图。HttpOnly／SameSite 不消除 XSS 风险；浏览器恢复可能保留会话 Cookie，关闭窗口不等于可靠退出，服务端到期／撤销事实才是依据。

## 个人资料自助与改密退出

- `/profile` 只在页内编辑昵称、头像和密码；昵称保存原始首尾空白与大小写，其他身份资料保持只读。头像上传前先读取已生效配置的 `GET /api/user/avatar-upload-policy`，响应只给正整数 `data.max_bytes` 并使用 `Cache-Control: no-store`；规则读取失败只禁用选图与上传，不回退到默认上限，恢复默认仍可用。
- 密码 `PUT /api/user/password` 返回明确 200 后，前端先清除三项敏感输入，再复用现有 Cookie Logout。密码已改与退出结果分开处理；Logout 的 500 或网络中断保留用于重试的 Cookie 和 `admin-workbench.auth.logout-pending`，显示“服务端退出未确认”，不伪报成功、不重交密码、不自动试登录。
- 恢复网络后只重试 `POST /api/user/logout`；用户也可通过既有“重新登录”显式完成 Login 与真实 context 读取。只有 Logout 成功或显式登录完整成功后才清除待确认标记，退出流程不新增二次确认。

### 安全的本地命令示例

优先用工作台完成验证码与真实登录。需要 curl 诊断时，以下 Bash 示例仅展示协议，不表示已执行；在受控终端禁用 shell tracing，不使用 `-v`、`--trace`、`-i`，不要打印响应、Cookie jar 或密码。`login.json` 由受保护的临时文件提供上述四个字段，使用真实一次性验证码，不能提交版本库；不要把密码直接写在命令行。HTTPS 使用受信任证书，不使用 `-k`。

```bash
umask 077
work="$(mktemp -d)"
base='https://admin.example.com'
origin='https://admin.example.com'
# 将已安全准备的 login.json 放在 "$work/login.json"；不要 echo 其内容。
curl --silent --show-error --output /dev/null --write-out '%{http_code}\n' \
  --cookie "$work/cookies" --cookie-jar "$work/cookies" \
  -H "Origin: $origin" -H 'X-Admin-Request: 1' -H 'Content-Type: application/json' \
  --data-binary "@$work/login.json" "$base/api/login"
attempt="$(openssl rand -hex 16)"
curl --silent --show-error --output /dev/null --write-out '%{http_code}\n' \
  --cookie "$work/cookies" --cookie-jar "$work/cookies" \
  -H "Origin: $origin" -H 'X-Admin-Request: 1' -H 'Content-Type: application/json' \
  -H "X-Refresh-Attempt-ID: $attempt" --data '{}' "$base/api/refresh"
curl --silent --show-error --output /dev/null --write-out '%{http_code}\n' \
  --cookie "$work/cookies" --cookie-jar "$work/cookies" \
  -H "Origin: $origin" -H 'X-Admin-Request: 1' -H 'Content-Type: application/json' \
  --data '{}' "$base/api/user/logout"
# 仅在人工确认最后一次 Logout 为 200 后，执行 rm -rf -- "$work" 清理本次专用目录。
# 非 200／网络失败保留受保护 jar，通过相同 Logout 协议重试，不宣称退出成功。
```

jar 含 Refresh 凭据，即使是 HttpOnly 也必须保护和清理。上例故意丢弃含 Access 的响应，不用于业务调用。非 200 不等于退出成功，不要在失败后直接执行最后的清理步骤并声称已撤销。未知 Refresh 结果应在首次尝试起 30 秒内复用原 attempt 和 jar；不得并发运行这些控制命令或立即换 ID 重试。`auth:refresh-attempt:<sid>:<attempt>` 保存固定结果 30 秒，重放不续 TTL；超过窗口不要重放旧 Cookie，转人工恢复／显式登录。

## 状态与故障处置

| 状态 | 处置 |
|---|---|
| `400 / HTTP_REQUEST_INVALID` | 修正 JSON／Header；旧 JSON Refresh 协议不兼容，不自动重交密码或验证码 |
| Refresh `401 / AUTHN_REFRESH_TOKEN_INVALID` | 当前凭据无效，响应删除 Cookie；异常重用只撤销该 SID，重新登录 |
| `403 / AUTHN_CSRF_INVALID` | 检查精确外部 Origin、JSON 和 Header、防代理改写；不清有效 Cookie，不当作退出成功 |
| `409 / AUTHN_REFRESH_SUPERSEDED` | 原幂等结果已被更新代取代，无 Set-Cookie；不能用旧结果覆盖新 Cookie，不当成 401 清会话 |
| 控制请求 `500`／网络中断 | 结果不能假定成功；真实 Redis 故障不是凭据无效，不清用于重试的 Cookie |

启动先 Cookie Refresh，再读取真实 `GET /api/user/context`，两者成功才显示工作台。依赖安全上下文、原生 Web Locks、可用 localStorage；能力失败显示不可用，不降级为仅内存认证或声称已服务端登出。原生 `admin-workbench.auth` 锁覆盖控制请求及响应体／Cookie 完成；`.state` 短锁只协调状态，不等待网络。

Access 尚有效时续期失败保留编辑；到期后挂起而不卸载编辑器，不自动重放业务请求。只有同 SID 且真实上下文一致的手动恢复可继续原编辑，新身份必须清除旧编辑。业务 403 保留页面显示拒绝，不把所有 403 都解释为 CSRF。

用户通过原有未保存修改保护后，退出先持久化 `admin-workbench.auth.logout-pending` 再等待网络锁；不新增退出确认。网络／500 时显示“服务端退出未确认”和重试入口，标记跨刷新／标签保留，阻止自动恢复。恢复网络后使用真实 Cookie Logout 重试；只有确认退出或新的显式 Login **及真实 context 成功**才清标记，不能通过删除 localStorage／普通恢复按钮绕过退出意图。

三项协调 key 为 `admin-workbench.auth.event`、`.logout-pending`、`.refresh-attempt`，只保存 epoch、事件、随机操作 ID、时间和非授权 SID，不含凭据、用户资料或权限。不要把主题 key、浏览器事件或存储内容当成授权事实。

## 验证建议

在隔离环境验证验证码一次性消费、密码错误计数、锁定解除、成功清理、用途／SID 隔离、Cookie 范围、真实上下文恢复、Redis 轮换并发／固定幂等窗口／重用撤销、跨标签退出待确认及授权变化后旧凭据失效。命令门禁见 [testutil](../../testsupport/testutil/README.md)；自动测试不替代真实浏览器故障与多标签验收，本文不声明未执行场景已通过。
