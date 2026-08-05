# Admin

本项目是一个使用 Go 开发的后台管理系统后端。系统当前行为以 `openspec/specs/` 为准，计划变更以 `openspec/changes/` 为准。
Go + Gin + GORM 后台管理系统。

## 代码结构

项目采用业务模块优先的分层单体。`main.go` 只启动 `internal/app`；App 是唯一组合根、Gin 业务路由注册点和 AutoMigrate/Seed 编排入口。配置、日志、数据库、Redis 和 MinIO 构造集中在 `internal/platform`。

业务实现位于固定的 `internal/identity`、`authorization`、`navigation`、`apimetadata`、`organization`、`dictionary`、`files`、`audit`、`routecatalog`、`apidoc` 和 `uploadsecurity` 模块。复杂模块按 `adapters -> application -> domain` 依赖，跨模块调用使用调用方声明的最小 Contract。

详细所有权、推荐阅读入口和架构验证命令见 [业务模块代码导航](docs/module-navigation.md)。完整文档入口见 [docs 使用说明](docs/README.md)，领域术语见 [CONTEXT.md](CONTEXT.md)，长期架构决策见 [docs/adr/](docs/adr/)，当前行为规格见 [openspec/specs/](openspec/specs/)。

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

Linux、Docker、Kubernetes 部署时不必依赖 `.env` 文件，可以直接注入同名环境变量，详细示例见 [docs/runbooks/configuration.md](docs/runbooks/configuration.md)。

超级管理员会在启动时按新 RBAC 体系自动兜底创建，管理员身份只看：

```text
users -> user_roles -> roles.code = "admin"
```

## 启动

```bash
go run .\main.go
```

本地 API 文档：

```text
http://localhost:8080/docs
http://localhost:8080/docs/openapi.json
```

MinIO 本地启动示例：

```bash
minio.exe server D:\WORK\minio\data --console-address ":9001"
```
