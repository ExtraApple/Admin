# 授权与审计端口按需引入，不留未接线接缝

## 状态

已接受。实施于 OpenSpec Change `openspec/changes/remove-unwired-ports/`（批次 1）。

## 背景

`docs/reviews/wire-audit.md` 的实现审计在「端口实现」维度发现一类反复出现的形态：
**接口已声明、字段已加入 `Dependencies` 或方法已实现，但在组合根从未注入，或在整个生产代码中零引用。**
这类代码既没有执行路径、无法从外部观察，又在阅读时陈述"这里存在某项能力"。

最严重的实例是 `internal/files/application` 的 `authorize()`：它有 8 处调用点**确实会执行**，
但函数体第一步是 `s.deps.Authorization == nil` 时 `return nil`，而 `Dependencies.Authorization`
在 `internal/app/files.go` 从未被赋值 —— 于是 8 处调用恒为「放行」。
审计过程中该处被反复误判为"文件模块已有对象级授权"，是本轮审计最主要的误判来源。

审计第一稿把「未接线」一律视为缺口，因此误报了 `MessagingMetrics`、通知投影 Contract、
邮件/短信 Consumer。核对 `docs/adr/0007-internal-messaging-domain-decisions.md` 后确认，
它们是有书面依据的**有意预留扩展点**。这暴露了真正需要裁决的问题：
**如何区分"有意预留的接缝"与"未生效的缺口"**，以及**删除是否会造成不可逆的信息损失**。

## 决策

### 1. 判定规则：以书面依据区分「有意预留」与「未生效」

在 ADR 或 OpenSpec 规格中存在明确「本次不实现 / 未来扩展点 / 保留可接入点」表述的接缝，
属**设计**，不得删除；没有此类表述的属**缺口**，按本 ADR 处理。

**理由**：仅凭"零调用方"无法区分二者，而两种情形需要相反的处置 ——
前者删除会违背既有决策，后者保留会把"未实现"伪装成"已实现"。

### 2. 未接线端口一律删除，不用注释保留

删除而非保留的核心论据：`authorize()` 与 `UserVisible()` 的**存在本身在陈述
"这些操作应该被判权"**。删除后未来读者（含二次开发者）不会知道这里曾有判权意图，
可能直接写下无判权的文件操作 —— 这一信息损失由本 ADR 的第 5 节补偿。

**否决的替代方案**：保留代码并加 `// 未接线，勿使用` 注释。
理由：注释不改变 8 处调用点看起来在检查权限这一事实，
而 `docs/reviews/wire-audit.md` 已证明该形态会反复导致误判。

**同一规则向前适用**：新增授权或审计端口时必须有真实调用方与组合根注入。
不为"以后可能要用"预留空接缝，不创建空操作或伪实现（与 ADR 0007:105 一致）。

### 3. 本决策删除的端口及其原始签名

以下签名从代码中删除，在此保留为将来**重新引入的输入**。
重新引入时必须同时满足：有 OpenSpec 规格出处、有组合根注入、有覆盖其行为的测试。

#### 3.1 `internal/files/application` — 对象级授权端口

```go
type AuthorizationScope interface {
	Authorize(context.Context, uint, Operation, uint) error
}

type Operation string

const (
	OperationUpload     Operation = "upload"
	OperationList       Operation = "list"
	OperationDetail     Operation = "detail"
	OperationUpdate     Operation = "update"
	OperationDelete     Operation = "delete"
	OperationDownload   Operation = "download"
	OperationPreview    Operation = "preview"
	OperationRevalidate Operation = "revalidate"
	OperationBrowse     Operation = "browse"
)

type Dependencies struct {
	// ...
	Authorization AuthorizationScope
	// ...
}
```

`service.go` 中的接线与调用点：

```go
func (s *Service) authorize(ctx context.Context, actor uint, op Operation, id uint) error {
	if s == nil || s.deps.Authorization == nil {
		return nil
	}
	return s.deps.Authorization.Authorize(ctx, actor, op, id)
}

func inputOperation(mode AccessMode) Operation {
	if mode == ModePreview {
		return OperationPreview
	}
	return OperationDownload
}
```

被删除的 8 处调用点（原始 `service.go` 行号为变更前状态）：

| 原行号 | 调用 | 所在方法 |
| --- | --- | --- |
| 81 | `s.authorize(ctx, input.UploaderID, OperationUpload, 0)` | `Upload` |
| 242 | `s.authorize(ctx, actor, OperationList, 0)` | `List` |
| 263 | `s.authorize(ctx, actor, OperationDetail, id)` | `Get` |
| 302 | `s.authorize(ctx, actor, OperationUpdate, id)` | `Update` |
| 335 | `s.authorize(ctx, actor, OperationDelete, id)` | `Delete` |
| 358 | `s.authorize(ctx, actor, OperationBrowse, 0)` | `Browse` |
| 382 | `s.authorize(ctx, actor, OperationRevalidate, id)` | `Revalidate` |
| 455 | `s.authorize(ctx, input.UserID, inputOperation(input.Mode), input.FileID)` | `Open` |

> 审计文档曾把该数量记为「7 处」，实施时逐一核对为 **8 处**。
> 数量差异不影响决策，但重新引入时必须按 8 个操作点评估。

`OperationDownload` 与 `OperationPreview` 仅由 `inputOperation` 使用，
其余 7 个 `Operation` 常量仅由上述调用点使用 —— 因此 `Operation` 常量集整体删除。

**设计意图（据代码形态推断，无书面出处）**：让 Files 在路由级权限码之外，
再按操作类型与资源 ID 做一次对象级判权。它对应的实现从未存在。

#### 3.2 `internal/files/application` — 直接审计端口

```go
type AuditMetadataSink interface {
	Record(context.Context, AuditMetadata)
}

type Dependencies struct {
	// ...
	Audit AuditMetadataSink
	// ...
}

func (s *Service) recordAudit(ctx context.Context, metadata AuditMetadata) {
	if s.deps.Audit != nil {
		s.deps.Audit.Record(ctx, metadata)
	}
}
```

`recordAudit` 的 2 处调用点（`Upload` 中的普通文件上传、消息图片上传）一并删除。
`AuditMetadata` 结构体与 `UploadValidationAccepted` / `UploadValidationRejected` 常量**保留** ——
它们仍由 HTTP 适配器写入 Gin Context，被 Audit 中间件消费（见第 4 节）。

**保留的实际审计通道**：文件操作的可观察审计事实由
`internal/audit/adapters/http` 的路由级审计中间件产生，与本端口无关。
本端口的意图是把审计写入下推到 Application 层，但组合根从未注入实现。

#### 3.3 `internal/authorization/application` — 资源可见性方法

```go
func (service *Service) UserVisible(ctx context.Context, operatorID, targetID uint) error
func (service *Service) OrganizationVisible(ctx context.Context, operatorID, organizationID uint) error
```

两者分别复用 `VisibleUserIDs` / `VisibleOrganizationIDs`，在目标不在可见范围时返回
`CodeInvalidUser`。生产代码与测试均零引用，且未被任何接口声明，
因此删除后没有接口不再被满足。`VisibleUserIDs` / `VisibleOrganizationIDs` / `ResolveUserScope` /
`ResolveOrganizationScope` **保留** —— 它们是列表过滤的实际通道。

#### 3.4 `internal/messaging/application` — 直接审计端口

```go
// MessagingAuditSink records allowlisted metadata of a Messaging operation.
type MessagingAuditSink interface {
	Record(context.Context, MessagingAuditEntry)
}

// MessagingAuditEntry excludes content, external URLs, media bytes,
// credentials, and infrastructure errors.
type MessagingAuditEntry struct {
	ActorID        uint
	MessageCopyID  uint
	OrganizationID uint
	EventID        string
	ConsumerName   string
	ReplayCycle    uint
	Action         string
	Result         string
	ReasonCode     string
}
```

该接口没有 `Dependencies` 字段、没有实现、零调用方；配套的
`audit_contract_test.go` 只做 `var _ =` 接口断言，无行为测试，一并删除。

**保留的实际审计通道**：消息操作由路由级审计（`DefaultAuditCategory = "message"`，
`internal/messaging/adapters/http/routes.go:139`）与受控运行日志覆盖，
满足 `openspec/specs/internal-messaging/spec.md:93`「写入受控审计**或**运行日志」。
本变更不移除这两条通道，该规格要求不变。

### 4. 顺带收敛：审计上下文键单一定义

`internal/files/application` 与 `internal/audit` 曾各自定义字符串值相同的
`upload_audit_metadata` 上下文键。写入方（Files）与读取方（Audit）靠**约定而非编译期保证**一致，
任一侧改动都会导致上传审计元数据**静默丢失**。

现在单一来源为 `internal/platform/httpresponse.UploadAuditMetadataKey`。
归属方是该常量描述的是 HTTP 上下文键语义（`httpresponse` 是信封与上下文约定的所有者），
而非 upload 业务语义，因此不放在 Files。

### 5. 文件模块的访问授权裁决：删除而非补实现

`authorize()` 删除后，「文件模块的访问授权只由路由级权限码决定」这一契约将失去唯一的代码痕迹。
裁决依据是**规格出处**：`openspec/specs/file-management/spec.md` 中
`Authorization` 零命中、`数据范围` 零命中；`上传者` 仅作为记录字段出现
（`:15` 要求数据库记录包含上传者 ID，`:315` 要求消息图片临时记录带上传者引用），
均未要求把它作为访问判据。

因此本次是**删除**，不是「暂缓实现」：补实现等于引入从未被要求的新需求，属独立变更。

为防重复误判，新增能力规格 `file-access-contract`
（`openspec/changes/remove-unwired-ports/specs/file-access-contract/spec.md`），
把该契约写成可引用的事实，而不是只能靠读代码推断的隐含行为。

**该规格的范围边界（验收时核实后收紧）**：契约只覆盖**普通文件记录**
（`purpose = managed_file`）与上述 9 个文件操作。消息图片的临时 File Record
（`purpose = message_image`）在绑定时**仍按上传者归属校验** ——
`internal/files/adapters/gorm/repository.go:101` 的 `uploader_id = ?` 是硬前置条件，
数量不匹配时返回 `ErrStateConflict`。该约束属消息能力的最小 Contract，
不由本决策删除。规格初稿的绝对措辞（"`uploader_id` SHALL NOT 作为访问判据"）
与实现不符，已在规格中标为「范围排除」。

### 6. 有意保留的扩展点（禁止在后续审计中作为缺口删除）

| 接缝 | 书面依据 |
| --- | --- |
| `MessagingMetrics` / `ConsumerDLQPendingObservation` | `docs/adr/0007-internal-messaging-domain-decisions.md:95`（且实际已完整接线） |
| 通知投影 Contract（`NotificationProjectionReader` / `ProjectNotificationRecipients`） | `docs/adr/0007:87`；`openspec/specs/internal-messaging/spec.md:83-89` |
| 邮件 / 短信 Consumer | `docs/adr/0007:87,105`；`openspec/specs/internal-messaging/spec.md:83-89` |
| `ExternalProxy`（外链与图片代理） | `docs/adr/0007:61,126`；所有者确认留给二次开发 |

`docs/adr/0007:105` 的原话是「只保留稳定的事件发布 Contract 和后续可接入的消费者扩展点，
**不创建空操作或伪实现**」—— 它同时是保留上表、以及本 ADR 第 2 节禁止新增空接缝的依据。

## 约束与验证

- 本决策不改变任何可观察行为：路由集合、权限码、HTTP 状态码与业务 JSON、
  数据库结构、Redis key、MinIO bucket 与后台任务集合均不变。
- 验证要求：`go build ./...`、架构边界测试、全量 `go test ./...`、
  `-tags=mysql_integration` 真实 MySQL 门禁、变更前后 120 条路由快照逐条比对。
- 「既有测试全绿」是**结果**而非**证明**。删除的前置证据是**读取点可达性**：
  每个待删项先定位其唯一读取点，再确认该读取点在当前配置下不可达。
- 顺序约束：接线护栏变更（`docs/reviews/wire-guardrail-design.md`）**必须在本决策落地后**实施 ——
  护栏若把 `files.Dependencies.Authorization` 标为 `wiring: required`，
  在字段仍存在时会使护栏立即失败。

## 后果

- 「已声明的能力」与「生效的能力」在 Files、Authorization、Messaging 三个模块恢复一致。
- 授权与审计端口的接入成本变为显式的：重新引入需要规格出处、组合根注入与行为测试三者齐备。
- 有意预留的扩展点获得了明确的书面边界，后续实现审计不再重复报告它们。
- 删除的行为契约（文件访问授权来源）改由 `file-access-contract` 规格承载，
  不再依赖代码痕迹；这与 `docs/README.md`「当前系统行为以 `openspec/specs/` 为准」一致。
- 本 ADR 记录的是**决策理由与原始签名**，不是实现指南；
  第 3 节的代码块已从仓库删除，仅在此作为历史与重新引入的输入保留。
