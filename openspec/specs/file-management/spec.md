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
系统 SHALL 兼容保留历史文件，并允许管理员显式重新验证单个历史对象。普通文件V1类型降级 SHALL 仅作用于managed_file及兼容旧NULL/空用途，不改变消息图片专用用途已验证状态。

#### Scenario: 历史记录迁移
- **WHEN** V1 数据字段首次上线
- **THEN** 系统 SHALL 将既有文件记录回填为 `legacy_unverified`
- **AND** 迁移 SHALL NOT 读取、解压、删除或重写 MinIO 对象

#### Scenario: 上传类型策略收窄
- **WHEN** 本策略上线时存在状态为 `validated` 且规范 MIME 不属于 PDF、TXT、CSV 的管理员文件
- **THEN** 系统 SHALL 将该记录更新为 `legacy_unverified`
- **AND** 系统 SHALL 保留对象和原有元数据
- **AND** 迁移 SHALL NOT 读取、删除或重写 MinIO 对象
- **AND** 该普通文件降级 SHALL NOT 作用于message_image专用用途

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
- **THEN** 系统 SHALL NOT 为V1重新验证而自动批量读取或扫描历史存储对象
- **AND** 本限制 SHALL NOT 禁止迁移完成后独立后台任务按消息图片清理契约处理过期未绑定图片

#### Scenario: 新队列迁移不降低消息图片验证状态
- **WHEN** 启动迁移创建消息图片清理队列且已有validated消息图片
- **THEN** 系统 SHALL 保持这些消息图片的验证状态
- **AND** 重复迁移 SHALL NOT 预登记清理任务、删除对象或自动升级未知历史验证状态

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
系统 SHALL 支持按配置将旧文件从热 bucket 轮转到冷 bucket。所有未绑定message_image和存在pending/dead清理队列的对象 SHALL 排除轮转；普通文件与已绑定且无清理队列的图片继续遵守轮转规则。

#### Scenario: 文件轮转未启用
- **WHEN** 配置未启用文件轮转
- **THEN** 系统跳过轮转

#### Scenario: 轮转符合条件的文件
- **WHEN** 文件轮转已启用
- **AND** 热 bucket 中存在超过配置天数阈值且未被清理保护排除的文件
- **THEN** 系统最多处理配置的 batch size 数量
- **AND** 系统将文件从热 bucket 移动到冷 bucket
- **AND** 移动成功后更新数据库元数据中的 bucket 字段

#### Scenario: 轮转时数据库更新失败
- **WHEN** 对象移动成功，但数据库 bucket 字段更新失败
- **THEN** 系统尝试将对象移回热 bucket
- **AND** 系统继续处理后续文件

#### Scenario: 未绑定消息图片不参与轮转
- **WHEN** message_image尚未绑定，无论绑定期限是否已过以及是否已登记清理
- **THEN** 系统 SHALL NOT 将该图片选作轮转候选
- **AND** 清理候选与可轮转图片 SHALL 不重叠

#### Scenario: 切换前排空旧轮转
- **WHEN** 部署首次启用C5清理
- **THEN** 部署过程 SHALL 先停止旧实例及其在途Copy和反向Move，再启动新清理
- **AND** 系统 SHALL NOT 将“检查热冷桶不存在”当作旧复制已经结束的证据



### Requirement: 消息图片专用用途与可见性
系统 SHALL 支持内部消息调用的专用 JPEG、PNG、WebP 临时图片用途，并按消息当前可见性控制读取；该能力 SHALL NOT 放宽普通管理员文件入口的拒绝策略。已登记pending/dead清理的图片 SHALL 拒绝新的绑定与读取资格查询。

#### Scenario: 临时消息图片绑定
- **WHEN** 图片通过完整格式、大小和尺寸验证且在 15 分钟绑定期内被消息创建、编辑或发布引用
- **THEN** Files SHALL 在同一事务中绑定当前操作者持有的临时记录到逻辑消息
- **AND** 过期、已绑定或不属于操作者的记录 SHALL 被拒绝
- **AND** Files SHALL 在与清理登记一致的短事务行锁协议内检查无pending/dead清理记录，不能使用过时请求时间绕过绑定期

#### Scenario: 消息图片读取和审计
- **WHEN** 用户读取消息图片
- **THEN** Files SHALL 先验证消息当前可见性并只返回规范 MIME 的图片
- **AND** 响应和 Audit metadata SHALL NOT 包含图片内容、MinIO bucket、object key、签名 URL、存储凭据或解析器原始错误

#### Scenario: 通过消息图片入口上传
- **WHEN** 已认证用户或具备消息管理权限的管理员通过消息图片专用入口上传 JPEG、PNG 或 WebP
- **AND** 图片通过完整解码、大小、格式和尺寸验证
- **THEN** Files SHALL 创建带消息图片用途、上传者引用和 15 分钟绑定时限的临时 File Record
- **AND** 临时记录 SHALL NOT 具有消息所有者引用，普通管理员文件上传接口 SHALL NOT 接受该图片

#### Scenario: 普通文件入口上传图片保持拒绝
- **WHEN** 客户端通过 `POST /api/admin/files` 上传 JPEG、PNG、WebP、SVG 或其他图片
- **THEN** 系统 SHALL 保持 HTTP 415 拒绝行为
- **AND** 系统 SHALL NOT 因消息图片用途写入普通文件记录

#### Scenario: 临时图片过期清理
- **WHEN** 临时消息图片超过 15 分钟绑定时限且尚未绑定
- **THEN** Files SHALL 在有界后台任务中先持久化清理队列，再删除受信范围内对象
- **AND** 对象删除全部成功或确认不存在后 SHALL 同事务物理删除File Record和队列
- **AND** 对象删除失败 SHALL 保留可定位记录，按有限重试与dead规则处理，不保证到期瞬间完成存储删除

#### Scenario: 消息图片不可见时拒绝读取
- **WHEN** 关联消息已撤销、私信关系被删除、消息已过期或用户不再符合动态受众
- **THEN** Files SHALL 拒绝图片读取
- **AND** 系统 SHALL NOT 返回图片内容或存储状态

#### Scenario: 消息图片安全审计
- **WHEN** 消息图片验证成功或因大小、格式、解码、尺寸策略被拒绝
- **THEN** Audit metadata SHALL 记录用途、清洗文件名、大小、声明 MIME、检测 MIME、`accepted`/`rejected` 结果和策略版本或稳定原因码
- **AND** metadata SHALL NOT 记录图片内容、object key、签名 URL、MinIO 凭据或解析器原始错误

#### Scenario: 清理登记先于绑定提交
- **WHEN** 清理登记已提交且后续绑定或读取资格查询开始
- **THEN** Files SHALL 拒绝该图片的新绑定与读取
- **AND** 系统 SHALL NOT 宣称能撤回此前已交付的reader

#### Scenario: 绑定先于清理登记提交
- **WHEN** 合法绑定先提交，清理随后重新检查同一File Record
- **THEN** Files SHALL NOT 登记或删除已绑定图片
- **AND** 多图片绑定 SHALL 保持消息外层事务原子性和稳定ID锁顺序

### Requirement: 消息图片持久化清理队列

Files SHALL 拥有 `message_image_cleanup_jobs` 队列，包含稳定ID、唯一file_id、冻结的完整bucket/object_name、唯一SHA-256路径键、pending/dead状态、retry_count、next_retry_at、last_error_code及创建更新时间。`retry_count` SHALL 由数据库 CHECK 约束限制在0–24；路径键 SHALL 按bucket、NUL分隔符和object_name原始字节生成，不使用文件内容摘要；原始错误 SHALL NOT 持久化。

每个对象 SHALL 逐条在短数据库事务中锁定并重查用途、未绑定和到期资格，提交队列后才调用存储。系统 SHALL NOT 在事务重试回调内执行存储操作，不增加File Record清理状态字段、processing状态或租约。

#### Scenario: 登记失败不丢失对象定位
- **WHEN** 队列写入或事务提交失败
- **THEN** File Record SHALL 保留且本次 SHALL NOT 执行对象删除
- **AND** 系统 SHALL 记录受控失败并继续其他候选

#### Scenario: 同对象重复登记
- **WHEN** 多实例登记同一路径和File Record
- **THEN** 数据库 SHALL 仅保留一个队列记录
- **AND** 已有pending或dead的重试次数与状态 SHALL NOT 被重新初始化

#### Scenario: 路径键冲突
- **WHEN** hash相同但完整路径不同，或同路径对应不同File Record
- **THEN** 系统 SHALL 拒绝新登记、保留原文件、输出受控冲突告警
- **AND** 系统 SHALL NOT 覆盖、截断路径或追加冲突序号绕过去重约束

#### Scenario: 存储成功而数据库终结失败
- **WHEN** 对象已删除但File Record和队列的终结事务失败
- **THEN** 队列 SHALL 保留以便后续幂等恢复
- **AND** 系统 SHALL NOT 把该次数据库失败统计为完整清理成功

### Requirement: 消息图片清理有限重试

初次删除 SHALL 不计入24次重试；后续pending到期后，系统 SHALL 以条件更新预留一次重试并把next_retry_at设为当前时间后一小时，成功预留后才执行存储。预留后崩溃 SHALL 消耗一次不确定调度尝试，不额外无限补次数。

第24次重试失败 SHALL 转dead；第24次预留后崩溃的pending SHALL 在下次资格时间转dead而不执行第25次存储尝试。成功或确定对象不存在后，系统 SHALL 条件物理删除File Record与队列。dead SHALL 保留人工处理，不再自动重试或重新登记。

#### Scenario: 多实例竞争同次重试
- **WHEN** 多实例读取相同pending记录和重试计数
- **THEN** 仅条件预留成功者 SHALL 执行本次存储尝试
- **AND** 其他实例 SHALL 跳过且不能重复递增计数

#### Scenario: 删除对象不存在
- **WHEN** 所有受信目标均已删除或确定为NoSuchKey
- **THEN** 系统 SHALL 将其作为幂等成功并尝试数据库终结
- **AND** NoSuchBucket、AccessDenied和超时 SHALL NOT 被当成NoSuchKey

#### Scenario: 达到重试终态
- **WHEN** 第24次重试失败或第24次预留崩溃后到达下次资格时间
- **THEN** 系统 SHALL 标记dead、清空下一重试时间并输出Warn重点事件
- **AND** 系统 SHALL NOT 执行第25次自动存储尝试

#### Scenario: 非法历史重试计数
- **WHEN** 迁移发现 `retry_count` 不在0–24范围，或MySQL服务端版本低于8.0.16、为MariaDB或无法确认 CHECK 实际执行
- **THEN** 迁移 SHALL 失败并保留原 File Record、清理队列和完整对象定位
- **AND** 系统 SHALL NOT 自动把非法计数归一化为dead、删除队列或访问对象存储
- **AND** 部署 SHALL 先人工处置；只有可信外部证据证明第24次存储尚未执行时，才允许将异常计数修为0–23，否则 SHALL 修为24并在下一轮只转dead、不访问对象存储

#### Scenario: 成功与迟到失败交错
- **WHEN** 成功实例已条件终结队列，另一个实例随后提交失败结果
- **THEN** 迟到失败 SHALL NOT UPSERT重建队列、复活File Record或覆盖较新尝试状态

#### Scenario: 人工完成 dead 处置
- **WHEN** 运维确认目标对象均已清除
- **THEN** 人工数据库处置 SHALL 同事务删除匹配的File Record和dead队列，避免只删队列后重新入队
- **AND** 系统 SHALL NOT 新增人工HTTP接口、专用命令或completed状态

### Requirement: 消息图片受信存储清理范围

清理 SHALL 始终保留并使用队列冻结的原始位置；额外位置只来自当前启用且非空的文件轮转热冷bucket，去重后使用，不猜测任意历史bucket。跨桶清理 SHALL 以本应用专属、无外部覆盖/复用的 `message-images/UUID` 命名空间以及非版本化、无Object Lock存储为部署前提。系统 SHALL NOT 仅凭任意同名对象推断业务归属。

#### Scenario: 同对象多个受支持副本
- **WHEN** 已满足专属命名空间前提且同一生成对象键在多个受支持位置存在
- **THEN** 系统 SHALL 删除所有这些对应副本后才终结数据库记录
- **AND** 任一位置错误 SHALL 保留队列，不宣称全副本已清理

#### Scenario: 历史原始 bucket 已不在当前配置
- **WHEN** 已登记对象的原始bucket不再出现在当前配置中
- **THEN** 系统 SHALL 继续按冻结原始位置重试
- **AND** 系统 SHALL NOT 改写队列路径或猜测其他历史bucket

#### Scenario: 归属或配置范围无法保证
- **WHEN** 部署无法确认专属命名空间、发现外部同名复用或历史位置遗漏
- **THEN** 部署 SHALL 在启用自动清理前完成人工核实和处置
- **AND** 运行中发现不可信生成键或数据库对象归属冲突时 SHALL 保留记录并受控失败，不自动跨桶删除

#### Scenario: 关闭轮转不证明历史冷桶为空
- **WHEN** 当前轮转未启用或冷桶配置为空
- **THEN** 新清理范围 SHALL 不自动扩展到默认冷桶
- **AND** 配置切换前 SHALL 处置或明确登记旧位置，不以当前配置缺失认定历史副本不存在
