## 1. 删除前置核实

- [ ] 1.1 重新核对 `internal/app/files.go` 的 `filesapplication.Dependencies` 字面量，确认 `Authorization` 与 `Audit` 仍未赋值（审计与实施之间可能有其他变更落地）
- [ ] 1.2 全库检索 `deps.Authorization` 与 `deps.Audit` 的读取点，确认各自仅有一处，且分别位于 `authorize` 与 `recordAudit` 内
- [ ] 1.3 检索 `AuthorizationScope`、`OperationDownload`、`OperationPreview`、`AuditMetadataSink`、`UserVisible`、`OrganizationVisible`、`MessagingAuditSink`、`MessagingAuditEntry` 的全部引用点，列出待同步删除的测试引用
- [ ] 1.4 记录变更前的 120 条路由快照（Method、Path、Access Level、Permission Code、顺序），作为不变性基线
- [ ] 1.5 运行 `go build ./...` 与 `go test ./... -count=1`，记录删除前的绿态基线

## 2. 删除文件模块的未接线授权端口

- [ ] 2.1 删除 `internal/files/application/contracts.go` 的 `AuthorizationScope` 接口与 `Dependencies.Authorization` 字段
- [ ] 2.2 删除 `internal/files/application/contracts.go` 中未被任何调用点使用的 `OperationDownload` 与 `OperationPreview` 常量；对 `Operation` 常量集逐一确认无剩余引用后整体删除
- [ ] 2.3 删除 `internal/files/application/service.go` 的 `authorize` 函数
- [ ] 2.4 删除 `service.go` 中 7 处 `s.authorize(...)` 调用；确认每处删除后其外层方法的错误返回路径不变（原先 `authorize` 恒返回 nil，故不改变控制流）
- [ ] 2.5 确认 `UploadInput.UploaderID` 等归属字段仍保留并继续写入，仅移除其作为访问判据的用法

## 3. 删除文件模块的未接线审计端口

- [ ] 3.1 删除 `internal/files/application/contracts.go` 的 `AuditMetadataSink` 接口与 `Dependencies.Audit` 字段
- [ ] 3.2 删除 `service.go` 中 `recordAudit` 的 `if s.deps.Audit != nil` 直接记录分支；保留 `AuditMetadata` 结构体（它仍被 HTTP 适配器写入 Gin Context 使用）
- [ ] 3.3 确认 `AuditMetadata` 的写入方（`internal/files/adapters/http/routes.go`）与读取方（`internal/audit/adapters/http/routes.go`）路径不变

## 4. 收敛审计上下文键常量

- [ ] 4.1 确认 `internal/files/application/contracts.go` 与 `internal/audit/service.go` 两处 `UploadAuditMetadataContextKey` 的当前字符串值一致
- [ ] 4.2 按「谁定义语义」确定归属方；将另一侧改为引用归属方常量，删除重复定义
- [ ] 4.3 确认读写两侧引用同一常量后，`c.Set` 与 `c.Get` 的键仍一致

## 5. 删除授权模块的未接线可见性方法

- [ ] 5.1 删除 `internal/authorization/application/service.go` 的 `UserVisible` 方法
- [ ] 5.2 删除同文件的 `OrganizationVisible` 方法
- [ ] 5.3 确认删除后无任何接口声明因此不再被满足（两方法未被任何接口声明）

## 6. 删除消息模块的未接线审计端口

- [ ] 6.1 删除 `internal/messaging/application/contracts.go` 的 `MessagingAuditSink` 接口与 `MessagingAuditEntry` 结构体
- [ ] 6.2 删除 `internal/messaging/application/audit_contract_test.go`（该文件仅做 `var _ =` 接口断言，无行为测试）
- [ ] 6.3 确认消息审计的实际通道未被触及：路由级审计 `DefaultAuditCategory = "message"` 与运行日志输出点保持原样

## 7. 记录架构决策

- [ ] 7.1 新增 `docs/adr/0008-*.md`，记录「授权与审计端口按需引入，不留未接线接缝」的决策；先确认 ADR 编号未被在途变更占用
- [ ] 7.2 ADR 中包含被删端口的原始接口签名与设计意图，作为将来重新引入的输入
- [ ] 7.3 ADR 中记录 A 类有意预留接缝清单（`MessagingMetrics`、通知投影 Contract、邮件与短信 Consumer、`ExternalProxy`）及其 ADR 0007 依据，避免后续审计重复报告
- [ ] 7.4 ADR 中记录本次删除的裁决依据：`openspec/specs/file-management/spec.md` 无对象级授权要求，故按「规格未要求」删除而非补实现

## 8. 验证行为中性

- [ ] 8.1 运行 `go build ./...`，确认所有引用已删净
- [ ] 8.2 运行 `go test ./testsupport -run TestArchitecture -count=1`，确认架构边界测试通过
- [ ] 8.3 运行 `go test ./... -count=1`，确认 160 个既有测试文件全部通过
- [ ] 8.4 运行 `go test -tags=mysql_integration ./... -count=1`（需 `ADMIN_TEST_MYSQL_DSN` 指向独立测试库），确认真实 MySQL 门禁通过
- [ ] 8.5 重新导出 120 条路由快照，与 1.4 的基线逐条比对，确认集合、顺序、Access Level 与权限码全部一致
- [ ] 8.6 抽查 `/api/admin/files` 与 `/api/admin/files/:id/download` 的响应信封与状态码，确认与变更前一致

## 9. 更新文档与审计记录

- [ ] 9.1 更新 `docs/reviews/wire-audit.md`：把四个待删项标记为已处理，并指向本变更
- [ ] 9.2 更新 `docs/reviews/contract-readiness.md` 的批次计划，标记批次 1 完成
- [ ] 9.3 确认 `docs/module-navigation.md` 中 `internal/files` 的所有权描述不因本次删除而失真

## 10. 验收

- [ ] 10.1 对照 proposal 的「非目标」逐条确认未被触碰：未新增授权能力、未移除 A 类接缝、未处理公告调度与消息图片清理、未引入接线护栏
- [ ] 10.2 对照 `specs/file-access-contract/spec.md` 的 5 个 Scenario 确认当前实现与新增规格一致
- [ ] 10.3 确认本变更未修改任何既有规格文件，也未改动路由、数据库结构、Redis key、MinIO bucket 与后台任务集合
- [ ] 10.4 确认护栏变更未被混入本变更（它必须在本次验收通过后独立实施）
