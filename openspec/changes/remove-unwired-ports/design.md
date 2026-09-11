## Context

实现审计（`docs/reviews/wire-audit.md`）在「端口实现」维度发现四个隔离端口 ——
接口已声明、字段已加入 `Dependencies` 或方法已实现，但在组合根从未注入、
或在整个生产代码中零引用。逐一核实读取点后确认它们在当前配置下**没有执行路径**。

本设计的核心问题不是"怎么删"，而是**如何确认删除是行为中性的**，
以及**如何避免删除造成不可逆的信息损失**。

约束来自项目现有架构决策：

- `docs/adr/0007-internal-messaging-domain-decisions.md:95` 与 `:105` 明确声明
  「只保留稳定的事件发布 Contract 和后续可接入的消费者扩展点，
  **不创建空操作或伪实现**」—— 这条决定了哪些接缝属有意预留、不可删除。
- `docs/agents/domain.md:52-56` 规定 ADR 的创建条件：
  改变成本高、缺背景会困惑、存在真实取舍。本变更的删除决策同时满足三条。

## Goals / Non-Goals

**Goals:**

- 移除四个未生效的依赖端口及其配套死代码，使"声明的能力"与"生效的能力"一致。
- **可证明地**保持全部可观察行为不变 —— 不是"相信它不变"，而是给出核验依据。
- 用一份 ADR 保存删除的裁决与理由，把信息损失降到可接受。

**Non-Goals:**

- 不为任何模块新增授权或审计能力。
- 不移除有意预留的扩展点（见下方决策 D0）。
- 不处理公告调度未注册、过期消息图片清理、消息长度上限死配置
  （分属独立变更，见 `docs/reviews/contract-readiness.md` 的批次计划）。
- 不引入接线护栏 —— 它必须在本变更完成后独立实施（见风险 R3）。

## Decisions

### D0｜先划清「有意预留」与「未生效」

**决策**：判定规则为 —— 在 ADR 或规格中存在明确"本次不实现/未来扩展点"表述的
接缝属**设计**；没有此类表述的属**缺口**。

**理由**：本审计第一稿把「未接线」一律视为缺口，导致
`MessagingMetrics`、`NotificationProjectionReader`、邮件/短信 Consumer
被误报。核对 ADR 0007 后确认它们是已声明的扩展点。

**A 类（保留，本变更不动）**：

| 接缝 | 书面依据 |
| --- | --- |
| `MessagingMetrics` | ADR 0007:95（且实际已完整接线） |
| `NotificationProjectionReader` / `ProjectNotificationRecipients` | ADR 0007:105 |
| 邮件 / 短信 Consumer | ADR 0007:87,105；`internal-messaging/spec.md:83-89` |
| `ExternalProxy` | 所有者确认留给二次开发；ADR 0007:126 |

**B 类（本变更删除）**：`AuthorizationScope`、`UserVisible`、
`OrganizationVisible`、`AuditMetadataSink`、`MessagingAuditSink`。

**替代方案**：为 `AuthorizationScope` 补实现（引入文件数据范围）。
**否决理由**：`openspec/specs/file-management/spec.md` 检索 "Authorization" 与
"数据范围" 均零命中 —— 该能力从未被要求，补实现等于引入新需求，
属独立变更而非本次清理。

### D1｜用「读取点可达性」证明行为中性，而非依赖测试通过

**决策**：对每个待删项，先定位其**唯一读取点**，再确认该读取点在当前配置下
是否可达。不可达 ⇒ 无行为可删。

**核实结果**：

| 待删项 | 唯一读取点 | 可达性 |
| --- | --- | --- |
| `files.Dependencies.Audit` | `files/application/service.go:510-511` | ❌ `if s.deps.Audit != nil` 恒为假 |
| `files.Dependencies.Authorization` | `files/application/service.go:56-59` | ⚠️ 7 处调用点会执行，但函数体第一步 `return nil` ⇒ 输出恒为"放行" |
| `UserVisible` / `OrganizationVisible` | 无（生产与测试均零引用） | ❌ 不可达 |
| `MessagingAuditSink` | 无（无 `Dependencies` 字段） | ❌ 不可达 |

**注意第二行**：`Authorization` 的调用点**确实会执行**。
删除它们会让 7 处调用消失 —— 净行为不变（都是放行），
但这是本变更中最需要谨慎的一处：**必须先确认 `deps.Authorization`
在所有构造路径下都为 nil**，否则删除会改变行为。

**替代方案**：只依赖"既有测试全绿"作为证据。
**否决理由**：测试通过是**结果**，不是**证明**。若某个行为恰好无测试覆盖，
测试通过不能说明行为未变。读取点可达性是更强的前提。

### D2｜信息损失用 ADR 补偿，而非保留死代码

**决策**：删除必须与 ADR 同批交付。

**理由**：`authorize()` 与 `UserVisible()` 的存在本身在陈述
"这些操作**应该**被判权"。删除后，未来读者（含二次开发者）
不会知道这里曾有判权意图，可能直接写下无判权的文件操作。

**替代方案**：保留代码但加 `// 未接线，勿使用` 注释。
**否决理由**：本变更的动因恰恰是"留着会误导"——
注释不改变 7 处调用点看起来在检查权限这一事实，
而 `docs/reviews/wire-audit.md` 的审计过程已证明该形态会反复导致误判。

### D3｜审计常量收敛为单一来源

**决策**：`files/application` 与 `audit` 两处各自定义的
`UploadAuditMetadataContextKey` 收敛为一处。

**理由**：两处当前字符串值相同（`files/application/contracts.go:158`
与 `audit/service.go:20`），但靠**约定而非编译期保证**一致。
`files` 侧写、`audit` 侧读，任一侧改动都会导致上传审计元数据静默丢失。

**取舍**：需要决定常量归属方。`audit` 是消费方且定义读取语义，
但 `files` 是写入方且该常量描述的是 upload 语义 —— 具体归属在 tasks 中确定。

### D4｜Capabilities 为空是刻意的

**决策**：本变更不产生 delta spec 文件，`## ADDED/MODIFIED/REMOVED Requirements` 均为空。

**理由**：删除的代码在规格中**没有任何对应要求**（见 D0 的核对结论），
因此不存在需要修改的规格行为。若强行新增一条"系统不提供文件对象级授权"
的规格要求，那是**引入新约束**，属 C6a 的范围，不应混入本次清理。

## Risks / Trade-offs

**[R1] `deps.Authorization` 可能在某个未检查的构造路径下被注入** →
在删除前逐一核对全部构造点（`internal/app/files.go` 及各测试夹具）。
已在 `internal/app/files.go:34-43` 确认未注入，但需在实施时重核一次，
因为审计与实施之间可能有其他变更落地。

**[R2] 删除造成不可逆的信息损失** → 由 D2 的 ADR 缓解。
ADR 必须包含被删端口的原始签名与设计意图，作为将来重新引入的输入，
而不只是"已删除"的结论。

**[R3] 护栏变更若与本变更同批或先行，会立即失败** →
设计上明确顺序：护栏的 `wiring: required` 注解依赖
`files.Dependencies.Authorization` **已不存在**。
本变更完成后，护栏才能通过。两者必须是两个 change。
依据：`docs/reviews/wire-guardrail-design.md` 第六节。

**[R4] 删除范围蔓延** → 明确非目标清单，并在 tasks 中限定文件范围。
特别地，不得顺手删除 A 类接缝（见 D0 表），
也不得顺手修改消息审计或文件授权的实际行为。

**[R5] 测试可能在删除后暴露既有的隐藏耦合** → 这是**期望结果**而非风险：
若某个测试确实引用了待删符号，说明审计遗漏了引用点，
应先核实该引用是否构成真实调用路径，再决定删除或保留。
`audit_contract_test.go` 是已知的一处（它只做 `var _ =` 接口断言）。

## Migration Plan

无数据迁移。无配置变更。无需回滚脚本 —— 回滚即恢复被删文件（单一提交内完成）。

部署无需特殊步骤：本变更不改变任何运行时可观察行为，
不引入新依赖，不改动数据库结构、Redis key 或 MinIO bucket。

## Open Questions

1. `UploadAuditMetadataContextKey` 的归属方（`files` 还是 `audit`）——
   在 tasks 中根据"谁定义语义"确定。
2. ADR 编号 —— 现有 ADR 至 `0007`，本变更的 ADR 应为 `0008`；
   需确认无其他在途变更同时占用该编号。
