## Why

现有 Go 测试和 Web 测试分别证明后端契约、应用层状态和组件行为，但没有一个跨真实浏览器、真实 HTTP、真实角色权限和真实依赖的联合验收矩阵。仅验证页面能打开或接口返回 200，会漏掉权限导航与 API 403 错位、响应信封解析失败、会话恢复误报、旧响应覆盖、文件资源失败和弱网下的错误提示。邮箱专项验收已经证明 Chrome DevTools MCP＋隔离栈＋curl／SQL／日志的证据方式可行，现在需要把这套方法扩展到整个工作台。

## What Changes

- 新增 `workbench-integration-acceptance` 能力，定义工作台前后端集成验收的环境、覆盖层、角色矩阵、故障范围、证据和通过标准。
- 新增 `matrix.md`：页面×接口×权限×期望的可执行矩阵，覆盖所有可见路由、前端引用的 API、代表性错误契约、四类角色／组织范围、离线／慢网、依赖故障、WebSocket 和代表页面可访问性／性能。
- 使用独立 MySQL、Redis、MinIO、Mailpit、RabbitMQ、Go 后端和 Vite；Chrome DevTools MCP 驱动真实浏览器，curl／SQL／Redis／日志做精确断言。
- 执行后新增 `docs/reviews/evidence/workbench-connectivity-YYYY-MM-DD.md`，记录实际状态码、网络失败、控制台错误、权限结果、数据库状态、截图、a11y／性能基线和清理结果。
- 非目标：不修改业务代码、路由、数据库 schema、权限码、菜单或前端页面；不新增 Playwright 或永久 E2E 框架；不把 94 个 Route Descriptor 全部做浏览器深测；不做全依赖混沌测试；不连接生产环境；不重复消息前端页面（当前无 UI）。

## Capabilities

### New Capabilities

- `workbench-integration-acceptance`: 工作台前后端在隔离环境下的真实浏览器集成验收边界、矩阵、角色权限、故障注入、WebSocket、可访问性、性能基线和证据要求。

### Modified Capabilities

无。本次不改变产品行为或接口契约。

## Impact

- 新增：`openspec/changes/verify-workbench-connectivity/matrix.md` 和 proposal／design／specs／tasks。
- 执行后新增：`docs/reviews/evidence/workbench-connectivity-YYYY-MM-DD.md` 与脱敏截图。
- 测试环境：Docker（MySQL 8.0、Redis 7、MinIO、Mailpit、RabbitMQ）、Go、Node／Vite、真实 Chromium。
- 不涉及业务代码、API、数据库迁移、Redis key、MinIO bucket 或权限码变更。
