## 1. 前置核实

- [x] 1.1 确认 `config.Messaging.MaxTitleRunes` 与 `MaxBodyRunes` 在生产代码中确实零引用（复核 W1 结论）—— 全库检索仅命中 `config.go`（声明 112-113、默认值 258-262、校验）与 `config_test.go`；`internal/uploadsecurity/filename.go` 的 `maxBodyRunes` 是同名局部变量，与之无关
- [x] 1.2 读取 `internal/messaging/domain/content.go` 确认 `MaxMessageTitleRunes` / `MaxMessageBodyRunes` / `MaxMessageHTMLBytes` 三者的当前取值与用途 —— `100` / `20_000` / `128 * 1024`；前两者用于标题与正文的 rune 计数校验，第三者用于清洗后 HTML 的字节上限
- [x] 1.3 读取 `internal/messaging/domain/content_test.go`，列出直接引用上述常量的测试，确认改动后是否需要调整 —— **实际为 5 个测试函数、8 处调用**（任务原记「4 处」偏低）；其中 3 个测试直接引用两个 rune 常量，1 个引用 `MaxMessageHTMLBytes`，1 个只传字面量。8 处全部需补 `limits` 参数
- [x] 1.4 确认 `internal/messaging/domain` 当前未导入任何 `platform` 或 `admin/internal/platform/*` 包（作为 R3 的基线）—— 生产文件的 `admin/` 导入数为 **0**（仅测试文件导入 `admin/internal/messaging/domain` 自身）
- [x] 1.5 运行 `go test ./... -count=1`，记录变更前绿态基线 —— 43 ok / 10 no-test-files / 0 FAIL

## 2. 在领域层引入可传入的长度上限

- [x] 2.1 在 `internal/messaging/domain/content.go` 新增 `ContentLimits` 值类型，含 `MaxTitleRunes` 与 `MaxBodyRunes`
- [x] 2.2 新增 `DefaultContentLimits()` 返回以 `MaxMessageTitleRunes` / `MaxMessageBodyRunes` 常量构造的默认值
- [x] 2.3 保留 `MaxMessageTitleRunes`、`MaxMessageBodyRunes`、`MaxMessageHTMLBytes` 常量不删除（作为默认值来源与域内自知约束）
- [x] 2.4 修改 `CompileMessageContent` 签名为接受 `ContentLimits` 参数
- [x] 2.5 在函数内校验 `limits` 的两个值均大于 0；非法值返回内部错误而非静默使用默认值 —— 新增 `ErrMessageLimitsInvalid`；该错误未被 HTTP 层映射，按 `errors.go` 的 `default` 分支落到 500 内部错误，正是任务要求的语义
- [x] 2.6 确认 `MaxMessageHTMLBytes` 的检查逻辑不变，且不受 `ContentLimits` 影响 —— 仍是独立常量与独立分支，未进入 `limits`
- [x] 2.7 确认 domain 包未新增任何 import（尤其不得引入 `platform/config`）—— import 块零改动

## 3. 经服务层传递上限

- [x] 3.1 在 `internal/messaging/application/service.go` 的 `Dependencies` 新增上限字段，标注 `// wiring: optional —— 缺失时 constructor 兜底 domain.DefaultContentLimits()（标题 100 / 正文 20000）`（同一行内写明兜底来源，通过批次 2 的护栏校验）
- [x] 3.2 在 `Service` 结构体新增对应字段，`NewService` 中为零值时填入 `domain.DefaultContentLimits()`
- [x] 3.3 更新 `announcement_service.go`（`CreateAnnouncement`）的 `CompileMessageContent` 调用传入上限
- [x] 3.4 更新 `announcement_service.go`（`EditAnnouncement`）的调用
- [x] 3.5 更新 `broadcast_service.go`（`CreateBroadcast`）的调用
- [x] 3.6 更新 `service.go`（`SendPrivateMessage`）的调用 —— 实际行号为 324（设计文档记 311，属陈旧行号）
- [x] 3.7 确认 4 处调用点全部更新，无遗漏 —— `grep CompileMessageContent` 显示生产代码仅这 4 处调用，且全部带 `service.contentLimits`

## 4. 组合根注入配置值

- [x] 4.1 在 `internal/app/messaging.go` 的 `Dependencies` 字面量注入由 `config.Messaging.MaxTitleRunes` / `MaxBodyRunes` 构造的 `ContentLimits`（新增 `messagingdomain` 导入）
- [x] 4.2 确认注入值在配置为零值时与 `DefaultContentLimits()` 一致 —— app 测试夹具的 `Config{}` 未设置 Messaging，注入值即零值，`NewService` 的零值判断替换为默认值，行为与今日一致
- [x] 4.3 确认 `internal/app` 是唯一把配置值转换为 `domain.ContentLimits` 的位置 —— 全库检索 `domain.ContentLimits` 与 `ContentLimits{`：生产代码仅 `app/messaging.go`（转换）、domain 定义处与应用层字段声明

## 5. 补充配置校验

- [x] 5.1 复核 `config.go` 现有校验：`MaxAudienceUsers` 已有 `[1, 100000]` 上下界，`MaxTitleRunes` / `MaxBodyRunes` 仅有下界
- [x] 5.2 为 `MaxTitleRunes` 与 `MaxBodyRunes` 补充上界校验 —— 最终取 `[1, 1000]` 与 `[1, 100000]`；两者均高于既有测试用值（标题 120、正文 25000），避免使 `config_test.go` 失效；128 KiB HTML 安全上限保持独立，不参与该边界推导
- [x] 5.3 为 `config.go` 的两个字段增加注释，说明其实际生效位置（由组合根转换为 domain `ContentLimits`，域层不读配置）
- [x] 5.4 确认非法配置在加载阶段失败而非静默使用默认值 —— `validateMessaging` 由 `Load` 调用；负数与超上界均报错，且错误信息指名具体配置键。**同时发现并裁决一处规格偏差**：原 Scenario 写「小于 1 即失败」，但 `0` 是全仓统一的「未配置」哨兵（`applyDefaults` 先填默认值再校验），故实际只有负数与超上界会失败。经裁决收紧规格措辞，并新增「未配置取默认值」Scenario；主代码零改动

## 6. 领域层测试

- [x] 6.1 更新 `content_test.go` 中既有 `CompileMessageContent` 调用，传入 `DefaultContentLimits()` —— **实际更新 8 处**（任务原记 4 处）
- [x] 6.2 新增测试：自定义上限生效（如 120 / 25000 时接受该长度）—— `TestCompileMessageContentUsesInjectedLimits`
- [x] 6.3 新增测试：自定义上限收紧时正确拒绝（如上限 50 时 51 字符被拒）—— `TestCompileMessageContentRejectsTightenedLimits`
- [x] 6.4 新增测试：`ContentLimits` 含非法值（0 或负数）时返回内部错误 —— `TestCompileMessageContentRejectsInvalidLimits`（6 个子用例：零值、单侧零、单侧负、双负）
- [x] 6.5 保留并确认既有的 Unicode 计数测试与 128 KiB HTML 上限测试仍通过 —— 两个测试函数未改断言，仅补参数，全部通过

## 7. 服务层与组合根测试

- [x] 7.1 在 messaging application 层新增或更新测试，确认 `Dependencies` 未注入上限时使用默认值 —— `TestServiceUsesDomainDefaultLimitsWhenNoneAreInjected`
- [x] 7.2 新增测试：注入自定义上限后，创建公告的校验使用该上限 —— 见 7.3 的 `CreateAnnouncement` 子测试
- [x] 7.3 新增测试：`SendPrivateMessage`、`CreateBroadcast`、`EditAnnouncement` 三条路径同样使用注入的上限（确认 4 个调用点无遗漏）—— `TestServiceAppliesInjectedLimitsOnEveryMessagePath`，四条路径各自断言「101 字符标题与 20001 字符正文被接受」+「121 字符标题、25001 字符正文被拒绝」
- [x] 7.4 在 `internal/platform/config` 新增测试：超出上界的配置被判为非法 —— `TestLoadRejectsOutOfRangeMessagingContentLimits`（5 个子用例，并断言错误信息指名配置键）；另补 `TestLoadTreatsZeroMessagingContentLimitsAsUnset` 与 `TestLoadAcceptsMessagingContentLimitsAtTheirBounds`

## 8. 验证默认行为不变

- [x] 8.1 运行 `go build ./...` —— exit 0
- [x] 8.2 运行 `go test ./internal/messaging/... -count=1` —— 全部 ok
- [x] 8.3 运行 `go test ./... -count=1`，确认既有 160 个测试文件全部通过 —— 43 ok / 10 no-test-files / 0 FAIL
- [x] 8.4 运行 `go test ./testsupport -run TestArchitecture -count=1`，确认 domain 层未引入 config 依赖（R3）—— ok
- [x] 8.5 以 `config.yaml` 默认值验证：标题 100 字符通过、101 字符被拒；正文 20000 字符通过、20001 字符被拒 —— `internal/app/messaging_content_limits_test.go` 的两个测试：解析仓库 `config.yaml` 断言其等于 domain 默认值，并以该值断言 4 个边界
- [x] 8.6 运行 `go test -tags=mysql_integration ./... -count=1`（需 `ADMIN_TEST_MYSQL_DSN`）—— 43 ok / 0 FAIL（独立探针库，用后 DROP）

## 9. 文档更新

- [x] 9.1 更新 `docs/runbooks/messaging.md`：新增「消息长度上限」一节 —— 配置项、默认值、允许范围、计数单位、`0` 的哨兵语义、启动时读取一次、调小上限对既有公告编辑的影响、以及 128 KiB HTML 上限不可配置
- [x] 9.2 更新 `docs/reviews/wire-audit.md` 与 `docs/reviews/contract-readiness.md`：标记 W1 已处理、批次 3 完成（含「批次 3 实施记录」）
- [x] 9.3 确认无需改动 `docs/module-navigation.md` —— 「模块索引」中 `internal/messaging` 的所有权描述仍准确；「验证入口」一节无需新增命令（本变更复用既有测试命令）

## 10. 验收

- [x] 10.1 对照 proposal 的「非目标」：未改变默认上限（100 / 20000 不变）、未做差异化上限（只有全局两个值）、未放宽 HTML 上限（128 KiB 仍是独立常量与独立分支）、未处理其他配置项（只改这两个字段的校验与注释）
- [x] 10.2 对照 `specs/internal-messaging/spec.md` 的 8 个 Scenario 确认均有对应测试覆盖：① 默认边界 → 8.5；② 超标题上限 → 8.5 与 6.x；③ 超正文上限 → 8.5 与 6.x；④ 自定义上限生效 → 6.2/7.3；⑤ Unicode 计数 → 6.5 既有测试；⑥ HTML 上限不可配置 → 6.5 既有测试 + 2.6；⑦ 非法上限被拒 → 7.4；⑧ 未配置取默认值 → 7.4 与 4.2
- [x] 10.3 确认本变更只 ADDED 一条规格要求，未修改 `internal-messaging` 规格的既有要求 —— `git status openspec/specs/` 无输出；delta 仅 `## ADDED Requirements`
- [x] 10.4 记录部署期风险核对结果：仓库内唯一 `config.yaml` 的两个配置项为 **100 / 20000**（等于默认值），故本变更上线后行为与既往完全一致；若某部署自行填入了非默认值，本变更上线后这些值将首次真正生效 —— 已写入 runbook 与「批次 3 实施记录」
