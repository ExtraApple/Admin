## Why

实现审计发现四个「已声明的依赖端口，在组合根从不注入」的实例
（`docs/reviews/wire-audit.md` 第一、五节）。经逐一核实读取点，它们在当前配置下
**没有执行路径**，因此既未提供任何能力，也无法从外部观察到。

其中 `files.Dependencies.Authorization` 的缺失会让 8 处对象级授权调用恒返回 nil，
使读者误以为文件模块存在数据范围校验。**留着比删掉更危险**：它把"未实现"
伪装成"已实现"，是本次审计反复误判同一处的根本原因。

这些端口是同一根因的四个实例，故合并为一个变更。

## What Changes

- **删除** `internal/files/application` 的 `AuthorizationScope` 接口、
  `Dependencies.Authorization` 字段、`authorize` 函数及其 8 处调用点，
  以及从未被使用的 `OperationDownload`、`OperationPreview` 常量。
- **删除** `internal/authorization/application` 的 `UserVisible` 与
  `OrganizationVisible` 方法（生产代码与测试均零引用）。
- **删除** `internal/files/application` 的 `AuditMetadataSink` 接口、
  `Dependencies.Audit` 字段与 `recordAudit` 中的直接记录分支；
  把两个包各自定义的 `UploadAuditMetadataContextKey` 常量收敛为单一来源。
- **删除** `internal/messaging/application` 的 `MessagingAuditSink` 接口、
  `MessagingAuditEntry` 结构体及其契约测试。
- **新增**一份 ADR，记录"授权与审计端口按需引入、不留未接线接缝"的决策，
  并保存被删端口的原始设计作为将来重新引入的输入。
- **不新增、不修改任何可观察行为**：完整业务 JSON、HTTP 状态码、路由集合、
  权限码、数据库结构、Redis key、MinIO bucket 与后台任务集合均不变。

**非目标**（本变更明确不做）：

- 不为文件模块引入对象级授权或数据范围。删除是**裁决结果**，不是"暂缓实现"。
- 不改变消息审计的实际通道。消息操作已由路由级审计与受控运行日志覆盖，
  本变更只移除未被采用的第二条实现路径。
- 不移除有意预留的扩展点（`MessagingMetrics`、通知投影 Contract、
  邮件与短信 Consumer、`ExternalProxy`）。删除它们会违背
  `docs/adr/0007-internal-messaging-domain-decisions.md:95` 与 `:105`。
- 不处理公告调度未注册与过期消息图片清理（属独立变更多个，
  见 `docs/reviews/contract-readiness.md` 的批次 5）。

## Capabilities

### New Capabilities

- `file-access-contract`: 固化「文件模块的访问授权只由路由级权限码决定、
  不存在对象级所有权或数据范围校验」这一当前有效行为。
  **动机**：删除 `authorize()` 的 8 个调用点后，该契约将失去唯一的代码痕迹，
  而审计过程已证明这一处会被反复误判为"已有对象级授权"。
  新增规格使它成为可引用的事实，而不是只能靠读代码推断的隐含行为。

### Modified Capabilities

无。经逐条核对，四个端口在当前配置下均无生效行为，
**没有任何既有规格要求发生改变**。

（对照说明：`openspec/specs/file-management/spec.md` 检索 "Authorization" 与
"数据范围" 均为零命中 —— 该规格从未要求对象级授权，故本次是 **ADDED** 而非
MODIFIED；`openspec/specs/internal-messaging/spec.md:93` 要求消息操作写入
"受控审计**或**运行日志"，该要求由路由级审计与运行日志满足，
本变更不移除这两条通道，规格不变。）

## Impact

### 受影响代码

| 模块 | 删除内容 |
| --- | --- |
| `internal/files/application` | `contracts.go`：`AuthorizationScope`、`Operation` 常量集、`Dependencies.Authorization`、`AuditMetadataSink`、`Dependencies.Audit`；`service.go`：`authorize`、`recordAudit` 直接分支、8 处 `authorize` 调用 |
| `internal/authorization/application` | `service.go`：`UserVisible`、`OrganizationVisible` |
| `internal/messaging/application` | `contracts.go`：`MessagingAuditSink`、`MessagingAuditEntry`；删除 `audit_contract_test.go` |
| `internal/audit` | `UploadAuditMetadataContextKey` 常量收敛后的引用调整 |

### 不受影响

路由（120 条）、权限码、HTTP 状态码与业务 JSON、数据库结构、
Redis key、MinIO bucket、后台任务集合（11 个）、OpenAPI 输出。

### 风险与验证

**风险等级 R0**（运行时风险可证明为零）：被删代码没有执行路径。
评估依据见 `docs/reviews/change-risk-assessment.md` 第二节。

**信息损失**：`authorize()` 的存在本身在说"这些操作应该被判权"。
缓解方式为本变更包含的 ADR —— 记录裁决与理由，否则删除是净损失。

**验证**：

1. `go build ./...` —— 确认引用删净
2. `go test ./... -count=1` —— 160 个既有测试文件全部通过
3. `go test -tags=mysql_integration ./... -count=1` —— 真实 MySQL 门禁通过
4. 对比变更前后的 120 条路由快照 —— 集合与顺序均不变

第 2 项是强证据：既有测试含 HTTP 契约、权限策略、审计与消息测试，
任何可观察行为改变都会使其失败。

### 相关文档

- 审计依据：`docs/reviews/wire-audit.md`、`docs/reviews/change-risk-assessment.md`
- 后续依赖：护栏变更（`docs/reviews/wire-guardrail-design.md`）**必须在本变更完成后**实施，
  否则 `files.Dependencies.Authorization` 标为 `required` 会让护栏立即失败
