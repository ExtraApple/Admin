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
| `jwt` | Token 有效期和存量 Token 兼容有效期 |
| `admin` | 超级管理员初始化资料和密码环境变量名 |
| `logger` | Zap 级别、格式、文件和轮转 |
| `file_upload` | 普通文件、头像大小及下载 URL 有效期 |
| `file_rotation` | 文件热冷 bucket 轮转 |
| `audit_log_archive` | 审计日志冷热归档 |
| `api_docs` | Swagger UI 与 OpenAPI JSON |

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

## 超级管理员初始化

Seed 依据 `users -> user_roles -> roles.code = admin` 判断超级管理员：

- 已有启用的超级管理员时不重置密码。
- 没有启用超级管理员时，使用 `admin` 配置和 `ADMIN_PASSWORD` 创建或恢复。
- 配置用户名已被普通用户占用时拒绝启动，避免隐式提权。

## 部署注意事项

- Linux、Docker 和 Kubernetes 不依赖 `.env` 文件，只要向进程注入同名环境变量。
- 挂载 `config.yaml` 的容器或 ConfigMap 必须随版本同步更新非敏感配置结构。
- 生产环境是否开启 `api_docs.enabled` 由暴露策略决定。
- JWT 存量兼容有效期应保持为切换时固定值；最后一个存量 Refresh Token 到期后再通过独立变更移除。

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

生产工作台另按 [README 部署规则](../../README.md#管理工作台) 配置同源 `/api`、`/docs` 优先代理和 SPA 回退；Vite 的 `ADMIN_API_TARGET` 仅用于开发代理。
