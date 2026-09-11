# 契约与实现审计 · 索引

> 本目录承载两个**不同**的审计：
>
> | 审计 | 问题 | 产出 | 文件 |
> | --- | --- | --- | --- |
> | **契约可信度调研** | 规格与代码对不对得上？ | 规格补齐清单 | 本文件表格中的模块文件 |
> | **实现审计** | 有没有写完但没生效的能力？ | 接线缺口清单 | [wire-audit.md](wire-audit.md) |

## 实现审计的进展

### 最终定论（2026-09-22）

**四个待删项均为死代码**（当前配置下无执行路径，已逐一核实读取点）——
**均已由批次 1（`remove-unwired-ports`）删除**：

| 项 | 位置 | 核实依据 |
| --- | --- | --- |
| `files.AuthorizationScope` + `Dependencies.Authorization` + 8 处 `authorize` 调用 + `Operation` 常量集 | `files/application/` | `service.go:56-59` nil 时提前 `return nil`；组合根不注入 |
| `authorization.UserVisible` / `OrganizationVisible` | `authorization/application/service.go:443,454` | 全库零引用、**测试亦零引用** |
| `files.AuditMetadataSink` + `Dependencies.Audit` + `service.go:510-511` | `files/application/` | 唯一读取点在 `if != nil` 内；实际审计走 Gin context |
| `messaging.MessagingAuditSink` + `MessagingAuditEntry` + `audit_contract_test.go` | `messaging/application/` | 生产零引用、无 `Dependencies` 字段；规格 `:93` 已由运行日志满足 |

**A 类（有意预留，不动）** —— `MessagingMetrics`、`NotificationProjectionReader`、
邮件/短信 Consumer、`ExternalProxy`。书面依据与判定见 [wire-audit.md](wire-audit.md) 第六节。

### 实现审计的维度完成度

```
✅ 端口实现审计         4 个隔离端口 + A/B 分类
✅ 依赖赋值清点         2 个 Dependencies 结构体，5 处未赋值
✅ 后台任务与孤儿方法    11 个任务 + 5 条孤儿方法
✅ 配置项是否被读取      ← 产出 W1（C7）
✅ 错误码是否被返回      ← 86 个全部有返回路径，零死错误码
✅ 功能开关是否生效      ← 3 个全部生效
⬜ Redis 键与 TTL       未系统核对（已知 4 个前缀均有 TTL）
⬜ 迁移/回填幂等性      4 个回填均已读，均用条件更新（天然幂等），未运行验证
```

**主要维度已完成。** 剩余两项判断产出概率低：
Redis 键只有 `identity/adapters/redis/store.go` 一个适配器；
4 个回填都用 `Where(... IS NULL / <> ?)` 条件更新。

### 变更批次计划（已确认 2026-09-22）

**分解原则**：按**同类根因**合并为 change，不按单个发现拆；**批次与阻塞关系显式声明**；
**不在调研阶段一次性生成全部 Change**。

```
批次 1  移除未生效的依赖端口              C1 + C1b + C3 + C4      ✅ 已完成 remove-unwired-ports
批次 2  依赖接线护栏                      C2                      ✅ 可立即开始（批次 1 已落地）
批次 3  消息长度上限接入配置              C7                      ✅ 需先定默认行为
批次 4  文档事实性修正                    C6a                     ✅ 范围已定
批次 5  公告调度与消息图片清理            C5                      ⛔ 阻塞于设计
押后    规格逐条核对                      C6b                     ❌ 边界未定，需继续调研
```

**Change 工件已创建（2026-09-22，批次 1 已实施）**：

| 批次 | Change 名 | 工件 | 任务数 | 实施状态 |
| --- | --- | --- | --- | --- |
| 1 | `remove-unwired-ports` | proposal · design · specs · tasks | 39 | ✅ 39/39 完成，待归档 |
| 2 | `add-dependency-wiring-guardrail` | proposal · design · specs · tasks | 45 | ⬜ 未开始（依赖批次 1） |
| 3 | `wire-message-length-limits` | proposal · design · specs · tasks | 48 | ⬜ 未开始 |
| 4 | `fix-stale-docs-and-specs` | proposal · design · specs · tasks | 55 | ⬜ 未开始 |

四个均已通过 `openspec validate`。

**批次 1 实施结果**：代码删除、ADR 与验证全部完成；
决策记录见 `docs/adr/0008-authorization-and-audit-ports-on-demand.md`，
行为契约由新增规格 `file-access-contract` 承载，
验证证据（build / 架构测试 / 全量测试 / MySQL 门禁 / 120 条路由快照 / HTTP 信封采样）
见 [wire-audit.md](wire-audit.md) 的「批次 1 实施记录」。

**4 个 change 覆盖除 C5 / C6b 外的全部已确认内容。**
本文件是计划记录，不创建 change；change 由各批次自行维护。

#### 为何 C1+C1b+C3+C4 合并为批次 1

它们不是四件不同的事，而是**同一类问题的四个实例**：

| 项 | 位置 | 核心事实 |
| --- | --- | --- |
| C1 | `files.AuthorizationScope` + `Dependencies.Authorization` + 8 处 `authorize` 调用 + `Operation` 常量集 | `service.go:56-59` nil 时提前 `return nil`；组合根不注入 ⇒ 恒放行 |
| C1b | `authorization.UserVisible` / `OrganizationVisible` | 全库零引用、**测试亦零引用** |
| C3 | `files.AuditMetadataSink` + `Dependencies.Audit` + `service.go:510-511` | 唯一读取点在 `if != nil` 内；实际审计走 Gin context |
| C4 | `messaging.MessagingAuditSink` + `MessagingAuditEntry` + `audit_contract_test.go` | 生产零引用、无 `Dependencies` 字段；规格 `:93` 已由运行日志满足 |

共同性质：**「声明的端口/实现，没有一个生效路径」**，且都是"留着比删掉更危险 ——
让读者以为存在校验"。删除后行为不变（可证明）。

**R0 的代价（不可忽略）**：删除有**信息损失** —— `authorize()` 的存在本身在说
"这些操作应该被判权"。缓解方式：**批次 1 必须同步写 ADR 记录决策与理由**，
否则是净损失。详见 [change-risk-assessment.md](change-risk-assessment.md) 第二节。

#### 为何 C2 不能与批次 1 同批

```
files.Dependencies.Authorization 一旦标注为 `wiring: required`
  ⇒ 护栏【当天即红】（组合根确实未接线）
  ⇒ C1 删除该字段后 C2 才能通过
```

顺序颠倒会让 C2 只能把 `Authorization` 强标为 `optional` —— 护栏沦为掩盖工具。
详见 [wire-guardrail-design.md](wire-guardrail-design.md) 第六节。

#### 为何 C6 拆成 C6a / C6b

```
C6a  已确认的事实性错误（范围已定，不需再调研）
     X1 `/api/health` 幽灵端点 · X2 OpenAPI 失败路径无测试 · X6 `identity:11` 措辞
     外链 :51 调整 · dict 的 4 条 📄未记录 · 3 条 ⚠️ 冲突
     + 首轮报告那 10 条"文档过期"（TODO.md / module-navigation.md 命令 /
       login-security.md 锁定档位 / roadmap 落后两个阶段）

C6b  剩余 10 个规格 / 578 SHALL 的逐条核对（边界未定）
     押后理由：C5 / C7 会改实现，现在补规格会被冲掉；
              且已核 223 SHALL 的结论是"实现完整、缺的是规格文字"，
              即它测的是文档而非代码
```

#### C7 的性质不同于批次 1

`W1`（`messaging.max_title_runes` / `max_body_runes` 是死配置）**不是死代码删除** ——
配置字段应保留，要做的是**把配置接到校验路径上**。

```
校验现状：messaging/domain/content.go:16-17 硬编码
          MaxMessageTitleRunes = 100 / MaxMessageBodyRunes = 20_000
配置现状：config.go:111-113,258-262 解析+默认；config_test.go:284 断言可加载
缺口：    config.Messaging.MaxTitleRunes / MaxBodyRunes 从未被读取
```

**属行为变更**（改配置会改变实际限制），风险等级同 R2 而非 R0。
**需先定**：默认值保持 100 / 20000 不动（仅让配置可覆盖），还是允许放宽。

### 环境事实（影响后续判断）

```
本机 admin 库已停用：audit_logs 截至 2026-08-05，1.17 MB
                     messaging 表零存在，menu_apis 0 行，apis 87 行（非 120）
→ "首次运行对存量数据的影响"在本机为零，但这不代表部署环境
→ 该库启动时会由 AutoMigrate 一次性补齐 10 张 messaging 表
```

>
> **方向演变记录**：契约调研核完 3 个模块 / 39 条路由 / 223 条 SHALL 后，
> 结论为零未实现、4 条规格漏写、3 条措辞不一致 —— 全是文档问题，无一条影响前端。
> 而首轮报告中真实的实现缺口全部来自"读组合根看哪些端口没被注入"。
> 故 2026-09-22 确认转向**实现审计**为主，文档补齐放到各模块实现完成之后。

## 为什么做这件事

项目处于「后端已验收、等待前端」阶段
（`openspec/changes/standardize-api-response-contract/` = `backend-accepted / release-blocked`）。
需要回答的是 **"契约能不能信、能力是否都已生效"**，不是"能不能上线"。

先前一次调研（2026-09-22 会话）用「构建/vet/测试门禁是否通过」作为口径。
该口径只能证明**代码自洽**，不能证明**代码与它声称的行为一致**：
门禁全绿的情况下，仍然存在未接线的依赖、无调用方的调度函数、
规格要求但代码不存在的端点。**该口径已废弃。**

## 方法

对每条路由与每条行为契约，核对三个契约存储位置：

| 位置 | 文件 | 承载什么 |
| --- | --- | --- |
| 规格 | `openspec/specs/<capability>/spec.md` | 对外可观察行为、业务约束 |
| 测试 | `internal/<module>/**/*_test.go` | 路由存在性、access level、权限码、错误状态 |
| 描述符 | `internal/<module>/http.go` 或 `adapters/http/routes.go` | access level、权限码、OpenAPI、公开错误码 |

判定分类（互斥）：

| 判定 | 含义 | 后续动作 |
| --- | --- | --- |
| ✅ 一致 | 至少两处声明且互相一致 | 无 |
| 📄 未记录 | 代码与测试有，规格无 | 补规格 |
| 🧪 未验证 | 只有规格声明，代码与测试无对应断言 | 补测试或补规格 |
| ⚠️ 冲突 | 两处声明不一致 | 定一个真相后统一 |
| ❌ 未实现 | 规格要求，代码无 | 补代码 |

## 调研纪律

1. 任何"规格要求 X"的主张必须附**逐字引用**，不接受只给行号。
   （先前调研中出现过"精确行号 + 编造结论"的引用，两次主要主张均为误报。）
2. 路由清单必须取自**运行时代码**，不接受转录稿。
   （先前调研中曾依据自制转录稿误判 `DELETE /api/admin/users/:id` 不存在。）
3. 任何"代码有缺陷"的结论必须**同时核对写入/删除路径的实际实现**，不接受从声明推断。
   （曾推断"字典软删除与唯一索引冲突"，核对后发现字典全部硬删除，结论不成立。）
4. 未做验证的怀疑写入「证据缺口」，**不得**写入「发现」。

## 进度

| 模块 | 规模 | 路由核对 | 行为契约核对 | 文件 |
| --- | --- | --- | --- | --- |
| dict-management | 45 行 / 9 路由 | ✅ 完成 | ✅ 完成 | [contract-readiness-dict.md](contract-readiness-dict.md) |
| api-management | 249 行 / 10 路由 | ✅ 完成 | ✅ 完成 | [contract-readiness-api-management.md](contract-readiness-api-management.md) |
| user-management | 20 路由 + 头像 22 Scenario + 邮箱验证 15 条 | ✅ 完成 | ✅ 完成 | [contract-readiness-user-management.md](contract-readiness-user-management.md) |
| auth | 186 行 | ✅ 完成（captcha/login/refresh/logout） | ⚠️ 部分（登录锁定档位、密码策略未逐条核对） | 见 user-management 文件 |
| identity | 121 行 | ✅ 完成 | ✅ 完成（邮箱验证 6 个 Requirement） | 见 user-management 文件 |
| auth | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| rbac | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| menu-management | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| file-management | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| organization-management | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| logging | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| internal-messaging | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| identity | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| route-catalog | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| modular-layered-architecture | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| access-version-storage | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |
| api-response-contract | 待测 | ⬜ 未开始 | ⬜ 未开始 | — |

## 已确认的决策

| 日期 | 决策 | 依据 |
| --- | --- | --- |
| 2026-09-22 | 规格正文承诺的能力（如"创建、查询、修改和删除"）视为**应覆盖完整 CRUD**；Scenario 缺失判定为**漏写**，不是有意省略 | 项目所有者确认 |
| 2026-09-22 | 跨模块问题汇入本索引，模块细节留在各自文件 | 项目所有者选择方案 (c) |

## 跨模块问题

按发现顺序记录。每条注明影响的模块范围。

### X1 · `/api/health` 幽灵端点

- **形态**：规格要求，代码不存在。
- **规格要求处**：`openspec/specs/modular-layered-architecture/spec.md:95`、`openspec/specs/route-catalog/spec.md:196`
- **其他描述处**：`docs/adr/0007-internal-messaging-domain-decisions.md:91`、`docs/runbooks/messaging.md:24`
- **代码现状**：无该路由。存活探测实际由 `/ping` 承担（`internal/app/http.go:21-23`）。
- **历史**：曾存在，在 `fca2fa5` 之前的版本中被 `/ping` + `/api/ready` 取代，但规格与文档未同步。
- **影响模块**：route-catalog、modular-layered-architecture、（ADR 与 runbook）
- **判定**：⚠️ 冲突（规格 vs 代码）

### X2 · `/docs/openapi.json` 的失败契约有实现、无测试

- **形态**：规格要求其行为契约，实现存在，但**无任何测试断言**。
- **规格要求处**：`openspec/specs/api-management/spec.md:93-97`
  > **WHEN** `GET /docs/openapi.json` 在成功文档提交前无法生成文档
  > **THEN** 系统返回 HTTP 500 四字段 JSON 错误信封
  > **AND** `error_code` SHALL 为稳定技术错误码
  > **AND** 响应 SHALL NOT 包含内部 Metadata、数据库或生成器错误
- **实现处**：`internal/apidoc/service.go:131-146` — `Document()` 出错时调用
  `httpresponse.WriteError(c, httpresponse.InternalErrorDefinition(), err, nil)`。
- **实现分析**：
  - 500 + `INTERNAL_ERROR` 稳定码 → 满足前两条；
  - `err` 作为 cause 传入，由 `httpresponse.WriteError` 的白名单机制处理
    （cause 存 Gin Context 供日志，不进入响应体）→ **结构性**满足第三条。
- **缺口**：`internal/apidoc` 的全部测试
  （`service_test.go:28,73,108`、`websocket_test.go:17`）只覆盖 `Document()` 的成功路径，
  **没有覆盖 `OpenAPI` handler 的失败路径**。第三条"不包含内部错误"因此无回归保护。
- **为何重要**：这个入口是前端导入 API 契约的入口（`docs/README.md:12`）。
  其失败行为是真实场景（Metadata 源出错），而第三条恰是**泄漏防护**。
- **影响模块**：api-management、route-catalog
- **判定**：🧪 未验证

### X2b · `/docs` 与 `/docs/openapi.json` 不受 Route Catalog 校验（设计如此）

- **设计依据**：`openspec/specs/route-catalog/spec.md:94` 明确将其列为
  "App 技术路由，不由 API Metadata 创建"；`internal/app/http.go:24-27` 直接注册到 Gin。
- **含义**：这两个入口不经过 `routecatalog.New` 的启动前校验，
  因此其契约完全依赖上述 X2 的人工测试覆盖，没有机械护栏。
- **判定**：设计事实（非缺陷），但与 X2 叠加后风险升高

### X6 · `identity:11` 的"对外响应"措辞易被误读为 HTTP 响应

- **形态**：规格措辞不精确，不会导致实现错误，但会使核对者误判为规格冲突。
- **规格原文**（`openspec/specs/identity/spec.md:11`）：
  > 对外响应只返回 `email`、`pending_email` 和 `email_verified`，
  > 不得返回 token、过期时间或 SMTP 状态。
- **实际约束对象**：**通知渠道 Contract**，不是 HTTP 用户资料响应。
  - 通知渠道：`internal/app/messaging.go:313-321` `LookupVerifiedEmail` →
    仅返回 `VerifiedEmail{Address}`（`messaging/application/contracts.go:24-25`
    注释："VerifiedEmail is the only email address a notification adapter may receive."）
  - HTTP 资料响应：`identity/dto.go:33-43` `UserInfo` 返回 9 个字段
    （含 avatar、role、status），由 `user-management:26-30` 要求。
- **误读风险**：核对 `user-management` 时曾据 `identity:11` 怀疑资料响应越界，
  经核对 `dto.go` 与 `messaging.go` 后排除。
- **建议**：补规格时将 `:11` 的"对外响应"改为"通知渠道 Contract 响应"。
- **影响模块**：identity、user-management
- **判定**：⚠️ 规格措辞问题（非实现缺陷）

### X7 · 审计元数据的白名单 + 拒绝 map 是值得复制的加固模式

- **形态**：实现优于规格的正面模式，建议在补规格时作为显式要求记录。
- **证据**：
  - `internal/audit/service.go:26-37` `UploadAuditMetadata` 为白名单结构体，
    不含 object name / URL / 凭据 / 摘要；
  - `service.go:39-71` `UploadMetadata` 只接受调用方自有的 typed struct，
    对 map 显式返回 nil，注释理由：
    "Maps are rejected so arbitrary Gin context values cannot smuggle fields into the log."
- **含义**："敏感字段不进日志"由**结构**保证，而非依赖开发者记得不写。
  同类模式还有 `avatar.go:204-207`（解码后比对头部与实际尺寸）与
  `AvatarUpdateOutcome` 三值枚举（把规格的两种相反补偿动作建模为类型）。
- **建议**：补规格时把这三处提升为显式要求，避免后续重构无意破坏。
- **影响模块**：logging、audit、user-management
- **判定**：方法性发现（正面）

### X3 · 规格质量在模块间差异极大

- **形态**：不是缺陷，但影响调研方法与结论外推。
- **证据**：dict-management 9 条路由中 4 条无规格场景；
  api-management 10 条路由全部有规格场景。
- **含义**："漏写"不是系统性习惯，而是**逐模块完成度差异**。
  不能用一个模块的结果外推另一个模块。
- **影响模块**：全部
- **判定**：方法性发现

### X5 · 规格文件名与模块名不是一对一关系

- **形态**：方法性发现，直接影响核对方式。若不注意会**系统性误判**覆盖率。
- **证据**：`internal/identity` 模块的 20 条路由由**四个规格文件**共同覆盖：
  `auth/spec.md`（captcha/login/refresh/logout）、`identity/spec.md`（邮箱验证）、
  `user-management/spec.md`（注册/资料/密码/上下文/头像/管理员操作）、
  `rbac/spec.md`（间接）。
- **反直觉处**：`openspec/specs/identity/spec.md` 共 121 行、6 个 Requirement，
  **全部内容都是邮箱验证，一条路由名都不提**，
  但它与该模块通过 `:19` 对 `PUT /api/user/info` 的引用实际耦合。
- **风险**：若按"模块 ↔ 同名规格"配对核对，`internal/identity` 的 15 条路由
  会被误判为"无规格覆盖"。
- **核对规则（新增）**：先枚举模块的全部路由，再**全库检索**每条路由的出现在哪份规格，
  不得按文件名假定配对。
- **影响模块**：全部；已知受影响者：identity / user-management / auth
- **判定**：方法性发现

### X4 · `docs/README.md:10` 的权威声明强于实际

- **原文**：`docs/README.md:10` — "当前系统行为以 `openspec/specs/` 为准"
- **实际**：`internal/dictionary/http_test.go:39` 的测试名为
  `TestRoutesExposeDictionaryHTTPContract`，且其断言范围（含 access level 与权限码）
  **超过**规格。路由契约在实践上由测试承载。
- **状态**：已确认规格应覆盖完整 CRUD（决策 1），因此该声明**方向上成立**，
  但当前规格尚不足以支撑它 —— 补齐规格是使其为真的工作，而非改写该声明。
- **影响模块**：全部
- **判定**：方法性发现
