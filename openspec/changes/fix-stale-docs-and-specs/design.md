## Context

契约可信度调研在已核范围内发现若干**已确认的事实错误** —— 规格或文档描述的
内容与代码实际行为不一致。本变更只修正这些，不做推测性改写。

错误分两类：

**A 类｜引用不存在的端点（X1）**

```
route-catalog/spec.md:196          "与 /api/health 的进程存活语义分离"
modular-layered-architecture:95    "GET /api/health SHALL 继续只报告进程存活"
docs/adr/0007:91                   "GET /api/health 继续只报告进程存活"
docs/runbooks/messaging.md:24      "GET /api/health 只表示进程存活"

实际：无该路由。存活探测由 /ping 承担（internal/app/http.go:21-23）。
```

**B 类｜规格内部不一致**

```
dict-management:10-11   正文"创建、查询、修改和删除字典类型"
                        场景只有查询/创建/删除 —— 4 条路由无任何规格引用
                        （PUT/DELETE 字典类型、PUT/DELETE 字典条目）
identity:11             "对外响应只返回 email、pending_email 和 email_verified"
                        实为通知渠道 Contract 的要求，措辞易读成 HTTP 资料响应
auth:180                "黑名单 TTL 与 token 剩余有效期对齐"
                        实际为固定 jwt.expire（15 分钟）
docs/module-navigation.md:44   go test ./initialize —— 该目录已在 fca2fa5 删除
TODO.md:62-63,70,81-86  多处状态与描述失真
```

约束：`AGENTS.md` 规定「不执行破坏性或高影响操作前确认范围」，
且首轮报告中**已被证伪**的两条（字典全部返回 400、缺少管理员创建用户端点）
不得进入本变更 —— 它们是调研误报，不是文档错误。

## Goals / Non-Goals

**Goals:**

- 消除规格中引用不存在端点的情形，使 `docs/README.md:10`
  「当前系统行为以 `openspec/specs/` 为准」重新成立。
- 使规格正文与其 Scenario 集合一致。
- 修正会误导执行者的文档（验证命令、TODO 状态、路线图阶段）。
- **不改任何运行时代码**。

**Non-Goals:**

- 不新增 `/api/health` 路由。
- 不完成剩余 10 个规格 / 578 条 SHALL 的核对（押后的 C6b）。
- 不修正首轮报告的误报条目。
- 不改写历史快照性质的 `docs/modify/mature-admin-system-comparison-*.md`。

## Decisions

### D1｜`/api/health` 修规格而非补代码

**决策**：把四处 `/api/health` 引用改为 `/ping`，不新增该路由。

**理由**：`/ping` 已承担进程存活语义（`internal/app/http.go:21-23`），
且 `route-catalog/spec.md:94` 明确 `/ping`、`/docs` 属"App 技术路由"。
补一个 `/api/health` 路由会引入**两个语义重复的存活端点**。

**替代方案**：新增 `/api/health` 路由以匹配规格。
**否决理由**：会产生无收益的重复端点，且需同步处理 API 元数据与权限码
（`/api/` 前缀会让它被纳入路由同步）。

**注意**：`/ping` 不带 `/api/` 前缀，因此**不受 API 元数据与权限中间件管辖**，
这正是"技术路由"的应有形态。

### D2｜ADR 修正时保留历史脉络

**决策**：`docs/adr/0007:91` 的 `/api/health` 改为 `/ping`，
并加一行说明该路由曾在更早版本名为 `/api/health`。

**理由**：ADR 通常记录决策时的状态，不应改写。但此处引用的是一个
**当时就不存在**的端点在后续行为中的角色 —— 保留不改会让读者据此寻找
不存在的路由。折中：修正引用 + 注明历史名称，既消除错误又保留脉络。

**替代方案**：完全不改 ADR，只在 specs 修正。
**否决理由**：ADR 0007 的该句是对 `/api/ready` 行为的规范性描述，
若与 specs 矛盾，读者会困惑以哪个为准。

### D3｜`auth:180` 采用"对齐实现"而非"修实现"

**决策**：把规格表述改为描述实际行为 —— TTL 取 `jwt.expire` 固定时长，
并新增一个 Scenario 说明"TTL 长于剩余有效期"时的行为。

**理由**：这是**保守方向的偏差** —— 黑名单保留时间比剩余有效期长，
不产生安全后果（token 自然过期后本就无法通过校验），只多占少量 Redis 内存。
修实现（按剩余有效期设 TTL）收益极小，却要改动 `Logout` 的签名与调用点。

**保留了一条待评估项**：若将来希望 TTL 精确化，应作为独立变更引入，
本文档记为已知取舍而非遗漏。

### D4｜`identity:11` 澄清作用域而非删改内容

**决策**：保留字段最小化要求，但把"对外响应"改为
"**通知渠道 Contract 响应**"，并新增一个 Scenario 明确
HTTP 资料响应由 `user-management` 规格约束、两者相互独立。

**理由**：核对时曾据 `identity:11` 怀疑用户资料响应越界
（`docs/reviews/contract-readiness-user-management.md` 第五节的记录）。
实际不冲突 —— 两条约束作用域不同。澄清措辞可避免后续重复误判。

### D5｜`dict-management` 采用 MODIFIED 补齐 Scenario

**决策**：按 `## MODIFIED Requirements` 重写三条 Requirement，
补齐四类缺失 Scenario（查询详情、修改类型、删除类型、修改条目、删除条目）。
并新增一条"类型编码不存在或未启用"的 Scenario 固化既有的空列表行为。

**理由**：项目所有者已确认（`docs/reviews/contract-readiness.md` 决策 1）
规格正文承诺的 CRUD 应完整覆盖。四条路由在代码与测试中均已存在
（`internal/dictionary/http.go:51,52,55,56`），仅规格缺失。

**注意**：OpenSpec 要求 `MODIFIED` 包含**完整**更新后内容，
故本条 delta 复制了三条 Requirement 全文，而非只写差异 ——
部分内容会在归档时丢失细节。

## Risks / Trade-offs

**[R1] `MODIFIED` 内容不完整会导致归档丢细节** →
每条 MODIFIED 均从 `openspec/specs/` 复制完整原文后编辑。
验证步骤含逐条对照原文确认完整性。

**[R2] 规格改动可能被误认为行为变更** →
proposal 的「非目标」与「不受影响」两节明确声明未改代码。
验证步骤含"不运行代码测试"的说明 —— 本变更不改变代码，既有测试状态不变。

**[R3] 范围蔓延至 C6b** →
明确限定为已核 5 个规格中的已确认错误。
未核的 10 个规格不在范围内，即使其中可能存在同类问题。

**[R4] TODO.md 的修正需要判断哪些条目标记完成** →
只修正**已有明确证据**的条目：
`文件上传安全策略`（`uploadsecurity` 模块已实现）、
`内部消息` 六项（模块已建成并有归档 change）。
其余未核实条目不改。

**[R5] 路线图阶段描述涉及历史判断** →
只修正**可机械验证**的部分：`internal/app` 组合根与 `internal/platform`
是否已建成、`main.go` 是否为薄入口 —— 这些有代码证据。
不重写路线图的阶段划分本身。

## Migration Plan

无。纯文档与规格变更，无部署步骤，无回滚需求
（回滚即恢复文件，单一提交内完成）。

验证方式：

```
1. openspec validate --specs        确认规格结构合法
2. 检索全库 /api/health 引用        确认除说明性文字外已清零
3. go test ./initialize ...         确认该命令确实失败（印证修正必要性）
   go test ./testsupport ...        确认正确命令通过
4. 逐条对照 MODIFIED 与原文          确认内容完整
```

## Open Questions

1. 首轮报告中的「文档过期」条目除已列出的外，是否还有应并入本变更的
   —— 当前采取保守范围，只含机制上可验证的条目。
2. `docs/runbooks/login-security.md` 的锁定档位描述与实现不一致
   （文档称三档并会重置计数，实现为四档且不重置）——
   该修正属 `auth` 规格范围之外，是否并入本变更需确认。
