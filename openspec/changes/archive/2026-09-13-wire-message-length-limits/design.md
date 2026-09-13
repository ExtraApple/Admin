## Context

`messaging.max_title_runes` 与 `messaging.max_body_runes` 是死配置 ——
被解析、被设默认、被校验、被测试断言可加载，但**没有任何代码读取它们**。
实际约束来自 `internal/messaging/domain/content.go:16-17` 的硬编码常量。

调用链现状：

```
domain.CompileMessageContent(title, markdown)   content.go:43
  ↑ 内部读 MaxMessageTitleRunes / MaxMessageBodyRunes 常量

调用点 4 处：
  announcement_service.go:41   CreateAnnouncement
  announcement_service.go:205  EditAnnouncement
  broadcast_service.go:45      CreateBroadcast
  service.go:311               SendPrivateMessage
```

关键架构约束（`openspec/specs/modular-layered-architecture/spec.md`
「模块内部按复杂度分层」与「核心层隔离框架和全局状态」）：

> `internal/messaging/domain` SHALL NOT 依赖 `internal/platform/config`
> 或其他模块的 Adapter。

因此上限**不能**由 domain 直接读配置，必须以值参数经 application 层传入。

另一项既有事实：`MaxMessageHTMLBytes = 128 * 1024` 是**安全上限**
（防止清洗后 HTML 体积爆炸），与可配置的业务长度上限性质不同，保持硬编码。

## Goals / Non-Goals

**Goals:**

- 使两个配置项真正生效，且**默认部署的行为完全不变**。
- 保持 domain 层纯净 —— 不引入对 config 的依赖，架构测试不得被破坏。
- 把长度上限这一约束补入主规格（当前主规格未记录，仅归档 delta 有）。

**Non-Goals:**

- 不改变默认数值（100 / 20000）。
- 不做按组织或按消息类型的差异化上限。
- 不放宽 `MaxMessageHTMLBytes`。
- 不引入运行时热更新 —— 上限仍为启动时读取。

## Decisions

### D1｜上限以值类型经 application 层传入 domain

**决策**：在 domain 定义 `ContentLimits{MaxTitleRunes, MaxBodyRunes int}`，
`CompileMessageContent(title, markdown string, limits ContentLimits)` 接受它。

**理由**：满足三层约束（domain 不依赖 config），且把"上限"建模为领域概念
而非配置结构的泄漏。`ContentLimits` 属于 domain，配置值在组合根转换为它。

**替代方案对比**：

| 方案 | 否决理由 |
| --- | --- |
| domain 直接读 `platform/config` | 违反三层约束，架构测试会拦截 |
| 上限作为包级可变变量，启动时 set | 引入全局可变状态，违背「核心层隔离全局状态」；且测试并行时互相污染 |
| 上限作为 `Service` 字段，由方法内使用 | 可行（见 D2），但仍需传参给 domain 函数，无法省去参数 |
| 在 handlers 层预校验长度，domain 不再校验 | 域的约束被移到适配器，Handler 数量增长时会漏；且 domain 的既有测试需重写 |

### D2｜上限存放在 `Service` 上，由 `Dependencies` 注入

**决策**：`messaging.Dependencies` 新增字段（如 `ContentLimits domain.ContentLimits`），
`NewService` 存入 `Service`，4 个调用点从 `service.contentLimits` 取值传入。

**理由**：4 个调用点分布在同一 `Service` 上（announcement / broadcast / service
三个文件的方法都属于 `*Service`），单一来源比逐点传参更不易漏。

**注意**：该字段应标注 `// wiring: optional —— constructor 兜底默认 100/20000`，
以便接线护栏（`add-dependency-wiring-guardrail`）通过，
且测试夹具无需全部改写。

**兜底值必须等于 `MaxMessageTitleRunes` / `MaxMessageBodyRunes`**，
使"未注入配置"的测试夹具行为与今日一致。

### D3｜保留常量作为默认值来源，不删除

**决策**：`MaxMessageTitleRunes` 与 `MaxMessageBodyRunes` 常量保留，
作为 `DefaultContentLimits()` 的来源。

**理由**：域内既有测试（`content_test.go` 的 4 个测试）直接引用这两个常量。
删除会迫使测试改写，且常量本身是"域的安全默认值"这一事实的载体。

**替代方案**：把默认值移到 config 层。
**否决理由**：默认值的语义属于域约束（"一条消息标题的合理上限"），
放在 config 层会让域失去自知。

### D4｜配置校验补上界

**决策**：`config.go:282` 现有校验为 `MaxTitleRunes < 1 || MaxBodyRunes < 1` 报错。
补充上界校验，防止配置出明显不合理的值。

**理由**：上限被调至极大值会让 `MaxMessageHTMLBytes` 成为唯一防线，
而后者是安全上限而非业务上限 —— 两者的错误语义不同，
应在上限层面就拒绝而非让请求在 HTML 体积检查处失败。

**取值**：上界在 tasks 中依实际需要确定；不得低于既有归档规格声明的默认值
（100 / 20,000）。

### D5｜规格补齐为 ADDED 而非 MODIFIED

**决策**：在 `internal-messaging` 规格中 **ADDED** 一条长度上限要求。

**理由**：主规格当前**没有**这条要求（压缩时丢失），
归档 delta 的 `:234` 有但归档不再生效。既然主规格无此要求，
补入即为 ADDED，而非修改既有要求。

## Risks / Trade-offs

**[R1] 默认值意外改变** →
兜底值与 `config.yaml` 默认值均须为 100 / 20000。
验证含显式断言：以默认配置构建时，标题 100 字符通过、101 字符被拒。

**[R2] 上限被调小导致既有消息无法编辑** →
`EditAnnouncement`（`announcement_service.go:205`）会对既有正文重新校验。
若运维把上限调小到低于既有消息长度，编辑该消息会失败。
这是**预期的收紧语义**，但需在规格与 runbook 中写明。
缓解：本轮不调默认值；在 `docs/runbooks/messaging.md` 增加一行说明。

**[R3] domain 层污染** →
若实现时图省事让 domain 读 config，架构测试
（`TestArchitectureCoreLayersAvoidFrameworkAndAdapterImportsSkeleton`）会失败。
这是护栏生效而非风险，但需在实施前明确参数化路径。

**[R4] 测试夹具大量改写** →
若把 `ContentLimits` 设为 `required` 且无兜底，
所有构造 `Dependencies` 的测试夹具都需补字段。
由 D2 的 optional 兜底规避。

**[R5] 上界取值缺乏依据** →
`config_test.go:284` 已用 25000 测试自定义正文上限，说明该量级被接受过。
上界应至少覆盖此值，避免使既有配置测试失效。

## Migration Plan

无数据迁移。配置项已存在且默认值不变，现有部署无需改动。

**部署注意**：若某部署的 `config.yaml` 已填入非默认值（如 120 / 25000），
本变更上线后这些值**将首次真正生效**。上线前应核对部署配置中的实际值，
确认其符合预期上限 —— 这是本变更唯一的部署期风险。

回滚：恢复硬编码常量读取即可，配置项回到"被忽略"状态（但不建议保留该状态）。

## Open Questions

1. `ContentLimits` 上界的合适取值 —— 需与 `MaxMessageHTMLBytes`（128 KiB）
   形成合理关系，避免业务上限使得安全上限先被触发。
2. 是否需要在 `docs/runbooks/messaging.md` 增加"调整长度上限"的操作说明，
   还是仅在配置注释中说明。
