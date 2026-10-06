## 1. 依赖与 UI 原语

- [x] 1.1 在 `web/package.json` 增加 Sidebar、Sheet、Collapsible、Separator、Tooltip 所需的 Radix primitives，并同步 `web/package-lock.json`；其余非 Radix 业务依赖版本保持不变。
- [x] 1.2 按项目现有 shadcn/ui 约定新增或整理本地 `Sidebar`、`Sheet`、`Collapsible`、`Separator`、`Tooltip` 和 `Breadcrumb` 组件，复用 `cn`、主题 token、Slot 和现有 Button，不引入整套后台模板。
- [x] 1.3 实现或复用 850px 移动断点判断，使 CSS 响应式断点与 Sidebar/Sheet 的渲染判断保持一致，并确保组件不使用 Cookie、localStorage 或 sessionStorage 保存状态。

## 2. 导航模型与权限过滤

- [x] 2.1 在工作台壳层或相邻的纯前端模块中定义三个展示层导航分组、稳定分组 ID、入口标签、图标、React Router 地址和当前页面匹配规则。
- [x] 2.2 使用现有 `canAccessAuthorizationOverview`、`canAccessAuthorizationRisks` 和 `canAccessModule` 过滤入口；过滤后移除空分组，确保展示分组不读取、修改或扩展后端 Menu、权限码和 API。
- [x] 2.3 提取可测试的分组匹配与初始化逻辑：首次挂载时当前地址所属可见分组展开，其余分组折叠；没有匹配分组时全部折叠；后续路由变化不得重置用户状态。

## 3. WorkbenchShell 壳层替换

- [x] 3.1 用本地 shadcn/ui Sidebar 结构替换 `WorkbenchShell.tsx` 的自定义 `<aside>`、`.nav-group` 和 `.nav-button` 标记，桌面端固定 228px 且不提供整体图标折叠模式。
- [x] 3.2 使用 Collapsible 渲染分组标题和入口列表，暴露准确的 `aria-expanded`、`aria-controls`、`aria-current`、可见焦点和键盘操作；允许用户折叠当前 active 分组。
- [x] 3.3 保留现有 brand、React Router `NavLink`、动态权限、当前入口高亮、`main` pathname 焦点恢复、用户头像、个人资料入口和退出流程，不改变页面路由或权限守卫。
- [x] 3.4 在 850px 及以下使用共享导航模型渲染移动 Sheet；Sheet 只承载导航，顶部继续承载菜单触发器、Breadcrumb、用户身份和退出操作。
- [x] 3.5 实现移动入口选择流程：导航到对应 React Router 地址、关闭 Sheet，并将焦点恢复到打开 Sheet 的菜单触发按钮；Escape、关闭按钮和遮罩点击也必须可关闭并恢复安全焦点。
- [x] 3.6 使用本地 Breadcrumb 组件替换自定义 breadcrumb，统一输出“工作台 / 当前页面”两级路径，并保持现有页面标签映射和可访问名称。

## 4. 壳层样式切换

- [x] 4.1 将 Sidebar、Sheet、分组菜单、Breadcrumb 和移动顶部栏适配现有冷白／深海青主题、Noto 字体、`--line` 边界、focus ring 和 44px 触控目标。
- [x] 4.2 删除已被替换且不再使用的 `.sidebar`、`.nav-button`、`.nav-group` 以及移动端导航换行样式；保留仍被其他页面使用的通用 token 和内容区样式。
- [x] 4.3 检查 375px、850px、1440px 布局，确保 Sidebar/Sheet 切换一致、顶部操作可用、主内容区域不产生页面级横向滚动。

## 5. 回归验证

- [x] 5.1 为分组过滤、空分组隐藏、当前路由分组初始化和路由切换保持状态添加 Vitest 回归覆盖，测试行为边界而不是组件转发或实现文本。
- [x] 5.2 启动真实前端工作台，按权限矩阵验证授权总览、风险清单、角色、用户、组织和个人资料入口；确认无权限入口不出现，空分组不出现，直接地址仍由现有路由与权限逻辑处理。
- [x] 5.3 在 375px、850px、1440px 视口执行键盘和触摸验收：分组展开/折叠、active 状态、菜单 Sheet 打开/关闭、入口选择后的焦点恢复、Breadcrumb、头像和退出操作。
- [x] 5.4 运行 `npm run typecheck`、`npm test` 和 `npm run build`，确认 Radix 依赖、锁文件、组件类型和生产构建均通过。

## 6. Radix 依赖统一升级与嵌套浮层回归

- [x] 6.1 按用户确认统一升级全部已引入的 Radix primitives 到兼容发布系列，同步锁文件，并检查 Sheet、AlertDialog、Select、DropdownMenu、Tooltip 共用的 FocusScope／DismissableLayer 版本。
- [x] 6.2 添加 Sheet 内触发确认框的取消／确认离开回归，验证浮层关闭后释放 pointer lock 和恢复焦点；在真实浏览器复验现有 Radix 控件并重新运行类型检查、测试、生产构建。

## 验收记录（2026-10-06）

- 依赖树：全部已引入的 Radix primitives 采用当前兼容稳定系列；FocusScope 只有 `1.2.0`，DismissableLayer 只有 `1.1.20`；升级前后非 Radix 包的已安装版本没有变化。
- 回归：`web/src/components/ui/sidebar.test.tsx` 在旧依赖下暴露嵌套浮层的焦点冲突，在统一依赖后通过；覆盖确认框获得焦点、取消后 Sheet 可继续使用、确认离开释放 pointer lock 和恢复菜单焦点。
- 真实后端：角色权限编辑页取消未保存确认后保留选择与 Sheet；确认离开转到个人资料，body pointer events 恢复 `auto`，焦点返回菜单按钮。验收期间角色、用户、组织没有管理写请求。
- 现有控件：RadioGroup 方向键切换并恢复原选择；DropdownMenu 方向键导航、Escape 关闭及动作确认取消；Select 键盘应用风险筛选、Escape 关闭并恢复触发器焦点；Checkbox 的未保存选择保留。Tooltip 用项目现有组件在浏览器临时挂载，验证 hover、可访问描述与 Escape 后保留触发器焦点，随后移除挂载。
- 壳层：375px、850px、1440px 无页面级横向滚动，桌面侧栏 228px，跨断点保留分组状态；验证真实触摸打开、Escape／关闭按钮／遮罩关闭、普通导航焦点恢复、头像入口和真实登出（`POST /api/user/logout` 为 200）。补齐 brand 链接的 44px 触控高度。
- 开发缓存：旧 Vite 预构建仍包含升级前的重复核心；重启并使用 `--force` 后真实嵌套浮层复验通过，操作说明已同步 README。
- 最终命令：`npm run typecheck`、`npm test`（5 个文件、85 项测试）、`npm run build` 均通过；仍有既有的单块超过 500 kB 构建提示，本次不扩大到代码分包。
