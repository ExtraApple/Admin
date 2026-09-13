# 实现审计 · 接线完整性

> 与契约可信度调研（本目录其他文件）是**两个不同的问题**：
> - 契约调研问「规格与代码对不对得上」→ 产出：规格补齐清单
> - **实现审计问「有没有写完但没生效的能力」→ 产出：接线缺口清单**
>
> 本次审计由方向确认触发（2026-09-22）：
> 契约调研核完 3 个模块 / 39 条路由 / 223 条 SHALL 后，结论是
> **零未实现、4 条规格漏写、3 条措辞不一致** —— 全是文档问题，
> **没有一条影响前端能否使用**。
> 而首轮报告中真实存在的实现缺口（Files 授权、公告调度、MessagingAuditSink、
> ExternalProxy）**没有一条是靠逐条核对规格发现的**，全部来自"读组合根看哪些端口没被注入"。
> 故转入实现审计。

---

## 审计方法

### 判定标准的演进（重要）

本审计第一稿把「未接线」一律视为缺口。经项目所有者提示并核对 ADR 后确认，
必须区分两类 —— 判定规则见第六节：

```
A 类  在 ADR 或规格中有「本次不实现 / 未来扩展点」表述  →  设计，不是缺口
B 类  无此类表述，但规格要求其行为 / 或规格完全未提及    →  缺口
```

**首轮报告的 6 条发现按此标准重新归类**（见第六节）。

### 三个方面

```
① 构造器接线    77 个 New* 构造器 → 哪些从未在 internal/app 被调用
② 端口实现      113 个接口 → 哪些在生产代码中没有任何实现
③ 端口默认值    「可选依赖」在缺失时是放行还是拒绝
```

### 审计脚本与可复现性

方法②用 PowerShell 做机械比对（可重复执行）：

```powershell
# 收集 internal 下全部接口声明，统计每个名字在非测试代码中的出现位置。
# 出现次数 ≤ 3 且全部集中在声明文件内 ⇒ 孤立端口。
```

### 扫描器的已知局限（必须声明）

机械扫描有**假阳性**，逐条核对是必需的：

| 假阳性类型 | 实例 | 原因 |
| --- | --- | --- |
| 字段名 ≠ 类型名 | `ObjectNameGenerator` | `service.go:30-32` 用 `deps.ObjectNames = UUIDObjectNames{}` 注入，扫描器认不出 |
| 隐式接口满足 | `CaptchaGenerator`、`PasswordHasher` | Go 隐式实现，具体类型无需提及接口名 |
| 实现方在其他包 | `Authorization`（messaging） | 由 `internal/app/messaging.go` 的适配器结构体实现 |
| 包内私有接缝 | `refreshNotifier`、`scriptEvaluator` | 包内自用，不经组合根 |

**因此：扫描器只用于**发现候选**，每一条都必须回读源码确认。**

---

## 一、确认的隔离端口（4 个）

「隔离」= 接口在生产代码中**只有声明、没有任何实现或引用**。

### S1 · `AuthorizationScope` —— 🔴 隔离，且缺失时静默放行

- **声明**：`internal/files/application/contracts.go:46-48`
- **被声明为依赖**：`contracts.go:240` `Dependencies.Authorization`
- **生产引用**：无（仅上述两处）
- **缺失行为**：**放行**

```go
// internal/files/application/service.go:55-60
func (s *Service) authorize(ctx context.Context, actor uint, op Operation, id uint) error {
    if s == nil || s.deps.Authorization == nil {
        return nil          // ← 未接线 = 允许一切
    }
    return s.deps.Authorization.Authorize(ctx, actor, op, id)
}
```

- **组合根**：`internal/app/files.go:34-43` 的 `Dependencies` 字面量中**没有 `Authorization`**
- **调用点**：`service.go:81,242,263,302,335,358,382` 共 7 处，全部因 nil 而空转
- **未使用的 `Operation` 常量**：`OperationDownload`、`OperationPreview`（`contracts.go:57-58`
  定义后被 `service.go:484-486` 引用，但同样经 `authorize` 空转）
- **规格依据**：**无**。`openspec/specs/file-management/spec.md` 检索
  "Authorization"/"数据范围" **零命中**；`docs/modify/layered-monolith-restructuring.md:581-592`
  的 Files 所有权清单也不含数据范围。
- **判定**：🔴 **真缺口** —— 要么补实现（则当前是水平越权：持有
  `admin.files.get` + `admin.files.id.get` 者可下载全系统文件），
  要么删除接口（则当前是死代码，且**误导读者以为有校验**）。
  **需要项目所有者裁决是 A 还是 B**（见"待裁决"）。

### S2 · `MessagingAuditSink` —— 🔴 隔离，且不是可选依赖

- **声明**：`internal/messaging/application/contracts.go:106-109`
- **配套结构体**：`MessagingAuditEntry`（`:113-123`），字段为白名单（不含正文/外链/媒体/凭据）
- **生产引用**：无
- **关键差异**：与 S3/S4 不同，它**连 `Dependencies` 字段都没有** ——
  `messaging/application/service.go:37-51` 的 `Dependencies` 不含此字段
- **规格依据**：`docs/adr/0007-internal-messaging-domain-decisions.md:65`
  要求消息发送/编辑/发布/撤销/分类/权限拒绝写入 Audit Log
- **现状**：仅路由级审计（`DefaultAuditCategory = "message"`，
  `messaging/adapters/http/routes.go:139`）覆盖
- **判定**：🔴 **真缺口**（功能性与 S1 同类，但无安全后果）

### S3 · `AuditMetadataSink` —— 🟡 隔离，但实际通道是 Gin context（**无功能缺口**）

- **声明**：`internal/files/application/contracts.go:153-155`
- **配套结构体**：`AuditMetadata`（`:143-152`）—— 结构体**在用**，接口**没在用**
- **预备接缝**：`files/application/service.go:510-511`

```go
if s.deps.Audit != nil {        // 组合根从不注入 ⇒ 永不执行
    s.deps.Audit.Record(ctx, metadata)
}
```

- **实际审计通道**（另一个机制）：

```
files/adapters/http/routes.go:127,311   c.Set(application.UploadAuditMetadataContextKey, AuditMetadata{...})
audit/adapters/http/routes.go:68-70     c.Get(audit.UploadAuditMetadataContextKey) → audit.UploadMetadata(value)
```

两个包各自定义常量 `UploadAuditMetadataContextKey`，字符串值相同
（`files/application/contracts.go:158`、`audit/service.go:20`），**由约定而非编译期保证一致**。

- **影响**：文件上传审计**功能正常**（走 context 通道）。该接口是**未采用的第二条实现路径**，
  属死代码。
- **判定**：🟡 死代码（非功能缺口）。但**两个审计机制并存**是理解障碍，
  且同名常量靠约定同步存在将来分叉风险。

### S4 · `ExternalProxy` —— 🔴 隔离（构造器从未被调用）

- **声明与实现**：`internal/messaging/application/external_proxy.go:50` `NewExternalProxy`
- **辅助接口**：`ExternalIPResolver`（`:22`）—— 同文件内自用
- **生产引用**：构造器**从未在 `internal/app` 出现**
- **规格依据**：`openspec/specs/internal-messaging/spec.md`（外链代理）；
  `docs/adr/0007:61`
- **判定**：🔴 **真缺口**（功能）

---

## 二、已排除的候选（扫描假阳性，逐条核对后排除）

保留记录以避免后续重复排查。

| 候选 | 排除依据 |
| --- | --- |
| `ObjectNameGenerator` | `files/application/service.go:30-32` 有默认值 `UUIDObjectNames{}`，**已正确接线** |
| `AuditMetadata`（结构体） | `service.go:111,149`、`routes.go:127,305` 多处使用，非孤儿 |
| `MessagingMetrics` | `dead_letter_recorder.go:14,21` 在包内使用，属包内接缝 |
| `RefreshSink` | `refresh_hub.go` 内 8 处使用，包内接缝 |
| `AccessVersionInvalidator` | `organization/service.go:16,25` 使用，包内接缝 |
| `CacheInvalidator` | `navigation/service.go:13` 使用，包内接缝 |
| `RoutePermissionSource` | `authorization/adapters/http/shared.go:18` 使用，跨包接线 |
| `RefreshRecoveryStore` | `websocket_gateway.go:40,48` 使用，跨包接线 |
| `NotificationProjectionReader` | `notification_projection.go` 内使用；ADR 0007:105 明确本 Change 只做站内消息与 WS 消费者，属**设计保留** |
| `CaptchaGenerator` / `PasswordHasher` 等 | Go 隐式接口实现，具体类型已注入 |

---

## 三、依赖赋值的机械清点（② 接线护栏的输入）

全库**只有 2 个 `Dependencies` 结构体**（其他模块用位置参数构造器）。

### `internal/files/application/contracts.go:234-246` —— 12 字段，组合根赋 8 个

组合根：`internal/app/files.go:34-43`

| 字段 | 组合根是否赋值 | 缺失时后果 |
| --- | --- | --- |
| `Repository` / `Storage` / `Validator` / `MessageImages` / `MessageImageValidator` | ✅ | — |
| `Transactions` | ✅ | （另有兜底 `directTransactionRunner{}`） |
| `Signer` | ✅ | — |
| `DownloadURLExpireSeconds` | ✅ | （另有兜底 300） |
| **`Authorization`** | ❌ | **静默放行**（S1） |
| **`Audit`** | ❌ | 直接记录路径不执行（S3，有替代通道） |
| `Clock` | ❌ | 兜底 `ClockFunc(time.Now)`（`service.go:24-26`） |
| `ObjectNames` | ❌ | 兜底 `UUIDObjectNames{}`（`service.go:30-32`） |

### `internal/messaging/application/service.go:37-51` —— 13 字段，组合根赋 12 个

组合根：`internal/app/messaging.go:61-67`

| 字段 | 组合根是否赋值 | 缺失时后果 |
| --- | --- | --- |
| `Messages` / `Categories` / `Inbox` / `Notifications` / `Outboxes` | ✅ | — |
| `ConsumerDeadLetters` / `ConsumerDeadLetterReplay` | ✅ | — |
| `Identity` / `Organizations` / `Authorization` / `Files` | ✅ | — |
| `Transactions` | ✅ | （另有兜底 `directTransactionRunner{}`） |
| **`Clock`** | ❌ | 兜底 `ClockFunc(time.Now)`（`service.go:70-72`） |

**结论**：5 处未赋值中，**2 处是真缺口、1 处是死代码、2 处有安全兜底**。
三者在代码外观上完全一样 —— 区别只在 `NewService` 里有没有兜底分支。
**这正是接线护栏要解决的可见性问题**（设计见 [wire-guardrail-design.md](wire-guardrail-design.md)）。

---

## 五、后台任务与孤儿方法扫描（① 后台任务扫描）

### 已注册的后台任务（共 11 个）

| 来源 | 任务名 | 周期 |
| --- | --- | --- |
| `app/files.go:52` | `file-rotation` | 24h ticker |
| `app/audit.go:66` | `audit-log-archive` | 24h ticker |
| `app/app.go:174` | `identity-email-verification-cleanup` | — |
| `app/messaging.go:79` | `messaging-refresh-subscriber` | — |
| `app/messaging.go:114` | `messaging-outbox` | 1s |
| `app/messaging.go:115` | `messaging-consumer` | — |
| `app/messaging.go:116` | `messaging-cleanup` | 立即执行 + 1h |
| `app/messaging.go:128` | `messaging-dlq-recorder-1` … `-5` | 每级一个 |

`messaging-cleanup`（`app/messaging.go:211-225`）覆盖快照清理与终态死信 30 天清理
（`CleanupAudienceDeliveries`、`CleanupFinalConsumerDeadLetters`），**实现完整**。
WebSocket ticket 依赖 Redis TTL 自清理，无需任务。

### 孤儿方法（生产代码中只有定义、无任何调用方）

扫描方式：统计每个「导出的、接收 `context.Context` 的 Service 方法」在
**全部非测试代码**（含 `internal/app` 与各模块 adapter）中的出现次数，次数 = 1 即为孤儿。

| 方法 | 位置 | 判定 |
| --- | --- | --- |
| `CleanupExpiredMessageImages` | `files/application/service.go:196` | 🔴 **真缺口** |
| `PublishDueAnnouncements` | `messaging/application/announcement_service.go:99` | 🔴 **真缺口** |
| `ExpireDueAnnouncements` | `messaging/application/announcement_service.go:103` | 🔴 **真缺口** |
| `UserVisible` | `authorization/application/service.go:443` | ✅ 已删除（批次 1） |
| `OrganizationVisible` | `authorization/application/service.go:454` | ✅ 已删除（批次 1） |
| `ExternalProxy.Fetch` | `messaging/application/external_proxy.go` | 🔴 真缺口（即 S4） |
| `revalidationReader` | `files/application/service.go` | ⬜ 假阳性：私有方法 |

### 🔴 新增发现 A｜`CleanupExpiredMessageImages` 无调用方

与公告调度**完全同型**：实现完整、有测试、有仓储支持，只是没有任务调用。

```
files/application/service.go:196   CleanupExpiredMessageImages()
  → files/application/service.go:206  deps.MessageImages.DeleteExpiredMessageImages(ctx, now, limit)
  → files/adapters/gorm/repository.go:112  完整实现
       SQL: purpose='message_image' AND logical_message_id 为空
            AND binding_expires_at IS NOT NULL AND binding_expires_at <= now
       → 删除记录并清理对象存储

调用方：仅 files/application/message_image_test.go:116
internal/app：零引用
```

**后果（已核实）**：`CleanupExpiredMessageImages` 是**唯一从对象存储侧删除**过期消息图片的路径。

```
files/application/service.go:196-216
  → DeleteExpiredMessageImages(ctx, now, limit)      删除数据库记录
  → 对每条记录调用 Storage.Delete(ctx, bucket, objectName)   删除对象

对比：file-rotation 任务（app/files.go:52，24h）
  → FindRotationCandidates (repository.go:143-153)
       WHERE created_at < cutoff AND bucket = ?      ← 不区分 purpose
  → 只做 Storage.Move（热桶 → 冷桶）+ 更新 bucket 字段
  → 【不删除】数据库记录，也不删除对象
```

**即：没有调用方 ⇒ 过期未绑定的消息图片既不被清理记录、也不被清理对象，
只会被 30 天后的轮转任务从 `files` 搬到 `files-archive`，两边都持续累积。**

**未核实的部分（不作为发现）**：若某消息图片已在轮转后再被绑定引用，
其读取路径改为从 `files` 桶取对象、而对象已在 `files-archive` —— 该失败模式
**本轮未端到端跟踪**，不列入发现。

**规格依据**：`openspec/specs/file-management/spec.md` 与归档 change 的
`file-management` delta 规格要求"15 分钟绑定时限的临时 File Record"，
但**未声明过期清理由谁在何时执行**。这是**规格写了时限、未写清理义务**的实例。

### 🟠 新增发现 B｜`UserVisible` / `OrganizationVisible` 是第二处资源级授权层

```
authorization/application/service.go:443   func (service *Service) UserVisible(ctx, operatorID, targetID uint) error
authorization/application/service.go:454   func (service *Service) OrganizationVisible(ctx, operatorID, organizationID uint) error
```

两个方法在生产代码中**只有定义、无任何调用方**（全库检索仅命中定义行）。

**形态上与 S1 相同**：都是"资源级的可见性/授权判定方法，写了但没接线"。
区别在于：

```
S1  files/application   AuthorizationScope 接口 + 7 处调用点（调用点存在，依赖未注入）
B   authorization/application  两个具体方法（连调用点都没有）
```

**含义**：你裁决的 S1(B)「删除未接线的授权层」**可能不止 files 一处**。
在动手删除之前，应确认 `UserVisible`/`OrganizationVisible` 是否
属于同一个未成型的资源级授权层 —— 若是，应一并处置。
**这一条需要裁决**（见第八节）。

---

## 六、关键区分：有意预留的扩展点 vs 真正未生效的能力

**这一节修正了本审计的判定标准。** 项目所有者的提示（"有一部分 RabbitMQ 内容留做了后续接口"）
经核对**成立**，且 ADR 0007 有明确书面依据。因此「未接线」必须分成两类。

### 判定规则

> **如果一个 seam 在 ADR 或规格中有明确的"本次不实现/未来扩展点"表述，它是设计，不是缺口。
> 否则它就是缺口。**

### A 类｜有意预留的扩展点 —— 不是缺口

| Seam | 书面依据 | 原文 |
| --- | --- | --- |
| `MessagingMetrics` + `messagingZapMetrics` | **ADR 0007:95** | "通过 App 注入的供应商无关 `MessagingMetrics.RecordConsumerDLQPending(...)` best-effort 记录首条 `pending` 告警观测。该 Contract 只有这一无返回值的方法……Messaging 不管理指标名称、通用标签 map 或告警供应商" |
| `NotificationProjectionReader` / `ProjectNotificationRecipients` | **ADR 0007:105** | "本次只实现站内消息和 WebSocket 消费者。邮件、短信不实现具体适配器、发送逻辑或第三方配置；**只保留稳定的事件发布 Contract 和后续可接入的消费者扩展点，不创建空操作或伪实现**" |
| 邮件 / 短信 Consumer | **ADR 0007:87,105**；`internal-messaging/spec.md:83-89` | 同上；规格 Requirement 标题即"**未来**通知 Consumer SHALL 通过最小 Messaging Projection Contract…" |

**其中 `MessagingMetrics` 实际是完整接线的**（本节初稿曾把它列为候选项）：

```
contracts.go:127             接口定义（ADR 0007:95 声明的 Contract）
app/messaging.go:286         messagingZapMetrics 适配器实现 RecordConsumerDLQPending
app/messaging.go:96          注入 ConsumerDeadLetterRecorderConfig.Metrics
dead_letter_recorder.go:84   调用
```

扫描器之所以把它列为候选，是因为**适配器结构体实现了接口但未提及接口名**。
这是机械扫描的第 5 类假阳性（见第一节"扫描器的已知局限"）。

### B 类｜真正未生效 —— 规格要求，无 deferral 表述

| Seam | 规格原文 | 代码现状 | 判定 |
| --- | --- | --- | --- |
| 外链代理 | `internal-messaging/spec.md:51`（Requirement 正文，**非**"未来"章节）<br>"**外链代理只允许 HTTPS 公网目标，拒绝私网、环回、云元数据、不受控重定向、超时、超大响应和 MIME 不匹配。**" | `ExternalProxy` + `NewExternalProxy` 完整实现，**构造器从未被调用**；`ExternalProxy.Fetch` 无调用方 | 🔴 **规格要求未生效** |
| 消息审计 | `internal-messaging/spec.md:93`（Requirement 正文）<br>"**关键消息操作、权限拒绝、Outbox/Consumer/DLQ 处置 SHALL 写入受控审计或运行日志**" | `MessagingAuditSink` + `MessagingAuditEntry`（白名单结构体）已定义，**连 `Dependencies` 字段都没有**；仅路由级审计 `DefaultAuditCategory="message"` 覆盖 | ⚠️ **需核实覆盖范围** |
| 文件授权 | `file-management/spec.md` **零命中** "Authorization"/"数据范围" | `AuthorizationScope` 声明 + 8 处调用点，依赖未注入 ⇒ 静默放行 | ✅ 已删除（C1，批次 1） |
| 资源可见性 | 无规格出处 | `UserVisible` / `OrganizationVisible` 零调用方 | ✅ 已删除（C1b，批次 1） |

**关于"消息审计"的措辞**：规格写的是"受控审计**或**运行日志"。
路由级审计中间件确实为消息入口记录了审计；但 ADR 0007:65 的表述更严格
（要求发送/编辑/发布/撤销/分类/权限拒绝**写入 Audit Log**）。
两者是否等价**本轮未核实**，故标 ⚠️ 而非 🔴 —— 见第九节证据缺口。

### A 类与 B 类的最终划定（已确认 2026-09-22）

项目所有者确认：**有意预留的扩展点就是下列三项，无其他**。
`ExternalProxy` 不在预留清单内，故归 B 类。

| Seam | 类别 | 书面依据 | 处置 |
| --- | --- | --- | --- |
| `MessagingMetrics` + `messagingZapMetrics` | **A** | **ADR 0007:95** | 不动（且已完整接线） |
| `NotificationProjectionReader` / `ProjectNotificationRecipients` | **A** | **ADR 0007:105** | 不动 |
| 邮件 / 短信 Consumer | **A** | **ADR 0007:87,105**；规格 `:83-89` | 不动 |
| `ExternalProxy` / `ExternalProxy.Fetch` | **A** | 所有者确认：**留给二次开发**；ADR 0007:126（「风险」章节）"服务端代理外链和图片会引入 SSRF、超时、大小、MIME、缓存和审计风险，后续设计必须把这些约束写成可测试行为" | **不动**（保留为二次开发接入点）；需同步调整规格 `:51`，见下 |
| `MessagingAuditSink` | **B** | 规格 `:93` 为 Requirement 正文、无 deferral | 先核实覆盖范围，再决定接线或删除 |
| `AuthorizationScope` | **B** | `file-management/spec.md` 零出处 | C1 删除（已裁决） |
| `UserVisible` / `OrganizationVisible` | **B** | 无规格出处 | C1b 待裁决 |

### 外链的当下实际保护（已核实 —— 无安全风险）

决定不做代理后，需确认消息外链在**协议层面**是否仍受约束。已核实**受约束**：

```
internal/messaging/domain/content.go:41   safeExternalURL = regexp.MustCompile(`(?i)^https://`)

content.go:83   policy.AllowAttrs("href", "title").Matching(safeExternalURL).OnElements("a")
content.go:84   policy.AllowAttrs("alt", "height", "src", "title", "width")
                      .Matching(safeExternalURL).OnElements("img")

↑ bluemonday 的 Matching 对 href/src 【强制】匹配 ^https://，不匹配即剥离属性
另有两道前置防线：unsafeMarkdownDestination、unsafeHTMLProtocol（拦 javascript:/data:/vbscript:）
```

**即：外链在协议层面已被强制为 HTTPS。未实现的只是「服务端代理抓取」这个平台能力。**

**推论**：既然不做代理，服务端不抓取外部 URL，**SSRF 威胁模型不成立**。
规格 `:51` 中"拒绝私网、环回、云元数据、不受控重定向、超时、超大响应和 MIME 不匹配"
这一串约束**只在实现代理时才有意义**。

### 由此产生的规格调整（属 C6）

规格 `:51` 当前把"外链代理"与"内容清洗"写在**同一个 Requirement 正文**中，
且未标注 deferral。既然确认不做，应调整为二者之一：

```
选项 1  从 :51 移除代理相关表述，仅保留内容清洗与图片用途约束
        并把"HTTPS 强制"这一【已实现】行为显式写进规格（当前规格未提及 content.go 的强制）
选项 2  改为"未来能力"章节，并注明当前以协议级 HTTPS 强制替代
```

**我建议选项 2** —— 与既有 `:83-89`「未来通知 Consumer」的写法一致，
且保留二次开发的接入说明。本项归入 **C6**（规格与代码一致性补齐）。

**为何 `ExternalProxy` 归 B 而非 A**：ADR 0007:126 的原文是
"服务端代理外链和图片会引入 SSRF、超时、大小、MIME、缓存和审计风险，
**后续设计必须把这些约束写成可测试行为**"。该句位于 ADR 的
**「风险」章节而非「决策」章节** —— 它规定的是"若实现该能力，约束须可测试"，
不是"本 Change 不实现该能力"。三条依据（规格正文无 deferral、
ADR 章节位置、所有者确认）一致指向 B 类。

**A 类不产生 change，只产生一条登记记录。** 见第七节变更清单的说明。

---

### 对计划的含义

```
C4（端口与能力对齐）
  ├─ A 类三项【不动】—— 删除会违背 ADR 0007:95 与 :105
  │   登记为"有意预留"，避免下次审计重复报告
  ├─ ExternalProxy：规格要求未生效 → 接线 或 从规格移除该要求
  └─ MessagingAuditSink：先核实规格覆盖范围，再决定接线或删除

本次审计的判定标准已在顶部同步更新。
```

---

## 七·补｜维度 ①②③ 扫描结果（2026-09-22）

在已完成的三个维度（端口实现 / 依赖赋值 / 后台任务与孤儿方法）之外，
补充扫描三个同型维度。**净产出 1 条新发现。**

### ① 配置项是否真被读取 —— 🟠 1 条新发现

**方法**：提取 `config.go` 全部 70 个字段，检查每个字段在生产代码
（排除 config 包自身与测试）中是否有 `.Field` 形式的引用。

**结果**：11 个候选，其中 9 个是 `*Env` 字段（合理 —— 它们只在 config 包内用于读环境变量，
如 `PasswordEnv`、`SecretEnv`、`UsernameEnv`）。**2 个是实质性发现：**

#### 🟠 W1｜`messaging.max_title_runes` / `max_body_runes` 是死配置 ✅ 已修复（批次 3）

```
配置链路（完整）
  config.yaml:47-49        messaging.max_title_runes: 100 / max_body_runes: 20000
  config.go:111-113        字段声明（yaml tag）
  config.go:258-262        解析 + 设默认值 100 / 20000
  config.go:282            校验 >= 1
  config_test.go:284       断言自定义值 120 / 25000 能被正确加载   ← 测试"证明"配置生效

实际约束（硬编码，与配置无关）
  messaging/domain/content.go:16-17
      MaxMessageTitleRunes = 100
      MaxMessageBodyRunes  = 20_000
  content.go:47,50         用这两个常量做校验

→ config.Messaging.MaxTitleRunes / MaxBodyRunes 【从未被读取】
```

**后果**：把 `config.yaml` 的 `max_title_runes` 改成 500，标题上限**仍然是 100**。
配置测试会绿 —— 因为它只验证"解析正确"，不验证"有人使用"。

**这是比"孤儿方法"更隐蔽的形态**：解析了、设默认了、校验了、测试覆盖了，
但**没有生效路径**。孤儿方法至少还能通过"零调用方"扫出来；
死配置需要一个专门的"字段读取"扫描才能发现。

**对照组**：同一配置块里的 `MaxAudienceUsers` 是**真正生效**的
（`app/messaging.go:103` → `event_consumer.go:104`），说明这是单点疏漏而非整体设计。

### ② 错误码是否真被返回 —— ✅ 零发现

**方法**：提取全部错误码常量（86 个），检查每个码的字符串字面量在生产代码中的出现次数。

**结果**：26 个初筛候选，**逐个核实后全部为假阳性**：

```
messaging 的 21 个 MSG_* 码    → 定义在 errors.go:14-34，经常量标识符在
                                classifyMessageError（errors.go:88-110）中返回
                                ← 扫描器用字符串字面量匹配，漏掉标识符引用形态
IDENTITY_USER_NOT_FOUND        → 经 CodeUserNotFound 常量使用
                                （application/service.go:18 构造，http/errors.go:135 分类）
auth / dict / org / file / nav 等其余码 → 同类情况
amqps / data_access / operation / rabbitmq_confirm_timeout
                               → 非错误码（配置文件取值、审计分类、失败码常量）
```

**结论：86 个声明的错误码全部有返回路径，未发现死错误码。**

### ③ 功能开关是否真生效 —— ✅ 零发现

| 开关 | 读取点 | 效果 | 判定 |
| --- | --- | --- | --- |
| `api_docs.enabled` | `app/app.go:142` | 门控 `/docs` 与 `/docs/openapi.json` 注册 | ✅ |
| `file_rotation.enabled` | `app/files.go:46`（门控 job）+ `app/build.go:95`（门控冷桶创建） | 两处一致 | ✅ |
| `audit_log_archive.enabled` | `app/audit.go:63` | 门控归档 job | ✅ |

三个开关的 `Enabled` 与其余字段（Days/RetentionDays/BatchSize/Title/Version/Description）
均在 `app` 层被实际读取，无死配置。

### 本轮净结论

```
新增发现    W1  messaging 的两个消息长度上限配置是死配置
零发现      ② 错误码（86 个全部有返回路径）
            ③ 功能开关（3 个全部生效）
假阳性      26 个错误码候选 —— 扫描器认不出「常量标识符引用」形态（第 6 类假阳性）
```

**W1 的处置**：属于 C6（规格与代码一致性补齐）还是 C1 系列？倾向**单列**——
它不是死代码删除（配置字段仍应保留），而是**把配置接到校验路径上**，
属行为变更（改配置会改变实际限制），故风险等级同 R2 而非 R0。

---

## 八、跨端口的共性：可选依赖默认「放行」而非「拒绝」

四个隔离端口中，**两个在缺失时静默降级为允许**：

```
S1  AuthorizationScope    nil → return nil        （放行）  🔴 安全影响
S3  AuditMetadataSink     nil → 不记录             （无操作）🟡 审计影响（但有替代通道）
S2  MessagingAuditSink    无字段 → 无法注入        （无操作）🔴 审计影响
S4  ExternalProxy         未构造 → 功能不可用      （无操作）🔴 功能影响
```

**模式**：这些端口的零值语义是「跳过检查」，而不是「失败关闭」。
加上 `routecatalog` 与架构边界测试**只校验形状、不校验接线**，
导致"声明了但没接线"不会在任何门禁上暴露。

这与首轮报告发现的 Files 授权缺口是**同一个根因**，只是当时只看到一条实例。

---

## 四、待裁决（需项目所有者决定，代码无法回答）

### S1 的处置方向

```
A. 补实现 —— 认为文件应受数据范围约束
   → 需要定义：管理员文件的可见范围规则是什么？
   → 当前状态即为水平越权，属安全缺陷
   → 需要 OpenSpec change（新增数据范围要求）

B. 删除接口 —— 认为管理员文件是全局管理资源
   → 删除 AuthorizationScope、Dependencies.Authorization、
     7 处 authorize 调用、以及 Operation 常量集
   → 保留 UploaderID 仅作审计用
   → 理由：留着比删掉更危险，它让人以为存在校验
```

**这一条必须由你裁决。** 规格里没有任何依据，两种都是合法选择。

### S2/S4 的处置

```
S2 MessagingAuditSink   → 接线上（把消息操作写入审计）或删除
S4 ExternalProxy        → 接线上（外链代理）或从规格中移除该要求
```

两者都需要先确认「这个能力是否还要做」。

---

## 六、变更分解与完成顺序（已确认 2026-09-22）

### 原则

```
每个部分【单独】开 Change —— 不合并、不批量
但【完成顺序显式声明】—— 不按发现顺序临时决定
且【逐轮创建】—— 不在调研阶段一次性生成全部 Change
```

**为何单独开**：本项目的历史 change 粒度本就是「小 change 独立推进」
（`add-dictionary-crud` 8 任务、`add-organization-crud` 8 任务、
`split-authorization-http-adapter` 14 任务）。合并会跨越能力边界，导致验收标准不清。

**为何要声明顺序**：本审计的发现之间存在**硬依赖**，见下。

### 硬依赖（顺序的根据）

```
C2 接线护栏  ──依赖──▶  C1 必须先完成
  │
  └─ 原因：files.Dependencies.Authorization 一旦标注为 required，
          护栏【当天即红】（组合根确实未接线）。
          C1 删除该字段后，C2 才能落地。
          若顺序颠倒，C2 只能把 Authorization 强标为 optional —— 护栏沦为掩盖工具。
```

### 计划中的 Change 清单（已确认 2026-09-22，见 [contract-readiness.md](contract-readiness.md)）

**分解原则**：按**同类根因**合并，不按单个发现拆。

| # | 覆盖发现 | 状态 |
| --- | --- | --- |
| **C1** | S1(B)：`AuthorizationScope` + `Dependencies.Authorization` + 8 处 `authorize` 调用 + 未被调用的 `OperationDownload`/`OperationPreview` 常量 | ✅ **已实施**（批次 1） |
| **C1b** | `authorization.UserVisible` / `OrganizationVisible`（第五节发现 B） | ✅ **已实施**（零引用确认含测试） |
| **C2** | 护栏：23 条 `wiring` 注解 + 1 个 Go AST 架构测试 | ✅ **已实施**（批次 2） |
| **C3** | `AuditMetadataSink` 死代码 + `deps.Audit` + 两个 `UploadAuditMetadataContextKey` 常量合一 | ✅ **已实施**（规格已由 GIN context 通道满足） |
| **C4** | `messaging.MessagingAuditSink` + `MessagingAuditEntry` + 其契约测试 | ✅ **已实施**（规格 `:93` 已由运行日志满足） |
| **C5** | `PublishDueAnnouncements` / `ExpireDueAnnouncements` + `CleanupExpiredMessageImages` | ⬜ 扫描完成，**阻塞于设计** |
| **C6a** | 已确认的事实性错误：X1 / X2 / X6 / 外链 `:51` / dict 4 条 📄未记录 / 3 条 ⚠️ 冲突 + 首轮报告 10 条"文档过期" | ✅ 范围已定 |
| **C6b** | 剩余 10 个规格 / 578 SHALL 的逐条核对 | ❌ 边界未定，需继续调研 |
| **C7** | **W1**：`messaging.max_title_runes` / `max_body_runes` 死配置接入校验路径 | ✅ **已实施**（批次 3） |

**C5 的两条是同一形态**：实现完整、有仓储与测试支持、只是没有后台任务调用。
`CleanupExpiredMessageImages` 在首轮报告中未被发现（当时只掌握了公告调度一条）。

**A 类（有意预留，不动）不产生 change**：`MessagingMetrics`、`NotificationProjectionReader`、
邮件/短信 Consumer、`ExternalProxy`。删它们会违背 ADR 0007:95 与 :105。

### 完成顺序（批次）

```
批次 1  移除未生效的依赖端口        C1 + C1b + C3 + C4      ✅ 已完成 remove-unwired-ports
批次 2  依赖接线护栏                C2                      ✅ 已完成 add-dependency-wiring-guardrail
批次 3  消息长度上限接入配置        C7                      ✅ 已完成 wire-message-length-limits
批次 4  文档事实性修正              C6a                     ✅ 范围已定
批次 5  公告调度与消息图片清理      C5                      ⛔ 阻塞于设计
押后    规格逐条核对                C6b                     ❌ 需继续调研
```

**4 个 change 覆盖除 C5 / C6b 外的全部已确认内容。**

### 批次 1 实施记录（`openspec/changes/remove-unwired-ports/`）

**状态**：代码、ADR 与验证全部完成，待归档。

| 覆盖 | 实际删除内容 |
| --- | --- |
| C1 | `files/application`：`AuthorizationScope`、`Operation` 常量集（9 个）、`Dependencies.Authorization`、`authorize`、`inputOperation` 与 8 处调用点 |
| C3 | `files/application`：`AuditMetadataSink`、`Dependencies.Audit`、`recordAudit` 与 2 处调用点；`UploadAuditMetadataContextKey` 收敛为 `platform/httpresponse.UploadAuditMetadataKey` 单一来源（读写两侧改为引用同一常量） |
| C4 | `messaging/application`：`MessagingAuditSink`、`MessagingAuditEntry`、`audit_contract_test.go` |
| C1b | `authorization/application`：`UserVisible`、`OrganizationVisible` |

**与本节原记录的差异**：`authorize` 调用点实际为 **8 处**（`Upload` / `List` / `Get` /
`Update` / `Delete` / `Browse` / `Revalidate` / `Open`），本文件多处原记为 7 处，已更正。
差异不影响裁决，但重新引入文件级授权时须按 8 个操作点评估。

**验证证据**（全部通过）：

```
go build ./...                                         → exit 0
go test ./testsupport -run TestArchitecture -count=1   → ok
go test ./... -count=1                                 → 43 ok / 10 no-test-files / 0 FAIL
go test -tags=mysql_integration ./... -count=1         → 43 ok / 0 FAIL（独立探针库）
120 条路由快照（集合 + 声明顺序 + Access Level + 权限码） → 与变更前逐行一致
/api/admin/files 与 /files/:id/download 的 5 个 HTTP 采样（状态码 + 响应信封）→ 逐字节一致
```

**决策记录**：`docs/adr/0008-authorization-and-audit-ports-on-demand.md`
（含被删端口的原始签名、A 类接缝清单与其 ADR 0007 依据、以及"按规格未要求删除"的裁决依据）。

**行为契约固化**：删除后「文件访问授权只由路由级权限码决定」失去代码痕迹，
由新增规格 `file-access-contract`（`openspec/changes/remove-unwired-ports/specs/file-access-contract/spec.md`）承载。

**未触碰**：A 类接缝（`MessagingMetrics`、通知投影 Contract、邮件/短信 Consumer、`ExternalProxy`）、
C5（公告调度与消息图片清理）、C6a/C6b、C7，以及护栏 C2 —— 均按批次计划保持独立。

### 批次 2 实施记录（`openspec/changes/add-dependency-wiring-guardrail/`）

**状态**：注解、护栏测试、负向验证与文档全部完成，待归档。

| 覆盖 | 实际交付 |
| --- | --- |
| C2 | 两个 `Dependencies` 结构体共 **23 个字段**全部标注 `// wiring: required|optional`（files 6+4，messaging 11+2）；新增 AST 护栏测试 `TestArchitectureDeclaredDependenciesAreWired` |

**与设计原文的差异**：设计写「14 条注解 + 需修正组合根 2 处」，
实际为 **23 条注解、组合根 0 处改动** ——
`Authorization` 与 `Audit` 已由批次 1 删除，其余 optional 字段沿用构造器兜底
（策略见 [wire-guardrail-design.md](wire-guardrail-design.md) 第七节）。

**护栏实际拦截的形态**（均经负向验证实测失败，随后恢复）：
required 字段漏赋值、字段缺注解、注解拼写错误、
`type X = Dependencies` 类型别名、位置参数式依赖字面量。

**已知局限**：静态检查无法求值，`messaging.ConsumerDeadLetterReplay` 在
RabbitMQ 未配置时组合根赋 `nil` —— 字段标 `required` 且护栏通过，
但 nil 由 `admin_service.go:198` 的守卫降级为受控错误。
这是 design R5 声明的取舍，不是护栏缺陷。

**验证证据**：

```
go build ./...                                    → exit 0
go test ./testsupport -run TestArchitecture       → ok（17 个架构测试）
go test ./... -count=1                            → 43 ok / 10 no-test-files / 0 FAIL
go test -tags=mysql_integration ./... -count=1    → 43 ok / 0 FAIL（独立探针库）
```

### 批次 3 实施记录（`openspec/changes/wire-message-length-limits/`）

**状态**：代码、配置校验、测试与文档全部完成，待归档。

| 覆盖 | 实际交付 |
| --- | --- |
| C7 / W1 | 死配置的标题与正文上限接入校验路径：新增领域值类型 `ContentLimits` + `DefaultContentLimits()`；`CompileMessageContent` 接受上限参数；4 处调用点全部传入注入上限；组合根把 `config.Messaging` 两个值转换为 `ContentLimits` 注入；配置补充上界校验与字段注释 |

**关键设计约束（已满足）**：`internal/messaging/domain` **未新增任何 import** ——
上限以值类型经 application 层传入，域层不读配置（架构测试通过）。

**与设计/任务的差异**：

```
domain 新增错误      设计未提及 → 实际新增 ErrMessageLimitsInvalid
                     非法上限（0 或负数）在域层返回它，未被 HTTP 映射，
                     落到 errors.go 的 default 分支 ⇒ 500 内部错误（符合 tasks 2.5）
content_test.go 调用点  任务记为「4 处」→ 实际 8 处调用（5 个测试函数），已全部更新
spec 第 7 个 Scenario  原写「小于 1 即失败」，但 0 是配置的「未配置」哨兵（→取默认值）
                     ⇒ 经裁决收紧措辞为「负数或高于上限」，并新增「未配置取默认值」Scenario
上界取值             MaxTitleRunes [1, 1000]、MaxBodyRunes [1, 100000]
                     依据：≥ 既有测试用值（120 / 25000）且为明显的健全性边界；
                     128 KiB HTML 安全上限保持独立，不参与该边界推导
```

**验证证据**：

```
go build ./...                                     → exit 0
go test ./testsupport -run TestArchitecture        → ok（domain 层无 config 依赖）
go test ./... -count=1                             → 43 ok / 10 no-test-files / 0 FAIL
go test -tags=mysql_integration ./... -count=1     → 43 ok / 0 FAIL（独立探针库）
默认值不变断言                                      → config.yaml = 100/20000 = domain 默认值；
                                                     标题 100 通过 / 101 拒；正文 20000 通过 / 20001 拒
配置生效断言                                         → 注入 120/25000 后四条路径
                                                     （CreateAnnouncement / EditAnnouncement /
                                                     CreateBroadcast / SendPrivateMessage）
                                                     均接受 101 字符标题与 20001 字符正文，并拒绝超限值
```

**部署期风险核对（tasks 10.4）**：仓库内唯一的 `config.yaml` 使用默认值 100 / 20000，
因此本变更上线后行为与既往完全一致；若某部署另行填入了非默认值，本变更上线后这些值
**将首次真正生效**。运维说明见 `docs/runbooks/messaging.md` 的「消息长度上限」一节。

`C1+C1b+C3+C4` 合并的理由：它们**不是四件不同的事**，而是
「声明的端口/实现，没有一个生效路径」这一根因的四个实例，
且都是"留着比删掉更危险 —— 让读者以为存在校验"。

**批次 1 必须同步写 ADR** —— 删除有信息损失（`authorize()` 的存在本身
在说"这些操作应该被判权"）。详见 [change-risk-assessment.md](change-risk-assessment.md) 第二节。

**本文件与索引均不创建 Change** —— 它们是计划记录，Change 待逐轮创建。

### 与在途 Change 的关系

`openspec/changes/standardize-api-response-contract/` 处于
`backend-accepted / release-blocked`，其 §8–§10 阻塞于前端 `web/` 不存在。
**C1–C6 均不与之冲突**，但需注意：

- 该 change 的 `route-catalog` / `api-management` delta spec 已合入主规格；
- C1/C2 触及 `files` 与 `testsupport`，不在其影响面内；
- C6 会改写 `openspec/specs/`，应避免与其未完成的 §10.6（更新长期规格导航）重叠。

---

## 七、下一步

本审计只覆盖**接线完整性**一个维度。尚未覆盖：

```
① 后台任务注册     已发现 1 条（公告调度未注册），需系统扫描全部 BackgroundJob
② 门禁覆盖         ✅ 已由批次 2 补齐 —— required 字段是否被组合根显式装配，
                   由 TestArchitectureDeclaredDependenciesAreWired 强制
                  缺一条"每个声明的必需端口是否都被注入"的断言
③ 其余模块的端口    本审计的候选来自全库扫描，但逐条核对只做了 12 个候选
```

建议顺序：**①（可机械扫描）→ ②（防复发的护栏）→ S1/S2/S4 的裁决落地**。
