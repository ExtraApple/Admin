## Why

契约可信度调研在已核的 5 个规格、39 条路由中发现若干**已确认的事实错误**
（`docs/reviews/contract-readiness.md` 的跨模块问题 X1/X6 与三个模块文档）。

其中 X1 最严重：`route-catalog` 与 `modular-layered-architecture` 两份规格、
ADR 0007 与 `docs/runbooks/messaging.md` 共四处引用 `GET /api/health`，
但**该路由不存在**——存活探测实际由 `/ping` 承担
（`internal/app/http.go:21-23`）。按 `docs/README.md:10`
「当前系统行为以 `openspec/specs/` 为准」，这是**规格在描述不存在的端点**。

同时 `docs/module-navigation.md:44` 给出的验证命令指向
`./initialize`——该目录在 `fca2fa5` 已被删除，实测报
`stat D:\WORK\GO\admin\initialize: directory not found`。

本变更只修正**已确认**的错误，不做任何推测性改写。

## What Changes

**规格修正**（`openspec/specs/`）：

- `modular-layered-architecture`: `/api/health` → `/ping`（`:95`）。
- `route-catalog`: `/api/health` 的存活语义 → `/ping`（`:196`）。
- `user-management`: 补齐字典模块发现的同类缺口——该规格的
  「管理员用户列表 / 修改 / 删除 / 状态 / 强制下线」5 条 Requirement
  已有 Scenario，但 `字典类型 CRUD` 与 `字典条目 CRUD` 两条 Requirement
  正文承诺"创建、查询、修改和删除"，Scenario 只落实部分。修正为与正文一致。
- `identity`: `:11` 的"对外响应"措辞改为"通知渠道 Contract 响应"，
  消除被误读为 HTTP 资料响应的歧义（X6）。
- `auth`: `:180` 的黑名单 TTL 表述对齐实现——当前实现使用固定
  `jwt.expire` 而非"token 剩余有效期"。修正为描述实际行为，
  或明确该差异为已知且保守的取舍。

**文档修正**（`docs/`、`TODO.md`）：

- `docs/module-navigation.md:44`: `go test ./initialize` → `go test ./testsupport`。
- `docs/runbooks/messaging.md:24`: `/api/health` → `/ping`。
- `docs/adr/0007`: `:91` 的 `/api/health` → `/ping`。
- `TODO.md`: 修正文件管理、内部消息两组失真条目。
- `docs/modify/project-improvement-roadmap.md`: 修正阶段定位
  （`internal/app` 组合根与 `internal/platform` 已建成，分层重构已完成）。

**非目标**（明确不做，避免范围蔓延）：

- **不修改任何运行时代码**。本变更只改文档与规格。
- 不新增 `/api/health` 路由。裁决为"规格描述错误"而非"实现缺失"，
  故修正规格而非补代码。
- 不完成契约调研尚未核对的 10 个规格 / 578 条 SHALL（属押后的 C6b）。
- **不包含首轮报告中被证伪的条目**：
  「字典模块把所有错误返回 400」（实为完整分类映射）、
  「缺少管理员创建用户端点」（规格本无此要求）—— 这两条是调研误报，
  不是文档错误。
- 不改写 `docs/modify/` 下属于历史快照性质的文件（`mature-admin-system-comparison-*`），
  只修正 `project-improvement-roadmap.md` 这一仍在充当执行依据的文件。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `modular-layered-architecture`: 修正引用不存在的 `/api/health` 的 Scenario。
- `route-catalog`: 同上——该 Scenario 用 `/api/health` 表述存活语义分离。
- `dict-management`: 补齐 `字典类型 CRUD` 与 `字典条目 CRUD` 两类 Requirement 的
  Scenario。两条正文承诺"创建、查询、修改和删除"，Scenario 只落实部分；
  代码侧 9 条路由中 4 条（`PUT`/`DELETE` 字典类型、`PUT`/`DELETE` 字典条目）
  在规格中无任何引用。修正后正文与 Scenario 一致。
- `identity`: `:11` 的"对外响应"措辞改为"通知渠道 Contract 响应"（X6）。
- `auth`: `:180` 的黑名单 TTL 表述对齐实现，或标明该差异为已知的保守取舍。

## Impact

### 受影响文件

| 文件 | 变更 |
| --- | --- |
| `openspec/specs/modular-layered-architecture/spec.md` | `:95` `/api/health` → `/ping` |
| `openspec/specs/route-catalog/spec.md` | `:196` `/api/health` → `/ping` |
| `openspec/specs/user-management/spec.md` | 补齐 2 类 Requirement 的 Scenario |
| `openspec/specs/identity/spec.md` | `:11` 措辞澄清 |
| `openspec/specs/auth/spec.md` | `:180` TTL 表述对齐实现 |
| `docs/module-navigation.md` | `:44` 验证命令路径 |
| `docs/runbooks/messaging.md` | `:24` `/api/health` → `/ping` |
| `docs/adr/0007-*.md` | `:91` `/api/health` → `/ping` |
| `TODO.md` | 文件管理、内部消息两组条目 |
| `docs/modify/project-improvement-roadmap.md` | 阶段定位 |

### 不受影响

路由（120 条）、权限码、HTTP 状态码与业务 JSON、数据库结构、Redis key、
MinIO bucket、后台任务集合、OpenAPI 输出，以及全部生产代码（`internal/` 中
除下述测试夹具外的文件、`main.go`）。

**唯一例外（实施时经裁决新增）**：为做到全库不再出现 `/api/health` 这一幽灵端点字样，
两个测试文件中的**合成示例路径**被改名为 `/api/probe-health`：

| 文件 | 变更 |
| --- | --- |
| `internal/app/app_test.go` | 示例 `Descriptor.Path`、快照断言与请求路径 |
| `internal/routecatalog/catalog_test.go` | 示例 `Descriptor.Path`、断言与重复路由错误信息 |

这些字符串是测试夹具数据，不代表真实端点；改名后不改变任何断言语义
（`routecatalog` 与 `app` 两个包测试全部通过）。

### 关于 ADR 的取舍

ADR 记录的是**决策时的状态**，通常不应改写。但 `docs/adr/0007:91` 引用的是
一个**当时就不存在**的端点在后续行为中的角色——若保留不改，
读者会据此寻找 `/api/health`。本变更采取折中：**修正为 `/ping`
并加一行注明该路由曾在更早版本名为 `/api/health`**，保留历史脉络而不留下错误引用。

### 风险与验证

**风险等级 R0**（纯文档与规格变更，无运行时代码改动）。

**风险**：规格改动会影响 OpenSpec 的归档行为——
`## MODIFIED Requirements` 需包含**完整**的更新后要求内容，
部分内容会导致归档时细节丢失。

**验证**：

1. `openspec validate --specs` —— 确认全部规格仍通过校验
2. 逐条对照 `## MODIFIED Requirements` 与 `openspec/specs/` 原文，
   确认每条的完整内容已复制并按新行为编辑（不得只写差异）
3. 检索确认全库已无 `/api/health` 引用（除本变更的说明性文字）
4. `pwsh -c "go test ./initialize -run TestArchitecture"` 确认该命令确实失败，
   而 `go test ./testsupport -run TestArchitecture` 通过（验证文档修正的必要性）
5. **不运行**代码测试 —— 本变更未改代码，既有测试状态不变

### 相关文档

- 发现依据：`docs/reviews/contract-readiness.md` X1、X6；
  `docs/reviews/contract-readiness-dict.md`（4 条未记录路由）；
  `docs/reviews/contract-readiness-user-management.md` 契约 B
- 押后部分：C6b（剩余 10 个规格 / 578 条 SHALL）
