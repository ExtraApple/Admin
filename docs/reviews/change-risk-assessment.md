# 改动风险评估 · 与"当前可正常运行"的对比

> 起因：项目所有者提问「目前的情况是可以正常运行完整模块，你改动之后呢」。
> 本文不提出新方案，只对**已经在讨论中的删除/改动**做风险分级，
> 并核实每条的依据。**结论可能与直觉相反。**

---

## 一、先纠正一个前提：这些都不是"能正常运行的代码"

提问隐含一个假设：*被删的东西正在运行，删了会破坏它。*
但经核实，**待删的每一项在当前配置下都不产生任何行为**。

### 核实方法

对每个待删项，检查其**读取点**是否可达。

| 待删项 | 读取点 | 当前是否可达 | 依据 |
| --- | --- | --- | --- |
| `files.Dependencies.Audit` | `files/application/service.go:510-511`（全库唯一） | ❌ **不可达** | `if s.deps.Audit != nil` —— 组合根从不注入 ⇒ 恒为 nil ⇒ 分支永不进入 |
| `files.Dependencies.Authorization` | `files/application/service.go:56-59`（全库唯一） | ⚠️ **可达，但输出恒定** | `if ... \|\| s.deps.Authorization == nil { return nil }` ⇒ 恒为 nil ⇒ 恒返回 nil |
| `authorization.UserVisible` | 无（生产与测试均零引用） | ❌ **不可达** | 全库检索仅命中定义行 |
| `authorization.OrganizationVisible` | 无（生产与测试均零引用） | ❌ **不可达** | 同上 |
| `messaging.MessagingAuditSink` | 无（生产零引用，测试仅 `var _ =` 断言） | ❌ **不可达** | 连 `Dependencies` 字段都没有 |

**关键差异**：`Authorization` 那一项**有 7 个调用点会执行**，
但函数体在第一步就返回，所以调用点执行了、判断没发生。
删除它会让这 7 处调用点一起消失 —— **净行为不变**（都是放行）。

---

## 二、风险分级

### R0｜零行为变更（可证明）

```
删除 items：
  files.Dependencies.Audit + AuditMetadataSink 接口 + service.go:510-511
  files.Dependencies.Authorization + AuthorizationScope 接口
    + Operation 常量集 + 7 处 authorize 调用 + authorize 函数本身
  authorization.UserVisible + OrganizationVisible
  messaging.MessagingAuditSink + MessagingAuditEntry + audit_contract_test.go
```

**为何是 R0**：这些代码**在当前配置下没有执行路径**。
删除它们不可能改变任何 HTTP 响应、任何数据库写入、任何后台行为。

```
风险来源           是否存在
  行为变更          ❌ 无执行路径 ⇒ 无行为可改
  编译失败          ⚠️ 需一并删除引用者（见下）
  测试失败          ⚠️ 仅 audit_contract_test.go 一处（它只验证接口存在）
  数据迁移          ❌ 不涉及
  配置变更          ❌ 不涉及
```

**唯一需要小心的是"引用者是否删干净"** —— 这是编译期问题，`go build` 会立刻报出，
不会漏到运行时。

### R0 的**非零**代价（修正：先前"零风险"的措辞过于绝对）

删除**不是没有成本**，只是成本不体现在运行时。有三条必须写明：

| 代价 | 说明 | 缓解 |
| --- | --- | --- |
| **丢失一个有用的信号** | `authorize()` 的存在本身在说"这些操作**应该**被判权"。删掉后，未来的读者（含二次开发者）不会知道这里曾有判权意图，可能直接写下无判权的文件操作 | **必须**在 ADR 记录决策与理由；否则这是净损失 |
| **二次开发的 re-entry 成本** | 若将来要做文件数据范围，需重新设计接口 + 7 个调用点 + `Operation` 常量集 | 在审计文档保留本文与 `wire-audit.md` 第一节作为设计输入 |
| **掩盖一个可能的设计缺陷** | 另一种解释是：`AuthorizationScope` 是**为将来准备的接缝，只是没接线**。若属此类，正确处置是**补实现**而非删除 | 已由所有者裁决为 B（删除）。此项因此**不再是风险，但也应记录"曾考虑过 A"** |

**所以准确的表述是**：删除的**运行时风险为零**，但**有信息损失**。
信息损失可用文档弥补 —— 这正是本文存在的意义。

### R1｜新增护栏，不改生产行为

```
C2：14 条 wiring 注解 + 1 个 Go AST 架构测试
```

注解是注释，测试是新文件。**不触碰任何生产代码路径。**

```
风险来源           是否存在
  行为变更          ❌ 仅新增注释与测试
  测试失败          ⚠️ 若标注错误会导致新测试红 —— 但那是新测试，不是既有测试
```

**注意**：C2 必须在 C1 之后（否则 `Authorization` 标为 `required` 会立刻红）。

### R2｜行为变更（真正的风险在这里）

```
C5：注册后台任务
  ├─ PublishDueAnnouncements   → 定时公告会开始真的发布
  ├─ ExpireDueAnnouncements    → 已发布公告会开始真的过期
  └─ CleanupExpiredMessageImages → 过期的未绑定图片会被真的删除
```

**这三项与 R0 性质完全不同，必须先想清楚再动：**

| 项 | 变更后的新行为 | 新风险 |
| --- | --- | --- |
| `PublishDueAnnouncements` | 到点的 `scheduled` 公告自动转 `published` 并投递 | ① 首次运行会**批量发布历史上所有到点的定时公告**；② 若调度周期与 `transitionDueAnnouncements` 的批量读-改-写并发（多副本），可能整批 `ErrStateConflict` 回滚 |
| `ExpireDueAnnouncements` | 超过有效期的 `published` 公告自动转 `expired` | ① 首次运行会**批量过期历史公告**；② 用户可能正在阅读一条刚被过期的公告 |
| `CleanupExpiredMessageImages` | 删除过期未绑定图片的 DB 记录 + MinIO 对象 | **不可逆**：对象被删除后无法恢复；若判断条件有误会误删 |

**这三项都不是"接线"，是"启用一个新行为"。**
且第①②条意味着：**第一次启动任务时会对存量数据做批量变更** —— 这是需要单独设计
（比如加首次运行的 dry-run、批量上限、或人工确认）的事情。

### R3｜需要先核实，暂不判定

```
C4：MessagingAuditSink
  规格 :93 要求"关键消息操作、权限拒绝、Outbox/Consumer/DLQ 处置 SHALL 写入受控审计或运行日志"
  当前：路由级审计 DefaultAuditCategory="message" 覆盖 HTTP 入口
  未核实：Outbox/Consumer/DLQ 处置（非 HTTP 路径）是否有审计/运行日志
  → 若已覆盖，删；若未覆盖，这是【规格要求的能力缺失】，不是死代码
```

---

## 二·补｜C4 边界核实结果：**规格已满足**

### 非 HTTP 路径的运行日志覆盖（规格 `:93` 允许"受控审计**或**运行日志"）

| 环节 | 日志点 | 位置 |
| --- | --- | --- |
| Outbox 处置 | 10 处：`claim_failed`、`lease_renew_failed`、`lease_lost`、`publish_failed`、`mark_published_failed`、`published`、`failure_record_failed`、`failure_recorded`（含 `dead_lettered` 标志） | `outbox_worker.go:71-122` |
| Consumer 处置 | 3 处：`failed`、`duplicate_or_superseded`、`completed`（含 `audience_observed_count`） | `event_consumer.go:117,160,264` |
| DLQ Recorder 处置 | 4 处：`recorded`、`record_failed`、`metrics_recorded`、`metrics_failed` | `dead_letter_recorder.go:53,56,81,85` |
| DLQ 重放处置 | 7 处：`claim_failed`、`lease_unavailable`、`publish_failed`、`return_failed`、`finalize_failed`、`lease_lost`、`replayed`（含 `replay_cycle`） | `dead_letter_replay.go:51-79` |
| 任务级 | 6 处 `*_deferred`（outbox/consumer/dlq_recorder/refresh_subscription/snapshot_cleanup/dead_letter_cleanup） | `app/messaging.go:165-218` |
| 观测 | `messaging_consumer_dlq_pending`（consumer / failure_code / pending_count / oldest_pending_age） | `app/messaging.go:294` |

### 日志安全性（规格 `:93` 末句要求）

```
规格：「日志不得写入正文、HTML、图片、外链 URL、Token、凭据或基础设施原始错误」

核实：internal/messaging 全模块 grep `err.Error()` → 【零命中】
      三处失败均映射为稳定码：
        outbox_worker.go:126      outboxFailureCode(err)
        consumer.go:141           consumerFailureCode(err)
        event_consumer.go:133     eventConsumerFailureCode(err)
```

### C4 结论

**规格 `:93` 已满足。`MessagingAuditSink` 是"已定义的替代实现路径，因运行日志已满足规格而未被采用"**
—— 属死代码，**不是能力缺失**。

→ 可并入第一阶段（R0）删除。C4 不再需要单独阶段。

---

## 二·补二｜C5 存量影响分析

### 关键事实：本机 `admin` 库是**停用的开发库**

```
配置指向：config.yaml:9  db: admin

该库最后活动：audit_logs 时间范围 2026-06-27 → 2026-08-05 23:05（273 条）
              users 最后更新 2026-08-05 22:59，且残留测试账号 verify_1785940497201_user
库大小：      1.17 MB

messaging 表：admin 库【零存在】；全实例检索 information_schema 亦无 messages 表
              （模型表名见 messaging/adapters/gorm/models.go:20-167，共 10 张）
menu_apis：   0 行
apis：        87 行（而非 120 —— 说明 8/5 之后新增的路由从未同步过）
```

**所以"首次运行对存量数据的影响"在本机环境为零**（无消息数据）。
但这是对**本机陈旧开发库**的陈述，不是对部署环境的结论。

### 与代码规模无关的两条设计问题（已核实）

**问题 1：批量无上限**

```
announcement_service.go:123   query := MessageListQuery{OrganizationIDs: ..., Kinds: ..., Statuses: ...}
                              ↑ 【不设 Limit】
repository.go:219-221         if query.Limit > 0 { db = db.Limit(query.Limit) }
                              ↑ Limit 为 0 时不加 LIMIT ⇒ 取出全部匹配行
announcement_service.go:138   在【单个事务】内 for 循环逐条变更
```

**问题 2：全批 all-or-nothing**

```
announcement_service.go:139-147
  for _, message := range messages {
      domain.TransitionMessage(...)          // 状态机校验
      service.messages.ChangeMessage(tx, ...) // 乐观并发 CAS（aggregate_version）
  }
  → 任一条 CAS 失败 ⇒ 整个事务回滚 ⇒ 本批全部公告都不发布
  → 且调用点无 SELECT FOR UPDATE（与 navigation.ChangeAPIPermissionCode 的加锁做法不同）
```

**含义**：消息表规模小时（当前状态）无影响；规模增长后，
一次调度会持有长事务并对每条公告逐一 CAS，多实例并发时冲突面随批量线性放大。

**这两条与本机数据无关，是代码性质** —— 因此即使本机零存量，
接入调度前仍应先解决批量上限与并发语义。

---

## 三、回答"改动之后呢"

### 对 R0 与 R1 的回答

**改动之后，系统行为与现在完全一致。**

不是"我们相信它一致"，而是**可证明的一致**：被删的代码没有执行路径，
新增的是注释与测试。验证方式：

```
1. go build ./...                     ← 编译通过（引用删干净）
2. go test ./... -count=1              ← 全部既有测试通过
3. go test -tags=mysql_integration ./... -count=1   ← 真实 MySQL 门禁通过
4. 对比 120 条路由快照                  ← 路由集合未变
```

第 2 条是**强证据**：既有的 160 个测试文件里，有 HTTP 契约测试、
权限策略测试、审计测试、消息测试 —— 如果删除改变了任何可观察行为，它们会红。

**唯一真正改变的是**：读代码的人不再看到 9 个"看起来在检查权限"的调用点。

### 对 R2 的回答

**改动之后，系统行为会变化，且变化不可逆（第 3 项）。**

所以我**不建议把 C5 与 R0 类改动混在一批做**。C5 需要独立评估：

```
先做的事（不写代码，纯分析）：
  ├─ 读数 announcement_service.transitionDueAnnouncements 的批量语义
  │   → 确认首次运行对存量数据的影响范围
  ├─ 确认定时公告在历史上是否真的产生过数据
  │   → 若生产库里 scheduled 状态的消息为 0 条，首次运行无存量影响
  └─ 设计首次运行的安全阀（批量上限 / dry-run / 人工触发）
```

**我此前把 C5 与 C1/C3 并列成"同类缺口"，这个并列是错的。**
它们的形态相似（都是写了没接线），但**风险等级差两个量级**。

---

## 四、修正后的建议

```
第一阶段（R0 + R1，可证明零行为变更）
  C1 + C1b + C3    删除死代码
  C2               接线护栏（在 C1 之后）
  验证：go build + go test ./... + MySQL 门禁 + 路由快照对比

第二阶段（R3，先核实再定）
  C4               核实 Outbox/Consumer/DLQ 的审计覆盖
                   → 已覆盖则删；未覆盖则转为"能力缺失"另行处置

第三阶段（R2，独立评估，不与上面混批）
  C5               三项都改变运行时行为，需先分析存量数据影响与并发语义

第四阶段
  C6               规格补齐（纯文档，无代码风险）
```

**与你原则的一致性**：项目 AGENTS.md 写的是
"不要为了尚未完成的复杂性而牺牲当前已经可用的产品"。
R0 类改动**不会牺牲任何当前可用的东西** —— 删掉的是没有执行路径的代码。
R2 类则会**改变当前可用行为**，所以它才需要单独论证。

---

## 五、仍需你裁决（不变）

```
Q1  C1b：UserVisible / OrganizationVisible 是纯测试专用之外的完全死代码 → 建议删
Q2  C5：公告调度 + 消息图片清理 —— 这三项要不要做？如果做，需要先做存量影响分析
Q3  C4：先核实 Outbox/Consumer/DLQ 审计覆盖，再决定
```
