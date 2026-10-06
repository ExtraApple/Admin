## Context

当前 `web/src/components/WorkbenchShell.tsx` 使用自定义 `<aside>`、React Router `NavLink`、自定义 breadcrumb 和 CSS class 组织工作台壳层。权限入口由 `canAccessAuthorizationOverview`、`canAccessAuthorizationRisks` 和 `canAccessModule` 过滤；路由切换后通过 `main` ref 将焦点移到主要内容。`web/src/styles.css` 负责 `.app-shell`、`.sidebar`、`.topbar` 和窄屏换行导航，现有 UI 目录还没有 Sidebar、Sheet、Tooltip、Separator、Collapsible 或 Breadcrumb 组件。

本变更只处理已认证工作台的导航表现层。后端菜单树仍是权限事实来源，但展示层分组不写回菜单模型，也不新增权限码、路由或 API。现有冷白／深海青主题、中文字体、228px 内容布局和 850px 响应式断点是视觉兼容约束。

## Goals / Non-Goals

**Goals:**

- 使用可维护的 shadcn/ui + Radix 本地组件源码替换自定义工作台导航壳层。
- 将可访问入口稳定映射到“授权核对”“身份与组织”“账户”三个展示层导航分组。
- 在桌面端保持 228px Sidebar；在 850px 及以下视口使用移动 Sheet。
- 让分组展开状态在当前 `WorkbenchShell` 挂载周期内由用户控制，不写入浏览器持久化存储。
- 为 Sheet 提供关闭、路由切换和焦点恢复语义。
- 使用官方 Breadcrumb 组合统一两级“工作台 / 当前页面”路径。
- 保留现有权限过滤、React Router 导航、active 状态、身份头像、退出和页面主焦点行为。
- 删除旧壳层 CSS，避免两套导航实现并存。

**Non-Goals:**

- 不修改后端菜单树、菜单权限码、API、数据模型、Redis、MinIO 或数据库。
- 不改变任何管理页面、授权风险计算、DataTable、表单、详情页或写入流程。
- 不把展示层导航分组持久化到 `localStorage`、`sessionStorage` 或服务端。
- 不提供 Sidebar 整体折叠为图标栏；本批次只折叠导航分组。
- 不把详情对象名称加入 Breadcrumb；详情页仍使用两级路径和页面自身标题／返回入口。
- 不引入新的表单库、状态管理库或后台模板。

## Decisions

### 1. 采用本地 shadcn/ui + Radix 组件源码

在 `web/src/components/ui/` 增加官方结构的 Sidebar、Sheet、Tooltip、Separator、Collapsible 和 Breadcrumb 组件，使用现有 `cn`、Lucide 和 Tailwind 主题变量。按需补充对应 Radix primitives 依赖：Dialog、Tooltip、Separator 和 Collapsible；复用已存在的 Slot、Button、Input、Skeleton 等依赖。

选择本地源码而不是直接引入成套后台模板，是因为项目已经采用 shadcn/ui 的可定制路线，且业务权限、路由和主题需要保留。选择官方 Radix primitives 而不是手写 Sheet/焦点行为，是为了复用可访问的 modal、dismiss、focus scope 和 tooltip 语义。

### 2. Sidebar 不做全局图标折叠，分组独立折叠

WorkbenchShell 使用 228px 的 Sidebar，并禁用整体 icon collapse。导航内容使用 SidebarGroup、SidebarMenu 和 Collapsible 组合；每个分组有稳定 ID、标题、图标和经过权限过滤的入口。

三组固定为：

- `authorization-review`：授权总览、风险清单。
- `identity-organization`：角色、用户、组织。
- `account`：个人资料。

入口配置保留现有权限函数和 React Router `to` 地址。展示分组是前端壳层概念，不读取或修改后端 Menu 树。

### 3. 分组状态使用挂载周期内 React state

WorkbenchShell 初始化时根据当前 pathname 计算当前分组，将该分组设为展开，其他分组设为折叠。此后分组状态完全由用户控制；用户可以折叠当前分组，路由切换不自动展开或折叠任何分组。没有可见入口的分组不渲染。

不使用 Cookie、localStorage 或 sessionStorage，避免引入 UI 偏好持久化、旧版本 key 清理和会话生命周期耦合。桌面 Sidebar 与移动 Sheet 使用同一份 `openGroups` 状态。

### 4. 移动端使用 Sheet，不复制导航数据

850px 及以下由 Sidebar 的移动变体渲染 Sheet。顶部显示菜单触发按钮、两级 Breadcrumb、用户身份和退出按钮；Sheet 只渲染同一份权限过滤后的导航分组，不复制另一套菜单配置。

打开 Sheet 时使用 Radix Dialog 的焦点管理；点击入口后先完成 React Router 导航，再关闭 Sheet，并将焦点恢复到触发按钮。Escape、遮罩点击和关闭按钮均可退出 Sheet。退出登录保持在顶部独立操作区域，不混入导航分组。

### 5. Breadcrumb 只做两级路径表达

使用本地 shadcn Breadcrumb 组件，将现有自定义 breadcrumb 替换为 React Router `Link` 与当前页面 `BreadcrumbPage`。第一项固定为“工作台”，第二项沿用当前壳层的 route-to-label 映射。对象详情、编辑维度和返回上下文仍由页面标题与 `PageHeading` 负责，不在本批次扩展路径模型。

### 6. 保持现有主题与壳层尺寸

官方组件的结构和交互适配现有语义 token：冷白表面、深海青主色、`--line` 边界、Noto 字体和现有 focus ring。Sidebar 宽度固定为 228px，移动断点保持 850px。删除旧 `.sidebar`、`.nav-button`、`.nav-group` 及移动端换行导航规则；保留仍被其他页面使用的通用 token、按钮、Card 和内容区样式。

### 7. 验证以用户可见行为为主

实现后在 375px、850px、1440px 视口检查布局；用键盘验证菜单按钮、分组按钮、导航入口、Sheet 关闭和焦点恢复；使用至少一个权限受限上下文确认空分组隐藏且没有出现越权入口。现有页面 API 行为不在此变更中重新实现，只验证壳层没有绕过权限或改变路由。

## Risks / Trade-offs

- **[中] 官方组件源码与现有 Tailwind/CSS token 可能冲突。** 通过沿用现有语义变量、限定壳层选择器并删除旧导航规则降低风险；不直接复制官方默认颜色。
- **[中] 新增 Radix primitives 会增加 package-lock 变更。** 只引入 Sidebar 必需依赖，完成后运行前端类型检查、测试和生产构建。
- **[中] 当前分组允许折叠，可能暂时隐藏当前 active item。** 这是已确认的用户控制取舍；Breadcrumb、页面标题、`aria-current` 和内容焦点仍提供当前页面定位。
- **[中] 850px 需要 CSS 与移动 Sheet 判定一致。** 使用共享断点常量或同一 850px 约定，避免 CSS 已进入移动布局但 JS 仍渲染桌面导航。
- **[低] 静态展示分组可能落后于未来权限入口。** 新增入口时必须在同一导航配置中补充分组、标签、图标和权限谓词；不自动从后端菜单树推断展示分组，以避免改变本变更的权限语义。
