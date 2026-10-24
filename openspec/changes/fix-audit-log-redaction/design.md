## Context

A2 真实审计发现，通用 `SanitizeBody` 只遮蔽已知敏感字段，消息 `title` 等字段仍可进入 Audit Log Body；`SanitizeQuery` 采用 denylist，文件下载的 `signature` 和 `expires` 因未列入 denylist 而被保存。

当前规格已经禁止消息正文、签名 URL、Token、对象路径和基础设施原始错误进入审计数据。本 change 只使实现回到该契约，不处理历史记录。

## Goals / Non-Goals

**Goals:**

- 为消息路径提供固定 `[message omitted]` Body。
- 为 Query 建立显式 allowlist。
- 永不保存 `signature`、`expires`、ticket、Token、Authorization 或未批准参数。
- 保持 multipart `[multipart omitted]` 和上传安全 metadata allowlist。
- 用 sanitizer unit test 与 HTTP middleware test 验证最终持久化形状。

**Non-Goals:**

- 不删除、迁移或回写历史 `audit_logs`/`audit_log_archives`。
- 不改变审计查询路由、权限、分页、组织范围或模型。
- 不改变 RuntimeLogger 事件名、字段白名单或 UploadAuditMetadata。
- 不增加组织字段或第三方日志依赖。

## Decisions

### 1. Body 采用路径感知固定省略

新增 `SanitizeBodyForPath(method, path, body)`（名称可按现有命名调整）。对于私信、群发、公告、编辑、发布、分类和消息图片相关 JSON 请求，直接返回 `[message omitted]`；不解析、不保存 title、Markdown、HTML、图片 ID 或正文。

其他 JSON 请求继续使用现有敏感字段递归遮蔽。multipart 继续由既有逻辑返回 `[multipart omitted]`。

备选方案是扩展敏感字段 denylist，将 `title` 加入遮蔽；这仍会让未预见的消息字段泄露，因此不采用。

### 2. Query 使用显式 allowlist

`SanitizeQuery` 只保留 `page`、`size`、`user_id`、`method`、`path`、`status`、`category`、`start_time`、`end_time`。所有其他键丢弃，尤其是 `signature`、`expires`、ticket、Token 和 Authorization 类参数。

文件下载即使带签名查询，也只留下空 Query；不保存签名存在性或值。

### 3. 历史记录不自动清理

修复只影响新写入 Audit Log。历史正文和签名记录保留，后续数据清理/保留策略另建 change，避免在请求路径执行不可逆删除。

## Risks / Trade-offs

- 消息审计无法再通过 Body 复核标题或正文，但稳定 action/resource_id metadata 仍保留，符合最小审计原则。
- 新 Query allowlist 可能丢弃尚未登记的非敏感筛选键；新增筛选必须显式更新 allowlist 和测试。
- 审计数据历史清理仍未完成，部署后需要独立评估历史记录保留策略。
