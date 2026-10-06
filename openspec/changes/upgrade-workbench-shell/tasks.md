## 1. 依赖与 UI 原语

- [ ] 1.1 在 `web/package.json` 增加 Sidebar、Sheet、Collapsible、Separator、Tooltip 所需的 Radix primitives，并同步 `web/package-lock.json`，不改动现有业务依赖版本。
- [ ] 1.2 按项目现有 shadcn/ui 约定新增或整理本地 `Sidebar`、`Sheet`、`Collapsible`、`Separator`、`Tooltip` 和 `Breadcrumb` 组件，复用 `cn`、主题 token、Slot 和现有 Button，不引入整套后台模板。
- [ ] 1.3 实现或复用 850px 移动断点判断，使 CSS 响应式断点与 Sidebar/Sheet 的渲染判断保持一致，并确保组件不使用 Cookie、localStorage 或 sessionStorage 保存状态。

## 2. 导航模型与权限过滤

- [ ] 2.1 在工作台壳层或相邻的纯前端模块中定义三个展示层导航分组、稳定分组 ID、入口标签、图标、React Router 地址和当前页面匹配规则。
- [ ] 2.2 使用现有 `canAccessAuthorizationOverview`、`canAccessAuthorizationRisks` 和 `canAccessModule` 过滤入口；过滤后移除空分组，确保展示分组不读取、修改或扩展后端 Menu、权限码和 API。
- [ ] 2.3 提取可测试的分组匹配与初始化逻辑：首次挂载时当前地址所属可见分组展开，其余分组折叠；没有匹配分组时全部折叠；后续路由变化不得重置用户状态。

## 3. WorkbenchShell 壳层替换

- [ ] 3.1 用本地 shadcn/ui Sidebar 结构替换 `WorkbenchShell.tsx` 的自定义 `<aside>`、`.nav-group` 和 `.nav-button` 标记，桌面端固定 228px 且不提供整体图标折叠模式。
- [ ] 3.2 使用 Collapsible 渲染分组标题和入口列表，暴露准确的 `aria-expanded`、`aria-controls`、`aria-current`、可见焦点和键盘操作；允许用户折叠当前 active 分组。
- [ ] 3.3 保留现有 brand、React Router `NavLink`、动态权限、当前入口高亮、`main` pathname 焦点恢复、用户头像、个人资料入口和退出流程，不改变页面路由或权限守卫。
- [ ] 3.4 在 850px 及以下使用共享导航模型渲染移动 Sheet；Sheet 只承载导航，顶部继续承载菜单触发器、Breadcrumb、用户身份和退出操作。
- [ ] 3.5 实现移动入口选择流程：导航到对应 React Router 地址、关闭 Sheet，并将焦点恢复到打开 Sheet 的菜单触发按钮；Escape、关闭按钮和遮罩点击也必须可关闭并恢复安全焦点。
- [ ] 3.6 使用本地 Breadcrumb 组件替换自定义 breadcrumb，统一输出“工作台 / 当前页面”两级路径，并保持现有页面标签映射和可访问名称。

## 4. 壳层样式切换

- [ ] 4.1 将 Sidebar、Sheet、分组菜单、Breadcrumb 和移动顶部栏适配现有冷白／深海青主题、Noto 字体、`--line` 边界、focus ring 和 44px 触控目标。
- [ ] 4.2 删除已被替换且不再使用的 `.sidebar`、`.nav-button`、`.nav-group` 以及移动端导航换行样式；保留仍被其他页面使用的通用 token 和内容区样式。
- [ ] 4.3 检查 375px、850px、1440px 布局，确保 Sidebar/Sheet 切换一致、顶部操作可用、主内容区域不产生页面级横向滚动。

## 5. 回归验证

- [ ] 5.1 为分组过滤、空分组隐藏、当前路由分组初始化和路由切换保持状态添加 Vitest 回归覆盖，测试行为边界而不是组件转发或实现文本。
- [ ] 5.2 启动真实前端工作台，按权限矩阵验证授权总览、风险清单、角色、用户、组织和个人资料入口；确认无权限入口不出现，空分组不出现，直接地址仍由现有路由与权限逻辑处理。
- [ ] 5.3 在 375px、850px、1440px 视口执行键盘和触摸验收：分组展开/折叠、active 状态、菜单 Sheet 打开/关闭、入口选择后的焦点恢复、Breadcrumb、头像和退出操作。
- [ ] 5.4 运行 `npm run typecheck`、`npm test` 和 `npm run build`，确认 Radix 依赖、锁文件、组件类型和生产构建均通过。
