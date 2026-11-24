## Context

本次变更面向 `web/` 中的 React 19 + TypeScript + Vite + Tailwind CSS + Radix/shadcn/ui 正式管理工作台，不涉及 Go 后端 `internal/` 业务模块、路由注册或数据库结构。

当前代码现状：
- **导航与壳层**：[WorkbenchShell.tsx](web/src/components/WorkbenchShell.tsx) 渲染固定 `228px` 侧边栏（仅移动端有抽屉触发器，桌面端不可收起）、底部重复展示当前用户名的 `SidebarFooter`、以及单行非吸顶 `.topbar`（仅展示 `工作台 / 一级名称` 两级面包屑）。[workbench-navigation.ts](web/src/lib/workbench-navigation.ts) 的 `getInitialOpenGroups` 在首次挂载时只展开当前路由所属分组，其余分组默认折叠。
- **路由与编辑保护**：[App.tsx](web/src/App.tsx) 与 [ManagementPages.tsx](web/src/pages/ManagementPages.tsx) 以 URL 作为列表筛选、分页、审计查询及详情/编辑页的唯一事实来源；所有编辑表单通过 [shared.tsx](web/src/components/management/shared.tsx) 中的 `UnsavedGuard`（React Router `useBlocker`）阻止未保存离开；会话登出或切账号通过 `WorkbenchShell` 中 `<SidebarProvider key={sessionId}>` 重建整棵壳层子树。
- **排版与信息密度**：[styles.css](web/src/styles.css) 与 [management.css](web/src/management.css) 使用大号衬线 `h1`（最高 `38–39px`）、较大顶部边距（`.content` 最高 `56px`）、装饰性英文 eyebrow 前缀、全站桌面端 `min-height: 44px` 控件与强制 `88px`/`100px` 表格行高；[shared.tsx](web/src/components/management/shared.tsx) 在路由切换时 `.focus()` 到 `h1[tabIndex=-1]` 触发全局 `:focus-visible` 青色粗框；`.ui-badge` 缺少 `white-space: nowrap` 导致部分计数徽章折行。
- **现有依赖**：`web/package.json` 已内置 `@radix-ui/react-dropdown-menu`、`@radix-ui/react-collapsible`、`@radix-ui/react-dialog`、`@radix-ui/react-alert-dialog`、`lucide-react`，未安装 `@radix-ui/react-context-menu`。

## Goals / Non-Goals

**Goals:**
- 在桌面端（`>850px`）落地双层吸顶顶栏（上层工具栏 + 下层纯路由已打开页面栏），按一级模块去重并记住各模块最近完整 URL，同时保持与侧边栏导航的清晰分工。
- 保障页签关闭、中键关闭、单页签右键菜单与最右侧纯图标下拉菜单（含固定/取消固定与批量关闭）均严格遵守 `UnsavedGuard` 未保存保护、单页签兜底规则、已固定页签保护与权限收敛，且不引入 Keep-Alive 或 Web Storage 持久化。
- 升级侧边栏为默认全展开可见分组、桌面端可完整收起/展开、移除底部重复账号块，并将顶栏面包屑升级为静态完整层级（带 `#ID` 且中间父级可点击）。
- 在保留现有冷白／深海青亮暗色系与移动端 `44px` 触控标准前提下，优化工作台主界面无衬线页头、消除程序聚焦刺眼外框、提升桌面端控件与表格信息密度并修复徽章折行。

**Non-Goals:**
- 不实现页面组件内存 Keep-Alive 保活、表单草稿跨页签缓存或滚动位置缓存。
- 不按单个对象 ID 拆分多个详情/编辑页签，不设默认强制钉住的首页签（仅支持用户会话内手动固定/取消固定），不提供页签内强制刷新按钮。
- 不将已打开页签、固定状态、侧边栏收起状态或分组展开状态写入 `localStorage`/`sessionStorage` 或服务端。
- 不修改登录/注册页品牌样式，不更换主题色板或组件库，不新增第三方 npm 依赖，不修改后端接口、权限码、模型、Redis key 或 MinIO bucket。

## Decisions

### 1. 在 `workbench-navigation.ts` 集中维护路由元数据、全展开初始分组与静态层级面包屑

**方案**：
- 将侧边栏可见分组、一级模块页签定义（`key`、`label`、`rootPath`、`canAccess`）及静态层级面包屑解析集中定义在 `web/src/lib/workbench-navigation.ts`，作为壳层导航的单一事实来源。
- `getInitialOpenGroups(groups)` 改为将 `groups` 中每个可见分组均初始化为 `true`（不可见分组仍保持 `false`）；`useWorkbenchNavigation` 维持“首次挂载初始化、路由切换不覆盖用户手动折叠状态”的语义不变。
- 新增 `getWorkbenchBreadcrumbs(pathname): WorkbenchBreadcrumbItem[]`（包含 `{ label: string; to?: string }`）：
  - 根节点始终为 `{ label: '工作台', to: '/' }`。
  - 按精确路由模式静态匹配全部管理子路由与独立编辑页：
    - `/roles/:id` → `角色管理 (/roles)` / `角色详情 #:id`
    - `/roles/:id/(permissions|menus|data-scope)` → `角色管理 (/roles)` / `角色详情 #:id (/roles/:id)` / `编辑权限|编辑菜单|编辑数据范围`
    - `/users/:id` → `用户管理 (/users)` / `用户详情 #:id`
    - `/users/:id/(roles|organizations)` → `用户管理 (/users)` / `用户详情 #:id (/users/:id)` / `编辑角色|编辑组织`
    - `/organizations/:id` → `组织管理 (/organizations)` / `组织详情 #:id`
    - `/organizations/:id/move` → `组织管理 (/organizations)` / `组织详情 #:id (/organizations/:id)` / `调整上级`
  - 其余一级页（授权总览、风险清单、菜单管理、接口管理、审计日志、个人资料、访问说明）及 404 返回两级静态路径。

**理由与替代方案**：静态路由层级零闪烁、零跨页异步状态耦合，且实体全称已在详情/编辑页主内容中清晰展示；未采用详情页异步加载后再回填顶栏名称的方案，避免切换瞬间文字跳动与卸载清理复杂度。

### 2. 纯函数驱动的内存页签状态机与“先导航、提交后再移除”的 `UnsavedGuard` 协作机制

**方案**：
- 新增 `web/src/lib/workbench-tabs.ts`，封装纯函数状态流转：
  - `resolveWorkbenchTab(pathname, search, context)`：按一级模块归并（9 个业务/个人资料模块 + 兜底 `no-management`），默认 `pinned: false`；`/`、`/login`、`/register`、`/logout`、404（`*`）及当前 `context` 无权限访问的路径均返回 `null`。
  - `syncVisitedTab(tabs, visited)`：若模块页签已存在则原位更新 `url` 为 `${pathname}${search}` 并保留 `pinned` 状态；若不存在则以 `pinned: false` 追加到末尾。当进入任意正常模块时自动移除兜底的 `no-management` 页签；`no-management` 仅在无其他可访问页签时作为唯一项存在。
  - `canPinTab(tab)` 与 `togglePinTab(tabs, targetKey)`：仅业务模块与 `profile` 允许固定（排除 `no-management`）。固定／取消固定为纯内存操作，不触发路由导航，严格维持 `[...pinnedTabs, ...unpinnedTabs]` 排序不变量：固定时移至左侧已固定区末尾（保持已固定页签各自的固定先后顺序）；取消固定时移至剩余已固定页签之后（未固定区首位）。
  - `pruneUnauthorizedTabs(tabs, context)`：当 `context` 更新（如重新读取授权或权限变化）时过滤掉失去访问权限的模块页签（包括已固定页签）。
  - `canCloseTab(tabs, targetKey?)`、`getBatchCloseState(tabs, anchorKey)`、`planCloseTab(tabs, activeKey, targetKey)` 与 `planBatchCloseTabs(tabs, activeKey, anchorKey, mode: 'others' | 'right')`：
    - 仅剩 1 个页签或目标页签为已固定态时不可关闭（返回无操作）。
    - 批量关闭自动跳过所有已固定页签；若目标范围不存在可关闭的未固定页签，对应批量关闭项禁用。
    - 关闭非当前页签或批量关闭不含当前激活页签（包括当前激活页签本身已固定而被跳过的情形）时，立即更新内存数组，不触发路由导航。
    - 关闭当前激活页签时，目标切换页签优先取右侧相邻，无右侧则取左侧相邻；批量关闭（右键非激活页签执行「关闭其他」或关闭包含激活页签的「关闭右侧」）目标页签为该操作基准页签。
- **未保存修改保护不变量**：凡需离开当前页签的关闭操作，不预先从状态数组删除当前页签，而是记录 `pendingRemoval = { removeKeys: WorkbenchTabKey[]; expectedTargetKey: WorkbenchTabKey }` 并调用 `navigate(targetTab.url)`：
  - 若无拦截或用户在 `UnsavedGuard` 弹窗确认“放弃修改并离开”，路由提交后 `location.key` 变化且新路由匹配 `expectedTargetKey`，状态机在同步新路由时一并移除 `removeKeys` 并清空 `pendingRemoval`。
  - 若用户在 `UnsavedGuard` 选择取消停留，路由未提交，原页签与当前编辑态完整保留；后续任何主动点击页签或侧边栏的导航会清空未兑现的 `pendingRemoval`，避免延迟误删。
- 页签状态挂载在 `WorkbenchLayout` 内，由 `<SidebarProvider key={sessionId}>` 自动保证登出或切换账号时清空内存，不读写任何 Web Storage。

**理由与替代方案**：相比提前 `setTabs` 删除再在取消时回滚，`useBlocker` 位于子页面组件内无法向父壳层同步“取消”回调，因此“先导航、提交后才移除”是与 React Router `useBlocker` 唯一无竞态、无跨层耦合的安全方案。

### 3. 零新增依赖实现页签栏与单页签右键菜单（复用现有 `DropdownMenu`）

**方案**：
- 新增 `web/src/components/WorkbenchTabBar.tsx`，在 `WorkbenchShell` 中紧贴 `.topbar` 下方渲染，桌面端（`>850px`）显示，窄屏（`≤850px`）通过 `useIsMobile()` 与 CSS 双重隐藏（页签状态仍在 `WorkbenchLayout` 中持续累计，视口切回桌面端即无缝呈现）。
- DOM 结构采用语义化 `<nav aria-label="已打开页面">` + 单行横向滚动容器（`overflow-x: auto; scrollbar-width: none`，激活页签切换或因固定发生位移导致索引变化时调用 `scrollIntoView({ block: 'nearest', inline: 'nearest' })`）。每个页签为外层容器包裹主点击 `<Link>`（已固定页签保留完整中文模块名并在文字左侧渲染 `12px` `Pin` 图标）与同级关闭 `<button aria-label="关闭{label}">`（仅剩最后 1 个页签或已固定页签隐藏关闭按钮；避免 `<a>` 内嵌 `<button>` 的无效交互嵌套）；在页签容器处理鼠标中键（仅可关闭页签拦截 `onMouseDown` `button === 1` 并在 `onAuxClick` 触发关闭）。
- 页签操作复用现有 `web/src/components/ui/dropdown-menu.tsx`（`@radix-ui/react-dropdown-menu`），不新增 `@radix-ui/react-context-menu` 依赖：
  - 最右侧提供常驻 `28px × 28px` 纯图标幽灵下拉按钮（不渲染「页签操作」文字，仅显示 `ChevronDown` 下拉箭头并保留 `aria-label="页签操作"` 与 `title="页签操作"`）；菜单顶部提供针对当前激活页签的「固定当前页签」／「取消固定当前页签」（无激活页签如 404 或处于 `/no-management` 时禁用），经 `DropdownMenuSeparator` 分隔后提供「关闭其他页签」「关闭右侧页签」。
  - 每个页签响应 `onContextMenu`（`event.preventDefault()`），打开一个挂在该页签上的受控 `DropdownMenu`（以页签内轻量锚点为 trigger），顶部提供相对该右键页签的「固定页签」／「取消固定」（`/no-management` 禁用），经分隔线后提供「关闭其他页签」「关闭右侧页签」，自动继承现有 Radix 键盘导航、Escape 关闭、点击外部关闭及 Portal 亮暗色主题。

**理由与替代方案**：优先复用项目已有 Radix 依赖，避免为了两项菜单引入新包或手写易错的坐标定位与焦点陷阱逻辑。

### 4. 桌面端侧边栏完整收起与双层吸顶壳层层级

**方案**：
- 在 `web/src/components/ui/sidebar.tsx` 的 `SidebarProvider` 中新增仅内存态的 `openDesktop`（默认 `true`）与 `setOpenDesktop`：
  - 桌面端（`!isMobile`）收起时完整隐藏侧边栏（退出主布局流，不渲染为图标栏，且不可被 Tab 误聚焦），主内容区自然扩展占用原 `228px` 宽度。
  - `SidebarTrigger` 取消桌面端隐藏类 `min-[851px]:hidden`，在移动端继续控制 `openMobile` Sheet 并保留原有焦点恢复，在桌面端控制 `openDesktop` 并更新 `aria-expanded` 与 `aria-label`（`收起导航` / `展开导航`）。
- 在 `WorkbenchShell.tsx` 中移除侧边栏底部的 `SidebarSeparator` 与 `SidebarFooter`（消除与顶栏账号菜单重复的用户名展示）。
- 用 `.workbench-header` 包裹上层 `.topbar`（桌面 `min-height: 52px`）与下层 `WorkbenchTabBar`（桌面 `height: 38px–40px`），设置 `position: sticky; top: 0; z-index: 20; background: var(--surface)`；层级低于 Radix Portal 浮层（下拉菜单、Dialog、Sheet 等 `z-index: 40+`），确保吸顶顶栏不遮挡任何浮层。

**理由与替代方案**：完整收起比纯图标折叠栏实现简单且符合主规格“侧边导航不提供整体折叠为图标栏的模式”，同时显著改善审计日志 7 列表、接口管理表等宽表格的桌面端阅读空间。

### 5. 工作台无衬线排版、程序聚焦静默化与桌面端紧凑信息密度

**方案**：
- **作用范围隔离**：所有排版与密度收紧限定在工作台壳层与业务页（`.app-shell`、`.management`、`.profile-page`、`.no-management`），登录／注册页（`.auth-screen` / `.auth-aside` / `.auth-content`）完全保持原衬线品牌样式与尺寸。
- **页头与焦点静默化**：
  - [shared.tsx](web/src/components/management/shared.tsx) 中 `PageHeading` 将 `eyebrow` 设为可选且默认不再渲染装饰性英文前缀；各管理页清理纯英文装饰 eyebrow（保留必要中文语义提示，如范围带中的“当前查看范围”、详情卡片中的中文区块标题）；工作台 `h1` 统一使用无衬线 `var(--body)`、`22px–24px` 粗体，内容区顶部内边距收敛至 `20px–24px`。
  - 保留 `WorkbenchShell` 中 `main.current?.focus()` 与 `PageHeading` 中 `heading.current?.focus()` 以维持读屏器路由切换播报，同时在 CSS 中对 `main.content[tabindex="-1"]:focus` 与 `.mg-heading h1[tabindex="-1"]:focus` 显式设置 `outline: none`，消除切页时的刺眼青色外框；所有按钮、链接、输入、菜单仍由 `:focus-visible` 保持清晰键盘焦点框。
- **桌面提密 + 移动保触控**：
  - 在 `styles.css` 与 `management.css` 中，桌面端（`>850px`）将常规按钮/输入框/筛选器收敛至 `36px`，表内操作按钮/`.mg-list-link`/分页控件收敛至 `32px–34px`，侧边栏分组标题与菜单项收敛至 `36px–38px`；取消桌面端表格 `min-height: 88px` 与用户表 `height: 100px`，调整为表头 `40px`、单元格 `padding: 10px 16px`；数字列、分页计数与状态码/耗时列添加 `font-variant-numeric: tabular-nums`；卡片内边距收敛至 `16px–20px`。
  - 为 `.ui-badge`、`.mg-status`、`.mg-route-state` 统一补齐 `white-space: nowrap; flex-shrink: 0`，根治计数/状态徽章折行。
  - 在 `@media (max-width: 850px)` 中显式保留交互控件 `min-height: 44px` 触控标准与现有移动卡片布局。

**理由与替代方案**：采用“桌面提密、移动维持 `44px`”的断点分层，可在不牺牲移动端无障碍触控合规的前提下，将桌面端表格单屏信息承载量从 4–5 行提升至 8–10 行。

## Risks / Trade-offs

- **关闭激活页签与 `UnsavedGuard` 竞态** → 采用“先导航、路由提交匹配目标后再移除待关页签”的状态机设计，并在 Vitest 中用带 `useBlocker` 的真实 Router 测试覆盖“取消停留”与“确认离开”双路径。
- **桌面端收起按钮影响现有移动 `sidebar.test.tsx` 契约** → 保持 `isMobile` 分支逻辑、`Sheet` 挂载及 `onCloseAutoFocus` 焦点恢复不变，仅补充桌面端 `openDesktop` 状态分支与新单测。
- **吸顶双层顶栏 `z-index` 与表格内浮层/Portal/跳过链接冲突** → 将 `.workbench-header` 固定在 `z-index: 20`，确保 `.skip-link`（`z-index: 70`）及 Radix Portal（DropdownMenu、Select、Dialog、Sheet）层级严格高于吸顶顶栏，并在真实浏览器滚动状态下验证遮挡关系。
- **桌面提密误伤登录页或移动端触控高度** → 所有紧凑密度规则通过 `.app-shell` / `@media (min-width: 851px)` 与 `@media (max-width: 850px)` 精确隔离，避免修改 `.auth-screen` 全局基准，并通过 `1440px`、`1024px`、`850px`、`375px` 亮暗双主题真实浏览器验证核对。
- **页签栏在中等桌面宽度（`851px–1100px`）溢出引发页面级横向滚动** → 外层容器与页签滚动容器严格设置 `min-width: 0; overflow-x: auto`，右侧操作按钮 `flex-shrink: 0`，并用 9 个页签全开场景在 `851px–1024px` 宽度下验证 `documentElement.scrollWidth <= innerWidth`。

## Migration Plan

无数据库迁移、无后端接口/权限/路由变更、无浏览器存储键迁移。前端按标准 `web/` 构建交付即可；如需回滚，回退到上一版前端静态构建产物即可恢复原有单层顶栏与默认折叠分组行为，不影响任何已登录会话或服务端数据。

## Open Questions

无未决产品或架构问题；全部交互、壳层、排版、密度及页签纯图标下拉／手动固定决策均已确认。
