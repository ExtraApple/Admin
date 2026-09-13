## 1. 前置校验

- [x] 1.1 确认 `remove-unwired-ports` 变更已验收完成（`files.Dependencies.Authorization` 与 `Audit` 字段已不存在）
- [x] 1.2 全库检索确认已无 `AuthorizationScope`、`AuditMetadataSink`、`authorize` 残留；若仍有残留则停止，先完成前置变更
- [x] 1.3 运行 `go test ./... -count=1` 记录当前绿态基线

## 2. 为文件模块依赖结构体增加接线注解

- [x] 2.1 读取 `internal/files/application/contracts.go` 的 `Dependencies`，逐一核对 `NewService`（`service.go:23-37`）的兜底分支
- [x] 2.2 为 `Clock` 标注 `// wiring: optional —— constructor 兜底 ClockFunc(time.Now)`
- [x] 2.3 为 `ObjectNames` 标注 `// wiring: optional —— constructor 兜底 UUIDObjectNames{}`
- [x] 2.4 为 `Transactions` 标注 `// wiring: optional —— constructor 兜底 directTransactionRunner{}`
- [x] 2.5 为 `DownloadURLExpireSeconds` 标注 `// wiring: optional —— constructor 兜底 300`
- [x] 2.6 为 `Repository`、`Storage`、`Validator`、`MessageImages`、`MessageImageValidator`、`Signer` 标注 `// wiring: required`，并写明缺失后果
- [x] 2.7 确认全部字段均已标注，无遗漏（6 required + 4 optional = 10 字段，与结构体字段数一致）

## 3. 为消息模块依赖结构体增加接线注解

- [x] 3.1 读取 `internal/messaging/application/service.go` 的 `Dependencies`，核对 `NewService`（`service.go:69-83`）的兜底分支
- [x] 3.2 为 `Clock` 标注 `// wiring: optional —— constructor 兜底 ClockFunc(time.Now)`
- [x] 3.3 为 `Transactions` 标注 `// wiring: optional —— constructor 兜底 directTransactionRunner{}`
- [x] 3.4 为 `Messages`、`Categories`、`Inbox`、`Notifications`、`Outboxes`、`ConsumerDeadLetters`、`ConsumerDeadLetterReplay`、`Identity`、`Organizations`、`Authorization`、`Files` 标注 `// wiring: required`，并写明缺失后果
- [x] 3.5 确认全部 13 个字段均已标注，无遗漏（11 required + 2 optional = 13 字段，与结构体字段数一致）

## 4. 处理组合根中 optional 字段的赋值

- [x] 4.1 确认 `internal/app/files.go:34-43` 中被标 `optional` 的 4 个字段是否显式赋值（`Transactions`、`DownloadURLExpireSeconds` 已赋值；`Clock`、`ObjectNames` 未赋值）
- [x] 4.2 确认 `internal/app/messaging.go:61-67` 中 `Clock` 是否显式赋值（未赋值；`Transactions` 已赋值）
- [x] 4.3 决定并统一策略：**统一为「依赖构造器兜底」** —— 组合根显式赋值一切能提供真实实现的 optional 字段（`Transactions` 用平台事务运行器、`DownloadURLExpireSeconds` 用配置值）；`Clock` 与 `ObjectNames` 无更优实现可注入，两处 root 均一致地省略，由注解声明兜底来源。无需改动组合根代码
- [x] 4.4 若选择显式赋值，补充赋值并确认与构造器兜底值语义一致 —— 未选该分支，`Clock`/`ObjectNames` 保持省略（兜底值与显式赋值等价，补写只会增加噪音）

## 5. 实现接线完整性架构测试

- [x] 5.1 在 `testsupport/architecture_boundary_test.go` 新增测试函数，与既有 16 个架构测试同风格（`go/ast` 遍历非测试生产文件）—— `TestArchitectureDeclaredDependenciesAreWired`
- [x] 5.2 实现第一遍：收集全部 `type Dependencies struct` 的字段与注解；无注解字段报错
- [x] 5.3 严格校验注解格式，只接受精确的 `// wiring: required` 与 `// wiring: optional`；其他形式报错（正则 `^// wiring: (required|optional) —— .+$`，并强制写明后果/兜底来源）
- [x] 5.4 实现第二遍：扫描 `internal/app` 的非测试文件，收集全部字段字面量中已赋值的字段名
- [x] 5.5 实现第三遍：以「模块路径 + 字段名」为键，校验每个 `required` 字段已赋值；键不得使用裸字段名（两模块均有 `Clock`/`Transactions`）—— 键为 `admin/internal/<模块路径>.<字段名>`
- [x] 5.6 检测并报错：类型别名声明（`type X = Dependencies`）
- [x] 5.7 检测并报错：位置参数形式的依赖字面量
- [x] 5.8 报错信息包含模块路径与字段名，便于定位

## 6. 验证护栏有效（含负向验证）

- [x] 6.1 运行 `go build ./...`
- [x] 6.2 运行 `go test ./testsupport -run TestArchitecture -count=1`，确认新护栏通过
- [x] 6.3 运行 `go test ./... -count=1`，确认既有 160 个测试文件全部通过
- [x] 6.4 **负向验证**：临时移除组合根中一个 `required` 字段的赋值，运行新测试，确认**确实失败**且失败信息指对该字段 —— 移除 `files` 的 `Validator` 赋值后失败于 `admin/internal/files/application.Validator is declared required but never assigned in internal/app`
- [x] 6.5 恢复 6.4 的临时改动，确认测试重新通过（文件 SHA-256 恢复后与改动前一致）
- [x] 6.6 **负向验证**：临时删除某字段的 `wiring` 注解，确认测试失败并提示缺少注解；随后恢复 —— 删除 `Clock` 注解后失败于 `internal/files/application/contracts.go: Dependencies field Clock needs an exact ... annotation`
- [x] 6.7 **负向验证**：临时把某注解改为拼写错误（如 `// wiring: requird`），确认测试失败而非静默通过；随后恢复 —— `requird` 后失败于 `Dependencies field Storage needs an exact ... annotation`
- [x] 6.8 运行 `go test -tags=mysql_integration ./... -count=1`（需 `ADMIN_TEST_MYSQL_DSN`），确认真实 MySQL 门禁不受影响（独立探针库，43 ok / 0 FAIL，用后 DROP）

## 7. 复核注解判定

- [x] 7.1 逐一复核每个 `optional` 注解：确认构造器**确实**存在兜底分支，且兜底值不改变业务语义、不放宽校验
- [x] 7.2 特别复核 `Transactions`：确认 `directTransactionRunner{}` 兜底不会削弱事务语义到不可接受（该兜底使事务退化为直通执行）—— **可接受**：`files` 与 `messaging` 的唯一生产构造点是 `internal/app/files.go:40` 与 `internal/app/messaging.go:66`，两者都显式注入 `platformdatabase.NewTransactionRunner(resources.DB)`，兜底分支在生产不可达；仅当单元测试夹具省略 `Transactions` 时生效，而夹具使用内存假实现，无真实事务可退化。注解已写明「事务退化为直通执行」，读者不会误认
- [x] 7.3 对所有 `required` 字段确认组合根赋的是**真实实现**而非 `nil`（静态检查无法求值，需人工确认）—— 17 个 required 字段中 16 个赋真实实现；**1 个例外**：`messaging.ConsumerDeadLetterReplay` 在 RabbitMQ 未配置时赋 `nil`（`internal/app/messaging.go:53,63`）。该 nil 有显式守卫（`admin_service.go:198` 返回 `ErrMessagingDependency`），不会 panic，重放端点按受控依赖不可用降级。这是 design R5 已声明的静态检查局限，故该字段仍标 `required`（它没有构造器兜底，标 `optional` 会违反 D4）；已在 `docs/reviews/wire-guardrail-design.md` 记录该例外

## 8. 记录与收尾

- [x] 8.1 更新 `docs/reviews/wire-guardrail-design.md`：把「待实施」标记为已实施，并记录实际测试函数名（新增第七节，含 `TestArchitectureDeclaredDependenciesAreWired`、与设计原文的差异表、optional 策略、R5 局限与负向验证证据）
- [x] 8.2 更新 `docs/reviews/wire-audit.md` 与 `docs/reviews/contract-readiness.md`：标记批次 2 完成（两处均新增「批次 2 实施记录/结果」）
- [x] 8.3 确认 `docs/module-navigation.md` 的「验证入口」一节是否需要补充新测试的运行方式 —— **无需改动**：护栏与既有 16 个架构测试同包同前缀，已由现有 `go test ./testsupport -run TestArchitecture` 覆盖；该节 `:44` 的 `./initialize` 路径错误属批次 4 任务 6.1 的范围，本变更不重复修改

## 9. 验收

- [x] 9.1 对照 proposal 的「非目标」逐条确认：未改变生产运行时行为（生产代码 diff 仅新增注释行 + gofmt 对齐，`internal/app` 零改动）、未校验依赖语义正确性（只校验字段是否出现）、未覆盖无字段端口（未新增规则）、未校验后台任务注册（未触及）
- [x] 9.2 对照 `specs/modular-layered-architecture/spec.md` 的 6 个 Scenario 确认均有对应测试覆盖：① required 已装配→6.2 通过；② required 漏装配→6.4；③ 缺注解→6.6；④ 注解不规范→6.7；⑤ optional 省略装配→6.2（`Clock`/`ObjectNames` 省略仍通过）；⑥ 别名或位置参数→补充验证 A/B
- [x] 9.3 确认本变更未修改任何既有规格要求（仅 ADDED 一条新要求）—— `git status openspec/specs/` 无输出，delta spec 仅含 `## ADDED Requirements`
- [x] 9.4 确认负向验证（6.4-6.7）结果已记录 —— 这是护栏有效性的唯一证据，缺此证据不得验收（证据落在 6.4/6.6/6.7 的勾选项与 `docs/reviews/wire-guardrail-design.md` 第七节，含补充验证 A/B）
