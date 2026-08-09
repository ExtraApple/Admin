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
- 配置引用了不存在或不合法的环境变量时，服务启动失败。

## 敏感环境变量

```text
MYSQL_PASSWORD
REDIS_PASSWORD
MINIO_USERNAME
MINIO_PASSWORD
JWT_SECRET
ADMIN_PASSWORD
```

`REDIS_PASSWORD` 可以为空；其他必需值应由开发环境 `.env`、CI/CD Secret、Docker Secret、Kubernetes Secret 或云 Secret Manager 注入。

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
