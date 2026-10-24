## Context

Messaging 在确认消息当前可见后，通过 App 组合根调用 Files 的消息图片 Contract。Files 返回的是 Files-owned 安全错误；当图片已经登记清理、对象不存在或访问不合法时，组合根直接透传该错误，Messaging HTTP 无法识别，最终返回 `MSG_INTERNAL_ERROR`。

错误翻译必须位于模块边界，不能让 Messaging application 或 HTTP 层依赖 Files 的具体 error 类型。

## Goals / Non-Goals

**Goals:**

- 在 App 组合根将 Files 错误翻译为 Messaging-owned 错误。
- access-invalid、未找到、未绑定、已登记清理统一返回 `ErrNotFound`。
- 存储/持久化未知失败返回受控内部错误。
- 保持消息可见性检查先于 Files 调用。
- 覆盖正常二进制读取、404 资源拒绝和 500 基础设施失败。

**Non-Goals:**

- 不新增公开错误码或路由。
- 不修改 Files domain/application 错误定义。
- 不改变消息可见性、图片绑定、清理登记或对象存储策略。
- 不返回 Files 原始错误、对象路径、bucket 或 SDK 错误。

## Decisions

### 1. 在 messagingFilesReader 做边界翻译

`messagingFilesReader.OpenMessageImage` 调用 Files service 后，使用 Files 的稳定错误分类判断：

- access invalid / object not found → `messagingapplication.ErrNotFound`；
- 其他 Files 错误 → `messagingapplication.ErrMessagingDependency` 或等价受控内部错误。

Messaging HTTP 既有错误分类即可输出 `MSG_NOT_FOUND` 或 `MSG_INTERNAL_ERROR`。Files 的原始错误只留在内部日志边界，不进入 HTTP 响应。

备选方案是让 Messaging application 导入 `uploadsecurity`，这会反向耦合业务模块；或新增公共错误码，增加 API 兼容面，均不采用。

### 2. 可见性检查保持在 Messaging application

`OpenVisibleMessageImage` 继续先读取当前 Inbox visibility。不可见消息直接返回 `ErrNotFound`，不触发 Files；可见消息才调用 adapter。

### 3. 二进制成功响应保持不变

成功时继续返回规范 image/jpeg、image/png 或 image/webp 原生响应，不通过 JSON envelope，不改 Content-Type/流语义。

## Risks / Trade-offs

- Files 未来新增错误分类必须明确归入“资源不可见”或“内部失败”，否则 adapter 应安全地选择内部错误。
- 404 会隐藏“图片不存在”和“图片已登记清理”的差异，这是防止资源状态枚举的有意取舍。
- 本 change 不处理历史审计数据或 A2 归档报告。
