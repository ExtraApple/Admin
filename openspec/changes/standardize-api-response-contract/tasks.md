## 1. 分阶段边界与契约基线

- [ ] 1.1 固化 Backend Part、Frontend Part、Backend Acceptance、Frontend Acceptance 和 Integration Acceptance 的状态与依赖关系；明确 Backend Accepted 仍为 Release Blocked
- [ ] 1.2 固化全部 `/api/*`、`/ping`、API 404/405、OpenAPI 失败、文件下载失败及认证/权限中间件的当前响应清单
- [ ] 1.3 建立目标 HTTP 状态、顶层 error_code、字段级 error_code、英文 fallback、data Schema 和所有者矩阵，明确现有 Upload Security/File 错误码保留项
- [ ] 1.4 建立前后端共享契约 Fixture，覆盖成功、无返回值、空集合、字段校验、认证、授权、冲突、限流、服务端失败和原生响应例外

## 2. Backend Part：Platform HTTP 响应与错误上下文

- [x] 2.1 创建 `internal/platform/httpresponse` 的泛型四字段 Envelope、无返回值表示、ValidationErrorData 和 FieldError 类型
- [x] 2.2 实现成功写入辅助，强制 `code == HTTP status`、空 `error_code`、`msg=success`、无返回值为 null、空集合为 `[]`
- [x] 2.3 实现错误写入辅助，强制非空稳定 error_code、安全英文 msg、白名单 data Schema，并把原始错误和公开错误元数据放入 Gin Context
- [x] 2.4 实现公共 Error Definition 类型和校验辅助，使运行时映射与 Route Descriptor 引用同一份定义且不使用全局可变注册表
- [x] 2.5 增加 Platform 单元测试，覆盖字段完整性、状态一致性、null/空集合、白名单错误 data 和内部 cause 不序列化

## 3. Backend Part：Route Catalog 与 OpenAPI 错误契约

- [x] 3.1 扩展 Route Catalog Response 以携带顶层和字段级公开错误定义，并在 Snapshot 中深拷贝全部错误元数据
- [x] 3.2 增加错误码格式、单一所有者、状态/消息/Schema 一致性、422 字段码和统一错误信封的启动前校验
- [x] 3.3 在 App 组合 Route Descriptor 时按 Public、Authenticated、PermissionControlled 加入实际认证和 API Metadata 中间件错误定义
- [x] 3.4 扩展 API Doc 生成固定成功/错误信封、逐状态顶层 error_code enum、422 字段级 enum 和安全 data Schema
- [x] 3.5 保持 `/docs` HTML、成功 `/docs/openapi.json`、二进制和头像成功响应的原生 Content-Type 与 Schema
- [x] 3.6 增加 Route Catalog 和 API Doc 测试，覆盖所有权冲突、定义漂移、只读快照、中间件错误组合、原生成功协议和 OpenAPI enum

## 4. Backend Part：App 请求日志与恢复边界

- [x] 4.1 实现 App 级 HTTP 请求/错误日志中间件，记录 method、匹配 path、status、latency、client_ip 和 user_agent，排除请求体、Authorization、密码、验证码和 Token
- [x] 4.2 让 5xx 分类错误记录内部 cause 和公开 error_code，让预期 4xx 只保留请求日志且不重复写 error 日志
- [x] 4.3 保持 401/403 经过现有异步 Audit 链路，同时禁止内部 cause 进入审计记录
- [x] 4.4 用自定义恢复中间件替换裸 `gin.Recovery()`：未提交响应返回 `HTTP_INTERNAL_ERROR` 信封，已提交原生响应只记录并终止
- [x] 4.5 增加请求日志、5xx cause、4xx 去重、401/403 审计、panic 恢复和已提交响应测试

## 5. Backend Part：模块错误分类

- [x] 5.1 在 Identity Application 建立 AUTHN/IDENTITY typed Error，覆盖验证码、凭据、锁定、Token、用户状态、用户 CRUD 和字段校验并保留内部 cause
- [x] 5.2 在 Authorization Application 建立 AUTHZ typed Error，覆盖角色、权限、权限分组、保护资源、关联和数据范围错误
- [x] 5.3 在 Navigation 和 API Metadata 建立 NAV/API_META typed Error，覆盖菜单、绑定、API 状态、权限策略和事务冲突
- [x] 5.4 在 Organization、Dictionary 和 Audit 建立 ORG/DICT/AUDIT typed Error，覆盖不存在、重复、领域校验和持久化失败
- [x] 5.5 复用 Upload Security classified Error 并保持全部现有公开值，补齐 Files/Identity Adapter 到统一公共 Error Definition 的映射
- [x] 5.6 为各模块增加 CodeOf/Unwrap、公开映射完整性和未知内部错误回退测试

## 6. Backend Part：后端 HTTP 干净切换

- [x] 6.1 迁移 Identity 登录、注册、Refresh、用户、自助资料和认证中间件到统一响应，落实 401/403/422/429 及登录安全 data 和 Retry-After
- [x] 6.2 迁移 API Metadata 权限中间件和 CRUD Handler，区分未配置、禁用、缺失 Permission Code、权限不足与内部策略失败
- [x] 6.3 迁移 Authorization、Navigation、Organization、Dictionary 和 Audit HTTP Adapter 的成功、校验、冲突、不存在和内部失败响应
- [x] 6.4 迁移 Files、头像上传及其他 Upload Security 消费点，保留既有错误码、HTTP 413/415/422/404/409/503 语义和默认头像行为
- [x] 6.5 迁移 `/ping`、OpenAPI 生成失败、`/api` NoRoute/NoMethod 和未提交 panic 到统一信封，保留成功 CORS OPTIONS 无业务 JSON body
- [x] 6.6 保持文件下载、头像、Swagger HTML 和成功 OpenAPI JSON 的原生成功协议，并确保流提交后的失败只记录不追加 JSON
- [x] 6.7 删除所有旧成功/错误 DTO、`gin.H` 响应分支、`AbortWithStatusJSON` 旧信封和向客户端直接输出 `err.Error()` 的路径

## 7. Backend Acceptance：后端验收

- [x] 7.1 运行后端统一响应契约测试，覆盖每个业务模块的成功、无返回值、空集合、400、401、403、404、405、409、413、415、422、429、500 和 503
- [x] 7.2 运行认证安全测试，确认用户名不存在与密码错误不可区分、验证码为 422、锁定为 429、Refresh 拒绝统一为 401 且内部原因不泄漏
- [x] 7.3 运行 Route Catalog/OpenAPI 一致性测试，确认每条路由的运行时状态、顶层/字段级 error_code、消息和 data Schema 与文档一致
- [x] 7.4 运行文件、头像和 OpenAPI 原生协议测试，确认成功内容未包装、提交前错误信封化、提交后错误只记录
- [x] 7.5 运行 `go test ./... -count=1` 并确认后端请求/响应 JSON、状态码、消息、权限码和外部行为符合契约
- [x] 7.6 扫描并确认后端 HTTP 路径不再存在旧信封、直接 `err.Error()` 序列化或未声明公开错误码
- [x] 7.7 记录 Backend Acceptance 结果并将状态标记为 `backend-accepted / release-blocked`；不得将其标记为 Change 完成

## 8. Frontend Part：前置条件与 API Client

- [ ] 8.1 创建根目录 `web/`，记录前端框架、包管理器、集中 API Client、错误本地化位置、原生响应入口和前端验证命令
- [ ] 8.2 在 `web/` 集中 API Client 实现四字段信封解析、HTTP status/code 一致性校验和成功 data 解包
- [ ] 8.3 定义前端 typed API Error，保留顶层/字段级 error_code、英文 fallback 和批准的安全详情
- [ ] 8.4 建立 error_code 本地化映射和未知码 fallback，禁止通过匹配英文 msg 决定业务分支
- [ ] 8.5 将 422 `data.fields` 接入表单字段错误展示，支持同字段多个错误且不依赖后端 Go 字段名
- [ ] 8.6 将登录剩余次数、登录锁定时间和 Retry-After 接入认证页面状态
- [ ] 8.7 为下载、头像和原始 OpenAPI 请求保留显式 blob/image/raw JSON 路径，禁止成功原生响应进入信封解析器
- [ ] 8.8 迁移所有直接读取旧 `code/msg/data` 组合或旧错误文本的前端调用点并删除旧兼容分支

## 9. Frontend Acceptance：前端验收

- [ ] 9.1 运行前端 API Client 成功、失败、字段错误、登录锁定、未知 error_code 和原生响应测试
- [ ] 9.2 运行前端错误本地化、表单字段映射和认证状态测试
- [ ] 9.3 运行前端类型检查、完整测试和构建
- [ ] 9.4 记录 Frontend Acceptance 结果并确认所有前端旧响应兼容分支已删除

## 10. Integration Acceptance：前后端联合验收与最终发布门禁

- [ ] 10.1 启动真实后端和 `web/`，确认前端 API Client 使用真实后端地址和真实 OpenAPI/响应契约
- [ ] 10.2 使用浏览器验证登录失败/锁定、受保护路由、字段校验、列表空集合和未知错误码 fallback
- [ ] 10.3 使用浏览器验证文件下载、头像读取、Swagger HTML、原始 OpenAPI JSON 和成功 CORS 预检
- [ ] 10.4 使用共享契约 Fixture 对比前端解析结果和后端实际响应，确认错误码、状态、字段详情和安全 data 一致
- [ ] 10.5 确认后端和前端没有双信封、版本 Header、旧字段别名、兼容端点或直接依赖英文消息的逻辑
- [ ] 10.6 更新 README、API Client 说明和受影响长期规格导航，明确 breaking contract、原生协议例外、Backend Accepted 非发布状态和联合验收规则
- [ ] 10.7 对照 proposal、design 和全部 Delta Spec 完成最终验收；只有 Backend Acceptance、Frontend Acceptance 和 Integration Acceptance 全部通过时才可完成或归档 Change
