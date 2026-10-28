## Context

后端 `harden-menu-api-management-contracts` 已完成菜单类型校验、菜单/API 绑定与清空、Route Catalog 状态、影响预览和 fingerprint 提交。正式前端目前有 `WorkbenchShell`、统一 `api.ts` 请求层、`models.ts` 数据模型、`management.css` 管理样式和现有角色/用户/组织页面，但没有菜单管理页或接口管理页。

本 Change 只补齐正式 React/Vite 工作台的 UI 和调用方，不重新定义后端路由、菜单、权限或 Route Catalog 事实。管理员页面必须使用真实接口、统一 JSON 错误信封和现有会话协调机制。

## Goals / Non-Goals

**Goals:**

- 在工作台新增受权限过滤的“配置管理”导航组和 `/menus`、`/apis` 路由。
- 提供可直接访问、可刷新恢复的菜单树管理和接口元数据管理页面。
- 将菜单类型边界、API 绑定/清空、RouteState、同步预览和 fingerprint 提交流程表达为清晰的 UI。
- 对高影响操作展示安全影响摘要，处理确认、陈旧冲突、未保存修改、403、加载、空数据和失败状态。
- 复用现有主题、shadcn/ui/Radix 组件、管理样式、认证请求层和可访问性约定。
- 通过组件/请求测试和真实浏览器验收证明桌面、窄屏、键盘和亮暗主题行为。

**Non-Goals:**

- 不新增后端 HTTP 路由、权限码、数据库表/列、Redis key、MinIO bucket 或 Route Catalog 来源。
- 不在前端创建第二套路由、权限或菜单事实。
- 不提供手工创建 API；API 新记录来自 Route Catalog 同步。
- 不编辑 API Method、Path 或 `NeedAuth`；这些字段在首版只读展示。
- 不实现新的角色授权模型、审批、草稿、发布、批量编辑或离线缓存。
- 不把 `localStorage`、`sessionStorage` 或其他 Web Storage 用于 Token、菜单、权限或 API 元数据持久化。
- 不在前端静默重放失败写请求，不把本地表单状态当作已保存事实。

## Decisions

### 1. 使用现有 typed request layer

所有页面数据类型放入 `web/src/lib/models.ts`，请求方法放入 `web/src/lib/api.ts`，页面组件不直接调用 `fetch`。统一复用现有认证 Token 注入、JSON 信封校验、字段错误解析、会话失效和网络错误映射。

**Alternative rejected:** 在页面组件中分别实现 `fetch` 和响应解析。这样会复制认证、错误和重试边界，并可能绕过当前会话协调。

### 2. 工作台导航增加独立配置管理分组

在 `workbench-navigation.ts` 增加“配置管理”分组，入口为 `/menus` 和 `/apis`。入口显示由 `admin.menus.get`、`admin.apis.get` 对应的真实用户上下文权限决定；写按钮再次按写权限控制。React Router 直接访问仍由后端 401/403 保证安全。

**Alternative rejected:** 将入口放入“身份与组织”或固定展示入口。菜单/API 管理属于配置域，固定入口会暴露不可用功能并与现有权限过滤约定冲突。

### 3. 菜单页采用树 + 详情编辑布局

桌面端使用左侧菜单树和右侧详情/编辑区；窄屏纵向堆叠。节点选择使用 URL `menu_id` 表达，刷新和直接访问可恢复对象。右侧表单根据 type 显示合法字段：目录无 path/component，页面要求 path，按钮无 path/component 且只允许合法父级。创建、修改、删除、绑定、清空和同步都先走适用的 preview，再提交 fingerprint。

**Alternative rejected:** 为每个菜单节点使用独立编辑页。独立页会丢失树上下文，并让父级、类型和绑定关系的确认成本更高。

### 4. 接口页采用服务端分页表格和详情抽屉

接口列表使用服务端分页和 query 筛选，桌面显示表格，窄屏显示卡片；详情和编辑使用抽屉/面板。Method、Path、NeedAuth、RouteState 和 API ID 只读；名称、分组、权限码、备注、排序、status、need_audit 可编辑。页面不提供手工创建 API，Route Catalog 同步是新增元数据的入口。

**Alternative rejected:** 前端一次加载全部 API 或允许直接编辑 Method/Path/NeedAuth。前者不适合规模增长，后者会把前端变成第二套路由/安全策略来源。

### 5. 影响预览由共享确认状态驱动

高影响操作统一经历 `idle -> previewing -> awaiting-confirmation -> submitting -> success/error/conflict` 状态。确认视图只显示受影响菜单/API/角色/用户数量、版本变化、同步新增/跳过/冲突和安全说明。`NAV_CONFLICT`、`API_META_CONFLICT` 进入冲突状态，保留用户草稿并要求重新读取/预览，禁止自动重放。

**Alternative rejected:** 保存后再弹出提示或使用乐观更新。后端 fingerprint 的价值是阻止覆盖竞争事实，乐观更新会把未提交事实误显示为已保存。

### 6. 绑定和同步使用专门工作流

按钮菜单详情显示已绑定 API 和候选 API 双区选择器；候选支持关键词、Method、Path、状态和认证筛选。显式清空使用 `clear=true`，不把空数组当解绑。菜单同步和 API 同步均展示新增、跳过、漂移和冲突差异；存在冲突时禁止提交。API 生成按钮从 API 详情发起，选择目录/页面父级后预览并提交。

**Alternative rejected:** 把 API 绑定、清空和同步塞入普通编辑表单的一次保存。这样难以表达授权影响，也容易把空集合误当作解绑。

### 7. 状态、权限和未保存保护优先于局部便利

页面为首次加载、空结果、403、网络错误、字段校验、预览冲突和提交失败分别提供可恢复状态。表单 dirty 时阻止切换对象、离开页面和刷新；保存失败保留输入。所有状态同时使用文字、语义颜色、图标和可访问名称，不只依赖颜色。

**Alternative rejected:** 只使用全局 Toast 和统一错误页。管理配置错误需要用户知道哪个对象、哪种事实和哪一步失败，Toast 不足以承载恢复路径。

### 8. 验证以真实 HTTP 和浏览器为准

组件/请求测试覆盖类型字段、URL 状态、错误映射、预览提交和冲突状态；浏览器验证覆盖真实登录上下文、菜单/API 页面、绑定/清空/同步、刷新保持、亮暗主题、375px/850px/1440px 视口和键盘焦点。

**Alternative rejected:** 只验证静态渲染或 mock 数据。此 Change 的风险集中在真实权限、fingerprint、错误信封和未保存状态边界。

## Risks / Trade-offs

- 菜单树和接口表的交互状态较多，组件需要避免把服务端事实、编辑草稿和预览候选混在同一对象中；代价是增加局部状态类型。
- 高影响操作多一次预览请求和确认步骤，降低了保存即时性，但避免覆盖竞争配置并符合后端安全契约。
- RouteState 可能因代码部署和元数据不同步而频繁变化，页面必须把诊断展示和同步操作分开，不能把漂移自动修复。
- 写权限隐藏依赖当前用户上下文，页面仍需正确处理后端 403，因为权限可能在页面打开后变化。
- 窄屏双栏布局会退化为纵向布局，菜单树与详情之间的切换需要保持 URL、焦点和未保存保护一致。
