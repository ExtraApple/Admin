## ADDED Requirements

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

## MODIFIED Requirements

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
