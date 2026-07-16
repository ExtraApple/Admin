## Why

当前普通文件上传仅限制大小并信任客户端文件名和 MIME，头像上传也只检查扩展名；伪装文件、损坏图片、危险 Office 容器、超大 multipart 请求和任意头像 URL 因而可能进入 MinIO 或数据库。需要在继续开放 PDF、Office 和文本类业务文件前，为管理员文件上传和普通用户头像建立一致、可审计、可迁移的 V1 安全边界。

## What Changes

- 为 `POST /api/admin/files` 和 `POST /api/user/avatar` 增加解析 multipart 前的请求体限制、空文件拒绝、系统生成 object key、文件名清洗、真实类型检测和稳定安全错误分类。
- 管理员普通文件继续只支持单文件上传，V1 白名单限定为 JPEG、PNG、WebP、PDF、DOCX、XLSX、PPTX、UTF-8 TXT 和 UTF-8 CSV；危险格式、旧版 Office、宏文档、压缩包、脚本及类型不一致文件在写入 MinIO 前拒绝。
- 对 DOCX、XLSX、PPTX 执行受资源上限保护的 Office Open XML 容器校验，拒绝宏、ActiveX、OLE、嵌入文件、加密文档、路径穿越和自动加载外部资源。
- 用户头像完整解码、限制尺寸和像素、移除元数据、缩放至最大 1024×1024，并按透明通道重新编码为 JPEG 或 PNG后再存储。
- **BREAKING** `PUT /api/user/info` 不再接受 `avatar` 字段；头像只能通过专用上传接口修改，或通过 `DELETE /api/user/avatar` 恢复默认头像，数据库不再接受外部头像 URL。
- 为文件记录增加格式策略验证状态、检测 MIME、验证时间、策略版本和失败原因；上线前的记录回填为“历史未验证”，不在启动或迁移期间读取 MinIO 批量扫描。
- 增加受权限控制的附件下载、图片预览和单文件重新验证路由；**BREAKING** 文件详情不再暴露可绕过响应头和状态校验的原始 MinIO 下载 URL。
- 普通文件默认附件下载，仅验证通过的 JPEG、PNG、WebP 可预览；历史未验证文件只能强制附件下载，策略封锁文件禁止下载和预览。
- 上传成功和拒绝结果写入审计日志的结构化安全元数据，不记录文件内容、multipart 原文、预签名 URL、存储凭据或底层解析器错误。
- 在 `config.yaml` 新增 `file_upload.max_size_mb`、`file_upload.avatar_max_size_mb` 和 `file_upload.download_url_expire_seconds`，非法值导致服务启动失败。
- V1 不接入 ClamAV，不提供通用压缩包、旧版 Office、宏文件、脚本、可执行文件、批量上传、上传配额或业务数据导入。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `file-management`: 修改管理员上传、文件元数据、改名、详情、下载、预览和历史文件过渡行为，并增加单文件重新验证。
- `user-management`: 修改头像上传、存储、返回和资料修改行为，并增加恢复默认头像。
- `logging`: 为文件和头像上传增加不含文件内容的结构化安全审计元数据。

## Impact

- **路由**：
  - 修改 `POST /api/admin/files`、`GET /api/admin/files/:id`、`PUT /api/admin/files/:id`、`POST /api/user/avatar`、`PUT /api/user/info`。
  - 新增公开只读头像路由 `GET /api/avatars/:user_id`、`GET /api/avatars/default`，以及 `GET /api/admin/files/:id/download`、`GET /api/admin/files/:id/preview`、`POST /api/admin/files/:id/revalidate`、`DELETE /api/user/avatar`。
- **权限码**：新增路由经现有 API 同步机制生成 `admin.files.id.download.get`、`admin.files.id.preview.get`、`admin.files.id.revalidate.post`；普通用户头像路由只要求 JWT。
- **模型和迁移**：扩展 `files`、`users`、`audit_logs`、`audit_log_archives`；历史文件和头像需要状态回填，但不在迁移中读取对象内容。
- **对象存储**：继续使用私有 `files` bucket 和 `image` bucket；object key 始终由系统生成，文件下载、文件预览和头像读取均通过应用受控接口输出。
- **代码区域**：涉及 `initialize`、`model`、`dto`、`service`、`handler`、`middleware`、`router`、`utils`、API 文档元数据和自动化测试。
- **依赖**：使用 Go 标准库及现有 MIME/图片能力完成格式检查；如实现需要新增图片缩放库，必须选择纯 Go、可维护且不引入外部扫描服务的依赖。
