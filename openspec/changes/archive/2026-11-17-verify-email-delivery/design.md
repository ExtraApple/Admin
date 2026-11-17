## Context

邮箱验证功能已实现，现有自动化覆盖发送适配器、应用层状态转换、HTTP 错误映射和关闭 SMTP 时的策略行为。缺少的是真实 Mailpit、真实 HTTP、真实浏览器和真实 SMTP 配置切换的联合验收，以及可复核的验收证据。

当前仓库默认 `smtp.enabled:false`，`browser_session.origin` 为 `http://localhost:5173`；本地基础设施未运行。Docker 和 Mailpit 镜像可用，旧 `a2-acceptance-*` 容器已停止且包含历史验收数据，不能作为干净基线。

## Goals / Non-Goals

**Goals:**

- 用独立环境完成本地 Mailpit 全链路验收，覆盖既有邮箱验证状态机和稳定错误码。
- 用真实 Chromium 验证浏览器链接、fragment 清除和个人资料页状态。
- 在本地通过后，用真实 QQ SMTP 做一次可选的投递冒烟。
- 生成脱敏、可复核的 evidence，并只把已验证事实补进 runbook。
- 失败时先汇报、后修复，避免自动修改掩盖问题。

**Non-Goals:**

- 不改变用户管理、认证、授权、路由、数据库 schema、权限码或前端业务行为。
- 不新增 Playwright 测试框架或永久 E2E 测试套件。
- 不重复现有自动化已覆盖的并发、锁和事务竞争场景。
- 不连接生产数据库、生产 Redis、生产 MinIO 或真实用户邮箱。
- 不把 SMTP 授权码、密码、Cookie、Access token 或验证 token 写入仓库、日志、截图或聊天。

## Decisions

### 1. 使用全新隔离栈，不复用旧容器

在临时目录 `C:\Users\Rog\AppData\Local\Temp\email-acceptance-2026-11-09` 构建后端二进制并放置临时 `config.yaml` 与 `.env`；从该目录运行，使 `main.go` 读取临时配置，不修改仓库默认文件。

| 组件 | 端口 | 说明 |
|---|---|---|
| MySQL 8.0 | `13306` | 独立库 `admin_email_acceptance`，专用卷 |
| Redis 7 | `16379` | 会话、验证码 fixture、限流事实 |
| MinIO | `19000/19001` | 独立凭据与卷 |
| Mailpit | `11025/18025` | `--smtp-auth-accept-any --smtp-auth-allow-insecure` |
| 后端 | `18080` | 临时配置，`smtp.enabled:true` |
| Vite | `15173` | `ADMIN_API_TARGET=http://127.0.0.1:18080` |

临时配置关闭 RabbitMQ（`host` 为空并清空其 `*_env`），邮件链路不依赖消息模块。`browser_session.origin` 设为 `http://localhost:15173`，`allow_loopback_http:true`。

替代方案是复用 `a2-acceptance-*` 或直接修改仓库 `.env`；两者都会引入历史数据或污染默认配置，因此不采用。

### 2. Mailpit 固定使用 PLAIN 认证参数

后端发送器固定使用 `SMTPAuthPlain`。Mailpit 必须启用 `--smtp-auth-accept-any` 与 `--smtp-auth-allow-insecure`，临时 SMTP 配置使用 `tls_mode:disabled`、`host:127.0.0.1`、`port:11025` 和非空测试账号。缺少认证参数会表现为 503 `IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED`，不能误判为业务代码失败。

### 3. 真实注册 + Redis 验证码 fixture

测试用户通过真实 `POST /api/register` 创建，使用随机用户名和 `@example.test` 邮箱；每个场景使用独立用户和独立邮箱，避免每小时 3 次签发限制互相污染。

验证码不依赖 OCR：浏览器发起 `/api/captcha` 后读取最新 `captcha_id`，用 `redis-cli SET captcha:<id> 246810 EX 300` 写入已知值，再在登录或注册表单填写 `246810`。这是测试环境的 fixture，不修改产品验证码实现。

测试用户分配：

| 用户 | 初始状态 | 用途 |
|---|---|---|
| U1 | 注册并验证初始邮箱 | 已验证换绑、链接确认、hash 安全 |
| U2 | 注册但不验证 | 未验证当前邮箱直接替换 |
| U3 | 已验证 | 重发、旧 token 失效、新 token 确认 |
| U4 | 已验证 | 取消待验证、stale cancel |
| U5 | 已验证 | 错密码、重复邮箱、畸形／过期／已用 token |
| U6 | 已验证 | Mailpit 停止后的投递失败与恢复 |
| U7 | 有 pending | 关闭 SMTP 后确认仍可用 |
| U8 | 有 pending | 关闭 SMTP 后取消仍可用 |
| U9 | 已验证 | 第 4 次签发返回 429 |

### 4. 真实浏览器 + API/SQL/Redis 证据

用 Chrome DevTools MCP 驱动真实 Chromium 完成登录、资料页换绑、点击邮件链接、截图和 fragment 观察；用 curl 获取可复现的 HTTP 状态和错误码，用 SQL 验证 `pending_email`、`email_verified_at` 和 token 哈希，用 Redis 控制验证码与限流前置条件。

不引入 Playwright：本次是一次性联合验收，浏览器自动化只用于可观察 UI 行为；边界状态用现有 API 和数据库断言更精确。

### 5. 场景顺序与限流隔离

先执行本地 happy path 和状态转换，再执行错误路径；`429`、Mailpit 停止和关闭 SMTP 重启放在最后，避免重启或限流污染前面结果。并发场景不重复执行，引用 `internal/identity/application/email_change_test.go` 与 `internal/identity/adapters/gorm/repository_test.go` 的现有结果。

### 6. 失败先汇报、后修复

任一场景失败时立即停止当前阶段，保留容器、Mailpit 邮件、日志和数据库状态，不自动修改代码或配置。汇报必须包含：阶段／场景、预期、实际、HTTP 状态或业务错误码、截图路径、Mailpit message ID、脱敏日志、DB/Redis 状态、未执行范围和建议修复选项。操作者查看并确认后再修复，修复后只重跑失败场景和相关回归。

本地失败未解决前不进入 QQ 阶段；QQ 失败不回滚已通过的本地结果，只记录为外部 SMTP／凭据阻塞。

### 7. QQ 真实外发使用同一实例切换

本地矩阵通过后，操作者把 QQ 地址和授权码写入临时 `.env`，不发送到聊天。停止后端，临时配置改为 `smtp.qq.com:465`、`tls_mode:implicit`，`SMTP_FROM` 与认证账号一致，重启同一实例。先验证 465＋implicit；失败时先汇报，经确认后再尝试 587＋`starttls_required`，不自动切换。

### 8. 证据与文档边界

新建 `docs/reviews/evidence/email-delivery-2026-11-09.md`，记录环境、端口、镜像、自动化门禁、场景结果、HTTP 状态／错误码、Mailpit message ID、脱敏截图、数据库／日志安全检查和清理结果。未执行项标记为未执行，不写成通过。

测试通过后只把已实际观察到的前置条件、命令和结果补进 `docs/runbooks/email-delivery.md`；QQ `SMTPAuthPlain` 等未实测断言继续保持待验证或附带条件。归档的 `complete-profile-self-service/evidence.md` 不修改。

### 9. 自动化门禁

执行 `go test ./...`、`npm test`、`npm run typecheck`。`npm run build` 不作为邮件链路必要门禁，若执行则单独记录结果和既有 chunk 警告。

### 10. 清理

证据落盘后停止后端、Vite 和容器，删除 `email-acceptance-*` 容器、卷和临时配置；保留 evidence 与必要的脱敏日志片段。临时 `.env` 中的 QQ 授权码在清理时删除。

## Risks / Trade-offs

- **端口或旧容器冲突**：启动前检查 `13306/16379/19000/19001/11025/18025/18080/15173`，不复用旧容器；冲突时改用同组备用端口并同步临时配置。
- **MySQL/Redis/MinIO 启动慢**：使用健康检查等待，不以固定睡眠代替；健康检查失败先汇报，不继续启动后端。
- **浏览器验证码摩擦**：验证码 fixture 依赖读取最新 `captcha_id`；若页面重新生成验证码，重新读取 Redis，不绕过登录或伪造会话。
- **QQ 外部限制**：个人 QQ 邮箱可能受频率、反垃圾或授权码策略影响；QQ 失败只阻塞真实外发门禁，不否定本地验收。
- **token 泄漏**：截图前清空 fragment，证据中不展示邮件正文 token；日志只保留脱敏错误码和 message ID。
- **测试顺序污染**：限流和 SMTP 重启场景后置，独立用户隔离计数；不通过清空生产数据或降低限流阈值来“通过”。
- **验收范围扩张**：不新增永久测试框架、不改业务代码；发现真实缺陷时先汇报，再由操作者决定是否新建修复 change。
