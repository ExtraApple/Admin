## 1. 导航元数据与内存页签状态层

- [x] 1.1 更新 `web/src/lib/workbench-navigation.ts`：将 `getInitialOpenGroups` 改为默认展开全部可见展示层分组（隐藏分组保持关闭），并保持路由切换不覆盖用户手动折叠状态。
- [x] 1.2 在 `web/src/lib/workbench-navigation.ts` 新增静态层级面包屑解析 `getWorkbenchBreadcrumbs(pathname)`，覆盖 `/roles/:id`、`/roles/:id/(permissions|menus|data-scope)`、`/users/:id`、`/users/:id/(roles|organizations)`、`/organizations/:id`、`/organizations/:id/move` 及全部一级页面与 404，返回可点击的父级路径与 `#ID` 标签。
- [x] 1.3 新增 `web/src/lib/workbench-tabs.ts`，实现纯函数页签状态机：按一级模块去重（最多 9 个业务/个人资料模块）、记录最新完整 URL（含查询参数）、`/no-management` 仅作唯一页兜底、404 与无权路径不入栏、单页签不可关闭兜底、关闭激活页签优先右切否则左切、右键/操作菜单批量关闭（「关闭其他」「关闭右侧」）、权限变化自动裁剪失权页签，以及配合 `UnsavedGuard` 的待提交移除状态流转。
- [x] 1.4 更新 `web/src/lib/workbench-navigation.test.tsx` 并新增 `web/src/lib/workbench-tabs.test.ts`，覆盖全展开分组初始化、静态层级面包屑、一级模块去重与 URL 记忆、特殊页面收录、单页签保护、关闭相邻切页、批量关闭规划与权限裁剪。
- [x] 1.5 扩展 `web/src/lib/workbench-tabs.ts` 与 `web/src/lib/workbench-tabs.test.ts`：为 `WorkbenchTab` 增加仅内存态 `pinned` 字段，新增 `canPinTab`（排除 `no-management`）与 `togglePinTab`（维护 `[...pinnedTabs, ...unpinnedTabs]` 左侧归组顺序，`syncVisitedTab` 更新 URL 时保留 `pinned`）；更新 `canCloseTab`、`getBatchCloseState`、`planCloseTab` 与 `planBatchCloseTabs` 使已固定页签免疫单关、中键关闭与批量关闭，并在无可关闭未固定目标时禁用对应批量关闭项，补充完整单元测试。

## 2. 响应式壳层、侧边栏收起与已打开页面栏组件

- [x] 2.1 更新 `web/src/components/ui/sidebar.tsx`：在 `SidebarProvider` 增加仅当前挂载周期内存保留的桌面端 `openDesktop` / `setOpenDesktop` 状态（默认展开），支持桌面端完整收起侧边栏（不渲染为纯图标栏、收起态不可误聚焦），让 `SidebarTrigger` 同时适配桌面端收起/展开与移动端 Sheet 开关并保持原有移动焦点恢复。
- [x] 2.2 新增 `web/src/components/WorkbenchTabBar.tsx`：实现桌面端单行横向滚动页签栏、激活页签自动滚入可视区、点击切换回模块上次完整 URL、单页签 `×` 与鼠标中键关闭（仅剩 1 个页签隐藏关闭按钮并禁用关闭）、复用现有 `DropdownMenu` 实现最右侧「页签操作」下拉菜单与单页签右键菜单（「关闭其他页签」「关闭右侧页签」）。
- [x] 2.3 更新 `web/src/components/WorkbenchShell.tsx`：移除侧边栏底部 `SidebarSeparator` 与 `SidebarFooter` 重复账号区块；用吸顶双层顶栏 `.workbench-header` 包裹 `.topbar`（桌面左侧 `SidebarTrigger` + 静态层级 `Breadcrumb` + 右侧外观与账号菜单）和桌面端 `WorkbenchTabBar`；接入内存页签状态（随 `sessionId` 重置、随 `context` 变化裁剪失权页签）并确保“先导航、提交后移除”严格遵守 `UnsavedGuard`，侧边栏菜单点击回到模块根入口路径。
- [x] 2.4 更新 `web/src/components/ui/sidebar.test.tsx` 并扩展 `web/src/components/WorkbenchShell.test.tsx`，覆盖桌面端侧边栏收起/展开、双层顶栏层级面包屑点击、顶栏页签与侧边栏导航分工、中键与右键/下拉批量关闭、仅剩单页签不可关闭、以及在真实 React Router `useBlocker` 下取消离开保留原页签与确认离开后移除页签。
- [x] 2.5 更新 `web/src/components/WorkbenchTabBar.tsx`、`web/src/components/WorkbenchShell.tsx`、`web/src/styles.css` 与 `web/src/components/WorkbenchShell.test.tsx`：移除最右侧下拉按钮的可见「页签操作」文字，改为 `28px × 28px` 纯图标 `ChevronDown` 幽灵按钮（保留 `aria-label="页签操作"` 与 `title="页签操作"`）；在最右侧下拉菜单顶部新增针对当前激活页签的「固定当前页签／取消固定当前页签」（404 无激活页签或 `/no-management` 时禁用）加分隔线，在单页签右键菜单顶部同步新增针对目标页签的「固定页签／取消固定」加分隔线；已固定页签保留完整中文标题、左侧展示 `12px` `Pin` 图标、隐藏 `×` 并忽略中键；滚动追随监听激活页签索引变化，并补充组件集成测试。

## 3. 工作台页头排版、焦点静默化与桌面端信息密度

- [x] 3.1 更新 `web/src/components/management/shared.tsx` 的 `PageHeading` 与各工作台页面（`AuthorizationOverviewPages.tsx`、`RolePages.tsx`、`UserPages.tsx`、`OrganizationPages.tsx`、`ConfigurationPages.tsx`、`AuditLogPages.tsx`、`ProfilePage.tsx`）：去除或弱化纯装饰性英文 eyebrow 前缀，保留语义中文提示、返回链接与数字列的等宽数字样式标记。
- [x] 3.2 更新 `web/src/styles.css` 与 `web/src/management.css` 的双层吸顶顶栏（`z-index` 低于 Radix Portal 与 `.skip-link`）、页签栏亮暗色与选中/悬浮/禁用样式、工作台无衬线页头（`h1` `22px–24px` 粗体、内容区顶部边距 `20px–24px`），并对 `main.content[tabindex="-1"]:focus` 与 `.mg-heading h1[tabindex="-1"]:focus` 消除程序聚焦刺眼外框（保留键盘交互控件 `:focus-visible`）。
- [x] 3.3 在 `web/src/styles.css` 与 `web/src/management.css` 落地桌面端（`>850px`）信息密度与徽章防折行：桌面常规按钮/输入框/筛选器 `36px`、表内操作与分页 `32px–34px`、侧栏项 `36px–38px`、桌面表格取消 `88px`/`100px` 强制行高改紧凑自然行高（表头 `40px`，单元格 `10px 16px`）、数字列 `tabular-nums`、卡片内边距 `16px–20px`、全站 `.ui-badge`/`.mg-status`/`.mg-route-state` 统一 `white-space: nowrap; flex-shrink: 0`；在 `@media (max-width: 850px)` 严格保持 `44px` 触控高度与移动卡片布局，且不改变 `.auth-screen` 登录／注册页品牌样式。

## 4. 验证与规格同步

- [x] 4.1 运行前端完整静态检查、测试与构建门禁：`cd web && npm run typecheck && npm test -- --run && npm run build`，确认零报错且现有与新增测试全部通过。
- [x] 4.2 在真实浏览器打开正式工作台（连真实服务，覆盖亮色与暗色主题、桌面 `1440px` / 窄桌面 `1024px` / 断点 `850px` / 移动 `375px`），验证双层吸顶顶栏与浮层层级、默认全展开侧栏与桌面收起、多模块页签去重与子路由/筛选 URL 记忆、页签关闭/中键/右键与右侧菜单、编辑页未保存修改拦截（取消保留与确认离开）、静态层级面包屑、切页无刺眼焦点青框、表格密度与徽章不折行、登录页样式未受影响以及零页面级横向滚动。
- [x] 4.3 重新运行前端完整门禁（`cd web && npm run typecheck && npm test -- --run && npm run build`），并在真实浏览器（亮暗双主题）复验纯图标下拉按钮、下拉与右键固定／取消固定、左侧归组与滚动追随、已固定页签关闭与批量关闭保护以及控制台零报错。
- [x] 4.4 确认所有前端行为与测试完全符合 `openspec/changes/improve-workbench-readability-and-tabs/specs/admin-workbench/spec.md` 后，按 OpenSpec 流程将 delta 规格同步至 `openspec/specs/admin-workbench/spec.md` 并归档本 change。
