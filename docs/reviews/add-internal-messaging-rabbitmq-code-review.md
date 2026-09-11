# `add-internal-messaging-rabbitmq` Code Review Findings

## 审查范围

- 审查对象：当前工作区未提交变更。
- 固定基线：`HEAD` `091be6a6845c6dd3d762e830c96dbb8ca6b9daad`。
- `main...HEAD` 无提交差异；当前工作区包含本次未提交实现。
- 规范来源：`openspec/changes/add-internal-messaging-rabbitmq/` 及其 delta specs。
- 本文件只记录问题，不表示问题已经修复。

## Standards

共 6 项发现：5 项硬性文档违例，1 项判断性 smell。

### 硬性违例

1. **CONTEXT 混入实现细节**

   `CONTEXT.md:210-216` 新增 RabbitMQ/Redis/WebSocket payload、`RuntimeLogger`、`/api/ready`、DLQ 租约/重放等实现/API 细节。

   违反 `docs/agents/domain.md:34-42`：`CONTEXT` 只能维护术语、定义和业务边界，不能记录 API 细节、数据库结构、实现任务或设计草稿。

2. **Application 层直接依赖 WebSocket SDK**

   `internal/messaging/application/websocket_gateway.go:12,154-155,296` 直接依赖 `github.com/coder/websocket`，并暴露 SDK `StatusCode`。

   违反 `docs/adr/0006-modular-layered-monolith-contracts-and-composition-root.md:37-43` 及 `openspec/specs/modular-layered-architecture/spec.md:182-188` 的 Application 与基础设施/WebSocket 细节隔离要求。

3. **跨模块 Contract 未集中到 `contracts.go`**

   Messaging Application 将 Contract 分散到 `audit_contract.go`、`authorization_contract.go`、`identity_contract.go`、`organization_contract.go`、`message_image_files_contract.go` 等文件，没有统一的 `contracts.go`。

   违反 `docs/module-navigation.md:35` 及主架构规格 `openspec/specs/modular-layered-architecture/spec.md:79-85` 的 single `contracts.go` 要求。

4. **OpenSpec 主规格提前同步**

   活跃的 `add-internal-messaging-rabbitmq` change 尚未 archive，却修改或新增：

   - `openspec/specs/api-management`
   - `openspec/specs/file-management`
   - `openspec/specs/logging`
   - `openspec/specs/modular-layered-architecture`
   - `openspec/specs/organization-management`
   - `openspec/specs/route-catalog`
   - `openspec/specs/internal-messaging`
   - `openspec/specs/identity`

   违反 `openspec/README.md:26-34` 关于先在 `changes/` 中维护，完成实现和验证后 archive，再合并主规格的流程要求。

5. **运行日志字段超出白名单**

   `internal/app/messaging.go:162-164,176-178,190-192,201-203,213-218,285-289` 直接写入 `component`、`error_code`、`oldest_pending_age` 等字段。

   违反 `docs/runbooks/runtime-logging.md:42-46` 的 Messaging 运行日志字段白名单及 App 适配边界。

### 判断性 smell

- **Duplicated Code**：以下文件重复 validate → open channel → confirm → marshal → publish → await confirm 逻辑：
  - `internal/messaging/adapters/rabbitmq/publisher.go:35-66`
  - `internal/messaging/adapters/rabbitmq/failure_publisher.go:41-72`
  - `internal/messaging/adapters/rabbitmq/replay_publisher.go:20-51`

  该项属于判断性重复代码 smell，不是硬性违例。

## Spec

共 5 项发现：4 项高风险，1 项中风险。

1. **高风险：公告创建违反 draft-first 边界**

   规范要求：“公告创建始终产生 `draft`，立即发布和定时发布均须调用显式 publish 动作”（`openspec/changes/add-internal-messaging-rabbitmq/design.md:89`；`openspec/changes/add-internal-messaging-rabbitmq/specs/internal-messaging/spec.md:70-74`）。

   `internal/messaging/application/announcement_service.go:42,148-156` 根据 `PublishAt` 直接创建 `scheduled` 或 `published`，允许创建接口绕过显式发布动作。

2. **高风险：消息图片绑定未端到端实现**

   规范要求创建、编辑、发布时在同一事务绑定上传者拥有的临时图片（`openspec/changes/add-internal-messaging-rabbitmq/specs/internal-messaging/spec.md:245-250`；`openspec/changes/add-internal-messaging-rabbitmq/specs/file-management/spec.md:15-19`）。

   `internal/messaging/adapters/http/dto.go:52-115` 没有图片 ID 字段，Application 没有调用 `BindMessageImages`，只有未使用的 Contract。上传图片无法成为消息所属媒体或被正常读取。

3. **高风险：动态受众快照可能残留部分投递记录**

   规范要求租约/围栏丢失时回滚未完成快照，不写入部分用户快照（`openspec/changes/add-internal-messaging-rabbitmq/specs/internal-messaging/spec.md:352-359`；`design.md:158`）。

   `internal/messaging/application/event_consumer.go:166-187` 分批写入，`internal/messaging/adapters/gorm/repository.go:796-820` 依赖后续重投递清理。租约或围栏失效后，已写入的前置批次可能残留。

4. **高风险：邮箱确认与变更缺少用户级事务锁**

   规范要求同一用户通过事务锁串行化，遵循先提交者生效（`openspec/changes/add-internal-messaging-rabbitmq/specs/identity/spec.md:84-91`）。

   `internal/identity/adapters/gorm/repository.go:231-255` 的确认查询没有 `FOR UPDATE`，`internal/identity/application/user_service.go:67-107` 在事务外读取用户，存在确认与变更并发使用陈旧状态的风险。

5. **中风险：每小时三次邮箱签发额度存在并发竞态**

   规范要求自动签发和显式签发共享严格的每小时三次上限（`openspec/changes/add-internal-messaging-rabbitmq/specs/identity/spec.md:157-181`）。

   `internal/identity/application/email_verification.go:71-104` 先计数，随后由 `internal/identity/adapters/gorm/repository.go:201-214` 替换凭据。并发请求可能同时观察到未超限并签发超过三份凭据。
