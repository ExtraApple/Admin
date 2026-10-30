## Context

审计模块已有五个热表查询接口（`/api/admin/audit-logs` 及 login／operation／permission／data-access 变体）和对应读权限，但正式工作台没有页面，管理员只能通过 Swagger 查询。现有查询契约还有四个边界问题：`start_time`／`end_time` 用 `time.Parse` 按 UTC 解析，而记录时间来自 `time.Now()`、MySQL DSN 为 `loc=Local`，本地时区筛选会偏移；`size` 没有上限；非法时间被静默忽略；没有用户名筛选。冷表 `audit_log_archives` 只有归档写入，没有查询接口。

前端已有统一的 `api.ts` 请求层、`management-query.ts` URL 状态模式、`Pagination`／`ResourceState`／`ErrorNotice` 等管理组件，以及按权限裁剪的工作台导航。本次沿用这些模式，不引入新的状态库或 UI 依赖。

## Goals / Non-Goals

**Goals:**

- 新增 `/audit-logs` 只读查询页面，按权限裁剪“全部／登录／操作／权限变更／数据访问”页签。
- 统一筛选项、时间窗口、分页和 URL 恢复语义。
- 修正后端时间解析、`size` 上限、非法时间校验，并增加 `username` 包含匹配。
- 在详情 Sheet 中安全展示 `query`／`body`／`metadata`／`user_agent`。
- 用单元、组件和真实浏览器验收覆盖权限、边界和响应式行为。

**Non-Goals:**

- 不新增详情接口、后端路由、权限码、数据库表／列、Redis key 或 MinIO bucket。
- 不查询冷表，不提供导出、删除、实时 tail、跨日志关联或状态码范围筛选。
- 不修改归档策略、审计记录范围或脱敏规则。
- 不新增菜单种子；页面可见性由权限码决定。

## Decisions

### 1. 单页 + 页签，直接映射五个现有接口

页面固定为 `/audit-logs`，页签分别调用对应接口：全部 → `/api/admin/audit-logs`，登录 → `/login-logs`，操作 → `/operation-logs`，权限变更 → `/permission-logs`，数据访问 → `/data-access-logs`。不使用通用接口传 `category` 来模拟所有页签，因为专用接口的权限码和类别约束是既有契约；操作页签按规格同时返回 `operation` 与 `permission`。

**Alternative rejected:** 五个独立路由。列、筛选、分页和详情完全一致，拆分会复制列表逻辑；也会让页签权限裁剪和 URL 状态分散到五份代码。

### 2. 可见性由权限码决定，不依赖菜单记录

新增 `canAccessAuditLogs(context)`：五个审计读权限任一存在即可显示“审计”分组和 `/audit-logs`。页签再按各自权限裁剪；“全部”不可用时回退到第一个有权限的页签。现有 `hasPermission` 对 `admin` 角色的全权限语义保持不变。

**Alternative rejected:** 只用种子菜单 `/system/audit-logs`（`admin.audit-logs.get`）放行。这样只有登录日志权限的角色即使能调用接口也看不到页面，与五个独立读权限的模型冲突。

**Alternative rejected:** 为五个日志类型各补一条菜单种子。会改变菜单事实和同步目录，超出本次范围；权限码本身已足够表达可见范围。

### 3. 查询规范化放在应用层，仓储只消费已解析边界

在 `audit` 包增加 `NormalizeListRequest`：

- 调用 `NormalizePage`，保留默认 `page=1`、`size=10`，并把 `size` 上限固定为 50。
- 用 `time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)` 解析非空 `start_time`／`end_time`，写入请求的非序列化字段 `StartAt`／`EndAt`。
- 任一非空时间无法解析，或 `StartAt` 晚于 `EndAt`，返回 `NewError(CodeValidationInvalid, ...)`。
- HTTP handler 继续先调用 `NormalizePage` 以便响应回显裁剪后的 `page`／`size`；`QueryService.list` 调用 `NormalizeListRequest` 完成时间校验。

`AuditLogListRequest` 增加 `Username string`（`form:"username"`）以及 `StartAt`／`EndAt time.Time`（`form:"-"`、`json:"-"`）。GORM 仓储改用 `StartAt`／`EndAt` 过滤，不再自行解析字符串，避免应用层与适配层使用不同时区。

**Alternative rejected:** 继续在仓储里 `time.Parse`。适配层会继续把本地时间当 UTC，且校验错误要跨层伪造；也不利于直接用单元测试覆盖时间语义。

**Alternative rejected:** 全链路改用 UTC 存储。需要迁移历史数据并改变 `created_at` 的展示语义，风险远大于修正解析位置。

### 4. 筛选语义保持窄而明确

- `username`、`path`：包含匹配（`LIKE %value%`）。
- `user_id`、`method`、`status`、`category`：精确匹配，沿用现有条件。
- `status` 为精确 HTTP 状态码，不提供 2xx／4xx 范围；范围筛选需要新的后端契约，本期不做。
- 前端不单独暴露 `category` 筛选，页签承担类别选择；“全部”页签仍可能包含 `api` 类别。
- `username` 加入 `auditQueryAllowlist`，使审计查询请求自身的审计记录保留该筛选参数；不扩大敏感字段范围。

**Alternative rejected:** 在页面层用 `user_id` 代替用户名筛选。审计的首要问题是“谁”，强制管理员先查用户 ID 不可用。

### 5. 时间窗口和分页由前端明确发送

前端默认查询最近 24 小时，预设 1 小时／24 小时／7 天／30 天／90 天只负责写入绝对起止时间；URL 保存绝对时间，刷新不漂移。单次范围超过 90 天、开始晚于结束或格式非法时，页面显示字段错误且不发起请求。每页 10／20／50，默认 20；翻页和每页条数都写入 URL。

后端仍保留 `size` 默认 10（既有 API 行为），只增加 50 上限；页面显式发送 20。这样不改变既有调用方的默认响应。

**Alternative rejected:** URL 保存“最近 24 小时”这类相对预设。刷新或稍后打开会把窗口滚动到新的当前时间，审计复现不一致。

### 6. 详情复用列表载荷，右侧 Sheet 展示

列表响应已包含 `query`、`body`（写入时脱敏，最长 2000 字符）、`metadata`、`user_agent`，详情不再发请求。使用现有 Radix `Sheet`，内容按纯文本和格式化 JSON 渲染；长字段默认折叠，提供展开和复制；不执行 HTML，不展示未脱敏原文。列表列保持为时间、分类、用户、Method、Path、状态、耗时、客户端 IP。

**Alternative rejected:** 新增 `GET /api/admin/audit-logs/:id`。会重复同一份数据并扩大后端契约；如未来列表体积成为问题，再单独引入详情接口。

### 7. 前端状态集中在专用 URL 模块和单页面组件

新增 `audit-query.ts` 负责解析／编码 `type`、`page`、`size`、`start`、`end`、`method`、`status`、`path`、`username`、`user_id`，并提供 90 天和起止顺序校验。页面组件复用 `Pagination`、`ResourceState`、`ErrorNotice`、`Notice`、`PageHeading` 和 `Sheet`；桌面表格、窄屏卡片；状态颜色与文字并存。导航新增 `audit` 分组；路由在 `ManagementPages.tsx` 注册，使用自定义审计守卫而不是 `Guard module`（没有菜单模块可匹配）。

**Alternative rejected:** 复用 `management-query.ts` 的 API 查询类型。两者筛选项和校验规则不同，硬合并会让 API 管理状态多出无关字段。

### 8. 验收以真实接口和浏览器为准

后端补 `NormalizeListRequest`、`username` 过滤、`size` 上限和时间校验的单元／HTTP 测试；前端补 URL 状态、请求层和组件测试；最后用真实 MySQL／Redis 后端和浏览器验证权限裁剪、时间边界、URL 恢复、详情、空／错／403、375／850／1440 宽度和亮暗主题。

## Risks / Trade-offs

- [非法时间从“静默忽略”变为 422] → 这是有意的契约修正；在 proposal 中标记为 **BREAKING**，并在设计说明中记录迁移影响；前端不会发送非法值。
- [`username`／`path` 的包含匹配使用前导通配符，无法利用普通索引] → 以默认 24 小时窗口、90 天上限和时间索引约束扫描范围；本期不增加全文索引或前缀索引。
- [时间语义假设浏览器与服务器时区一致] → 页面明确使用服务器本地时间语义；跨时区部署需要后续新增时区参数，本期不做。
- [列表携带 `body`／`metadata`，响应体积随页大小增长] → `size` 上限 50、`body` 写入时截断到 2000 字符；详情复用列表数据，避免额外请求。
- [审计查询自身会写入 `data_access`，造成噪音] → 本期明确不排除，保留审计完整性；如后续需要，单独讨论排除规则。
- [权限在页面打开后变化] → 页签不永久缓存权限结果；请求返回 403 时保留筛选并显示权限说明，下一次上下文刷新后再裁剪。
- [操作页签包含权限变更日志] → 这是现行规格的行为；页签说明中明确“操作包含权限变更”，避免用户误以为重复。
- [热表边界] → 页面提示“不包含已归档数据”；冷表查询另立 change。
- [MySQL 排序规则决定 `username`／`path` 包含匹配的大小写行为] → 验收以当前部署排序规则为准，不在应用层强行统一大小写。

## Migration Plan

1. 先部署后端：新增 `username` 参数、`size` 上限和本地时间解析。除非法时间改为 422 外，既有合法请求行为不变。
2. 再发布前端：新增 `/audit-logs` 路由、导航和页面；未授予审计权限的用户看不到入口。
3. 无数据库迁移、无菜单种子变更、无权限码变更；回滚只需回退前端发布，后端新增参数可保持兼容。
4. 部署后用一个仅持有单一审计读权限的账号验证页签裁剪；用管理员验证五个页签、时间边界和详情。

## Open Questions

无。跨时区部署的时区参数、冷表查询和导出均为后续独立变更。
