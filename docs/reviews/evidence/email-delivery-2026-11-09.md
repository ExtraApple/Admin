# 邮箱投递联合验收证据

## 范围与环境

- 变更：`verify-email-delivery`
- 执行日期：2026-11-14
- 目标：在隔离环境验证邮箱验证的 Mailpit 全链路、浏览器链接、错误路径和关闭 SMTP 行为；QQ 真实外发作为独立门禁。
- 环境：独立 MySQL 8.0、Redis 7、MinIO、Mailpit、Go 后端和 Vite；未连接生产数据库、生产 Redis、生产 MinIO 或仓库 `.env`。
- 后端从临时目录运行构建产物，读取临时 `config.yaml` 与 `.env`；RabbitMQ 在临时配置中关闭。
- 仓库默认 `config.yaml`、`.env` 和业务代码未修改。

| 组件 | 地址／端口 | 实际镜像或进程 |
|---|---|---|
| MySQL | `127.0.0.1:13306` | `mysql:8.0`，库 `admin_email_acceptance` |
| Redis | `127.0.0.1:16379` | `redis:7` |
| MinIO | `127.0.0.1:19000/19001` | `minio/minio:latest` |
| Mailpit | `127.0.0.1:11025/18025` | `axllent/mailpit:latest`，启用 `--smtp-auth-accept-any --smtp-auth-allow-insecure` |
| 后端 | `http://127.0.0.1:18080` | 临时目录构建的 `admin-email.exe` |
| 前端 | `http://localhost:15173` | Vite `7.3.6`，`ADMIN_API_TARGET=http://127.0.0.1:18080` |

临时配置与日志位于 `C:\Users\Rog\AppData\Local\Temp\email-acceptance-2026-11-09`，不进入仓库。

## 自动化门禁

| 命令 | 实际结果 |
|---|---|
| `go test ./...` | 通过；54 个 Go 包 |
| `npm test`（`web/`） | 通过；14 个测试文件、184 个测试 |
| `npm run typecheck`（`web/`） | 通过 |

未执行 `npm run build`；它不是本次邮件链路的必需门禁。

## 本地 Mailpit 结果

隔离环境启动后：

- `GET /api/user/email-verification-policy` → `200`，`data.enabled:true`。
- 测试用户通过真实 `POST /api/register` 创建；验证码通过 Redis `captcha:<id>` 固定为测试值，未使用 OCR。
- 每个场景使用独立用户，未因每小时 3 次签发限制污染其他场景。

| # | 用户 | 操作 | 实际结果 |
|---|---|---|---|
| 4.1 | U1 | 已验证邮箱换绑 | `200`；`pending_email` 写入，当前邮箱保持，Mailpit 收到邮件 |
| 4.2 | U1 | 浏览器打开验证链接 | `200` 确认；当前邮箱更新、pending 清空、`email_verified:true`；URL 变为 `/profile`，fragment 清除 |
| 4.3 | U2 | 未验证邮箱直接换绑 | `200`；当前邮箱直接替换、pending 为空、`email_verified:false`；确认后为 `true` |
| 4.4 | U3 | 换绑后重发 | 换绑 `200`、重发 `200`；旧 token `422 IDENTITY_EMAIL_VERIFICATION_INVALID`，新 token `200` |
| 4.5 | U4 | 换绑后取消 | 取消 `200`；当前邮箱和验证状态不变；旧 token `422`；再次取消 `409 IDENTITY_EMAIL_PENDING_STALE` |
| 4.6 | U5 | 错误输入 | 错密码 `422 IDENTITY_VALIDATION_INVALID`（`current_password`）；重复邮箱 `409 IDENTITY_CONFLICT`；畸形／过期／已用 token 均 `422 IDENTITY_EMAIL_VERIFICATION_INVALID`；邮箱状态未改变 |
| 4.7 | U6 | Mailpit 停止后换绑 | `503 IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED`；pending 保留；Mailpit 恢复后重发 `200`，确认 `200` |
| 4.8 | U7／U8 | 关闭 SMTP 后确认／取消 | 策略 `200 enabled:false`；重发／换绑 `503 IDENTITY_EMAIL_VERIFICATION_DISABLED`；U7 确认 `200`；U8 取消 `200` |
| 4.9 | U9 | 第 4 次签发 | 第 1、2 次换绑 `200`；第 3 次换绑 `429 IDENTITY_EMAIL_VERIFICATION_RATE_LIMITED`；最终 pending 保持为第 2 次候选 |
| 4.10 | U1／全局 | 链接、DB、日志安全检查 | 邮件链接为 `http://localhost:15173/profile#email_token=`；浏览器最终 URL 为 `/profile`；确认请求资源为 `/api/user/email-verifications/confirm`；日志 `email_token` 计数为 0；token 哈希异常计数为 0 |

### 浏览器观察

- U1 在真实 Chromium 中登录后打开验证链接；页面显示“邮箱已验证”，个人资料邮箱为新地址，`pending_email` 为空。
- U9 在关闭 SMTP 后打开个人资料；页面显示“当前部署未启用邮件服务，邮箱换绑与重发暂不可用；如已收到验证码仍可验证，待验证邮箱仍可取消”，并展示待验证候选。
- 截图（已复制到仓库，未包含 token、Cookie、密码或授权码）：
  - [`login.png`](email-delivery-2026-11-09/login.png)
  - [`u1-confirmed.png`](email-delivery-2026-11-09/u1-confirmed.png)
  - [`u9-pending-disabled.png`](email-delivery-2026-11-09/u9-pending-disabled.png)
  - [`mailpit-inbox.png`](email-delivery-2026-11-09/mailpit-inbox.png)

### 数据库与安全抽查

- 测试用户共 9 个（ID 2–10），均使用 `@example.test` 地址；未使用真实用户邮箱。
- `email_verification_credentials.token_hash` 为 `char(64)` 且唯一；查询 `CHAR_LENGTH(token_hash) <> 64` 返回 `0`。
- 表中不存在明文 token 列；`email_token` 未出现在应用日志或后端 stdout。
- 邮件正文链接使用 `browser_session.origin`，未发现 `localhost:5173` 混入。
- Mailpit 默认使用内存存储；本次停止 Mailpit 后重启，重启前的邮件列表被清空。历史邮件 ID 已从保存的 API 响应中记录，重启后的最新 ID 示例：
  - U8 pending：`7A59yrXBjnKPAnO6f3Jn45`
  - U7 pending：`4WrJanqo2AuBP8NgtnpCP3`
  - U9 pending B：`1YxcXpcSy4o0XgjiztsMZN`
  - U9 pending A：`18H0EbtfJMWMsHWs6y7mjR`
  - U6 pending：`18s9RagDJB95ixQKJpoNc8`

早前场景保存的 message ID 示例：U1 change `0SpDy14FVGTIxc7JHsjeFM`、U2 `0X0fD9Y4JNkQNxzuj5hJjg`、U3 old/new `37A3Pl1dpmdY92ZRgxVsfd` / `5lpsMj3pG1RT8ZZAIe38z1`、U4 `1QAAG6lBs3x6BRuqXQcLVa`、U5 `6oiXKyGxxxq3VWTtJ7R955`。

## QQ 真实外发

已执行并成功。

- 凭据来源：仓库本地 `.env`（已被 `.gitignore` 忽略），未提交；证据中不出现 QQ 地址、授权码或 token。
- 配置：`smtp.qq.com:465`、`tls_mode: implicit`，`SMTP_FROM` 与 `SMTP_USERNAME` 一致，`SMTPAuthPlain`。
- 使用真实注册创建一个普通 QQ 测试用户；`POST /api/register` 返回 `200`，邮箱验证邮件成功外发。
- 通过同一 QQ 账号的 IMAP 收件箱读取最新验证邮件：主题“验证你的邮箱”，提取到 43 字符 token；token 未写入证据或命令输出。
- API 登录返回 `200`；`POST /api/user/email-verifications/confirm` 返回 `200`；随后 `GET /api/user/info` 返回 `email_verified:true`、`pending_email:""`，邮箱域名为 `qq.com`。
- `465＋implicit` 成功，未触发 587＋`starttls_required` 回退。

## 清理状态

已停止本地 Mailpit 验收和 QQ 冒烟的临时后端、Vite 与全部容器；删除 `email-acceptance-*` 与 `email-acceptance-qq-*` 容器、对应 MySQL／MinIO 卷，以及两个临时目录（含临时 `.env`、构建产物、token 和 IMAP 取信文件）。端口 `13306/16379/19000/19001/11025/18025/18080/15173` 均已关闭，无残留验收容器。

保留本 evidence 和截图；仓库 `config.yaml` 与业务代码未修改。仓库本地 `.env` 由操作者填写 QQ 凭据，已被 `.gitignore` 忽略，未提交；证据中不包含这些值。
