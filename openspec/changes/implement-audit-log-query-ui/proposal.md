## Why

审计日志的五个查询接口已经存在，但工作台没有对应页面，管理员只能通过 Swagger 查询。与此同时，现有查询契约存在会直接导致错误结果或滥用的缺口：`start_time`／`end_time` 按 UTC 解析而数据按本地时间存储，筛选会整体偏移时区；`size` 没有上限；非法时间被静默忽略；无法按用户名筛选。现在补齐只读查询页面，并同步修正这些查询契约边界。

## What Changes

- 新增工作台“审计”分组与 `/audit-logs` 只读查询页面，按“全部／登录／操作／权限变更／数据访问”页签切换，复用现有五个查询接口。
- 页面可见性由五个审计读权限中的任意一个决定；页签按各自权限裁剪；“全部”不可用时默认落到第一个有权限的页签。
- 统一查询语义：默认最近 24 小时，预设 1 小时／24 小时／7 天／30 天／90 天，最大 90 天；每页 10／20／50，默认 20。
- 筛选项：时间范围、Method、精确状态码、路径包含、用户名包含、用户 ID 精确匹配。
- 后端新增 `username` 包含匹配筛选；`size` 上限固定为 50；`start_time`／`end_time` 非法或开始晚于结束时返回 422 `AUDIT_VALIDATION_INVALID`；时间解析改为本地时区，消除现有 UTC 偏移。**BREAKING**：原来发送非法时间并依赖静默忽略的调用方现在会收到 422。
- 列表展示时间、分类、用户、Method、Path、状态、耗时、客户端 IP；`query`／`body`／`metadata`／`user_agent` 只在详情 Sheet 中按纯文本或 JSON 展示，长字段折叠、可复制。
- 查询、分页和页签状态写入 URL，支持刷新、直达与浏览器前进后退恢复。
- 非目标：不新增详情接口；不查询冷表 `audit_log_archives`；不提供导出、删除、实时 tail、跨日志关联或状态码范围筛选；不新增后端路由、权限码、数据库表／列、Redis key 或 MinIO bucket；不排除审计查询自身产生的 `data_access` 记录。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `logging`: 审计日志查询的筛选能力、分页上限、时间解析与校验行为发生变化。
- `admin-workbench`: 新增审计日志查询页面、导航分组、页签权限裁剪、URL 状态和详情展示要求。

## Impact

- 后端：`internal/audit/dto.go`、`internal/audit/service.go`、`internal/audit/adapters/gorm/repository.go` 及对应测试；不新增路由、权限码或数据库结构。
- 前端：`web/src/lib/models.ts`、`web/src/lib/api.ts`、新增审计查询 URL 状态模块与页面组件、`web/src/pages/ManagementPages.tsx`、`web/src/lib/navigation.ts`、`web/src/lib/workbench-navigation.ts`、`web/src/management.css` 及测试。
- 规格：`openspec/specs/logging/spec.md`、`openspec/specs/admin-workbench/spec.md`。
- 数据与基础设施：无迁移、无 Redis／MinIO／RabbitMQ 变更；冷热归档策略不变，页面只查询热表。
