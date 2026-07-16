## 1. 配置、依赖与数据模型

- [x] 1.1 在 `initialize/server.go` 增加 `file_upload` 配置结构，并在 `config.yaml` 写入 `max_size_mb: 50`、`avatar_max_size_mb: 2`、`download_url_expire_seconds: 300`
- [x] 1.2 在配置初始化阶段校验普通文件上限为 1–100 MiB、头像上限为 1–10 MiB、临时 URL 有效期为正整数，并为非法值补充启动失败测试
- [x] 1.3 将 MIME 检测和图片处理所需的纯 Go 库确认为直接依赖，更新 `go.mod`/`go.sum`，且不引入 ClamAV、原生图像库或外部扫描服务
- [x] 1.4 扩展 `model.File`，增加检测 MIME、验证状态、策略版本、稳定失败原因和验证时间字段，并定义 `legacy_unverified`、`validated`、`blocked`、`validation_error` 常量及 `file-upload-v1` 策略版本
- [x] 1.5 扩展 `model.User`，增加头像 object name、规范 MIME、验证状态和验证时间字段，同时保留旧 `avatar` 字段用于兼容迁移
- [x] 1.6 扩展 `model.AuditLog` 与 `model.AuditLogArchive` 的结构化 `metadata` 字段，并同步审计查询 DTO
- [x] 1.7 在 AutoMigrate 后执行幂等数据回填：历史文件标记为 `legacy_unverified`，历史自定义头像标记为未验证，且迁移过程不得访问、解码、删除或重写 MinIO 对象
- [x] 1.8 为新增字段、默认值、幂等回填及“迁移不访问对象存储”补充数据库迁移测试

## 2. 上传安全核心组件

- [x] 2.1 建立不依赖 Gin 和 MinIO 的上传安全包，定义 `managed_file`、`avatar` 用途、受限输入、规范类型和统一验证结果接口
- [x] 2.2 定义上传安全错误类型及稳定 `error_code`，覆盖请求体、空文件、大小、文件名、类型、编码、图片、OOXML、状态、存储和持久化错误
- [x] 2.3 实现安全错误到 HTTP 400、413、415、422、404、409、500/503 的集中映射，并确保客户端消息不包含底层解析器、数据库或 MinIO 错误
- [x] 2.4 实现展示文件名清洗：截取最后路径段、移除控制和双向文本字符、规范空白与首尾点、限制 255 个 Unicode 字符并保留规范扩展名
- [x] 2.5 实现危险双扩展名检测，拒绝白名单扩展名前出现脚本、可执行、HTML、SVG、归档或其他危险扩展名的名称
- [x] 2.6 建立 JPEG、PNG、WebP、PDF、DOCX、XLSX、PPTX、UTF-8 TXT、UTF-8 CSV 的规范扩展名与 MIME 映射，明确拒绝 `application/octet-stream`
- [x] 2.7 实现扩展名、声明 MIME、签名检测 MIME 和专用验证器结果必须归一到同一规范类型的判定流程
- [x] 2.8 实现受配置上限约束的临时文件暂存和自动清理，避免把普通文件完整复制到内存，并为后续 ZIP `ReaderAt` 校验提供输入
- [x] 2.9 实现普通图片完整解码、PDF 头尾标记、UTF-8/BOM/NUL 的专用验证器，且 TXT/CSV 不猜测编码、不转换编码、不解析业务列
- [x] 2.10 为文件名、双扩展名、白名单、MIME 不一致、空文件、大小边界、图片/PDF/文本有效与损坏样例补充表驱动单元测试

## 3. Office Open XML 校验

- [x] 3.1 实现受限 ZIP 读取器，限制最多 2,000 个条目、单条目解压 50 MiB、累计解压 200 MiB，并在超限后立即停止读取
- [x] 3.2 实现单条目和整体压缩比 100:1 限制，拒绝压缩大小异常为零但解压内容非空的条目
- [x] 3.3 拒绝重复异常名称、绝对路径、盘符、反斜线绕过、`..` 路径穿越、损坏中央目录和嵌套归档内容
- [x] 3.4 校验 `[Content_Types].xml`、包关系、主文档关系以及与 DOCX/XLSX/PPTX 对应的 `word/`、`xl/`、`ppt/` 主体结构
- [x] 3.5 校验扩展名、声明 MIME、内容类型、主关系和主体目录一致，拒绝普通 ZIP 伪装及 Office 类型互相伪装
- [x] 3.6 拒绝宏内容类型与关系、`vbaProject.bin`、ActiveX、OLE、嵌入包/文件、脚本、可执行内容和加密或密码保护容器
- [x] 3.7 安全解析 OOXML XML，拒绝 DTD、DOCTYPE、外部实体及超过 100 层的嵌套，并让 XML 读取继续受 ZIP 字节上限约束
- [x] 3.8 默认拒绝 `TargetMode="External"` 关系，仅允许关系类型为普通 hyperlink 且目标 scheme 为 HTTP/HTTPS 的文本链接
- [x] 3.9 为合法 DOCX/XLSX/PPTX、伪装 ZIP、宏/ActiveX/OLE/嵌入物、外部关系、加密文档、路径穿越、ZIP bomb 和 XML 深度超限建立测试夹具与单元测试

## 4. 头像标准化

- [x] 4.1 实现 JPEG、PNG、WebP 头像头部探测，在完整像素分配前拒绝单边超过 8,192 或总像素超过 40,000,000 的输入
- [x] 4.2 完整解码头像并拒绝损坏图片、声明类型不一致、动画 WebP 和 APNG，不得只取第一帧降级放行
- [x] 4.3 规范图片方向并按比例缩放至最大 1,024×1,024，保证不会放大较小头像
- [x] 4.4 通过重新编码移除 EXIF、定位、设备及其他非像素元数据；无透明通道输出质量 85 JPEG，有透明通道输出 PNG
- [x] 4.5 校验重新编码结果仍不超过 `avatar_max_size_mb`，并只返回 `.jpg`/`image/jpeg` 或 `.png`/`image/png` 的可信输出
- [x] 4.6 为尺寸和像素边界、透明与非透明输入、WebP 转换、动画拒绝、元数据移除、缩放比例及输出大小补充单元测试

## 5. 对象存储与临时访问签名

- [x] 5.1 抽象上传、流式读取、查询、删除和列表所需的对象存储接口，使文件与头像 service 可使用内存或伪存储测试
- [x] 5.2 用现有 MinIO 客户端实现存储接口，确保普通文件和头像 object key 只由服务端 UUID 与规范扩展名生成
- [x] 5.3 增加流式读取及对象不存在/临时不可用错误归一化，不得向 handler 暴露 MinIO 凭据、原始错误或预签名 URL
- [x] 5.4 实现应用临时 URL 的 HMAC-SHA256 签名与验证，签名绑定用户 ID、文件 ID、访问模式、过期时间和验证状态
- [x] 5.5 从 JWT secret 按固定用途派生下载签名 key，并测试篡改用户、文件、模式、状态、签名或过期时间都会失败
- [x] 5.6 为对象写入后数据库失败的补偿删除、头像替换后的旧对象清理失败及孤立对象不自动认领补充伪存储测试

## 6. 管理员文件服务

- [x] 6.1 重构 `service.UploadFile`，复用统一验证管线，在任何 MinIO 写入前完成大小、名称、类型和内容校验
- [x] 6.2 上传成功时保存清洗展示名、规范 MIME、检测 MIME、大小、上传者、`validated` 状态、策略版本和验证时间
- [x] 6.3 实现“MinIO 写入失败不建记录”和“记录创建失败立即补偿删除对象”的语义，并记录受控运行日志
- [x] 6.4 更新文件列表与详情 DTO，返回验证状态、检测 MIME、策略版本、验证时间和稳定失败原因，但不返回存储凭据或原始 MinIO URL
- [x] 6.5 文件详情按状态生成短期应用下载 URL，并且仅为 `validated` JPEG/PNG/WebP 生成预览 URL；`blocked` 不生成任何访问 URL
- [x] 6.6 收紧文件改名逻辑，只允许修改清洗后的名称主体并保留已验证规范扩展名、bucket 和 object key
- [x] 6.7 实现下载访问决策：`validated` 使用规范 MIME，`legacy_unverified`/`validation_error` 强制 `application/octet-stream` 附件，`blocked` 拒绝访问
- [x] 6.8 实现预览访问决策，仅允许 `validated` JPEG/PNG/WebP，其他格式和状态返回稳定 409 错误
- [x] 6.9 实现单文件重新验证，只接受 `legacy_unverified`/`validation_error`，并分别更新为 `validated`、`blocked` 或 `validation_error`，不得重写或删除原对象
- [x] 6.10 保持 MinIO 浏览接口只返回对象元数据，不自动创建文件记录、赋予验证状态或提供绕过策略的下载地址
- [x] 6.11 为上传、详情 URL、改名、下载/预览状态矩阵、重新验证状态转换及存储补偿补充 service 测试

## 7. 用户头像与资料服务

- [x] 7.1 将头像上传从 handler 下沉到 service，执行头像标准化后写入 `image` bucket，并保存 object name、规范 MIME、`validated` 状态和验证时间
- [x] 7.2 使用 `avatars/<user-id>/<uuid>.<jpg|png>` 生成头像 object key，数据库不得再保存客户端 URL 或 MinIO 浏览地址
- [x] 7.3 实现头像替换补偿顺序：先写新对象、再更新数据库、成功后清理旧系统头像；新对象或数据库失败时保留原头像
- [x] 7.4 实现恢复默认头像 service：先清除可信头像标识，再尝试删除旧系统对象；清理失败不得回滚默认头像结果
- [x] 7.5 修改 `UpdateSelfReq` 和资料更新逻辑，使请求中出现 `avatar` 字段时明确拒绝，而不是忽略或写入外部 URL
- [x] 7.6 统一登录、当前用户、初始化上下文、管理员用户、组织成员和角色用户 DTO 的头像解析：仅可信对象返回 `/api/avatars/<user-id>`，历史外部 URL 及其他不可信状态一律返回 `/api/avatars/default`
- [x] 7.7 为头像首次上传、替换、恢复默认、数据库/存储失败补偿、历史 URL 过渡和资料接口拒绝 `avatar` 补充 service 测试

## 8. Handler、请求体限制与响应输出

- [x] 8.1 在 `FormFile`/`ParseMultipartForm` 前为管理员文件请求设置“配置上限 + 1 MiB”硬限制，为头像请求设置“配置上限 + 256 KiB”硬限制
- [x] 8.2 实现严格单文件 multipart 解析：必须且只能存在一个名为 `file` 的文件 part，拒绝缺失、额外文件 part、空文件和损坏表单
- [x] 8.3 重构管理员文件上传 handler，只负责身份提取、受限输入组装、service 调用、审计上下文和统一错误响应
- [x] 8.4 增加管理员下载 handler，验证 JWT、动态权限及临时签名后流式代理对象，并设置清洗后的附件 `Content-Disposition`、明确 MIME、`nosniff` 和私有缓存头
- [x] 8.5 增加管理员预览 handler，验证权限和签名后以内联语义流式输出已验证图片，并设置规范 MIME、`nosniff` 和限制性缓存头
- [x] 8.6 增加单文件重新验证 handler，并对非法 ID、不允许状态、对象不存在、策略拒绝和基础设施错误返回规定的 HTTP 状态与 `error_code`
- [x] 8.7 重构头像上传 handler 复用请求体限制和头像 service，增加 `DELETE /api/user/avatar` 恢复默认头像 handler，并实现只输出标准化 JPEG/PNG 或内置默认 PNG 的公开头像读取 handler
- [x] 8.8 更新资料修改 handler，确保包含 `avatar` 的 JSON 在进入持久化前被拒绝，同时保留昵称和邮箱的原有校验
- [x] 8.9 统一相关接口响应格式，在现有数字 `code` 外增加稳定字符串 `error_code`，且不泄露 object key、签名 key、原始路径和底层错误
- [x] 8.10 为 multipart 超限/多文件/空文件、错误映射、下载响应头、预览限制、签名失效和资料头像字段拒绝补充 handler 测试

## 9. 路由、权限与 API 文档

- [x] 9.1 在 `router/router.go` 注册 `GET /api/admin/files/:id/download`、`GET /api/admin/files/:id/preview` 和 `POST /api/admin/files/:id/revalidate`
- [x] 9.2 在公开路由注册 `GET /api/avatars/:user_id`、`GET /api/avatars/default`，在用户 JWT 路由组注册 `DELETE /api/user/avatar`，并确认普通用户仍无通用业务文件上传路由
- [x] 9.3 通过现有 API 同步机制生成并校验 `admin.files.id.download.get`、`admin.files.id.preview.get`、`admin.files.id.revalidate.post` 权限码
- [x] 9.4 更新自动 OpenAPI 元数据，描述文件白名单、multipart 单文件要求、验证状态、下载/预览 URL、头像标准化和稳定错误码
- [x] 9.5 更新请求/响应 schema，移除 `PUT /api/user/info` 的可写 `avatar`，增加恢复默认头像、重新验证、下载和预览接口文档
- [x] 9.6 扩展 `router/router_test.go`，验证新增路由的 JWT/动态权限链路、方法与路径同步，并验证普通用户不能调用管理员文件接口

## 10. 上传安全审计

- [x] 10.1 定义受控上传审计 metadata，只允许 `purpose`、清洗文件名、大小、声明/检测 MIME、验证结果、原因码和策略版本
- [x] 10.2 让文件与头像 handler/service 在成功、策略拒绝和可安全分类的失败路径上把审计 metadata 写入 Gin context
- [x] 10.3 修改审计中间件，在 `c.Next()` 后读取上传 metadata 并持久化，同时继续把 multipart Body 固定记录为 `[multipart omitted]`
- [x] 10.4 确保审计 metadata 不包含文件内容、未清洗路径、object key、签名 URL、MinIO 凭据、JWT/HMAC key 或解析器原始错误
- [x] 10.5 更新审计查询和冷热归档逻辑，原样返回与复制 metadata，且归档事务语义保持不变
- [x] 10.6 为上传成功、各类拒绝、文件名尚不可用、metadata 脱敏、查询返回和归档复制补充审计测试

## 11. 集成验证与回归测试

- [x] 11.1 建立 V1 文件类型测试矩阵，覆盖每种允许格式、所有明确禁止格式、声明/检测不一致和危险双扩展名
- [x] 11.2 建立端到端伪存储测试，验证被拒绝文件不会写入对象存储或数据库，成功文件只使用系统生成 object key
- [x] 11.3 验证文件状态矩阵：`validated`、`legacy_unverified`、`validation_error`、`blocked` 的详情、下载、预览和重新验证行为均符合 delta spec
- [x] 11.4 验证头像状态与补偿矩阵，包括透明/非透明输出、历史 URL 默认化、替换失败和恢复默认后清理失败
- [x] 11.5 运行 `gofmt`、`go vet ./...` 和 `go test ./...`，修复本变更引入的格式、静态检查和回归问题

## 12. 文档同步与最终验收

- [x] 12.1 更新 `docs/modify/文件上传安全优化建议.md` 的实施状态，只保留实现说明和背景，避免重复复制 OpenSpec 的完整可观察行为
- [x] 12.2 在优化建议中保留 V2 后续项：ClamAV/外部恶意文件扫描、隔离区、批量重新验证、限流、配额、批量上传和新增类型评审
- [x] 12.3 更新 `docs/modify/README.md`、相关模块 API/Apifox 文档和部署配置说明，明确三个 `config.yaml` key、硬上限及前后端迁移影响
- [x] 12.4 对照 `file-management`、`user-management`、`logging` 三份 delta spec 逐条验收实现和自动化测试，不提前删除历史字段或读取历史 MinIO 对象
- [x] 12.5 运行 OpenSpec change 校验并确认所有任务完成；实现验收后再按项目流程同步主 specs 和归档该 change

## 13. 代码审查缺陷修复

- [x] 13.1 为头像数据库更新已提交但后续仓储读取失败的场景建立稳定回归测试，确保成功提交后不会补偿删除数据库正在引用的新头像对象
- [x] 13.2 将头像仓储更新收敛为单次持久化结果，并使用更新前已读取的用户副本构造返回值，避免把提交后的附加查询误判为更新失败
- [x] 13.3 在文件下载和预览返回成功响应前预读惰性对象 reader，确保首读失败被映射为对象不存在或存储不可用，而不是提交 `200` 后返回空正文
- [x] 13.4 在受控头像读取返回前预读惰性对象 reader，确保首读失败时回退到可解码的内置默认 PNG
- [x] 13.5 在重新验证暂存阶段保留已分类的 `STORAGE_OBJECT_NOT_FOUND` 和 `STORAGE_UNAVAILABLE`，避免降级为 `UPLOAD_BODY_INVALID`
- [x] 13.6 连续运行两次针对性回归测试，确认上述稳定复现用例全部由红转绿

## 14. Standards 风险修复

- [x] 14.1 为头像数据库提交结果未知时不删除新对象建立失败回归测试
- [x] 14.2 为头像仓储增加明确提交结果，并仅在确定未提交时补偿删除
- [x] 14.3 为零字节历史文件成功下载及头像空对象回退建立失败回归测试
- [x] 14.4 显式定义预读空流策略，并保留伴随首批字节返回的读取错误
- [x] 14.5 连续运行两次定向回归测试，并执行全量测试、vet 和 OpenSpec strict 校验

## 15. Spec 风险修复

- [x] 15.1 明确重复恢复默认头像的幂等场景，并建立 `RowsAffected = 0` 误判的失败回归测试
- [x] 15.2 已处于默认头像状态时直接返回成功，不执行无变化更新或对象删除
- [x] 15.3 删除仍可持久化任意外部头像 URL 的旧 `SetAvatar` 入口，并确认不存在调用引用
- [x] 15.4 连续运行两次定向回归测试，并执行全量测试、vet 和 OpenSpec strict 校验

## 16. 错误语义审查修复

- [x] 16.1 为包装 `io.EOF` 的已分类存储错误建立失败回归测试，确保预读不会把它当作正常空流或完整短流
- [x] 16.2 为 `NoSuchBucket` 和未知 HTTP 404 建立失败回归测试，并仅将明确的对象缺失错误映射为 `STORAGE_OBJECT_NOT_FOUND`
- [x] 16.3 将头像数据库提交结果的零值调整为 `Unknown`，并验证仓储惯用的 `return 0, err` 不会触发补偿删除
- [x] 16.4 连续运行两次定向回归测试，并执行全量测试、vet 和 OpenSpec strict 校验
