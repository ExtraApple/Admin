# 文件管理

> 本文只保留文件安全、状态和存储边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/file-management/spec.md`](../../openspec/specs/file-management/spec.md) 为准。

## 安全策略

管理员普通文件只接受：

| 类型 | 扩展名 | 规范 MIME |
|---|---|---|
| PDF | `.pdf` | `application/pdf` |
| TXT | `.txt` | `text/plain` |
| CSV | `.csv` | `text/csv` |

共同约束：

- 请求必须且只能包含一个名为 `file` 的非空文件 part。
- 文件大小、展示名称、扩展名、声明 MIME、检测 MIME 和内容验证必须一致。
- TXT/CSV 只接受有效 UTF-8，可带 BOM，不接受 NUL、UTF-16、GBK 或 GB2312。
- 图片只允许通过专用头像接口上传；Office、压缩包、HTML、SVG、脚本和可执行文件默认拒绝。
- 危险双扩展名即使最终扩展名属于白名单也会被拒绝。
- 校验失败发生在对象存储写入前；对象写入后数据库失败时尝试补偿删除。

`validated` 只表示通过当前格式策略，不代表已经经过病毒扫描。

## 存储与摘要

- 服务端生成 UUID object key，文件保存在私有 MinIO bucket。
- API 不返回 bucket、object key、存储凭据或 MinIO 直连 URL。
- `content_sha256` 针对实际写入对象的完整字节计算，不信任客户端摘要。
- 摘要不用于 object key、自动去重、唯一约束或恶意文件判定。

## 验证状态

| 状态 | 下载 | 预览 | 可重新验证 |
|---|---|---|---|
| `validated` | 规范 MIME 的附件下载 | HTTP 409 | 否 |
| `legacy_unverified` | `application/octet-stream` 附件 | HTTP 409 | 是 |
| `validation_error` | `application/octet-stream` 附件 | HTTP 409 | 是 |
| `blocked` | 禁止 | HTTP 409 | 否 |

重新验证成功会更新 MIME、摘要、策略版本和时间；确认不合规时变为 `blocked`；临时存储错误时保留对象并标记 `validation_error`。

## 访问边界

- 下载 URL 从文件详情取得，并受 JWT、动态权限、到期时间、签名和用户绑定约束。
- 下载强制附件语义，并设置 `nosniff` 和私有缓存策略。
- 管理员文件预览兼容路由始终在读取对象前返回 HTTP 409。
- 改名只允许修改安全展示名称主体，不能改变规范扩展名、bucket 或 object key。
- MinIO 浏览只返回对象元数据，不自动创建文件记录或赋予验证状态。
- 文件轮转在对象移动成功后更新记录 bucket；数据库更新失败时尝试移回热 bucket。

## 验证重点

使用 Swagger UI 验证合法文件、超限、类型不一致、危险双扩展名、历史状态重新验证、签名下载、预览拒绝、改名边界、孤立对象浏览和删除补偿。稳定错误语义以 OpenSpec 为准。
