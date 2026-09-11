## 1. 前置核实

- [ ] 1.1 确认 `config.Messaging.MaxTitleRunes` 与 `MaxBodyRunes` 在生产代码中确实零引用（复核 W1 结论）
- [ ] 1.2 读取 `internal/messaging/domain/content.go` 确认 `MaxMessageTitleRunes` / `MaxMessageBodyRunes` / `MaxMessageHTMLBytes` 三者的当前取值与用途
- [ ] 1.3 读取 `internal/messaging/domain/content_test.go`，列出直接引用上述常量的测试，确认改动后是否需要调整
- [ ] 1.4 确认 `internal/messaging/domain` 当前未导入任何 `platform` 或 `admin/internal/platform/*` 包（作为 R3 的基线）
- [ ] 1.5 运行 `go test ./... -count=1`，记录变更前绿态基线

## 2. 在领域层引入可传入的长度上限

- [ ] 2.1 在 `internal/messaging/domain/content.go` 新增 `ContentLimits` 值类型，含 `MaxTitleRunes` 与 `MaxBodyRunes`
- [ ] 2.2 新增 `DefaultContentLimits()` 返回以 `MaxMessageTitleRunes` / `MaxMessageBodyRunes` 常量构造的默认值
- [ ] 2.3 保留 `MaxMessageTitleRunes`、`MaxMessageBodyRunes`、`MaxMessageHTMLBytes` 常量不删除（作为默认值来源与域内自知约束）
- [ ] 2.4 修改 `CompileMessageContent` 签名为接受 `ContentLimits` 参数
- [ ] 2.5 在函数内校验 `limits` 的两个值均大于 0；非法值返回内部错误而非静默使用默认值
- [ ] 2.6 确认 `MaxMessageHTMLBytes` 的检查逻辑不变，且不受 `ContentLimits` 影响
- [ ] 2.7 确认 domain 包未新增任何 import（尤其不得引入 `platform/config`）

## 3. 经服务层传递上限

- [ ] 3.1 在 `internal/messaging/application/service.go` 的 `Dependencies` 新增上限字段，标注 `// wiring: optional —— constructor 兜底 DefaultContentLimits()`
- [ ] 3.2 在 `Service` 结构体新增对应字段，`NewService` 中为 nil/零值时填入 `domain.DefaultContentLimits()`
- [ ] 3.3 更新 `announcement_service.go:41`（`CreateAnnouncement`）的 `CompileMessageContent` 调用传入上限
- [ ] 3.4 更新 `announcement_service.go:205`（`EditAnnouncement`）的调用
- [ ] 3.5 更新 `broadcast_service.go:45`（`CreateBroadcast`）的调用
- [ ] 3.6 更新 `service.go:311`（`SendPrivateMessage`）的调用
- [ ] 3.7 确认 4 处调用点全部更新，无遗漏

## 4. 组合根注入配置值

- [ ] 4.1 在 `internal/app/messaging.go:61-67` 的 `Dependencies` 字面量注入由 `config.Messaging.MaxTitleRunes` / `MaxBodyRunes` 构造的 `ContentLimits`
- [ ] 4.2 确认注入值在配置为零值时与 `DefaultContentLimits()` 一致
- [ ] 4.3 确认 `internal/app` 是唯一把配置值转换为 `domain.ContentLimits` 的位置

## 5. 补充配置校验

- [ ] 5.1 复核 `config.go:279-283` 现有校验：`MaxAudienceUsers` 已有 `[1, 100000]` 上下界，`MaxTitleRunes` / `MaxBodyRunes` 仅有下界
- [ ] 5.2 为 `MaxTitleRunes` 与 `MaxBodyRunes` 补充上界校验，取值不得低于 25000（`config_test.go:284` 使用了该值）
- [ ] 5.3 为 `config.go` 的两个字段增加注释，说明其实际生效位置
- [ ] 5.4 确认非法配置在加载阶段失败而非静默使用默认值

## 6. 领域层测试

- [ ] 6.1 更新 `content_test.go` 中 4 处既有 `CompileMessageContent` 调用，传入 `DefaultContentLimits()`
- [ ] 6.2 新增测试：自定义上限生效（如 120 / 25000 时接受该长度）
- [ ] 6.3 新增测试：自定义上限收紧时正确拒绝（如上限 50 时 51 字符被拒）
- [ ] 6.4 新增测试：`ContentLimits` 含非法值（0 或负数）时返回内部错误
- [ ] 6.5 保留并确认既有的 Unicode 计数测试与 128 KiB HTML 上限测试仍通过

## 7. 服务层与组合根测试

- [ ] 7.1 在 messaging application 层新增或更新测试，确认 `Dependencies` 未注入上限时使用默认值
- [ ] 7.2 新增测试：注入自定义上限后，创建公告的校验使用该上限
- [ ] 7.3 新增测试：`SendPrivateMessage`、`CreateBroadcast`、`EditAnnouncement` 三条路径同样使用注入的上限（确认 4 个调用点无遗漏）
- [ ] 7.4 在 `internal/platform/config` 新增测试：超出上界的配置被判为非法

## 8. 验证默认行为不变

- [ ] 8.1 运行 `go build ./...`
- [ ] 8.2 运行 `go test ./internal/messaging/... -count=1`
- [ ] 8.3 运行 `go test ./... -count=1`，确认既有 160 个测试文件全部通过
- [ ] 8.4 运行 `go test ./testsupport -run TestArchitecture -count=1`，确认 domain 层未引入 config 依赖（R3）
- [ ] 8.5 以 `config.yaml` 默认值验证：标题 100 字符通过、101 字符被拒；正文 20000 字符通过、20001 字符被拒
- [ ] 8.6 运行 `go test -tags=mysql_integration ./... -count=1`（需 `ADMIN_TEST_MYSQL_DSN`）

## 9. 文档更新

- [ ] 9.1 更新 `docs/runbooks/messaging.md`：说明两个上限来自配置、默认值、以及调小上限会影响既有超长消息的编辑
- [ ] 9.2 更新 `docs/reviews/wire-audit.md` 与 `docs/reviews/contract-readiness.md`：标记 W1 已处理、批次 3 完成
- [ ] 9.3 确认无需改动 `docs/module-navigation.md`

## 10. 验收

- [ ] 10.1 对照 proposal 的「非目标」：未改变默认上限、未做差异化上限、未放宽 HTML 上限、未处理其他配置项
- [ ] 10.2 对照 `specs/internal-messaging/spec.md` 的 7 个 Scenario 确认均有对应测试覆盖
- [ ] 10.3 确认本变更只 ADDED 一条规格要求，未修改 `internal-messaging` 规格的既有要求
- [ ] 10.4 记录部署期风险核对结果：确认目标部署的 `config.yaml` 中两个配置项的**实际值**（若已填非默认值，本变更上线后它们将首次生效）
