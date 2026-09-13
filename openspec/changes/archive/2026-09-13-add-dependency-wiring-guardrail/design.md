## Context

`internal` 下只有两个 `Dependencies` 结构体 —— 其他模块使用位置参数构造器：

```
internal/files/application/contracts.go:234-246   12 字段，组合根赋 8 个
internal/messaging/application/service.go:37-51   13 字段，组合根赋 12 个
```

未赋值的 5 处中，**2 处是真缺口**（`Authorization` 静默放行、
`Audit` 分支永不进入），**3 处有构造器兜底**（`Clock` ×2、`ObjectNames`）。

**三者在代码外观上完全一样** —— 都是"字段没被赋值"，区别只在
`NewService` 里有没有兜底分支。这正是本护栏要解决的可见性问题。

约束：

- 现有 `testsupport/architecture_boundary_test.go` 已有 16 个架构测试，
  统一使用 `go/ast` 遍历非测试生产文件。新测试须与之一致。
- `docs/agents/domain.md:52-56` 规定 ADR 的创建条件（改变成本高、
  缺背景会困惑、有真实取舍）—— 本护栏属"防复发的机械约束"，
  由规格与 ADR 0008 承载即可，不单独建 ADR。

## Goals / Non-Goals

**Goals:**

- 使「声明了 `required` 依赖但组合根未接线」在**测试**中失败。
- 强制每个字段都必须显式声明 required 或 optional —— 消除"没注解"的默认态。
- 注解内容包含**缺失时的后果**，而不只是 required/optional 标记。
- 用负向验证证明护栏真的会失败，而不是一个恒绿的测试。

**Non-Goals:**

- 不校验依赖的语义正确性（只校验非 nil）。
- 不覆盖没有 `Dependencies` 字段的端口。
- 不校验后台任务注册。
- 不改变任何生产运行时行为。

## Decisions

### D1｜注解写在字段定义旁，而非集中清单

**决策**：`// wiring: required` / `// wiring: optional` 写在字段正上方。

**理由**：注解与被标注对象在同一行视野内，新增字段时作者必然看到，
从而必须做出显式决策。

**替代方案对比**：

| 方案 | 否决理由 |
| --- | --- |
| 在测试里维护 required 字段清单 | 清单与被标注对象相隔两个文件，会静默分叉 |
| 在组合根用注释标记"故意不赋" | 新增字段时作者不会去看组合根，等于没护栏 |
| 不注解，要求全部字段必须赋值 | `files.Clock` 等字段在测试夹具中本就可省略，会迫使大量夹具改写 |

### D2｜注解必须写明缺失后果，而非只有标记

**决策**：注解正文说明"缺失时会发生什么"。

**理由**：本项目的核心风险恰是"缺失后果不直观" ——
`Authorization` 缺失表现为静默放行而非报错。若注解只写 `required`，
读者仍需回读 `NewService` 才能判断严重性。

示例：

```go
// wiring: required —— 缺失即静默放行全部对象级授权
Authorization AuthorizationScope

// wiring: optional —— constructor 兜底 ClockFunc(time.Now)
Clock Clock
```

### D3｜只校验 required 字段在组合根被赋值

**决策**：护栏校验「required 字段是否出现在组合根字段字面量中」，
不校验 optional 字段。

**理由**：`optional` 的定义就是"有构造器兜底，可省略"。
若连 optional 也强制赋值，测试夹具需大量改写，且会把
`ClockFunc(time.Now)` 这类无害兜底变成噪音。

### D4｜标注判定标准

**决策**：`optional` 仅当满足全部两条时才可使用 ——
构造器存在**显式兜底分支**，且兜底值为**安全默认**
（不改变业务语义、不放宽校验）。

依据 `NewService` 的实际兜底情况：

| 字段 | 构造器兜底 | 标注 |
| --- | --- | --- |
| `files.Clock` | `service.go:24-26` → `ClockFunc(time.Now)` | optional |
| `files.ObjectNames` | `service.go:30-32` → `UUIDObjectNames{}` | optional |
| `files.Transactions` | `service.go:33-35` → `directTransactionRunner{}` | optional |
| `files.DownloadURLExpireSeconds` | `service.go:27-29` → 300 | optional |
| `messaging.Clock` | `service.go:70-72` → `ClockFunc(time.Now)` | optional |
| `messaging.Transactions` | `service.go:73-75` → `directTransactionRunner{}` | optional |
| 其余全部 | 无兜底 | required |

**注意**：`files.Authorization` 与 `files.Audit` 在
`remove-unwired-ports` 完成后**已不存在**，不参与标注。

### D5｜测试位置与实现方式

**决策**：新增测试放入 `testsupport/architecture_boundary_test.go`，
使用 `go/ast`，与既有 16 个架构测试同文件同风格。

**采集逻辑**：

```
第一遍  遍历 internal/**/非测试/*.go
        命中 `type Dependencies struct`，读取每个字段的注解
        无注解的字段 ⇒ 报错「字段缺少 wiring 注解」
第二遍  扫描 internal/app 的全部字段字面量，收集已赋值的字段名
第三遍  required 字段必须出现在已赋值集合中
```

**键的选择**：`模块路径 + 字段名`，不用裸字段名 ——
两个模块都有 `Clock` / `Transactions`，裸字段名会误判。

## Risks / Trade-offs

**[R1] 位置参数式字面量无法按名匹配** →
检测到 `Dependencies{a, b}` 形式即报错。当前代码库全部使用字段名式字面量。

**[R2] 类型别名可绕过** →
检测到 `type X = Dependencies` 形式的别名声明即报错。

**[R3] 注解书写错误会静默失效** →
只接受精确的 `// wiring: required` 与 `// wiring: optional`；
其余任何形式（含拼写错误）一律报错，不视为"未注解"以外的其他语义。

**[R4] 注解可能与实现脱节** →
若字段标注 `optional` 但构造器后来移除了兜底分支，护栏不会发现。
缓解：D2 要求注解写明兜底来源，review 时可对照。

**[R5] 护栏可能与真实构造路径脱节** →
护栏只检查"组合根字面量里有这个字段"，不检查赋的值是否非 nil
（如 `Authorization: nil` 会通过）。这是刻意的取舍：
静态检查无法求值。真正的防线是 `required` 字段在 review 中需确认赋了真实实现。

**[R6] 前置依赖未满足会使本护栏立即失败** →
由 proposal 的硬前置声明约束：必须在 `remove-unwired-ports` 完成后实施。
若顺序颠倒，只能把 `Authorization` 强标 optional，那等于用护栏掩盖缺口。

## Migration Plan

无数据迁移，无配置变更，无部署步骤。

回滚即删除新增测试与注解（单一提交内完成）。

实施顺序：

```
1. 确认 remove-unwired-ports 已验收
2. 为 25 个字段增加注解
3. 补齐组合根中 3 处 optional 字段的显式赋值（或确认可省略）
4. 新增 AST 测试
5. 运行全量验证
6. 负向验证：移除一个 required 赋值，确认测试失败，随后恢复
```

## Open Questions

1. `messaging.Dependencies.Clock` 与 `files.Dependencies.Clock` 在测试夹具中
   大量省略 —— 标注 `optional` 后不影响既有夹具，但需确认没有夹具
   依赖"Clock 为 nil 时不得工作"这一隐含语义。
2. 是否需要为 `routes` / `services` 之外的其他结构体（如
   `EventConsumerConfig`、`OutboxWorkerConfig`）也引入同类注解 ——
   本变更只覆盖真正的 `Dependencies` 结构体，其余待后续按需评估。
