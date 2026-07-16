## Context

管理员普通文件上传目前在 handler 解析 multipart 后才检查单文件大小，service 直接信任客户端文件名、扩展名和 `Content-Type`，随后把数据写入 `files` bucket。用户头像只检查扩展名和 2 MiB 大小，原文件直接写入 `image` bucket，数据库保存完整 URL；`PUT /api/user/info` 还可以绕过头像上传接口写入任意头像 URL。

文件详情当前返回 MinIO 预签名 URL，无法在访问时根据验证状态实施下载/预览策略，也无法稳定设置应用要求的附件语义和 `X-Content-Type-Options: nosniff`。`files` 表没有验证状态，审计日志虽然会省略 multipart 正文，但不能记录格式校验结果。

该变更横跨配置、上传解析、格式校验、图片处理、Office Open XML、MinIO、数据库迁移、审计、下载和用户资料，因此需要在现有 `handler -> service -> model/dto/utils` 分层内引入一套可独立测试的安全策略，而不是继续在两个 handler 中累积条件判断。

## Goals / Non-Goals

**Goals:**

- 在 multipart 解析和 MinIO 写入前实施有硬上限的单文件上传策略。
- 为普通文件和头像复用类型识别、文件名处理、错误分类和审计模型，同时保留用途差异。
- 对允许的图片、PDF、UTF-8 文本和 Office Open XML 实施可测试的内容校验。
- 让文件验证状态成为下载、预览、历史迁移和审计行为的依据。
- 头像只保存标准化后的 JPEG/PNG，并从资料修改接口收口到专用头像接口。
- 所有安全校验可用内存数据、临时文件和伪存储实现测试，不依赖真实 MinIO。

**Non-Goals:**

- 不接入 ClamAV、ICAP、云恶意文件扫描或隔离区。
- 不开放 ZIP/RAR/7Z、旧版 Office、宏文档、HTML、SVG、脚本和可执行文件。
- 不实现 CSV/XLSX 业务数据导入、模板字段校验或事务导入。
- 不实现多文件上传、断点续传、上传频率限制或用户存储配额。
- 不在启动、数据库迁移或首次访问时批量读取历史 MinIO 对象。
- 不对普通文件中的 PDF、Office、TXT、CSV 提供浏览器内联预览。

## Decisions

### 1. 使用用途策略驱动的统一校验管线

service 层引入可独立测试的文件安全组件，输入包含上传用途、声明文件名、声明 MIME、大小和受限数据源，输出统一的验证结果：

- 清洗后的展示名称；
- 规范扩展名和规范 MIME；
- 检测 MIME；
- 验证状态、策略版本和稳定原因码；
- 可写入 MinIO 的数据源；
- 对头像额外返回标准化后的尺寸、格式和数据。

上传用途至少区分 `managed_file` 和 `avatar`。类型识别、危险扩展名、空文件、名称清洗及错误类型共用；普通文件使用白名单和 Office/文本/PDF 校验，头像使用完整图片解码和重新编码。

选择 service 层组件而不是 handler 工具函数，是为了让 handler 只负责 HTTP 映射，让 MinIO 写入、数据库回滚和重新验证共享同一规则。组件不得依赖 Gin 或全局 MinIO 客户端。

### 2. 在表单解析前限制请求体，并继续只接收一个文件

上传路由在调用 `FormFile` 或 `ParseMultipartForm` 前使用受限 reader：

- 管理员普通文件请求体上限为配置文件上限加 1 MiB multipart 固定开销。
- 头像请求体上限为配置头像上限加 256 KiB multipart 固定开销。
- 请求中必须且只能存在一个名为 `file` 的文件 part；额外文件 part 被拒绝。
- 空文件被拒绝。

`file_upload.max_size_mb` 默认 50、程序硬上限 100 MiB；`avatar_max_size_mb` 默认 2、程序硬上限 10 MiB。配置为零、负数或超过硬上限时，配置初始化失败。请求体或文件大小超限使用 HTTP 413，不继续读取完整正文。

普通文件可以落到受大小约束的临时输入文件，以满足 ZIP `ReaderAt` 和多阶段检测要求；不得把 50 MiB 文件一次性复制到内存。Office 条目只允许流式读取和校验，不把容器完整解压到目录。头像因输入上限较小可以读取受限字节，但解码前必须先读取图片头部尺寸。

### 3. 使用规范类型表和格式专用验证器

V1 普通文件规范映射为：

| 类型 | 扩展名 | 声明 MIME |
| --- | --- | --- |
| JPEG | `.jpg`、`.jpeg` | `image/jpeg` |
| PNG | `.png` | `image/png` |
| WebP | `.webp` | `image/webp` |
| PDF | `.pdf` | `application/pdf` |
| DOCX | `.docx` | `application/vnd.openxmlformats-officedocument.wordprocessingml.document` |
| XLSX | `.xlsx` | `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` |
| PPTX | `.pptx` | `application/vnd.openxmlformats-officedocument.presentationml.presentation` |
| TXT | `.txt` | `text/plain` |
| CSV | `.csv` | `text/csv` |

声明扩展名、声明 MIME、签名检测和格式专用验证结果必须落在同一个规范类型。`application/octet-stream` 不作为白名单 MIME 放行。检测先使用文件签名/MIME 分类器，再由对应专用验证器确认；分类器结果不能绕过专用验证器。

- JPEG、PNG、WebP 必须能够完整解码，普通文件图片不重新编码。
- PDF 必须具有合法 PDF 头和结束标记，且只允许附件下载；V1 不承诺完整 PDF 语义或恶意脚本扫描。
- TXT、CSV 必须是有效 UTF-8，可带或不带 BOM，且不得包含 NUL；不猜测或转换 GBK、UTF-16 等编码。CSV 在该阶段不校验业务列。
- 中间包含危险扩展名的双扩展名，例如 `payload.exe.pdf`、`script.js.txt` 或 `image.svg.png`，即使最终内容合法也拒绝；普通的非危险多点名称仍可保留。

类型分类可以使用现有依赖树中的 `github.com/gabriel-vasile/mimetype`，图片解码和缩放使用 Go 标准库及 `golang.org/x/image`。这两个依赖在实现时转为直接依赖；不增加本地原生库或外部服务。

### 4. Office Open XML 使用受限 ZIP 和关系白名单校验

DOCX、XLSX、PPTX 不因为 ZIP 可打开就视为合法。验证器按流式方式执行：

- ZIP 条目最多 2,000 个。
- 单条目解压后最多 50 MiB，累计解压后最多 200 MiB。
- 单条目及整体压缩比均不得超过 100:1；压缩大小异常为零而解压内容非空时拒绝。
- 拒绝重复异常名称、绝对路径、反斜线绕过、盘符路径、`..` 路径穿越、损坏中央目录和嵌套归档内容。
- 必须存在 `[Content_Types].xml`、包关系文件和与扩展名对应的 `word/`、`xl/` 或 `ppt/` 主体及主文档关系。
- 内容类型、主关系、主体目录、扩展名和声明 MIME 必须一致。
- 拒绝宏内容类型、宏关系、`vbaProject.bin`、ActiveX、OLE、嵌入包、嵌入文件、脚本、可执行内容和加密/密码保护容器。
- 所有 `TargetMode="External"` 关系默认拒绝；只有关系类型为普通 hyperlink 且目标 scheme 为 HTTP 或 HTTPS 时允许。
- XML 解析禁用 DTD 和外部实体，拒绝 `DOCTYPE`，最大嵌套深度 100；XML 字节读取继续受单条目和累计解压上限约束。

验证只证明容器符合 V1 格式策略，不把结果命名为“病毒扫描通过”或“文件安全”。

### 5. 文件名和 object key 分离

文件展示名称先把 `\` 规范为 `/`，只保留最后一个路径段，然后：

- 移除控制字符、换行、双向文本控制字符；
- 清理首尾空格和点，合并连续空白；
- 允许中英文、数字、空格、连字符、下划线、点和常用中英文括号；
- 最长 255 个 Unicode 字符，超出时在保留规范扩展名的前提下截断；
- 清洗后主体为空时使用 `file` 或 `avatar` 安全默认主体。

普通文件记录只保存清洗后的展示名称。改名接口只修改主体，服务端保留已经验证的规范扩展名；客户端尝试改变、增加或伪装扩展名时返回验证错误。

MinIO object key 使用 UUID 和服务端选择的规范扩展名。普通文件不使用展示名称组成路径；头像只允许服务端生成 `avatars/<user-id>/<uuid>.<jpg|png>`。

### 6. 头像先检查尺寸，再完整解码和重新编码

头像输入只接受 JPEG、PNG、WebP，并采用以下固定安全上限：

- 单边最大 8,192 像素；
- 总像素最多 40,000,000；
- 输出最大 1,024×1,024，保持宽高比；
- 输出大小仍不得超过 `avatar_max_size_mb`。

验证器先使用图片头部读取尺寸，超限时不得进行完整像素分配；随后完整解码并标准化方向。动画 WebP 和 APNG 被拒绝，不取第一帧降级放行。

解码后的像素重新编码会移除 EXIF、定位、设备和其他非像素元数据：

- 无透明通道输出质量 85 的 JPEG；
- 有透明通道输出 PNG；
- WebP 仅作为输入格式，输出统一为 JPEG 或 PNG。

只有标准化输出成功后才写入 `image` bucket。数据库保存 `avatar_object_name`、规范 MIME 和头像验证状态，不保存客户端 URL。API DTO 不暴露 object key 或 MinIO URL，而是按以下固定规则返回应用地址：

- 只有 `validated` 状态、object name 归属于当前用户、UUID 和 `.jpg`/`.png` 扩展名规范且 MIME 一致时，返回 `/api/avatars/<user-id>`；
- 没有头像、只有历史 `avatar` URL、状态或对象标识不可信时，返回 `/api/avatars/default`；
- 历史 URL 不得被请求、代理、重定向或内联。

应用提供公开只读的 `GET /api/avatars/:user_id` 与 `GET /api/avatars/default`。头像读取不要求 JWT，使普通 `<img src>` 可以直接使用；接口只允许输出服务端标准化后的 JPEG/PNG 或应用内置的默认 PNG，不接受客户端 object key，不暴露私有 `image` bucket。用户不存在、没有可信头像、可信对象缺失或暂时无法读取时统一返回默认头像，避免泄露用户或存储状态。响应设置规范 `Content-Type`、`X-Content-Type-Options: nosniff` 和 `Cache-Control: public, max-age=300`；默认头像端点可以使用 `Cache-Control: public, max-age=86400`。

### 7. 文件和头像使用显式验证状态

`files` 表增加：

- `detected_content_type`
- `validation_status`
- `validation_policy_version`
- `validation_error_code`
- `validated_at`

状态常量为：

- `legacy_unverified`：策略上线前的历史记录；
- `validated`：通过当前格式策略；
- `blocked`：确认不支持、损坏或危险；
- `validation_error`：因 MinIO、网络或其他临时错误未能完成验证。

新上传文件只有验证通过后才允许创建记录，并显式写入 `validated`。历史记录迁移为 `legacy_unverified`。单文件重新验证成功后更新规范 MIME、状态、时间和策略版本；确认违反策略时更新为 `blocked`；临时错误时更新为 `validation_error`，但保留对象并允许再次调用重新验证。

`users` 表以新增字段保存 `avatar_object_name`、`avatar_content_type`、`avatar_validation_status` 和 `avatar_validated_at`。旧 `avatar` 字段仅作为迁移前遗留数据保留，新代码不再把它作为可信资源。历史自定义头像标记为未验证并返回默认头像，不在迁移中删除或读取旧对象。

策略版本使用稳定常量，例如 `file-upload-v1`，用于未来策略升级和重新验证。

### 8. 下载和预览通过受控应用接口

不再把 MinIO 原始预签名 URL 作为最终下载地址。`GET /api/admin/files/:id` 返回文件状态以及短期有效的应用下载 URL；仅可预览文件同时返回预览 URL。

临时 URL 使用 `expires` 和 HMAC-SHA256 签名，签名绑定当前用户 ID、文件 ID、访问模式、到期时间和验证状态。签名密钥从 JWT secret 通过固定用途字符串派生，避免新增密钥；路由仍要求 JWT 和动态 API 权限，签名不是权限校验的替代品。有效期使用 `file_upload.download_url_expire_seconds`，默认 300 秒。

- `GET /api/admin/files/:id/download`：`validated` 文件使用规范 MIME，其他允许下载的历史文件使用 `application/octet-stream`；始终设置附件 `Content-Disposition`、清洗名称、`X-Content-Type-Options: nosniff` 和私有缓存策略。
- `GET /api/admin/files/:id/preview`：仅允许 `validated` 的 JPEG、PNG、WebP，设置规范 MIME、内联 disposition、`nosniff` 和限制性缓存策略。
- `blocked` 文件不生成访问 URL且下载、预览均拒绝。
- `legacy_unverified` 和 `validation_error` 文件只生成强制附件下载 URL，不生成预览 URL。

应用从记录指定的私有 bucket 流式读取对象，因此轮转到冷 bucket 后仍按同一规则访问。MinIO 孤立对象不通过文件 API 暴露，也不因浏览接口存在而自动创建文件记录。

对象 reader 在成功响应提交前执行一次显式预读。预读区分调用用途：文件下载允许历史零字节对象，把首读 `(0, io.EOF)` 视为成功空流；头像读取拒绝空流并回退内置默认 PNG。首读 `(n > 0, io.EOF)` 重放已经读取的完整短流；任何伴随首批字节返回的非 `io.EOF` 错误都必须关闭 reader 并原样进入受控存储错误映射，不得被已读取字节掩盖。

头像读取同样通过应用代理，但不复用管理员文件下载签名：头像只包含已经重新编码的公开展示图片，地址不携带 object key，读取接口按用户记录再次确认可信对象标识。这样避免公开 `image` bucket，也避免要求前端为普通 `<img>` 请求额外获取 JWT 后再转换 Blob。

### 9. 明确上传和头像替换的补偿语义

所有格式验证在对象写入前完成：

- MinIO 写入失败时不创建数据库记录。
- 普通文件对象写入成功但数据库创建失败时，立即删除新对象；删除补偿失败只记录受控运行日志和审计原因，接口仍返回内部错误，不返回 object key 或底层错误。
- 头像先写入新对象，再更新用户对象标识；只有仓储明确确认数据库更新未提交时才删除新对象。
- 数据库更新返回错误但提交结果不确定时，接口返回受控持久化错误，但保留可能已经被用户记录引用的新对象，也不清理旧对象，为后续对账和孤立对象清理保留安全空间。
- 头像数据库更新成功后再尝试删除旧的系统生成头像对象；旧对象删除失败不回滚已经成功的新头像，只记录清理失败。
- 历史外部 URL 或无法确认归属的旧头像对象不自动删除。
- 恢复默认头像时先更新数据库状态，再删除旧的系统头像对象；删除失败采用同样的异步可清理语义。
- 用户已经处于默认头像状态时，恢复操作直接返回默认结果，不执行无变化 UPDATE，也不把 MySQL `RowsAffected = 0` 误判为持久化失败。
- 删除旧的 `SetAvatar` URL 写入入口；应用层不得继续提供把客户端 URL 持久化到兼容 `avatar` 字段的旁路。

这样避免先删除旧头像导致用户无头像，也避免把数据库事务扩展到不可事务化的 MinIO 操作。

### 10. 使用稳定错误码和 HTTP 状态

错误响应保留现有数字 `code`，并新增稳定字符串 `error_code`；`msg` 为可展示的中文信息，不包含底层解析错误。

| HTTP | 错误类别 |
| --- | --- |
| 400 | multipart 缺失/格式错误、文件名或请求参数错误 |
| 413 | 请求体、上传文件或解压资源超过上限 |
| 415 | 扩展名、声明 MIME、检测类型不匹配或类型不在白名单 |
| 422 | 文件损坏、编码无效、危险 Office 关系、宏、加密或图片解码失败 |
| 404 | 文件记录或对象不存在 |
| 409 | 当前验证状态不允许预览、下载或重复操作 |
| 500/503 | 数据库、MinIO 或临时基础设施错误 |

稳定原因码至少覆盖 `UPLOAD_BODY_TOO_LARGE`、`FILE_EMPTY`、`FILE_TOO_LARGE`、`FILE_NAME_INVALID`、`FILE_TYPE_NOT_ALLOWED`、`FILE_TYPE_MISMATCH`、`FILE_ENCODING_INVALID`、`FILE_CONTENT_INVALID`、`IMAGE_DIMENSION_LIMIT`、`IMAGE_DECODE_INVALID`、`OOXML_INVALID`、`OOXML_DANGEROUS_CONTENT`、`OOXML_RESOURCE_LIMIT`、`FILE_STATE_BLOCKED`、`STORAGE_UNAVAILABLE` 和 `PERSISTENCE_FAILED`。

### 11. 审计通过 Gin context 传递结构化安全元数据

上传 handler/service 把安全结果写入 Gin context，现有审计中间件在 `c.Next()` 后读取并写入新增的 `metadata` 字段。元数据使用受控结构：

- `purpose`
- `file_name`
- `file_size`
- `declared_mime`
- `detected_mime`
- `validation_result`
- `reason_code`
- `policy_version`

审计表和归档表都增加 `metadata`，归档时原样复制。multipart `Body` 继续固定记录 `[multipart omitted]`。元数据不得包含文件字节、原始未清洗路径、预签名/签名 URL、MinIO 凭据、object key 或解析器原始错误。

### 12. 历史重新验证采用显式单文件操作

V1 新增 `POST /api/admin/files/:id/revalidate`，只对 `legacy_unverified` 和 `validation_error` 状态执行。它从记录中的 bucket 流式读取对象并复用普通文件验证器：

- 成功后变为 `validated`；
- 策略不合规后变为 `blocked`；
- 临时读取错误后变为 `validation_error`；
- 不重写对象，不在一次请求中处理多个文件。

批量、定时、限速和可续跑重新验证任务留到后续 change。MinIO 浏览接口只返回对象元数据，不赋予验证状态，也不自动认领孤立对象。

## Risks / Trade-offs

- [严格声明 MIME 可能拒绝客户端以 `application/octet-stream` 上传的合法文件] → V1 选择可解释的严格策略；客户端必须发送规范 MIME，后续仅通过 OpenSpec 和测试扩展兼容映射。
- [Office 校验不能替代反病毒扫描] → API、状态和文档只表述为“格式策略验证通过”，危险类型继续拒绝，ClamAV 留到 V2。
- [通过应用代理下载增加服务带宽和连接占用] → 使用流式 MinIO reader、不缓冲完整对象，并保留短期签名 URL；未来可在可信反向代理层实现等价响应头后重新评估直连。
- [图片完整解码仍有 CPU 成本] → 先做文件、尺寸和像素上限检查，再完整解码；头像输入默认只有 2 MiB。
- [历史文件短期内仍可被管理员下载] → 仅强制 `application/octet-stream` 附件下载，禁用预览；确认不合规后立即封锁。
- [MinIO 补偿删除可能失败并产生孤立对象] → 不泄漏对象路径，记录受控运行日志；孤立对象只允许运维查看，后续独立清理。
- [新增状态字段和受控 URL 会影响现有前端] → DTO 保留清晰状态字段并在 OpenAPI 文档中更新；部署时前后端需同步处理 `download_url`、`preview_url` 和默认头像。
- [公开头像读取可能被用于枚举用户 ID] → 不可信、缺失和不存在用户统一返回默认头像，不返回用户资料或对象状态；接口只输出标准化图片。

## Migration Plan

1. 增加配置结构和启动校验，在 `config.yaml` 写入三个 `file_upload` 默认值。
2. 通过 GORM AutoMigrate 增加文件、用户和审计字段；随后执行幂等补数据：
   - 现有文件的空验证状态写为 `legacy_unverified`；
   - 现有用户头像记录写为历史未验证，系统默认头像可标记为默认状态；
   - 不读取 MinIO、不删除对象。
3. 部署新的验证器、上传 service、受控下载/预览、单文件重新验证和头像标准化流程。
4. 同步新增路由到 API 元数据和权限表，为管理员角色补齐新权限；更新 OpenAPI 文档。
5. 验证新上传、历史下载、封锁、头像回退和失败补偿后，再让前端使用新的状态及 URL 字段。

回滚时保留新增数据库字段，不执行破坏性降级。旧 `avatar` 字段和历史对象在 V1 中不删除，因此旧版本可回退到迁移前头像数据；回滚版本不会识别新头像对象和文件验证状态，回滚前应停止新上传。已经写入的新对象由后续重新部署 V1 或运维清理处理。

## Open Questions

无。V1 的产品边界和实现默认值已在本 change 中固化；病毒扫描、批量重新验证、配额、限流和新增文件类型通过后续独立 change 处理。
