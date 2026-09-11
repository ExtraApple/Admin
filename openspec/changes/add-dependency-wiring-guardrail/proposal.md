## Why

实现审计发现四个「已声明的依赖端口在组合根从不注入」的实例
（`docs/reviews/wire-audit.md` 第一、三节）。它们的共同形态是：
字段已加入 `Dependencies`、构造器已就位、测试已通过，
**但没有任何门禁会发现它没被接线**。

现有护栏只校验**形状** —— `routecatalog` 校验路由契约、
`testsupport/architecture_boundary_test.go` 的 16 个测试校验导入方向与
`AutoMigrate`/Gin 的唯一性 —— **没有一条校验依赖是否被注入**。
`files.Dependencies.Authorization` 因此长期静默存在，
并使 7 处对象级授权调用恒返回"放行"。

本变更引入该缺失的门禁，使同类缺口在测试中失败而非在生产中静默降级。

## What Changes

- **为两个 `Dependencies` 结构体的全部字段增加接线注解**：
  `// wiring: required` 或 `// wiring: optional`，并写明缺失时的后果。
- **新增一个基于 Go AST 的架构测试**，校验规则：
  `required` 字段必须在组合根的字段字面量中显式赋值；
  任何字段缺少注解即失败。
- **修正两处现存遗漏**：`files.Dependencies` 缺 `Clock` 与 `ObjectNames` 的
  赋值或注解；`messaging.Dependencies` 缺 `Clock` 的赋值或注解。
- **不改变任何生产运行时行为**：新增的是注释与测试；
  组合根仅补上「显式赋 nil 等价值」或使用既有构造器兜底。

**非目标**：

- 不新增、不修改任何业务能力或可观察行为。
- 不校验依赖的**语义正确性**（只校验"非 nil"）。行为正确性由各模块单元测试覆盖。
- 不覆盖「接口有字段但实现在语义上是错的」这类问题。
- 不覆盖连 `Dependencies` 字段都没有的端口（如已删除的 `MessagingAuditSink`）；
  该类由 ADR 0007 的扩展点契约约束，不在本护栏范围。
- 不校验后台任务是否被注册（属独立变更）。

## Capabilities

### New Capabilities

无。护栏是开发期约束，不产生可观察的运行时行为，
故不建立新能力规格。

### Modified Capabilities

- `modular-layered-architecture`: 在既有 `Requirement: App 统一装配系统` 中
  **ADDED** 一条要求 —— 组合根必须显式装配每个 `Dependencies` 字段，
  且该约束由自动化架构测试强制。既有要求内容不变，故使用 ADDED 而非 MODIFIED。

（选择该落点的理由：该规格已承载「App 是唯一组合根」的架构约束，
接线完整性属于同一关注点。不新建 capability 是因为它不对应任何产品能力。）

## Impact

### 受影响代码

| 位置 | 变更 |
| --- | --- |
| `internal/files/application/contracts.go` | `Dependencies` 12 个字段增加 `wiring` 注解 |
| `internal/messaging/application/service.go` | `Dependencies` 13 个字段增加 `wiring` 注解 |
| `internal/app/files.go` | 按注解补齐 `Clock` 与 `ObjectNames` 的显式赋值 |
| `internal/app/messaging.go` | 按注解补齐 `Clock` 的显式赋值 |
| `testsupport/architecture_boundary_test.go` | 新增接线完整性测试 |

### 前置依赖（硬）

**本变更必须在 `remove-unwired-ports` 完成之后实施。**

```
files.Dependencies.Authorization 与 Audit 一旦标注为 `wiring: required`
  ⇒ 本护栏立即失败（组合根确实未接线）
  ⇒ 只有 remove-unwired-ports 删除这两个字段后，本护栏才能通过
```

若顺序颠倒，唯一出路是把它们强标为 `optional` —— 那会让护栏沦为掩盖工具，
正是本变更要消除的问题。依据见 `docs/reviews/wire-guardrail-design.md` 第六节。

### 不受影响

路由（120 条）、权限码、HTTP 状态码与业务 JSON、数据库结构、Redis key、
MinIO bucket、后台任务集合（11 个）、OpenAPI 输出。

### 风险与验证

**风险等级 R1**（新增护栏，不改生产行为）：注解是注释，测试是新文件。

**风险**：标注错误会导致新测试失败。这是期望行为，但需在实施时逐一核对
每个 `optional` 判定 —— 只有构造器确有兜底分支的字段才可标 `optional`。

**验证**：

1. `go build ./...`
2. `go test ./... -count=1` —— 既有 160 个测试文件全部通过
3. `go test ./testsupport -run TestArchitecture -count=1` —— 新护栏通过
4. **负向验证**：临时移除组合根中某一 `required` 字段的赋值，
   确认新测试**确实失败**；随后恢复。这是护栏有效性的唯一证据。

### 相关文档

- 护栏设计：`docs/reviews/wire-guardrail-design.md`（规则、14 条注解清单、
  AST 测试草案、5 类假阳性处置）
- 缺口来源：`docs/reviews/wire-audit.md`
- 风险分级：`docs/reviews/change-risk-assessment.md`
