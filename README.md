# Admin

Go + Gin + GORM 后台管理系统。

## 配置

项目使用 `config.yaml + .env` 混合配置：

- `config.yaml` 保存配置结构和非敏感默认值。
- `.env` 保存密码、密钥等敏感值。
- `.env.example` 是模板，可以提交到 Git。
- `.env` 已加入 `.gitignore`，不要提交真实密码。

本地启动前复制 `.env.example` 为 `.env`，并填写：

```env
MYSQL_PASSWORD=
REDIS_PASSWORD=
MINIO_USERNAME=minioadmin
MINIO_PASSWORD=minioadmin
JWT_SECRET=
ADMIN_PASSWORD=
```

超级管理员会在启动时按新 RBAC 体系自动兜底创建，管理员身份只看：

```text
users -> user_roles -> roles.code = "admin"
```

## 启动

```bash
go run .\main.go
```

MinIO 本地启动示例：

```bash
minio.exe server D:\WORK\minio\data --console-address ":9001"
```

## Git

```bash
git status
git add .
git commit -m "message"
git pull --rebase origin main
git push origin main
```
