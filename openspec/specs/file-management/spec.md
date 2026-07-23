# file-management Specification

## Purpose

文件管理允许管理员上传、查询、查看详情、改名、删除、浏览和轮转存储在 MinIO 中的文件，并在 MySQL 中维护文件元数据。

## Requirements

### Requirement: 管理员上传文件
系统 SHALL 允许具备 `admin.files.post` 权限的管理员通过 `POST /api/admin/files` 上传一个通过 V1 格式策略验证的文件，并在 MinIO 和数据库中保存受信任的对象及元数据。

#### Scenario: 上传成功
- **WHEN** 管理员上传一个非空、未超过配置上限且通过 V1 格式策略验证的文件
- **THEN** 系统 SHALL 使用服务端生成的 UUID object key 和规范扩展名将文件存入私有 `files` bucket
- **AND** 系统 SHALL 创建包含清洗展示名称、bucket、object key、规范 Content-Type、检测 MIME、大小、上传者 ID、`validated` 状态、策略版本和验证时间的数据库记录
- **AND** 响应 SHALL 返回文件元数据且不得返回 MinIO 凭据或原始对象直连 URL

#### Scenario: multipart 请求不合法
- **WHEN** 请求缺少名为 `file` 的文件 part、包含多个文件 part、文件为空或 multipart 格式损坏
- **THEN** 系统 SHALL 在写入 MinIO 前拒绝请求
- **AND** 系统 SHALL 返回 HTTP 400 和稳定安全错误码

#### Scenario: 上传文件超过大小限制
- **WHEN** 整个请求体或单个文件超过当前普通文件配置上限
- **THEN** 系统 SHALL 返回 HTTP 413
- **AND** 系统 SHALL NOT 完整解析超限 multipart 正文
- **AND** 系统 SHALL NOT 写入 MinIO 或数据库

#### Scenario: 文件类型不受支持或不一致
- **WHEN** 文件扩展名、声明 MIME、检测类型或格式验证结果不属于同一个 V1 允许类型
- **THEN** 系统 SHALL 在写入 MinIO 前拒绝文件
- **AND** 系统 SHALL 返回 HTTP 415 和稳定安全错误码

#### Scenario: 文件内容损坏或危险
- **WHEN** 文件属于允许扩展名但内容损坏、编码无效或无法完成完整策略检查
- **THEN** 系统 SHALL 在写入 MinIO 前拒绝文件
- **AND** 系统 SHALL 返回 HTTP 422

#### Scenario: MinIO 上传失败
- **WHEN** 文件验证通过但 MinIO 对象写入失败
- **THEN** 系统 SHALL NOT 创建数据库文件记录
- **AND** 系统 SHALL 返回不包含底层存储错误和对象路径的服务错误

#### Scenario: 对象上传后元数据写入失败
- **WHEN** MinIO 上传成功，但数据库元数据创建失败
- **THEN** 系统 SHALL 尝试删除刚上传的 MinIO 对象
- **AND** 系统 SHALL 返回不包含对象路径和底层数据库错误的服务错误
- **AND** 补偿删除失败 SHALL 只记录受控运行日志，不把对象视为已成功上传的文件记录

### Requirement: 文件上传安全配置
系统 SHALL 从 `config.yaml` 读取文件上传大小和临时访问 URL 有效期，并在服务启动时验证配置。

#### Scenario: 使用合法配置
- **WHEN** `file_upload.max_size_mb` 位于 1 到 100、`avatar_max_size_mb` 位于 1 到 10 且 `download_url_expire_seconds` 为正数
- **THEN** 系统 SHALL 使用这些值限制上传和临时访问 URL

#### Scenario: 使用非法配置
- **WHEN** 文件大小配置为零、负数或超过程序硬上限，或临时 URL 有效期不是正数
- **THEN** 系统 SHALL 启动失败
- **AND** 系统 SHALL 返回明确的配置项错误
- **AND** 系统 SHALL NOT 静默使用更宽松的回退值

### Requirement: 普通文件 V1 类型策略
系统 SHALL 只允许 PDF、UTF-8 TXT 和 UTF-8 CSV 进入管理员普通文件存储。

#### Scenario: 上传 PDF
- **WHEN** PDF 的 `.pdf` 扩展名、`application/pdf` 声明 MIME、文件头和结束标记一致
- **THEN** 系统 SHALL 接受文件
- **AND** 文件 SHALL 只能通过附件下载访问

#### Scenario: 上传 UTF-8 文本
- **WHEN** `.txt` 或 `.csv` 文件使用匹配的规范 MIME 且内容为带 BOM 或不带 BOM 的有效 UTF-8
- **THEN** 系统 SHALL 接受文件
- **AND** 系统 SHALL NOT 在普通上传阶段解析业务列或导入业务数据

#### Scenario: 上传非 UTF-8 文本
- **WHEN** TXT 或 CSV 包含无效 UTF-8、UTF-16、GBK、GB2312 或 NUL 内容
- **THEN** 系统 SHALL 拒绝上传
- **AND** 系统 SHALL NOT 猜测、转换或替换原编码

#### Scenario: 通过管理员文件接口上传图片
- **WHEN** 管理员通过普通文件接口上传 JPEG、PNG、WebP、SVG 或其他图片
- **THEN** 系统 SHALL 返回 HTTP 415 和稳定类型错误
- **AND** 系统 SHALL NOT 写入 MinIO 或数据库

#### Scenario: 上传危险或不支持格式
- **WHEN** 文件为可执行文件、脚本、HTML、压缩包、任意 Office 文档或其他不在白名单中的格式
- **THEN** 系统 SHALL 返回 HTTP 415
- **AND** 系统 SHALL NOT 写入 MinIO

#### Scenario: 上传危险双扩展名
- **WHEN** 文件名在允许的最终扩展名前包含 `.exe`、`.js`、`.html`、`.svg` 或其他危险扩展名
- **THEN** 系统 SHALL 拒绝上传，即使最终内容属于白名单格式

### Requirement: 管理员文件内容摘要
系统 SHALL 对管理员文件实际写入 MinIO 的内容计算服务端 SHA-256，并将摘要作为文件元数据持久化。

#### Scenario: 新文件上传建立摘要
- **WHEN** 管理员上传的 PDF、UTF-8 TXT 或 UTF-8 CSV 通过当前策略验证
- **THEN** 系统 SHALL 对实际传给 MinIO 的完整字节计算 SHA-256
- **AND** 系统 SHALL 将 64 个小写十六进制字符的 `content_sha256` 与文件记录一起持久化
- **AND** 上传、列表和详情文件元数据响应 SHALL 返回该摘要

#### Scenario: 历史文件重新验证建立摘要
- **WHEN** `legacy_unverified` 或 `validation_error` 文件通过当前策略重新验证
- **THEN** 系统 SHALL 对从 MinIO 读取并通过验证的完整对象字节计算 SHA-256
- **AND** 系统 SHALL 将摘要与新的验证状态、策略版本和验证时间一起更新

#### Scenario: 既有记录没有摘要
- **WHEN** 数据库中存在本策略上线前创建且 `content_sha256` 为空的文件记录
- **THEN** 系统 SHALL 保留该记录
- **AND** 系统 SHALL NOT 在服务启动时自动读取 MinIO 或批量补算摘要

#### Scenario: 摘要用途受限
- **WHEN** 系统生成或使用管理员文件摘要
- **THEN** 系统 SHALL NOT 信任客户端提供的摘要
- **AND** 系统 SHALL NOT 将摘要用作 object key、唯一约束、自动去重或恶意文件判定依据
- **AND** 系统 SHALL NOT 将摘要写入对象路径、临时访问 URL 或普通运行日志

### Requirement: 管理员文件列表
系统 SHALL 提供已上传文件的分页元数据和验证状态列表。

#### Scenario: 查询文件列表
- **WHEN** 管理员调用 `GET /api/admin/files`
- **THEN** 系统 SHALL 按创建时间倒序返回文件元数据
- **AND** 响应 SHALL 包含总数、页码和每页数量
- **AND** 每条记录 SHALL 包含验证状态、检测 MIME、策略版本、验证时间和稳定失败原因

#### Scenario: 按对象名前缀查询文件
- **WHEN** 请求提供 prefix 查询参数
- **THEN** 系统 SHALL 只返回 `object_name` 以前缀开头的文件

#### Scenario: 列表包含历史或封锁文件
- **WHEN** 查询结果包含 `legacy_unverified`、`validation_error` 或 `blocked` 文件
- **THEN** 系统 SHALL 明确返回对应状态
- **AND** 系统 SHALL NOT 因旧扩展名或旧 Content-Type 自动把记录展示为验证通过

### Requirement: 管理员文件详情
系统 SHALL 提供文件元数据、验证状态和符合当前访问策略的短期应用下载 URL。

#### Scenario: 验证通过的文件存在
- **WHEN** 管理员查询状态为 `validated` 的文件
- **THEN** 系统 SHALL 返回包含 `content_sha256` 的持久化文件元数据和短期有效的应用下载 URL
- **AND** URL 有效期 SHALL 使用 `file_upload.download_url_expire_seconds`
- **AND** 系统 SHALL NOT 返回预览 URL
- **AND** 系统 SHALL NOT 返回原始 MinIO 预签名 URL

#### Scenario: 历史未验证或临时验证错误文件存在
- **WHEN** 管理员查询状态为 `legacy_unverified` 或 `validation_error` 的文件
- **THEN** 系统 SHALL 返回文件元数据和强制附件下载 URL
- **AND** 系统 SHALL 允许 `content_sha256` 为空
- **AND** 系统 SHALL NOT 返回预览 URL

#### Scenario: 策略封锁文件存在
- **WHEN** 管理员查询状态为 `blocked` 的文件
- **THEN** 系统 SHALL 返回文件元数据和封锁原因
- **AND** 系统 SHALL NOT 返回下载 URL 或预览 URL

#### Scenario: 文件不存在
- **WHEN** 请求的文件 ID 不存在
- **THEN** 系统 SHALL 返回 HTTP 404 和文件不存在错误

### Requirement: 管理员文件下载和预览
系统 SHALL 通过受 JWT、动态 API 权限、到期时间和签名共同约束的应用接口流式提供文件下载，并 SHALL NOT 为管理员普通文件提供内联预览。

#### Scenario: 下载验证通过文件
- **WHEN** 管理员使用未过期且签名有效的 `GET /api/admin/files/:id/download` URL 访问 `validated` 文件
- **THEN** 系统 SHALL 从记录指定的私有 bucket 流式返回对象
- **AND** 响应 SHALL 设置规范 `Content-Type`、附件 `Content-Disposition`、清洗文件名、`X-Content-Type-Options: nosniff` 和私有缓存策略

#### Scenario: 下载历史未验证文件
- **WHEN** 管理员下载 `legacy_unverified` 或 `validation_error` 文件
- **THEN** 系统 SHALL 使用 `application/octet-stream` 和附件语义返回
- **AND** 系统 SHALL NOT 内联展示文件

#### Scenario: 下载零字节历史文件
- **WHEN** 管理员下载对象内容为零字节的 `legacy_unverified` 或 `validation_error` 文件
- **THEN** 系统 SHALL 成功返回零字节附件
- **AND** 系统 SHALL NOT 将正常的流结束 `io.EOF` 映射为存储不可用

#### Scenario: 对象首批字节伴随存储错误
- **WHEN** 文件对象 reader 在首批读取中同时返回数据和非 `io.EOF` 存储错误
- **THEN** 系统 SHALL 在提交成功响应前关闭 reader 并返回对应的受控存储错误
- **AND** 系统 SHALL NOT 吞掉错误或把部分对象内容作为成功响应返回

#### Scenario: 尝试预览管理员普通文件
- **WHEN** 管理员调用 `GET /api/admin/files/:id/preview` 访问任意管理员普通文件
- **THEN** 系统 SHALL 在打开 MinIO 对象前返回 HTTP 409 和稳定状态错误
- **AND** 系统 SHALL NOT 返回对象内容

#### Scenario: 临时 URL 无效
- **WHEN** 下载 URL 已过期、签名错误、访问模式不匹配或签名绑定用户与当前用户不一致
- **THEN** 系统 SHALL 拒绝访问

### Requirement: 管理员修改文件元数据
系统 SHALL 允许管理员修改文件展示名称主体，但 SHALL NOT 修改 MinIO object key、已验证类型或规范扩展名。

#### Scenario: 修改文件名主体
- **WHEN** 管理员提交清洗后非空且不超过 255 个 Unicode 字符的名称主体
- **THEN** 系统 SHALL 更新数据库中的展示名称主体
- **AND** 系统 SHALL 保留原有规范扩展名、bucket 和 object key

#### Scenario: 修改扩展名或提交危险名称
- **WHEN** 管理员尝试改变扩展名、提交路径、控制字符、双向文本控制字符或危险双扩展名
- **THEN** 系统 SHALL 拒绝修改
- **AND** 原文件名称、验证状态、bucket 和 object key SHALL 保持不变

#### Scenario: 清洗后名称为空
- **WHEN** 提交名称在清洗后没有有效主体
- **THEN** 系统 SHALL 拒绝修改并返回稳定文件名错误

### Requirement: 历史文件验证状态和重新验证
系统 SHALL 兼容保留历史文件，并允许管理员显式重新验证单个历史对象。

#### Scenario: 历史记录迁移
- **WHEN** V1 数据字段首次上线
- **THEN** 系统 SHALL 将既有文件记录回填为 `legacy_unverified`
- **AND** 迁移 SHALL NOT 读取、解压、删除或重写 MinIO 对象

#### Scenario: 上传类型策略收窄
- **WHEN** 本策略上线时存在状态为 `validated` 且规范 MIME 不属于 PDF、TXT、CSV 的管理员文件
- **THEN** 系统 SHALL 将该记录更新为 `legacy_unverified`
- **AND** 系统 SHALL 保留对象和原有元数据
- **AND** 迁移 SHALL NOT 读取、删除或重写 MinIO 对象

#### Scenario: 重新验证成功
- **WHEN** 管理员通过 `POST /api/admin/files/:id/revalidate` 重新验证 `legacy_unverified` 或 `validation_error` 文件且对象通过当前策略
- **THEN** 系统 SHALL 更新规范 MIME、检测 MIME、`content_sha256`、状态 `validated`、策略版本和验证时间
- **AND** 系统 SHALL 只提供附件下载能力

#### Scenario: 重新验证确认不合规
- **WHEN** 重新验证发现文件是图片、Office、其他不支持类型、类型不一致、内容损坏或危险内容
- **THEN** 系统 SHALL 将文件状态更新为 `blocked`
- **AND** 系统 SHALL 禁止该文件下载和预览
- **AND** 系统 SHALL NOT 在 V1 自动删除对象

#### Scenario: 重新验证遇到临时错误
- **WHEN** MinIO、网络或其他临时错误导致无法完成检查
- **THEN** 系统 SHALL 将状态更新为 `validation_error`
- **AND** 系统 SHALL 保留对象并允许以后重试
- **AND** 系统 SHALL NOT 把临时错误标记为危险内容

#### Scenario: 对已验证或封锁文件重复重新验证
- **WHEN** 管理员对 `validated` 或 `blocked` 文件调用 V1 重新验证接口
- **THEN** 系统 SHALL 返回 HTTP 409
- **AND** 系统 SHALL 保持文件状态不变

#### Scenario: 系统启动时存在历史文件
- **WHEN** 服务启动或执行数据库自动迁移
- **THEN** 系统 SHALL NOT 自动批量扫描历史文件

### Requirement: 管理员删除文件
系统 SHALL 同时删除文件的 MinIO 对象和数据库元数据。

#### Scenario: 删除文件成功
- **WHEN** 管理员删除存在的文件
- **THEN** 系统删除 MinIO 对象
- **AND** 系统硬删除数据库元数据记录

#### Scenario: MinIO 删除失败
- **WHEN** MinIO 对象无法删除
- **THEN** 系统不删除数据库元数据记录
- **AND** 系统返回错误

### Requirement: MinIO 文件浏览
系统 SHALL 允许管理员按前缀浏览 `files` bucket 中的对象元数据，但 SHALL NOT 因浏览结果创建文件记录或赋予验证状态。

#### Scenario: 浏览文件
- **WHEN** 管理员调用浏览接口并提供可选 prefix
- **THEN** 系统 SHALL 返回 `files` bucket 中的 MinIO 对象元数据
- **AND** 响应 SHALL NOT 返回对象内容、存储凭据或绕过文件访问策略的下载 URL

#### Scenario: 浏览到孤立对象
- **WHEN** MinIO 对象没有对应数据库文件记录
- **THEN** 系统 SHALL 只把它作为孤立对象元数据返回
- **AND** 系统 SHALL NOT 自动导入、自动验证或允许通过文件详情接口下载该对象

### Requirement: 文件轮转
系统 SHALL 支持按配置将旧文件从热 bucket 轮转到冷 bucket。

#### Scenario: 文件轮转未启用
- **WHEN** 配置未启用文件轮转
- **THEN** 系统跳过轮转

#### Scenario: 轮转符合条件的文件
- **WHEN** 文件轮转已启用
- **AND** 热 bucket 中存在超过配置天数阈值的文件
- **THEN** 系统最多处理配置的 batch size 数量
- **AND** 系统将文件从热 bucket 移动到冷 bucket
- **AND** 移动成功后更新数据库元数据中的 bucket 字段

#### Scenario: 轮转时数据库更新失败
- **WHEN** 对象移动成功，但数据库 bucket 字段更新失败
- **THEN** 系统尝试将对象移回热 bucket
- **AND** 系统继续处理后续文件


