## 1. 前置核实

- [x] 1.1 全库检索 `/api/health`，列出全部引用位置 —— 实际比预期多：**2 份规格 + ADR 0007 + runbook + 2 个测试夹具**（`internal/app/app_test.go`、`internal/routecatalog/catalog_test.go` 把该字符串用作合成示例路由路径）
- [x] 1.2 确认 `/ping` 的实际注册点（`internal/app/http.go:21-23`）与其访问等级 —— `engine.GET("/ping", ...)`，技术路由，返回 `{"msg":"pong"}` 四字段信封
- [x] 1.3 确认 `/ping` 不带 `/api/` 前缀，因而未被纳入 API 元数据与权限中间件 —— `RegisterTechnicalHTTP` 直接注册在 engine 上，不经 Route Catalog，故不参与元数据同步与权限中间件
- [x] 1.4 验证 `go test ./initialize -run TestArchitecture -count=1` **确实失败** —— 实测 `stat D:\WORK\GO\admin\initialize: directory not found`，`FAIL ./initialize [setup failed]`
- [x] 1.5 验证 `go test ./testsupport -run TestArchitecture -count=1` 通过 —— `ok admin/testsupport 0.415s`
- [x] 1.6 运行 `openspec validate --specs`，记录变更前规格校验基线 —— `Totals: 15 passed, 0 failed`

## 2. 修正引用不存在端点的规格

- [x] 2.1 在 `specs/modular-layered-architecture/spec.md` delta 中提交 `App 统一装配系统` 的完整 MODIFIED 内容，`/api/health` → `/ping` —— 6 个 Scenario 全文复制，仅 `RabbitMQ 降级就绪` 一行改为 `/ping`
- [x] 2.2 在 `specs/route-catalog/spec.md` delta 中提交 `WebSocket 与 Messaging 入口声明` 的完整 MODIFIED 内容，`/api/health` → `/ping` —— 7 个 Scenario 全文复制，仅 `RabbitMQ 就绪` 一行改为 `/ping`
- [x] 2.3 逐条对照两份 delta 与其 `openspec/specs/` 原文，确认除目标行外内容逐字一致 —— 脚本比对 Scenario 集合：两份均为 **main=6/delta=6** 与 **main=7/delta=7**，无丢失、无新增

## 3. 补齐 dict-management 规格场景

- [x] 3.1 在 `specs/dict-management/spec.md` delta 中提交 `字典类型 CRUD` 的完整 MODIFIED 内容
- [x] 3.2 补齐「查询字典类型详情」Scenario —— **经核对后改为「查询字典类型只有列表入口」**：`internal/dictionary/http.go:48-56` 的 9 条路由中**不存在** `GET /api/admin/dict-types/:id`，原任务假设不成立；若照原样补 Scenario 会构成新的幽灵端点（与 X1 同类）。已改为显式声明"只有列表入口、SHALL NOT 提供对象级详情端点"
- [x] 3.3 补齐「修改字典类型」Scenario，含编码变更时的唯一性与条目类型编码同步 —— 对应 `service.go:68`（`TypeCodeExists`）与 `service.go:96`（`UpdateItemsTypeCode`，同事务）
- [x] 3.4 补齐「删除字典类型」Scenario，含级联删除与不存在时的拒绝 —— 对应 `service.go:118-121`（`DeleteItemsByTypeCode` + `DeleteType` 同事务）
- [x] 3.5 提交 `字典条目 CRUD` 的完整 MODIFIED 内容
- [x] 3.6 补齐「修改字典条目」与「删除字典条目」Scenario —— 对应 `service.go:152-207`（目标类型存在性、类型+值唯一性、空更新拒绝）与 `:217-225`
- [x] 3.7 提交 `按类型编码获取字典条目` 的完整 MODIFIED 内容，新增「类型编码不存在或未启用」Scenario 固化空列表行为 —— 对应 `http_test.go:56`（unknown → 200）与 `:235-239`（disabled → 200 且 `len(Data)==0`）
- [x] 3.8 对照 `internal/dictionary/http.go:48-56` 的 9 条路由，确认每条均有对应 Scenario —— 9 条全部覆盖：公开 items 1 条 + dict-types 4 条（列表/创建/修改/删除）+ dict-items 4 条（列表/创建/修改/删除）
- [x] 3.9 对照 `internal/dictionary/http_test.go` 的既有断言，确认新增 Scenario 描述与测试一致 —— 唯一性、级联删除、空更新拒绝、空列表行为均有对应断言或服务层实现

## 4. 澄清 identity 规格作用域

- [x] 4.1 在 `specs/identity/spec.md` delta 中提交 `已验证邮箱资格` 的完整 MODIFIED 内容
- [x] 4.2 把「对外响应」改为「通知渠道 Contract 响应」
- [x] 4.3 新增 Scenario 说明通知渠道 Contract 不返回多余字段
- [x] 4.4 新增 Scenario 明确 HTTP 资料响应由 `user-management` 规格约束、两者相互独立
- [x] 4.5 对照 `internal/app/messaging.go:318-326`（`LookupVerifiedEmail`，仅返回 `VerifiedEmail{Address}` + 可用性 bool）与 `identity/dto.go:33-43`（`UserInfo` 9 字段），确认两条约束的实际边界与规格描述一致

## 5. 对齐 auth 规格的登出 TTL 表述

- [x] 5.1 在 `specs/auth/spec.md` delta 中提交 `登出黑名单` 的完整 MODIFIED 内容
- [x] 5.2 把「黑名单 TTL 与 token 剩余有效期对齐」改为描述固定的 `jwt.expire` 时长
- [x] 5.3 新增 Scenario 说明「TTL 长于剩余有效期」时的行为与无安全后果的理由
- [x] 5.4 对照实现确认描述一致 —— `internal/app/identity.go:86` 传入 `durationMinutes(config.Jwt.Expire)`；`identity/adapters/http/routes.go:188` 传给 `Logout`；`application/service.go:251-252` 转给 `blacklist.Add`；`redis/store.go:76` 以该时长 `Set`。全链路均为固定 `jwt.expire`，与 delta 描述逐字吻合

## 6. 修正 module-navigation 的验证命令

- [x] 6.1 把 `docs/module-navigation.md:44` 的 `go test ./initialize` 改为 `go test ./testsupport`
- [x] 6.2 复核同文件「验证入口」一节的其余两条命令仍然可用 —— `go test ./internal/routecatalog ./internal/app` 与 `go test ./...` 均通过（见第 10 节）
- [x] 6.3 补充 wiring 护栏说明 —— 批次 2 已完成，故在该节加一句说明第一个命令已包含全部 17 个架构边界测试，其中 `TestArchitectureDeclaredDependenciesAreWired` 校验依赖接线；不新增冗余命令（同一 `-run TestArchitecture` 前缀已覆盖）

## 7. 修正 runbook 与 ADR 的端点引用

- [x] 7.1 把 `docs/runbooks/messaging.md` 的 `/api/health` 改为 `/ping` —— **实际行号为 44**（proposal/design 记 `:24`，因批次 3 在该文件新增「消息长度上限」一节而后移）；已按内容定位修正
- [x] 7.2 把 `docs/adr/0007-internal-messaging-domain-decisions.md:91` 的 `/api/health` 改为 `/ping`
- [x] 7.3 在 ADR 0007 该句后加说明：该存活路由在更早的文档中曾被写作 `GET /api/health`，实际从未注册过该路径
- [x] 7.4 全库复检 `/api/health` —— 见 10.2

## 8. 修正 TODO.md 的失真条目

- [x] 8.1 修正「上传文件（已支持图片）」→「上传文件（普通文件仅 PDF/TXT/CSV；图片走头像与消息图片用途）」—— 依据 `uploadsecurity.IsManagedFileType` 只允许 `TypePDF/TypeTXT/TypeCSV`，图片类型仅用于头像与消息图片用途
- [x] 8.2 修正「下载文件（支持预签名 URL、断点续传）」→「下载文件（应用侧 HMAC 签名链接；无预签名 URL、无断点续传）」—— 全库检索 `PresignedGetObject|PresignedPutObject|Content-Range|Accept-Ranges|http.ServeContent` 零命中
- [x] 8.3 勾选「文件上传安全策略（大小限制、MIME 白名单、危险扩展名拒绝）」—— `uploadsecurity` 已实现大小上限、`typesByMIME` 白名单与 `ValidateExtensionChain`
- [x] 8.4 勾选「内部消息」六项 —— 六项均有路由：发送 `POST /api/user/messages/private`、收件箱 `GET /api/user/messages`、分类管理 `GET/POST/PUT/DELETE /api/admin/message-categories`、标记已读 `PUT /api/user/messages/:id/read`、撤销 `POST /api/user/messages/:id/revoke` 与 `/api/admin/messages/:id/revoke`、删除收件箱 `DELETE /api/user/messages/:id/inbox`
- [x] 8.5 勾选「sha-256 验证头像上传完整度」—— 已实现：`identity/application/avatar_service.go:181` 写入 `ContentSHA256`，`identity/model.go:18` 持久化该列
- [x] 8.6 确认其余未核实条目保持原状 —— 未触碰「多语言字典」「绑定/验证联系方式」等条目

## 9. 修正路线图的阶段定位

- [x] 9.1 核对 `docs/modify/project-improvement-roadmap.md` 中关于阶段 4 的描述
- [x] 9.2 依据代码证据修正：条目 9「建立应用装配和基础设施层」标为已完成，并注明入口是仓库根目录的 `main.go` 而非 `cmd/admin`（仓库无 `cmd/` 目录）；`internal/platform` 现有 7 个子目录；条目 10「按业务模块迁移」标为已完成，列出实际模块目录与 ADR 0004/0005 依据
- [x] 9.3 只修正可机械验证的条目；未重写阶段划分本身，条目 11（自动化 API 文档增强）保持原状（其验收依赖仍在途的 `standardize-api-response-contract`）
- [x] 9.4 确认 `docs/modify/mature-admin-system-comparison-*.md` 等历史快照文件未被改动 —— `git status` 中无该目录文件

## 10. 验证

- [x] 10.1 运行 `openspec validate --specs` —— 15 passed / 0 failed；`openspec validate fix-stale-docs-and-specs` 亦通过
- [x] 10.2 全库检索 `/api/health` —— **残留已清零**：唯一剩余处为 `docs/reviews/contract-readiness.md` 的 X1 条目（该审计记录本身描述这个幽灵端点）与批次 4 工件自身的说明性文字；`openspec/specs/` 的旧文本将在**归档时**由 delta 替换；两个测试夹具的示例路径已按裁决改名为 `/api/probe-health`
- [x] 10.3 逐条对照 5 份 delta（7 条 MODIFIED Requirement）与 `openspec/specs/` 原文 —— 脚本比对 Scenario 集合，**无一条丢失既有 Scenario**；新增见第 3、4、5 节
- [x] 10.4 运行 `go test ./initialize -run TestArchitecture -count=1` 确认仍失败（该命令本就无效）—— 同 1.4
- [x] 10.5 运行 `go test ./testsupport -run TestArchitecture -count=1` 确认通过 —— ok
- [x] 10.6 用 `git status` 确认改动范围 —— 落在 `openspec/`、`docs/`、`TODO.md`，**外加经裁决新增的两个测试夹具**（`internal/app/app_test.go`、`internal/routecatalog/catalog_test.go` 的示例路径改名）；`main.go` 与其余生产代码零改动。proposal 的「不受影响」一节已同步记录该唯一例外

## 11. 验收

- [x] 11.1 对照 proposal 的「非目标」：未新增 `/api/health` 路由（改为修规格）、未处理 C6b、未含首轮误报条目（字典 400 映射与"缺少管理员创建用户端点"均未进入）、未改写 `docs/modify/mature-admin-system-comparison-*` 历史快照
- [x] 11.2 确认 `## MODIFIED Requirements` 每条的完整内容已复制并按新行为编辑 —— 7 条逐条比对，均无丢失
- [x] 11.3 确认 `dict-management` 的 9 条路由在修正后全部有规格 Scenario 覆盖 —— 同 3.8
- [x] 11.4 更新 `docs/reviews/contract-readiness.md`：X1 与 X6 各新增「处置（批次 4）」段；批次计划与 Change 表标记批次 4 完成，并新增「批次 4 实施结果」与两处实施期裁决
- [x] 11.5 记录仍押后的项 —— C6b（剩余 10 个规格 / 578 条 SHALL）；`docs/runbooks/login-security.md` 的锁定档位描述（本变更 design 的待评估项，本轮**未新增核对**，是否修订待定，已在 contract-readiness 中如实标注来源）
