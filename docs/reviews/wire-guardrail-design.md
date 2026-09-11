# 接线护栏设计（② 接线完整性断言）

> 目标：让「声明了但没接线」在**测试中失败**，而不是在生产中静默降级。
> 这是实现审计（[wire-audit.md](wire-audit.md)）的防复发措施。
> **本文只设计，不实施** —— 实施需独立 OpenSpec change。

---

## 一、要防的是什么

四个隔离端口（`wire-audit.md` 第一节），其中三个是「有字段但没赋值」：

```
files.Dependencies       12 字段，组合根赋 8 个
  ├─ Authorization       缺 → authorize() 静默 return nil        🔴
  ├─ Audit               缺 → 直接记录路径永不执行                🟡
  ├─ Clock               缺 → constructor 兜底 time.Now           ✅ 安全
  └─ ObjectNames         缺 → constructor 兜底 UUIDObjectNames{}  ✅ 安全

messaging.Dependencies   13 字段，组合根赋 12 个
  └─ Clock               缺 → constructor 兜底 time.Now           ✅ 安全
```

**关键观察**：这三种缺失（真缺口 / 死代码 / 安全兜底）在外观上**完全一样** ——
都是"字段没被赋值"。区别只在于 `NewService` 里有没有兜底分支。

而门禁层面**完全没有覆盖**：
`routecatalog` 校验路由契约、`architecture_boundary_test.go` 校验导入方向与
`AutoMigrate`/Gin 的唯一性，**没有一条检查依赖是否被注入**。

---

## 二、护栏规则

### 规则

> **每个 `Dependencies` 结构体的字段，必须在注释中标注 `// wiring: required`
> 或 `// wiring: optional`；`required` 字段必须在组合根的字段字面量中显式赋值。**

### 为什么标注放在字段定义处

```
备选 A：在测试里维护一份 required 字段清单
  ✗ 清单与被标注对象相隔两个文件，极易静默分叉

备选 B：在组合根用注释标记「这个字段故意不赋」
  ✗ 新增字段时作者不会想到去看组合根，等于没护栏

备选 C（采用）：注解写在字段定义旁
  ✓ 新增字段时，字段和注解在同一行，作者必然看到
  ✓ 验证点在组合根，失败信息精确指向「某模块某字段未接线」
  ✓ 读取依赖结构体时能立刻看出哪些是可空的
```

### 注解形式

```go
type Dependencies struct {
    Repository               Repository
    Storage                  ObjectStorage
    Validator                UploadValidator
    MessageImages            MessageImageRepository
    MessageImageValidator    UploadValidator

    // wiring: required —— 缺失即静默放行全部对象级授权
    Authorization            AuthorizationScope

    Transactions             TransactionRunner

    // wiring: optional —— constructor 兜底 ClockFunc(time.Now)
    Clock                    Clock

    // wiring: optional —— constructor 兜底 UUIDObjectNames{}
    ObjectNames              ObjectNameGenerator

    // wiring: optional —— 预留的第二审计通道；实际审计走 Gin context
    Audit                    AuditMetadataSink

    Signer                   Signer
    DownloadURLExpireSeconds int
}
```

**注解内容必须写明「缺失时的后果」**，而不是只写 required/optional。
理由：本项目的核心风险正是"缺失时的后果不直观"，注解是唯一能随代码一起演进的位置。

---

## 三、初始标注清单（14 条）

### `internal/files/application/contracts.go:234-247`

| 字段 | 标注 | 依据 |
| --- | --- | --- |
| `Repository` | required | 无兜底，缺失即 panic 级错误 |
| `Storage` | required | 同上 |
| `Validator` | required | `service.go:84-86` 缺失返回 internal error（非静默） |
| `MessageImages` | required | 消息图片绑定依赖 |
| `MessageImageValidator` | required | 同上 |
| `Authorization` | **required** | `service.go:56-58` 缺失即放行 —— **S1 待裁决后调整** |
| `Transactions` | optional | `service.go:33-35` 兜底 `directTransactionRunner{}` |
| `Signer` | required | 下载签名依赖 |
| `Clock` | optional | `service.go:24-26` 兜底 `ClockFunc(time.Now)` |
| `ObjectNames` | optional | `service.go:30-32` 兜底 `UUIDObjectNames{}` |
| `Audit` | optional | 预留通道，实际审计走 Gin context |
| `DownloadURLExpireSeconds` | optional | `service.go:27-29` 兜底 300 |

**注**：`Authorization` 若最终裁决为 B（删除接口），则本行连同字段一起移除。

### `internal/messaging/application/service.go:37-51`

| 字段 | 标注 | 依据 |
| --- | --- | --- |
| `Messages` / `Categories` / `Inbox` / `Notifications` / `Outboxes` | required | 仓储必填 |
| `ConsumerDeadLetters` / `ConsumerDeadLetterReplay` | required | 死信处置必填 |
| `Identity` / `Organizations` | required | 受众解析必填 |
| `Authorization` | required | 组织数据范围校验 |
| `Files` | required | 消息图片 |
| `Transactions` | optional | `service.go:73-75` 兜底 `directTransactionRunner{}` |
| `Clock` | optional | `service.go:70-72` 兜底 `ClockFunc(time.Now)` |

---

## 四、测试实现草案

位置：`testsupport/architecture_boundary_test.go`（与既有 16 个架构测试同文件同风格，使用 `go/ast`）

```go
func TestArchitectureDeclaredDependenciesAreWired(t *testing.T) {
    root := architectureRepositoryRoot(t)

    // 第一遍：收集所有 Dependencies 结构体的字段 → required/optional
    //   - 解析 internal/**/非测试/*.go
    //   - 命中 `type Dependencies struct`，按字段行上方的注释取注解
    //   - 无注解的字段 ⇒ 报错「字段缺少 wiring 注解」
    required := collectDependencyAnnotations(t, root)

    // 第二遍：扫描组合根的所有字段字面量，收集已赋值的字段名
    assigned := collectAssignedFields(t, filepath.Join(root, "internal", "app"))

    // 第三遍：required 字段必须出现
    for _, field := range required {
        if !assigned[field.Name] {
            t.Errorf("%s.%s is declared required but never assigned in internal/app",
                field.Owner, field.Name)
        }
    }
}
```

### 已知难点与处置

| 难点 | 处置 |
| --- | --- |
| 如何确认字面量属于某个 `Dependencies` 类型 | 第一遍同时收集「构造器调用 → 参数类型」。若某构造器的参数非常量类型，报错要求显式化 |
| 位置参数式字面量（`Dependencies{a, b}`）无法按名匹配 | 检测到位置参数式字面量直接报错（当前代码库全部使用字段名式） |
| 类型别名（`type Deps = Dependencies`）会绕过 | 检测到 `= Dependencies` 的别名声明即报错 |
| 两个模块有同名字段（如两个 `Clock`） | 键用 `模块路径 + 字段名`，不用裸字段名 |
| 注解书写错误（`//wiring:requird`） | 只接受精确的 `// wiring: required` / `// wiring: optional`，其余一律报错 |

---

## 五、这条护栏能抓什么、抓不到什么

### 能抓

```
✅ 新增 required 字段但组合根没接线        ← 本次三个缺口的共同形态
✅ 把 optional 字段改成 required 后忘记赋值
✅ 字段完全没有 wiring 注解（强制显式决策）
```

### 抓不到（需另行覆盖）

```
❌ MessagingAuditSink —— 它连 Dependencies 字段都没有
   → 本护栏看不见它
   → 需要另一条规则：ADR 要求的能力必须有对应端口且被接线
   → 建议在 S2 裁决时一并处理

❌ 接口有字段但实现在语义上是错的
   → 本护栏只验证「非 nil」，不验证「行为正确」
   → 由各模块的单元测试覆盖

❌ 后台任务未注册（如公告调度）
   → 属 ① 后台任务扫描的范围
```

---

## 六、实施影响面与顺序

```
新增注解        14 处（files 12 + messaging 13，去掉重叠）
新增测试        1 个（testsupport/architecture_boundary_test.go）
需要修正的现状   2 处：
                ├─ files.Dependencies    缺 Authorization 与 Audit 的赋值或注解
                └─ messaging.Dependencies 缺 Clock 的赋值或注解
```

**注意**：`files.Authorization` 一旦标注为 `required`，测试**当天就会失败**
（因为组合根确实没接线）。这是**期望行为** —— 护栏的价值就在于让已知缺口变成红灯。

因此本护栏对应计划中的 **C2**，**必须在 C1（移除文件模块未接线的授权端口）之后**
独立开 change 实施。

```
C1  移除文件模块未接线的授权端口     ← 先做：删除 files.Authorization 字段
C2  依赖接线完整性护栏（本文）        ← 后做：C1 完成后本护栏才能通过
```

完整变更清单与顺序依据见 [wire-audit.md](wire-audit.md) 第六节。
