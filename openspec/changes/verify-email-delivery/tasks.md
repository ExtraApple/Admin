## 1. 隔离环境与配置

- [x] 1.1 创建临时目录 `C:\Users\Rog\AppData\Local\Temp\email-acceptance-2026-11-09`，在仓库执行 `go build -o <临时目录>\admin-email.exe .`，后续从临时目录运行二进制。
- [x] 1.2 基于仓库 `config.yaml` 生成临时配置：后端 `18080`、MySQL `13306`、Redis `16379`、MinIO `19000`、Mailpit `11025`、`browser_session.origin: http://localhost:15173`、`smtp.enabled: true`、`tls_mode: disabled`；清空 RabbitMQ host／port／`*_env`。
- [x] 1.3 在临时目录生成 `.env`：隔离 MySQL／MinIO／JWT／admin 凭据，以及 `SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM` 的 Mailpit 测试值；不覆盖仓库 `.env`。
- [x] 1.4 启动独立容器：MySQL 8.0（库 `admin_email_acceptance`，卷）、Redis 7、MinIO（独立凭据与卷）、Mailpit（`--smtp-auth-accept-any --smtp-auth-allow-insecure`）；端口按 1.2 绑定到回环地址。
- [x] 1.5 等待真实健康检查通过：`mysqladmin ping`、Redis `PING`、MinIO live 端点、Mailpit `/api/v1/messages`；不以固定睡眠代替检查。
- [x] 1.6 从临时目录启动后端，确认日志显示平台初始化成功；启动 Vite `15173`，设置 `ADMIN_API_TARGET=http://127.0.0.1:18080`。
- [x] 1.7 调用 `GET /api/user/email-verification-policy`，确认返回 `enabled:true`；记录实际状态码和响应结构。
- [x] 1.8 建立失败门禁：任一场景失败立即停止，保留容器、邮件、日志和 DB 状态，先汇报后修复，不自动改代码或配置。

## 2. 自动化门禁

- [x] 2.1 在仓库执行 `go test ./...`，记录包数量、通过／失败结果。
- [x] 2.2 在 `web/` 执行 `npm test`，记录测试文件数和通过／失败结果。
- [x] 2.3 在 `web/` 执行 `npm run typecheck`，记录结果。
- [x] 2.4 若任一门禁失败，按 1.8 暂停并汇报；不得把门禁失败写成邮件链路通过。

## 3. 测试用户与验证码 fixture

- [x] 3.1 创建 U1：真实注册随机用户名／`@example.test` 邮箱，使用 Redis `captcha:<id>` 固定验证码完成登录，点击 Mailpit 初始验证邮件，确认初始邮箱已验证。
- [x] 3.2 创建 U2：真实注册并登录，但不点击初始验证邮件，保持当前邮箱未验证。
- [x] 3.3 创建 U3、U4、U5：真实注册并完成初始邮箱验证，分别用于重发、取消和错误 token 场景。
- [x] 3.4 创建 U6、U7、U8、U9：真实注册并完成初始邮箱验证；U7／U8 在关闭 SMTP 前先建立 pending。
- [x] 3.5 每个用户使用独立随机邮箱；记录用户 ID、初始邮箱和初始验证状态，不记录密码、Cookie 或 token 明文。
- [x] 3.6 确认每个用户的初始签发次数不超过 3 次／小时，避免限流污染后续场景。

## 4. 本地 Mailpit 场景

- [x] 4.1 U1 通过 `PUT /api/user/info` 提交新邮箱和当前密码；确认 `pending_email` 写入、当前已验证邮箱不变、Mailpit 收到邮件。
- [x] 4.2 U1 在真实浏览器打开 Mailpit 邮件中的 `/profile#email_token=...`；确认当前邮箱更新、待验证清空、`email_verified:true`，URL fragment 已清除。
- [x] 4.3 U2 提交新邮箱；确认未验证当前邮箱被直接替换、验证时间清空、Mailpit 收到新验证邮件；确认后变为已验证。
- [x] 4.4 U3 换绑后调用重发；确认旧 token 返回 422，新 token 可以确认；记录两次 Mailpit message ID。
- [x] 4.5 U4 建立 pending 后取消；确认当前邮箱和验证状态不变、旧 token 失效；再次取消返回 409 `IDENTITY_EMAIL_PENDING_STALE`。
- [x] 4.6 U5 验证错当前密码、重复邮箱、畸形 token、已使用 token 和 SQL 置过期 token；确认对应 422／409 错误码且邮箱状态不变。
- [x] 4.7 U6 建立 pending 后停止 Mailpit，触发换绑／重发；确认 503 `IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED` 且 pending 保留；恢复 Mailpit 后重发并确认成功。
- [x] 4.8 U7／U8 建立 pending 后关闭 SMTP 并重启后端；确认策略 `enabled:false`、换绑／重发 503 disabled、U7 确认可用、U8 取消可用。
- [x] 4.9 U9 连续签发至第 4 次；确认 429 `IDENTITY_EMAIL_VERIFICATION_RATE_LIMITED`，并记录前三次成功签发次数。
- [x] 4.10 检查浏览器链接 origin、hash 是否进入后续请求、数据库是否只存 token 哈希、日志／截图是否出现明文 token；记录实际观察。
- [x] 4.11 记录每个场景的 HTTP 状态／业务错误码、Mailpit message ID、关键 UI 状态和截图路径，未执行项明确标记。

## 5. 真实 QQ SMTP 冒烟

- [x] 5.1 本地全矩阵通过后，确认操作者已在临时 `.env` 提供 QQ 地址和授权码；聊天、命令输出和证据中不出现授权码。
- [x] 5.2 停止本地后端，临时配置切换为 `smtp.qq.com:465`、`tls_mode:implicit`，`SMTP_FROM` 与认证账号一致；重启同一后端实例。
- [x] 5.3 使用 QQ 专用测试用户发送／重发验证邮件，确认真实邮箱收到邮件并能完成确认；记录投递时间和脱敏结果。
- [x] 5.4 若 465＋implicit 失败，先暂停并汇报；仅经操作者确认后尝试 587＋`starttls_required`，不自动切换。
- [x] 5.5 若没有可用凭据或外部投递失败，标记 QQ 门禁未执行／外部阻塞；本地通过结果仍保留。

> 5.4 未触发：QQ `465＋implicit` 发送和确认成功，未进行 587 回退。5.5 不适用：QQ 门禁已执行成功。

## 6. 证据与文档

- [x] 6.1 创建 `docs/reviews/evidence/email-delivery-2026-11-09.md`，记录环境镜像、端口、临时配置来源（不含 Secret）、自动化门禁和场景矩阵结果。
- [x] 6.2 保存脱敏截图：登录、待验证状态、Mailpit 收件、确认成功、关键错误；截图不得显示 token、Cookie、密码或授权码。
- [x] 6.3 记录数据库抽查结果：`pending_email`、`email_verified_at`、`email_verification_credentials.token_hash`；说明 token 哈希长度和明文不存在。
- [x] 6.4 测试通过后更新 `docs/runbooks/email-delivery.md`，只补充已实际验证的前置条件、命令和事实；未验证的 QQ 断言保持待验证。
- [x] 6.5 执行敏感信息扫描，确认仓库、日志、evidence 和截图没有 SMTP 授权码、密码、Cookie、Access token 或验证 token 明文。

## 7. 清理与收尾

- [x] 7.1 停止后端、Vite 和所有临时容器；记录退出状态。
- [x] 7.2 删除 `email-acceptance-*` 容器、卷和临时 `.env`／配置；删除包含 QQ 授权码的临时文件。
- [x] 7.3 保留 evidence 和必要的脱敏日志片段；确认仓库默认 `config.yaml`、`.env` 和业务代码没有被修改。
- [x] 7.4 汇总本地验收、QQ 冒烟、未执行项和阻塞项，给出是否可以进入后续发布或修复 change 的结论。
