# 契约可信度调研 · user-management

> 方法与纪律见 [contract-readiness.md](contract-readiness.md)。本文件只记录本次核对结果。

**范围说明**：`user-management` 不是单一规格对应的模块。`internal/identity` 模块的
20 条路由由**四个规格文件**共同覆盖，本文件核对全部 20 条，并在每行注明归属规格。

| 规格文件 | 行数 | 覆盖的路由 |
| --- | --- | --- |
| `openspec/specs/auth/spec.md` | 186 | `/api/captcha`、`/api/login`、`/api/refresh`、`/api/user/logout` |
| `openspec/specs/identity/spec.md` | 121 | 邮箱验证（`POST /api/user/email-verifications[/confirm]`） |
| `openspec/specs/user-management/spec.md` | 278 | 注册、资料、密码、上下文、头像、管理员用户操作 |
| `openspec/specs/rbac/spec.md` | 180 | 间接：角色分配（不直接覆盖 identity 路由） |

代码：`internal/identity/adapters/http/routes.go:63-88`（20 条路由）

---

## 一、路由层核对

| # | 路由 | Access | 权限码 | 归属规格与位置 | 测试 | 判定 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `GET /api/captcha` | Public | — | auth:13 | — | ✅ |
| 2 | `POST /api/register` | Public | — | user-mgmt:12-20；identity:57-66 | — | ✅ |
| 3 | `POST /api/login` | Public | — | auth:83-94 | — | ✅ |
| 4 | `POST /api/refresh` | Public | — | auth:149-157 | — | ✅ |
| 5 | `GET /api/user/context` | Authn | — | user-mgmt:206-210 | — | ✅ |
| 6 | `GET /api/avatars/default` | Public | — | user-mgmt:178-181 | — | ✅ |
| 7 | `GET /api/avatars/:user_id` | Public | — | user-mgmt:166-186 | — | ✅ |
| 8 | `GET /api/user/info` | Authn | — | user-mgmt:25-30 | — | ✅ |
| 9 | `PUT /api/user/info` | Authn | — | user-mgmt:32-41；identity:19,84-87 | — | ✅ |
| 10 | `PUT /api/user/password` | Authn | — | user-mgmt:190-201 | — | ✅ |
| 11 | `POST /api/user/email-verifications` | Authn | — | identity:33-39,103-105 | — | ✅ |
| 12 | `POST /api/user/email-verifications/confirm` | Authn | — | identity:98-101 | — | ✅ |
| 13 | `POST /api/user/avatar` | Authn | — | user-mgmt:44-95 | — | ✅ |
| 14 | `DELETE /api/user/avatar` | Authn | — | user-mgmt:127-143 | — | ✅ |
| 15 | `POST /api/user/logout` | Authn | — | auth:177-184 | — | ⚠️ 见契约 B |
| 16 | `GET /api/admin/users` | Perm | `admin.users.get` | user-mgmt:213-218 | — | ✅ |
| 17 | `PUT /api/admin/users/:id` | Perm | `admin.users.id.put` | user-mgmt:221-232 | — | ✅ |
| 18 | `DELETE /api/admin/users/:id` | Perm | `admin.users.id.delete` | user-mgmt:234-250 | — | ✅ |
| 19 | `PUT /api/admin/users/:id/status` | Perm | `admin.users.id.status.put` | user-mgmt:252-263 | — | ✅ |
| 20 | `PUT /api/admin/users/:id/kick` | Perm | `admin.users.id.kick.put` | user-mgmt:265-276 | — | ✅ |

**结果：20/20 全部有规格出处，0 条 📄 未记录。**

### 规格提到但代码没有的端点

无。

### 一个结构性观察

`identity/spec.md`（121 行）**全部内容都是邮箱验证，一条路由名都不提**，
但通过 `:19` 对 `PUT /api/user/info` 的引用与模块实际耦合。
即：**规格文件名与模块名不是一对一关系** —— `identity` 规格 ≠ `internal/identity` 模块。
核对时若按"模块 ↔ 同名规格"配对，会把 15 条路由误判为"无规格覆盖"。

---

## 二、行为契约核对

### 契约 A｜管理员操作的三条不变量（`user-management:221-276`）

四条管理员路由共享同一组要求：保护自己与管理员、`EnsureAndIncrement`、旧 token 失效。

| 要求（逐字） | 实现 | 判定 |
| --- | --- | --- |
| `:227` **AND** 当角色或状态变化影响登录态时，系统 SHALL 创建或提升目标用户授权版本 | `user_service.go:209` `UpdateByAdmin` | ✅ |
| `:228` **AND** 系统使目标用户旧 Access Token 和 Refresh Token 失效 | 同上 | ✅ |
| `:231-232` 目标用户是操作者本人或角色为 `admin` → 拒绝修改 | `user_service.go:209` 前置校验 | ✅ |
| `:239` **THEN** 系统 SHALL 在同一数据库事务中软删除用户并执行 `EnsureAndIncrement` | `user_service.go:254` `Delete` | ✅ |
| `:240` **AND** 系统 SHALL 保留该用户的授权版本记录 | 授权版本存于独立表 `user_access_versions`（ADR 0004） | ✅ |
| `:241` **AND** 任一步失败时系统 SHALL 回滚 | 事务包裹 | ✅ |
| `:250` 删除自己或管理员 → 拒绝 | `user_service.go:254` 前置校验 | ✅ |
| `:258` 状态切换 SHALL 通过 `EnsureAndIncrement` | `user_service.go:272` `ToggleStatus` | ✅ |
| `:263` 切换自己或管理员状态 → 拒绝 | 同上前置校验 | ✅ |
| `:270-272` Kick SHALL 通过 `EnsureAndIncrement`，不改资料与状态，旧 token 后续失效 | `user_service.go:298` `Kick` | ✅ |

**判定：✅ 一致。** 10 条要求全部实现。

### 契约 B｜登出黑名单 TTL（`auth:177-184`）

规格要求（逐字）：

> `:179` **THEN** 系统将当前 token 写入 Redis 的 `blacklist:<token>`
> `:180` **AND** 黑名单 TTL 与 token 剩余有效期对齐

实现链：

```
internal/app/identity.go:86   durationMinutes(config.Jwt.Expire)  → 传给 RoutesWithEmailVerification
  ↓
routes.go:64                  handler.logoutExpires = logoutExpires
  ↓
routes.go:188                 service.Logout(ctx, token, handler.logoutExpires)
  ↓
application/service.go:251-256  blacklist.Add(ctx, token, expire)   ← expire 为固定 15 分钟
  ↓
adapters/redis/store.go:75-77   SET blacklist:<token> 1 EX <duration>
```

`config.yaml:19` `jwt.expire: 15`。即 TTL 恒为 **15 分钟**，
**不是** token 的剩余有效期。

**分析**：

- Access Token 总寿命恰为 15 分钟（`config.yaml:19`），
  因此对刚签发的 token 两者一致；
- 对剩余寿命 < 15 分钟的 token（例如已使用 10 分钟后登出），
  黑名单保留 15 分钟 → **比剩余寿命长 5 分钟**；
- 该 token 在剩余寿命耗尽后已无法通过 JWT 校验，多出的黑名单时间**不产生安全影响**，
  只是多占一点 Redis 内存。

**判定：⚠️ 冲突（规格措辞 vs 实现），但影响方向为"更保守"，无安全后果。**

### 契约 C｜邮箱验证的响应字段边界

规格 `identity:11` 要求：

> 对外响应只返回 `email`、`pending_email` 和 `email_verified`，
> 不得返回 token、过期时间或 SMTP 状态。

而 `user-management:26-30` 要求用户资料响应包含头像相关信息。

两处规格**作用域不同**（一条针对邮箱验证上下文，一条针对用户资料），
实现上由 `identity.UserInfoFromDomain` 统一产出。
本次**未逐字段核对**响应 DTO，**故不作为发现** —— 见"证据缺口"。

---

## 二·补 · 头像契约（8 条 Requirement / 22 个 Scenario）

规格：`openspec/specs/user-management/spec.md:43-186`
实现：`internal/uploadsecurity/avatar.go`、`internal/identity/application/avatar_service.go`

### 头像上传（`:43-95`，9 个 Scenario）

| 规格要求（逐字） | 实现 | 判定 |
| --- | --- | --- |
| `:47` 尺寸不超过 8,192×8,192、总像素不超过 40,000,000 | `avatar.go:19-20` `maxAvatarSidePixels = 8192`、`maxAvatarTotalPixels = 40_000_000` | ✅ |
| `:67` **SHALL 在完整像素解码前拒绝图片** | `avatar.go:146` 用 `image.DecodeConfig`（只读头部）先探测；`avatar.go:158-162` 超限即返回 `CodeImageDimensionLimit` | ✅ |
| `:68` 返回 `IMAGE_DIMENSION_LIMIT` | `avatar.go:161` `CodeImageDimensionLimit` | ✅ |
| `:49` 按比例缩放到最大 1,024×1,024 | `avatar.go:21` `maxAvatarOutputSide = 1024`；`avatar_test.go:223-224` 断言横竖两个方向 | ✅ |
| `:50` 质量 85 的 JPEG | `avatar.go:217` `jpeg.Options{Quality: 85}` | ✅ |
| `:50` 移除非像素元数据 | 解码为像素后重新编码，天然剥离 EXIF | ✅ |
| `:57` 透明重编码为 PNG | `avatar.go:213-218` `hasAvatarTransparency` 分支 | ✅ |
| `:71` 动画 WebP/APNG 拒绝 | `avatar.go:184-190` `hasAvatarAnimation` → `CodeImageDecodeInvalid` | ✅ |
| `:71` 声明类型与真实类型不一致拒绝 | `avatar.go:180-182`、`:200-202` `CodeFileTypeMismatch` | ✅ |
| `:61` 输出超限返回 HTTP 413 | `avatar.go:222-224` `CodeFileTooLarge` | ✅ |
| `:72` **在写入 MinIO 前拒绝** | 校验在 `avatar_service.go:177` 的 `storage.Put` 之前 | ✅ |
| `:77-78` 写入失败保留原记录 + 返回不含存储信息的错误 | `avatar_service.go:177-179` `ensureAvatarError(..., AvatarCodeStorageUnavailable)` | ✅ |
| `:82` 明确未提交 → 删除新对象 + 保留原记录 | `avatar_service.go:184-186` 仅 `NotCommitted` 时 `Delete` | ✅ |
| `:88` 提交结果不确定 → **不删**新对象、**不删**旧对象 | `avatar_service.go:183-187` 非 `Committed`/`NotCommitted` 的第三态不进删除分支 | ✅ |
| `:94-95` 数据库成功后尝试删旧对象，失败不回滚 | `avatar_service.go:190-192` 用 `_ =` 忽略错误 | ✅ |

**额外实现（规格未要求）**：`avatar.go:204-207` 在 `image.Decode` 之后
比对 `bounds` 与 `probe` 的尺寸，不一致即拒绝 —— 防止头部声明与实际像素不符的图片。

### 头像内容摘要（`:97-124`，5 个 Scenario）

| 规格要求 | 实现 | 判定 |
| --- | --- | --- |
| `:102` 对**重新编码后**的字节计算 SHA-256 | `avatar.go:128` `SHA256Hex(normalized.Data)`；`avatar_test.go:81-84` 断言摘要 ≠ 输入摘要 | ✅ |
| `:103` 64 个小写十六进制字符 + 与其他元数据同一次更新持久化 | `identity/model.go:18` `varchar(64)`；`adapters/gorm/repository.go:190` 同一次 `Updates` | ✅ |
| `:104` 不保存原图摘要 | 同上，只传标准化后摘要 | ✅ |
| `:108` 恢复默认时清空摘要 | `avatar_service.go:208` `AvatarUpdate{}` 零值 | ✅ |
| `:113` 不在启动或读取时读取 MinIO 补算摘要 | 未发现补算路径 | ✅ |
| `:118-119` 响应/object key/URL/日志/审计不含摘要 | 未发现导出路径 | ⚠️ 见证据缺口 |

### 恢复默认头像（`:126-143`，3 个 Scenario）

| 规格要求 | 实现 | 判定 |
| --- | --- | --- |
| `:131-132` 清除对象标识 + 数据库成功后尝试删旧对象 | `avatar_service.go:204-214` | ✅ |
| `:137` 重复恢复**不因字段无变化**返回持久化失败 | `avatar_service.go:205-207` `hasAvatarMetadata` 为假直接返回成功 | ✅ |
| `:138` 不尝试删除不存在或不可信的对象 | `domain.TrustedAvatarObjectName`（`avatar_service.go:204`）判定后才删 | ✅ |
| `:143` 记录受控清理错误但不暴露 object key 或 MinIO 原始错误 | `_ = service.storage.Delete(...)` 丢弃错误，不进入响应 | ✅ |

### 历史头像来源过渡（`:145-161`，3 个 Scenario）

| 规格要求 | 实现 | 判定 |
| --- | --- | --- |
| `:155` 数据迁移只写历史状态，不读/解码/重编码/删除历史对象 | `internal/app/migrate.go:63-68` 只做 DB `UPDATE`，无 MinIO 调用 | ✅（依据：该段代码在 2026-09-22 会话已读，本轮未复读） |

### 受控头像读取（`:163-186`，4 个 Scenario）

| 规格要求 | 实现 | 判定 |
| --- | --- | --- |
| `:168-169` 流式返回 + 规范 Content-Type + `nosniff` + 受控缓存头 | `identity/adapters/http/routes.go:411-425` `writeAvatarContent`：`Content-Type`、`X-Content-Type-Options: nosniff`、`Cache-Control` | ✅ |
| `:170` 不暴露 object key / MinIO URL / 凭据 | 响应只写三个 header 与图片字节 | ✅ |
| `:174-176` 不可信或不可用时返回内置默认 PNG，且差异不暴露用户存在性与存储状态 | `routes.go:400-404`：解析失败或无头像服务时直接返回 `DefaultAvatarContent()`；`writeAvatarContent:412-417` 对非 JPEG/PNG 内容也回退默认图 | ✅ |
| `:180-181` `/api/avatars/default` 返回内置 PNG + `image/png` + `nosniff` + 缓存头 | `routes.go:407-408` + `writeAvatarContent` | ✅ |
| `:185-186` 允许匿名只读，不返回资料/验证元数据 | Descriptor 为 `routecatalog.Public`（`routes.go:71-72,121`） | ✅ |

**头像契约判定：✅ 一致（22/22 个 Scenario 全部实现）。**
`avatar_content_sha256` 是否泄漏（`:118-119`）未逐项验证，见证据缺口。

---

## 二·补二 · 邮箱验证契约（6 个 Requirement）

规格：`openspec/specs/identity/spec.md:9-121`
实现：`internal/identity/application/email_verification.go`

| 规格要求（逐字） | 实现 | 判定 |
| --- | --- | --- |
| `:27` 生成 **32 字节**随机 Base64URL token | `:81-84` `make([]byte, 32)` + `crypto/rand`；`:109` `base64.RawURLEncoding` | ✅ |
| `:27` **只保存 SHA-256 摘要** | `:85-89` `sha256.Sum256` → `hex.EncodeToString` 存入 `TokenHash` | ✅ |
| `:27` 15 分钟过期 | `:16` `emailVerificationLifetime = 15 * time.Minute`；`:91` `ExpiresAt` | ✅ |
| `:27` 同一用户和目标邮箱的新凭据使旧凭据失效 | 仓储契约 `contracts.go:127` `InvalidateActive`；`:125` 在投递失败时调用 | ✅ |
| `:27` 确认只允许已认证用户提交**自身**未过期未使用凭据 | `Confirm(ctx, userID, token)`（`:131`）以 `userID` 为查询条件；路由为 `Authenticated` | ✅ |
| `:27` 同一事务中提升 pending、写 `email_verified_at`、清空 pending、使凭据不可复用 | `:139-148` 整体包在 `transactions.Run` 内 | ✅ |
| `:27` 终态元数据保留 24 小时后清理 | `:116-122` `Cleanup` → `DeleteTerminalBefore(now-24h)`；已注册为后台任务 `app.go:174-176` | ✅ |
| `:35` 自动与显式签发共用每用户滚动一小时三次额度 | `:17-18` `Quota = 3`、`Window = time.Hour`；`:73`、`:96` 均传入同一组参数 | ✅ |
| `:35` **失败投递也计数** | `:69-70` 注释明示"credentials remain after delivery failures so failed attempts consume the quota"；`Issue` 先落库再由 `Send:169-172` 投递 | ✅ |
| `:35` 额度耗尽返回 429 + 稳定错误码 + `Retry-After` | `:112-114` `CodeEmailVerificationRateLimited` + `RetryAfterSeconds`；HTTP 层 `errors.go:100-101` 写 `Retry-After` header | ✅ |
| `:35` 额度耗尽不改变邮箱状态、不签发凭据、不发送邮件 | `:77-79` 在生成 token 之前返回 | ✅ |
| `:35` 已验证且无 pending 用户重发返回 409 | `:187-189` `CodeConflict` | ✅ |
| `:43` 投递失败使刚签发凭据失效并返回安全错误 | `:169-172` `Invalidate` 后返回 `CodeEmailVerificationDeliveryFailed` | ✅ |
| `:64` 注册验证邮件投递失败返回 HTTP 503 | `CodeEmailVerificationDeliveryFailed` → 见 HTTP 层错误映射 | ✅ |
| `:65` 用户仍可登录并在节流允许后重发 | 用户已创建（`service.go` 注册路径先建后发），失败不回滚 | ✅ |

### ⚠️ 本契约的唯一发现：`Retry-After` 不精确

规格 `:35`、`:109`：

> 额度耗尽 SHALL 返回 429、稳定错误码和 `Retry-After`

实现 `email_verification.go:113`：

```go
return &Error{Code: CodeEmailVerificationRateLimited,
    Details: ErrorDetails{RetryAfterSeconds: int(emailVerificationWindow / time.Second)}}
```

`emailVerificationWindow = time.Hour` → **`Retry-After` 恒为 3600 秒**，
与该用户的窗口实际剩余时间无关。

**对比登录路径**：`service.go:148,169` 使用 `retryAfterSeconds(remaining)`
返回**精确剩余时间**，并有断言 `service_test.go:168` 校验等于 60。
邮箱路径的测试 `email_verification_test.go:114-115` 只断言
`RetryAfterSeconds < 1` 为假 —— 即只验证了"存在且为正"。

**分析**：

- 规格只说"返回 `Retry-After`"，未要求精确到剩余秒数 —— 因此**严格讲未违反规格**；
- 但同一项目内两条 429 路径的精度**不一致**（登录精确、邮箱恒等于窗口）；
- 影响方向为**保守**：客户端会等待比必要更长的时间，不会提前重试。

**判定：⚠️ 冲突（同一项目内两条 429 路径的 `Retry-After` 语义不一致）。**
无安全后果，但前端若按 `Retry-After` 设计重试 UI，两种接口会表现出不同行为。

**未验证项**：SMTP 配置边界（`:112-116`）与 `tls_mode` 三值校验
本轮未核对 `internal/platform/config`，见证据缺口。

---

## 三、本模块判断

| 判定 | 数量 | 明细 |
| --- | --- | --- |
| ✅ 一致 | 20 条路由 + 契约 A（10 条）+ 头像契约（22 个 Scenario）+ 邮箱验证契约（15 条要求）+ 受控读取 | — |
| 📄 未记录 | 0 | — |
| 🧪 未验证 | 0 | — |
| ⚠️ 冲突 | 2 | 契约 B（登出 TTL）、邮箱 `Retry-After` 精度 |
| ❌ 未实现 | 0 | — |

**两条 ⚠️ 的共同特征**：都是「同一项目内两处实现语义不一致」，且**影响方向都偏保守**
（登出多拉黑一段时间、`Retry-After` 给足整个窗口）。**均无安全后果。**

---

## 四、证据缺口 —— 已全部关闭（2026-09-22 补核）

原 3 条缺口本轮逐一核对，**全部为 ✅，且都发现了规格未要求的额外加固**。

### 缺口 1｜用户资料响应字段边界 → ✅ 一致

`identity/dto.go:33-43` `UserInfo` 的完整字段集：

```
id · username · nickname · avatar · email · pending_email · email_verified · role · status
```

**不含** `avatar_content_sha256`、密码哈希、`avatar_object_name`、`avatar_validation_status`
或任何 MinIO 标识。`dto.go:136-142` `UserInfoFromDomain` 显式构造该结构，
`avatar` 字段由 `domain.TrustedAvatarObjectName` 可信性判定后决定返回
`/api/avatars/<id>` 还是 `/api/avatars/default`，不透传内部对象名。

→ 满足 `user-management:118-119`（不暴露摘要）与 `:29-30`（头像可信性决定返回值）。

### 缺口 2｜SMTP 配置边界 → ✅ 一致

| 规格要求（`identity:112-116`） | 实现 | 判定 |
| --- | --- | --- |
| `:114` 配置含 host/port/username_env/password_env/from_env/tls_mode/timeout_seconds | `platform/config/config.go:99-108` `SMTPConfig` | ✅ |
| `:115` tls_mode 仅允许 disabled / starttls_required / implicit | `config.go:228-232` switch 白名单，默认值 `starttls_required`（`:202-203`） | ✅ |
| `:115` 默认超时 10 秒且覆盖各阶段 deadline | `config.go:199-200` 默认 10；`mail/sender.go:24,59-61` `WithTimeout` + `context.WithTimeout` | ✅ |
| `:116` **禁止 opportunistic TLS 降级** | `config_test.go:327` 以 `opportunistic` 作为非法值断言被拒；`sender.go:33` 用 `goMail.TLSMandatory` | ✅ |

**注**：`config.yaml:58` 生产配置亦为 `starttls_required`。该条安全要求守住。

### 缺口 3｜头像摘要是否进入日志与审计 → ✅ 一致（结构性保证）

`audit/service.go:26-37` 上传审计采用**白名单结构体**：

```go
type UploadAuditMetadata struct {
    Purpose, FileName, FileSize, DeclaredMIME, DetectedMIME,
    ValidationResult, ReasonCode, PolicyVersion
}
```

白名单中**没有** `ContentSHA256`、object name、URL 或凭据。
`service.go:39-71` `UploadMetadata` 只接受调用方自有的 typed struct，
并对 map 显式返回 nil，注释说明理由是：

> Maps are rejected so arbitrary Gin context values cannot smuggle fields into the log.

即摘要无法进入审计并非"恰好没人写"，而是**结构上没有字段可承载**。

---

## 五 · 一个被实现层消解的表面规格矛盾

核对缺口 1 时发现：`identity:11` 要求

> 对外响应只返回 `email`、`pending_email` 和 `email_verified`

而 `user-management:26-30` 要求资料响应返回头像与更多字段。两条初看冲突。

实际不冲突 —— **它们约束的是两个不同的契约**：

| 契约 | 实现 | 字段集 |
| --- | --- | --- |
| 用户资料响应 | `identity/dto.go:33-43` `UserInfo` | 9 个字段（含 avatar、role、status） |
| **通知渠道 Contract** | `internal/app/messaging.go:313-321` `LookupVerifiedEmail` | 仅 `VerifiedEmail{Address}` |

`messaging/application/contracts.go:24-25` 的注释明确：

> VerifiedEmail is the only email address a notification adapter may receive.

`messaging.go:319` 在 `user.EmailVerifiedAt == nil` 时返回 `("", false, nil)`，
即未验证邮箱**不进入投递渠道**，满足 `identity:13-15、68-71`。
并有测试 `app/identity_email_contract_test.go:14`
`TestMessagingIdentityReaderReturnsOnlyVerifiedEmail`。

**结论：`identity:11` 的字段白名单约束的是通知渠道 Contract，不是 HTTP 资料响应。**
规格措辞（"对外响应"）不够精确，容易误读为 HTTP 响应 —— 建议在补规格时一并澄清。
