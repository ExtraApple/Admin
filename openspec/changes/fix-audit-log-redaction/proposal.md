## Why

A2 真实审计发现，消息请求审计 Body 仍保留标题/Markdown 等正文片段，文件下载审计 Query 仍保留 `signature` 和 `expires` 查询键。当前通用 denylist 无法保证新增消息字段和签名参数不会进入 Audit Log，需要改为路径感知 Body 省略和显式 Query allowlist。

## What Changes

- 新增路径感知的审计 Body 清洗入口。
- Messaging 私信、群发、公告、编辑、发布和分类操作统一记录固定 `[message omitted]`，不保存标题、Markdown、HTML、图片引用或正文。
- multipart 请求继续使用固定 `[multipart omitted]`。
- 将审计 Query 改为显式 allowlist，仅保留既有审计查询所需的分页、操作者、方法、路径、状态、分类和时间范围字段。
- 永不保存 `signature`、`expires`、ticket、Token、Authorization 或未批准查询键。
- 保留现有上传安全 metadata allowlist和历史 Audit Log，不在本 change 删除或回写历史记录。
- 增加 sanitizer 单测和 HTTP middleware 审计证据测试。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `logging`: 明确消息审计 Body 使用固定省略标记，审计 Query 使用显式 allowlist，签名下载参数不得持久化。

## Impact

- 代码：`internal/audit/service.go`、`internal/audit/adapters/http/routes.go` 及审计测试。
- 规格 delta：仅记录已发布日志脱敏契约的实现修复；归档时使用 `--skip-specs`，不同步主规格。
- 数据：历史 `audit_logs` 不迁移、不删除；历史清理另建独立 change。
- 运行时：新审计记录不再保存消息正文或签名查询，上传安全 metadata 行为保持不变。
- 非目标：不增加组织字段、不实现审计组织范围过滤、不改变审计查询路由/权限、不改变日志事件名或 RuntimeLogger 白名单。
