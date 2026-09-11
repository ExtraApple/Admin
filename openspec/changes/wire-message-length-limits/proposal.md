## Why

实现审计维度①（配置项是否真被读取）发现：
`messaging.max_title_runes` 与 `messaging.max_body_runes` 是**死配置**
（`docs/reviews/wire-audit.md` 第七·补节 W1）。

```
配置链路完整：config.go:111-113 声明 → :258-262 解析+设默认 → :282 校验
              config_test.go:284 断言自定义值 120/25000 能被正确加载
实际约束硬编码：messaging/domain/content.go:16-17
              MaxMessageTitleRunes = 100 / MaxMessageBodyRunes = 20_000
```

**运维人员把 `max_title_runes` 改成 500，标题上限仍然是 100** —— 配置被解析、
被校验、被测试覆盖，却没有生效路径。这比"未接线端口"更隐蔽：
孤儿方法至少能被"零调用方"扫出来，死配置需要专门的字段读取扫描才能发现。

## What Changes

- **把两个长度上限接入校验路径**：`CompileMessageContent` 不再只读硬编码常量，
  而是接受来自配置的上限。
- **保持默认行为不变**：`config.yaml` 当前值 100 / 20000 与硬编码常量一致，
  因此默认部署的标题与正文上限**不变**。
- **补充规格**：主规格 `internal-messaging` 未记录长度上限；
  归档 delta 曾写明"100 个 Unicode 字符 / 20,000 个 Unicode 字符"。
  本变更把该约束补回主规格。
- **新增校验边界**：配置值必须为正；上限改动 SHALL NOT 放宽到使既有消息无法读取。

**非目标**：

- 不改变默认上限数值。
- 不新增按组织或按消息类型区分的差异化上限。
- 不改变 `MaxMessageHTMLBytes`（128 KiB HTML 上限）—— 它是安全上限，
  与可配置的业务长度上限性质不同，保持硬编码。
- 不处理其他死配置（本轮配置扫描仅发现这两个）。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `internal-messaging`: 补充并明确消息标题与正文长度上限这一要求。
  当前主规格未记录数值；归档 delta 曾以 Scenario 形式写明
  "标题不超过 100 个 Unicode 字符且 Markdown 正文不超过 20,000 个 Unicode 字符"。
  本变更将其补入主规格，并声明上限来自配置且默认值不变。

## Impact

### 受影响代码

| 位置 | 变更 |
| --- | --- |
| `internal/messaging/domain/content.go` | `CompileMessageContent` 接受长度上限参数；保留常量作为默认值来源 |
| `internal/messaging/application` | 调用 `CompileMessageContent` 的路径传入配置上限；确认 `Service` 是否能取得配置 |
| `internal/app/messaging.go` | 把 `config.Messaging.MaxTitleRunes` / `MaxBodyRunes` 注入 Messaging 组合 |
| `internal/platform/config/config.go` | 确认既有校验足够（`< 1` 报错），必要时补充上界 |

### 架构注意

`internal/messaging/domain` 是纯领域层，按 `modular-layered-architecture/spec.md`
的三层约束，**域层 SHALL NOT 依赖 `platform/config`**。
因此上限必须以**值参数**（如 `ContentLimits{MaxTitleRunes, MaxBodyRunes}`）
经 application 层传入，而不是让 domain 直接读配置。

这是本变更的主要设计约束，详见 design.md。

### 不受影响

路由（120 条）、权限码、HTTP 状态码与业务 JSON、数据库结构、Redis key、
MinIO bucket、后台任务集合（11 个）、OpenAPI 输出。

### 风险与验证

**风险等级 R2**（行为变更）：与批次 1 的死代码删除不同，
本变更确实改变行为 —— 一旦运维改配置，长度上限会真的变化。

**风险**：

1. 默认值必须保持 100 / 20000，否则既有可用行为改变。需在验证中显式断言。
2. 若上限被调小，既有超长消息可能无法再被编辑或重新发布。需明确该影响边界。
3. domain 层不得依赖 config —— 违反三层约束会被架构测试拦截。

**验证**：

1. `go build ./...`
2. `go test ./internal/messaging/... -count=1` —— 含长度上限既有测试
3. `go test ./... -count=1` —— 既有 160 个测试文件全部通过
4. `go test ./testsupport -run TestArchitecture -count=1` —— 确认 domain 层未引入 config 依赖
5. **默认值不变断言**：以 `config.yaml` 的 100 / 20000 构建应用，
   确认标题 100 字符通过、101 字符被拒；正文 20000 字符通过、20001 字符被拒
6. **配置生效断言**：把配置改为 120 / 25000，确认新上限生效
7. `go test -tags=mysql_integration ./... -count=1` —— 真实 MySQL 门禁通过

### 相关文档

- 发现依据：`docs/reviews/wire-audit.md` 第七·补节 W1
- 风险分级：`docs/reviews/change-risk-assessment.md` 第二节
- 归档规格原文：`openspec/changes/archive/2026-09-03-add-internal-messaging-rabbitmq/specs/internal-messaging/spec.md:234`
