## Why

当前管理员普通文件上传与用户头像上传共享了部分图片类型，导致两个业务入口的职责边界不够清晰，也扩大了后台普通文件接口需要维护的攻击面。现阶段项目仅是通用后台管理初始版本，应收敛为“管理员上传普通文件、普通用户只上传头像”，并为实际落库对象建立服务端 SHA-256 完整性基线。

## What Changes

- **BREAKING** 管理员普通文件接口 `POST /api/admin/files` 的 V1 白名单收敛为 PDF、UTF-8 TXT、UTF-8 CSV，不再接受 JPEG、PNG、WebP。
- 管理员普通文件统一采用附件下载语义，不再提供普通文件图片预览能力。
- 普通用户仍不能访问管理员普通文件接口，只能通过 `POST /api/user/avatar` 上传或修改头像。
- 用户头像继续只允许 JPEG、PNG、WebP，并在完整解码、去除元数据、缩放和重新编码后写入私有 `image` bucket。
- Office、压缩包、SVG、HTML、脚本、可执行文件及其他未列入对应入口白名单的格式继续拒绝。
- 服务端为实际写入 MinIO 的内容计算并持久化 SHA-256：
  - 管理员文件计算通过验证后写入 `files` bucket 的 PDF/TXT/CSV 字节；
  - 用户头像计算标准化并重新编码后写入 `image` bucket 的 JPEG/PNG 字节；
  - 重新验证历史管理员文件时，按当前策略重新读取对象并计算摘要。
- SHA-256 只作为内容指纹和后续完整性比较基线，不信任客户端摘要，不用作 object key、自动去重或恶意文件检测依据。
- 既有记录允许摘要为空；本次不自动批量读取历史 MinIO 对象，也不自动删除历史对象。
- 非目标：Office 支持、ClamAV 等外部恶意文件扫描、业务内容解析、自动去重、后台批量完整性巡检和新增手动完整性检查接口。这些能力留待后续版本按实际业务方向评估。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `file-management`：收窄管理员普通文件白名单，移除普通文件图片预览，并为上传及重新验证后的受信任对象保存服务端 SHA-256 摘要。
- `user-management`：明确普通用户仅能通过头像接口上传图片，并为标准化头像对象保存服务端 SHA-256 摘要。

## Impact

- 路由保持不变：`POST /api/admin/files`、`POST /api/admin/files/:id/revalidate`、管理员下载/预览路由、`POST /api/user/avatar` 和 `GET /api/avatars/:user_id` 不新增或改名。
- 权限保持不变：管理员普通文件接口继续使用现有 `admin.files.*` 权限；头像接口继续只要求已认证用户操作本人头像。
- MinIO bucket 保持不变：普通文件使用私有 `files` bucket，头像使用私有 `image` bucket。
- 数据模型新增可空摘要字段：`files.content_sha256` 与 `users.avatar_content_sha256`；不添加唯一索引。
- 主要影响 `service/uploadsecurity`、文件与头像 service、model/dto、GORM 自动迁移、OpenAPI/项目文档和对应测试。
- 已接入管理员图片上传或普通文件预览的客户端需要停止使用该能力；历史开发数据不执行破坏性自动迁移，兼容处理将在设计中明确。
