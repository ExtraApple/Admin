## 1. 后端查询契约

- [x] 1.1 在 `internal/audit/dto.go` 为 `AuditLogListRequest` 增加 `Username string`（`form:"username"`）和 `StartAt`／`EndAt time.Time`（`form:"-"`、`json:"-"`），保持现有请求字段兼容。
- [x] 1.2 在 `internal/audit/service.go` 增加 `NormalizeListRequest`：调用 `NormalizePage`，用 `time.ParseInLocation(..., time.Local)` 解析非空 `start_time`／`end_time`，写入 `StartAt`／`EndAt`；非法格式或开始晚于结束时返回 `CodeValidationInvalid`；把 `NormalizePage` 的 `size` 上限固定为 50。
- [x] 1.3 在 `internal/audit/service.go` 的查询参数白名单加入 `username`，确保审计查询自身的审计记录保留该筛选参数。
- [x] 1.4 在 `internal/audit/adapters/gorm/repository.go` 增加 `username LIKE ?` 过滤，并改用 `StartAt`／`EndAt` 比较 `created_at`，移除仓储内的字符串时间解析。
- [x] 1.5 确认 `internal/audit/adapters/http/routes.go` 的 list handler 继续用 `NormalizePage` 回显裁剪后的 `page`／`size`，并让 `CodeValidationInvalid` 映射为 422；必要时补充错误分支。
- [x] 1.6 增加 Go 测试：本地时区解析不偏移、`size` 超过 50 被截断、非法／倒序时间返回 422、`username` 包含匹配生效；运行 `go test ./...` 验证。

## 2. 前端数据层

- [x] 2.1 在 `web/src/lib/models.ts` 增加审计分类、日志记录、列表响应和查询状态类型。
- [x] 2.2 在 `web/src/lib/api.ts` 增加五个审计列表请求方法，复用现有认证、JSON 信封、字段错误和网络错误映射。
- [x] 2.3 新增 `web/src/lib/audit-query.ts`，实现 URL 状态解析／编码、默认最近 24 小时、预设时间窗、90 天上限和起止顺序校验。
- [x] 2.4 增加 `audit-query` 与 API 请求层测试，覆盖 URL 往返、默认窗口、非法范围、分页参数和 422 映射。

## 3. 页面、导航与交互

- [x] 3.1 在 `web/src/lib/navigation.ts` 增加 `canAccessAuditLogs`（五个读权限任一）和页签权限映射，不依赖菜单记录。
- [x] 3.2 在 `web/src/lib/workbench-navigation.ts` 增加“审计”分组和“审计日志”入口，更新分组 id、类型和初始化状态。
- [x] 3.3 在 `web/src/pages/ManagementPages.tsx` 注册 `/audit-logs` 路由，使用审计专用权限守卫而不是 `Guard module`。
- [x] 3.4 新增 `web/src/pages/AuditLogPages.tsx`：页签、筛选、服务端分页、桌面表格和窄屏卡片，复用 `Pagination`／`ResourceState`／`ErrorNotice`／`Notice`。
- [x] 3.5 实现日志详情 Sheet：展示 `query`／`body`／`metadata`／`user_agent`，纯文本或格式化 JSON、长字段折叠、复制，不执行 HTML。
- [x] 3.6 在 `web/src/management.css` 增加审计页签、筛选、HTTP 状态、详情和响应式样式，复用现有管理页面视觉与状态语义。
- [x] 3.7 增加组件测试：页签权限裁剪、默认页签、URL 恢复、筛选与分页、详情折叠、空／加载／失败／403。

## 4. 验证与交付

- [x] 4.1 运行 `cd web && npm run typecheck && npm test -- --run && npm run build`，并运行 `go test ./...`。
- [x] 4.2 使用真实后端和浏览器验收：五个页签、时间预设与自定义范围、用户名／路径／Method／状态筛选、分页、详情、刷新与前进后退。
- [x] 4.3 使用仅持有单一审计读权限的账号验证页签裁剪和 403 处理；检查 375／850／1440 宽度、亮暗主题、键盘关闭 Sheet 并归还焦点。
- [x] 4.4 将 `TODO.md` 中“审计日志查询”更新为 `[x]`，记录验证命令和结果。
- [x] 4.5 按 OpenSpec 流程把 delta 规格同步到 `openspec/specs/logging/spec.md` 和 `openspec/specs/admin-workbench/spec.md`，验收完成后归档 change。
