## 1. 前置核实

- [ ] 1.1 全库检索 `/api/health`，列出全部引用位置（预期：2 份规格 + ADR 0007 + runbook）
- [ ] 1.2 确认 `/ping` 的实际注册点（`internal/app/http.go:21-23`）与其访问等级
- [ ] 1.3 确认 `/ping` 不带 `/api/` 前缀，因而未被纳入 API 元数据与权限中间件
- [ ] 1.4 验证 `go test ./initialize -run TestArchitecture -count=1` **确实失败**，记录错误信息作为修正依据
- [ ] 1.5 验证 `go test ./testsupport -run TestArchitecture -count=1` 通过
- [ ] 1.6 运行 `openspec validate --specs`，记录变更前规格校验基线

## 2. 修正引用不存在端点的规格

- [ ] 2.1 在 `specs/modular-layered-architecture/spec.md` delta 中提交 `App 统一装配系统` 的完整 MODIFIED 内容，`/api/health` → `/ping`
- [ ] 2.2 在 `specs/route-catalog/spec.md` delta 中提交 `WebSocket 与 Messaging 入口声明` 的完整 MODIFIED 内容，`/api/health` → `/ping`
- [ ] 2.3 逐条对照两份 delta 与其 `openspec/specs/` 原文，确认除目标行外内容逐字一致（避免归档丢细节）

## 3. 补齐 dict-management 规格场景

- [ ] 3.1 在 `specs/dict-management/spec.md` delta 中提交 `字典类型 CRUD` 的完整 MODIFIED 内容
- [ ] 3.2 补齐「查询字典类型详情」Scenario
- [ ] 3.3 补齐「修改字典类型」Scenario，含编码变更时的唯一性与条目类型编码同步
- [ ] 3.4 补齐「删除字典类型」Scenario，含级联删除与不存在时的拒绝
- [ ] 3.5 提交 `字典条目 CRUD` 的完整 MODIFIED 内容
- [ ] 3.6 补齐「修改字典条目」与「删除字典条目」Scenario
- [ ] 3.7 提交 `按类型编码获取字典条目` 的完整 MODIFIED 内容，新增「类型编码不存在或未启用」Scenario 固化空列表行为
- [ ] 3.8 对照 `internal/dictionary/http.go:47-57` 的 9 条路由，确认每条均有对应 Scenario
- [ ] 3.9 对照 `internal/dictionary/http_test.go` 的既有断言，确认新增 Scenario 描述与测试一致

## 4. 澄清 identity 规格作用域

- [ ] 4.1 在 `specs/identity/spec.md` delta 中提交 `已验证邮箱资格` 的完整 MODIFIED 内容
- [ ] 4.2 把「对外响应」改为「通知渠道 Contract 响应」
- [ ] 4.3 新增 Scenario 说明通知渠道 Contract 不返回多余字段
- [ ] 4.4 新增 Scenario 明确 HTTP 资料响应由 `user-management` 规格约束、两者相互独立
- [ ] 4.5 对照 `internal/app/messaging.go:313-321` 与 `identity/dto.go:33-43`，确认两条约束的实际边界与规格描述一致

## 5. 对齐 auth 规格的登出 TTL 表述

- [ ] 5.1 在 `specs/auth/spec.md` delta 中提交 `登出黑名单` 的完整 MODIFIED 内容
- [ ] 5.2 把「黑名单 TTL 与 token 剩余有效期对齐」改为描述固定的 `jwt.expire` 时长
- [ ] 5.3 新增 Scenario 说明「TTL 长于剩余有效期」时的行为与无安全后果的理由
- [ ] 5.4 对照 `internal/app/identity.go:86`、`internal/identity/application/service.go:251-256`、`internal/identity/adapters/redis/store.go:75-77`，确认描述与实现一致

## 6. 修正 module-navigation 的验证命令

- [ ] 6.1 把 `docs/module-navigation.md:44` 的 `go test ./initialize` 改为 `go test ./testsupport`
- [ ] 6.2 复核同文件「验证入口」一节的其余两条命令仍然可用
- [ ] 6.3 考虑是否补充 wiring 护栏测试的运行命令（取决于 `add-dependency-wiring-guardrail` 是否已完成；若未完成则不加）

## 7. 修正 runbook 与 ADR 的端点引用

- [ ] 7.1 把 `docs/runbooks/messaging.md:24` 的 `/api/health` 改为 `/ping`
- [ ] 7.2 把 `docs/adr/0007-internal-messaging-domain-decisions.md:91` 的 `/api/health` 改为 `/ping`
- [ ] 7.3 在 ADR 0007 该句后加一行说明：该存活路由在更早版本名为 `/api/health`，现为 `/ping`
- [ ] 7.4 全库复检 `/api/health`：除上述说明性文字外应无残留

## 8. 修正 TODO.md 的失真条目

- [ ] 8.1 修正 `:62`「上传文件（已支持图片）」—— 普通文件策略只允许 PDF/TXT/CSV，图片仅进头像链路
- [ ] 8.2 修正 `:63`「下载文件（支持预签名 URL、断点续传）」—— 实际为应用侧 HMAC 签名链接，无预签名 URL、无 Range 支持
- [ ] 8.3 勾选 `:70`「文件上传安全策略」—— 已在 `internal/uploadsecurity` 完整实现
- [ ] 8.4 勾选 `:81-86`「内部消息」六项 —— 模块已建成并有归档 change
- [ ] 8.5 复检 `:24`「sha-256验证头像上传完整度」—— 已实现（`avatar_service.go:181`），确认是否应勾选
- [ ] 8.6 确认其余未核实条目保持原状，不做推测性修改

## 9. 修正路线图的阶段定位

- [ ] 9.1 核对 `docs/modify/project-improvement-roadmap.md` 中关于阶段 4（模块化分层重构）的描述
- [ ] 9.2 依据代码证据修正：「建立应用装配和基础设施层」与「按业务模块迁移」已完成
- [ ] 9.3 只修正可机械验证的条目；不重写阶段划分本身
- [ ] 9.4 确认 `docs/modify/mature-admin-system-comparison-*.md` 等历史快照文件未被改动

## 10. 验证

- [ ] 10.1 运行 `openspec validate --specs`，确认规格结构合法
- [ ] 10.2 全库检索 `/api/health`，确认除 ADR 说明性文字外已清零
- [ ] 10.3 逐条对照 5 份 delta 的 MODIFIED 内容与 `openspec/specs/` 原文，确认未丢失既有 Scenario
- [ ] 10.4 运行 `go test ./initialize -run TestArchitecture -count=1` 确认仍失败（该命令本就无效）
- [ ] 10.5 运行 `go test ./testsupport -run TestArchitecture -count=1` 确认通过
- [ ] 10.6 **不修改任何 `internal/` 或 `main.go`**；用 `git status` 确认改动只落在 `openspec/`、`docs/`、`TODO.md`

## 11. 验收

- [ ] 11.1 对照 proposal 的「非目标」：未新增 `/api/health` 路由、未处理 C6b、未含首轮误报条目、未改写历史快照文件
- [ ] 11.2 确认 `## MODIFIED Requirements` 每条的完整内容已复制并按新行为编辑
- [ ] 11.3 确认 `dict-management` 的 9 条路由在修正后全部有规格 Scenario 覆盖
- [ ] 11.4 更新 `docs/reviews/contract-readiness.md`：标记 X1、X6 与批次 4 已处理
- [ ] 11.5 记录仍押后的项：C6b（剩余 10 个规格 / 578 条 SHALL）与 `docs/runbooks/login-security.md` 的锁定档位描述
