# 配置管理

> 本文只保留当前配置来源、敏感信息边界和部署注意事项。具体默认值以仓库根目录 [`config.yaml`](../../config.yaml) 和 [`.env.example`](../../.env.example) 为准。

## 配置来源

```text
同目录 .env（可选）
→ config.yaml
→ *_env 指定的环境变量
→ 配置校验
→ 构造运行资源
```

- `config.yaml` 保存端口、地址、开关和非敏感策略。
- `.env` 或部署平台 Secret 保存密码和密钥，不得提交真实值。
- 已启用配置引用了不存在或不合法的环境变量时，服务启动失败；关闭的 SMTP 不解析其凭据引用。

## 敏感环境变量

```text
MYSQL_PASSWORD
REDIS_PASSWORD
MINIO_USERNAME
MINIO_PASSWORD
JWT_SECRET
ADMIN_PASSWORD
RABBITMQ_USER
RABBITMQ_PASSWORD
RABBITMQ_VHOST
SMTP_USERNAME
SMTP_PASSWORD
SMTP_FROM
```

RabbitMQ 和其他必需凭据应由开发环境 `.env`、CI/CD Secret、Docker Secret、Kubernetes Secret 或云 Secret Manager 注入；SMTP 的三项环境变量仅在 `smtp.enabled: true` 时必需。已有 `.env` 只补充缺项，不用模板覆盖真实凭据。凭据不得写入运行日志、审计日志、RabbitMQ 事件或 HTTP 错误响应。

## 主要配置段

| 配置段 | 用途 |
|---|---|
| `server` | HTTP 端口 |
| `mysql`、`redis`、`minio` | 基础设施连接和凭据环境变量名 |
| `jwt` | Access／Refresh 有效期（分钟，必须正数） |
| `admin` | 超级管理员初始化资料和密码环境变量名 |
| `logger` | Zap 级别、格式、文件和轮转 |
| `file_upload` | 普通文件、头像大小及下载 URL 有效期 |
| `file_rotation` | 文件热冷 bucket 轮转 |
| `audit_log_archive` | 审计日志冷热归档 |
| `api_docs` | Swagger UI 与 OpenAPI JSON |
| `browser_session` | 精确可信页面 Origin 与显式回环 HTTP 开发例外 |

### RabbitMQ 和 Messaging

| 配置段 | 关键字段 | 说明 |
|---|---|---|
| `rabbitmq` | `host`、`port`、`username_env`、`password_env`、`vhost_env` | Broker 连接和 Secret 引用 |
| `rabbitmq` | `connection_timeout_seconds` | 建连超时，默认 5 秒 |
| `rabbitmq` | `confirm_timeout_seconds`、`worker_lease_seconds` | Publisher Confirm 默认 10 秒，Outbox 租约默认 30 秒 |
| `rabbitmq` | `retry_delays_seconds`、`max_retries`、`dlq_retention_days` | 五级 `1/2/4/8/16` 秒重试和死信保留策略 |
| `messaging` | `max_audience_users`、`max_title_runes`、`max_body_runes` | 动态受众和消息内容上限；受众上限默认 100,000 |

RabbitMQ 事件只包含事件 ID、事件名称/版本、消息副本、组织和聚合版本等最小事实，不包含 Markdown、清洗 HTML、图片、URL 或凭据。

### SMTP 邮箱验证

| 配置段 | 关键字段 | 说明 |
|---|---|---|
| `smtp` | `enabled` | 默认 `false`；关闭时不读取 SMTP 凭据、不校验其连接参数，也不创建邮件发送器 |
| `smtp` | `host`、`port`、`username_env`、`password_env`、`from_env` | 邮箱验证 SMTP 连接和 Secret 引用；账号凭据不写 YAML 明文 |
| `smtp` | `timeout_seconds` | 建连、TLS、认证和发送的统一 deadline，默认 10 秒 |
| `smtp` | `tls_mode` | 仅允许 `disabled`、`starttls_required`、`implicit`；默认且生产推荐 `starttls_required`，禁止 opportunistic 降级 |

启用步骤：在 `config.yaml` 设置 `smtp.enabled: true`，配置真实 `host`／`port` 及 TLS 模式，在 `.env` 或 Secret 中注入非空的 `SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM`，然后重启。启用时 host、凭据和连接参数必须有效，否则启动失败，不回退为关闭；只有 host 的旧配置需要显式补上开关。`tls_mode: disabled` 仅关闭传输 TLS，不是邮件服务开关。

关闭邮件时仍可启动和登录，注册不会发送验证邮件，也不会自动将邮箱标记为已验证；重发验证邮件不能成功投递。已有邮箱、待验证状态和验证凭据不因切换开关而清除。

验证 token 只通过同步 SMTP 发送，不进入数据库、日志、审计或消息事件；投递失败返回稳定错误码，邮箱状态保留并允许节流窗口内重试。

## 文件上传配置校验

| Key | 合法范围 |
|---|---:|
| `file_upload.max_size_mb` | 1～100 MiB |
| `file_upload.avatar_max_size_mb` | 1～10 MiB |
| `file_upload.download_url_expire_seconds` | 正整数 |

非法值会导致启动失败，不会静默使用更宽松的默认值。multipart 请求体会额外保留有限协议开销，但不会提高实际文件大小上限。

### 头像上传规则的运行时边界

前端通过已认证的 `GET /api/user/avatar-upload-policy` 读取当前部署实际使用的头像文件上限；成功响应只含正整数 `data.max_bytes`，并带 `Cache-Control: no-store`。它不创建数据库记录、权限码或新的配置项，也不暴露 MinIO 细节。规则读取失败时只能禁用选图和上传，不能猜测 2 MiB 等默认值；恢复默认头像和昵称、密码操作保持可用。配置变更后必须重启后端并重新做规则与上传边界验收。

## 超级管理员初始化

Seed 依据 `users -> user_roles -> roles.code = admin` 判断超级管理员：

- 已有启用的超级管理员时不重置密码。
- 没有启用超级管理员时，使用 `admin` 配置和 `ADMIN_PASSWORD` 创建或恢复。
- 配置用户名已被普通用户占用时拒绝启动，避免隐式提权。

## 部署注意事项

- Linux、Docker 和 Kubernetes 不依赖 `.env` 文件，只要向进程注入同名环境变量。
- 挂载 `config.yaml` 的容器或 ConfigMap 必须随版本同步更新非敏感配置结构。
- 生产环境是否开启 `api_docs.enabled` 由暴露策略决定。
- 浏览器会话协议必须按下述同窗口切换规则发布，不能保留旧 JWT 兼容期。

### 浏览器会话与反向代理

根 `config.yaml` 当前是本地开发配置：

```yaml
jwt:
  expire: 15
  refresh_expire: 10080
browser_session:
  origin: http://localhost:5173
  allow_loopback_http: true
```

这表示浏览器访问 Vite 的 `http://localhost:5173`，不是 Go 上游地址 `http://127.0.0.1:8080`。`localhost` 与 `127.0.0.1`、不同端口都不是同一 Origin；Vite 改端口或改成 preview 时必须同步显式配置可信页面 Origin，不能因为上游不变而沿用。`allow_loopback_http` 的配置类型默认值为 **false**；根文件显式 true 仅供回环开发，不能复制为生产模板。它只允许主机为 `localhost` 或 `127.0.0.1` 的 HTTP，不允许局域网 IP、任意域名或通配符。

生产配置示例（替换为实际对外站点，不是已执行部署）：

```yaml
browser_session:
  origin: https://admin.example.com
  allow_loopback_http: false
```

Origin 必须是规范的 scheme＋host＋必要端口，无路径（包括尾 `/`）、query、fragment、userinfo、`null` 或通配符；省略默认端口，host 小写。Login／Refresh／Logout 精确校验它及 JSON／自定义 Header，不根据请求 Host、`X-Forwarded-*` 推断白名单或降低 Secure。反代向 Go 使用内部 HTTP 不改变对外 HTTPS 配置；保留原始 Origin 和控制 Header，原样转发 Set-Cookie，不添加 Domain、不扩大 Path、不剥离 Secure／HttpOnly／SameSite、不缓存控制响应。只将对外同源 `/api` 代理到 Go；不开放跨域凭据 CORS，不能用允许任意 Origin 的方式修复 403。

部署前检查浏览器安全上下文、原生 `navigator.locks` 和实际可读写 localStorage；禁用／隐私策略导致必要能力不可用时工作台明确不可用，不回退到不安全协议。开发 HTTP 回环虽可受浏览器视为可信上下文，仍需在目标浏览器检查能力；生产必须 HTTPS 和受信任证书。会话 Cookie 不设置长期有效期；浏览器关闭／重启不能作为服务端撤销保证。完整请求、Cookie 和错误排障见[登录安全策略](login-security.md)。

### 一次性切换与回滚边界

1. 在隔离环境准备配置与真实 MySQL／Redis、前后端联合及浏览器验收；保留受控备份和可发布构建，不记录凭据。本节是操作要求，不声称已执行生产操作。
2. 同一维护窗口部署新后端与新前端，更新可信外部 Origin、反代和配置结构；停止旧实例、避免旧缓存客户端继续请求，不混跑新旧协议。所有旧 Access／Refresh 均要求重新登录；不保留旧用途推断、JSON Refresh 或黑名单登出兼容路径。
3. 使用现有 AutoMigrate／Seed／Route Catalog 同步。保留管理员维护的 API Metadata 字段及历史，不批量回填历史 Public `need_auth`、不新增权限策略、不批量提升 `user_access_versions`、不清空 Redis。旧历史 key 自然到期，不通过 Redis purge 实现切换。
4. 回滚目标也必须严格校验 SID 和活动会话事实，不能让已撤销的新 Access 恢复有效。直接回到不检查 SID 的旧后端不安全；旧协议回滚需另行批准的事故方案及签名密钥切换，不能靠恢复旧 TTL 或只回滚前端。不得把此说明当作已获批准的生产回滚执行授权。

本 change 的发布顺序是先部署并验证后端 Route Catalog、错误映射、72 字节新密码校验和头像规则端点，再发布依赖这些事实的前端；不得让前端先假定上限、错误码或 Cookie 行为。该顺序不引入数据库／Redis／MinIO 迁移；回滚不得恢复旧凭据、降低授权版本检查或通过默认上限回退掩盖配置错误。

## 工作台隔离联调环境

工作台验收使用专属本地 Docker 资源，不连接或改写生产数据库、用户 `.env`、根配置及已有 MinIO 数据：

| 资源 | 隔离地址／标识 |
|---|---|
| MySQL | `127.0.0.1:13306`，数据库 `workbench_acceptance` |
| Redis | `127.0.0.1:16379` |
| MinIO S3 API | `127.0.0.1:19000` |

容器、卷及测试数据必须独立命名；凭据由隔离运行进程环境注入，不复用生产 Secret，也不在文档、命令输出或浏览器证据记录密码／Token。后端应由独立配置路径指向这些资源，保留实际所需配置结构和 Secret 引用；不可为联调覆盖用户配置。该表记录已指定的隔离资源边界，不替代已执行的启动与健康证据。

在隔离库使用现有 AutoMigrate／Seed 机制初始化管理员、路由目录、API 元数据和权限；数据准备只在该库创建真实测试用户、角色、组织及菜单关联。验收数据应能区分多角色、停用菜单、仅分配子菜单、可管理后代／定位祖先、隐藏组织归属、空结果、受保护用户与版本冲突，不以硬编码页面数据或模拟控件替代。注册身份和角色分配分开；普通用户需显式分配授权及运行时可见菜单。

最终可执行 smoke 操作与浏览器结果必须由真实运行补充：记录配置路径（不含 Secret）、容器／卷标识与启动健康命令、后端／前端地址和启动命令、数据准备命令及非敏感对象 ID、实际请求状态／错误码／版本前后值、浏览器截图或观察、执行时间与退出清理方式。尚无这些证据时不得标记验收通过；本手册不提供未经执行的凭据或假定成功的步骤。

## UI 验收账号

本地 UI 验收账号与密码见 [`ui-acceptance-test-accounts.md`](ui-acceptance-test-accounts.md)。密码只保存在同目录的被 `.gitignore` 忽略文件 `ui-acceptance-test-accounts.local.md`，不得提交或用于生产环境。

生产工作台另按 [README 部署规则](../../README.md#管理工作台) 配置同源 `/api`、`/docs` 优先代理和 SPA 回退；Vite 的 `ADMIN_API_TARGET` 仅用于开发代理。
