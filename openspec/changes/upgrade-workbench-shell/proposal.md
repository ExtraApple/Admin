## Why

当前工作台壳层使用自定义 sidebar、移动端换行导航和自定义 breadcrumb；随着授权核对、身份与组织、账户入口增加，导航层级和窄屏操作路径不够稳定。现在按既有 React + Tailwind + shadcn/ui + Radix 技术路线统一壳层，可改善导航可发现性、Sheet 焦点管理和分组一致性，同时不改变现有权限与业务契约。

## What Changes

- 使用官方 shadcn/ui + Radix Sidebar 结构替换当前工作台自定义壳层。
- 将可访问入口组织为三个展示层导航分组：
  - “授权核对”：授权总览、风险清单。
  - “身份与组织”：角色、用户、组织。
  - “账户”：个人资料。
- 保留现有权限过滤、React Router 路由、当前入口高亮、页面切换后的 main focus、用户头像和退出流程。
- 桌面端保持 228px Sidebar 宽度；首次挂载时展开当前路由分组，其余分组折叠；之后由用户控制分组展开状态，路由切换不强制改写状态。
- 没有任何可访问入口的分组不显示。
- 在 850px 及以下视口使用移动 Sheet 承载导航；Sheet 与桌面共享分组状态，选择入口后关闭并将焦点恢复到菜单按钮。
- 移动 Sheet 只承载导航，退出操作继续保留在顶部。
- 使用官方 Breadcrumb 组件统一为“工作台 / 当前页面”两级路径。
- 按官方组件需要新增 Radix primitives 依赖，并在完成切换后删除旧 sidebar、nav-button 和移动端换行导航样式。
- **非目标**：不修改后端菜单树、权限码、API、数据模型、业务页面、DataTable、表单、授权风险计算或详情页 Breadcrumb 层级。

## Capabilities

### New Capabilities

无。该变更只改造现有管理工作台的导航壳层，不引入新的业务能力。

### Modified Capabilities

- `admin-workbench`: 修改工作台导航分组、桌面/移动导航行为、Sheet 焦点恢复和两级 Breadcrumb 的可观察行为；现有认证、权限过滤、路由和窄屏可访问性约束保持不变。

## Impact

- **前端代码**：`web/src/components/WorkbenchShell.tsx`、`web/src/components/ui/` 中的 Sidebar 相关组件，以及 `web/src/styles.css` 的壳层样式。
- **依赖**：按官方 Sidebar 组成补充所需 Radix primitives；同步 `web/package.json` 与 `web/package-lock.json`。
- **路由/API/权限**：无新增或修改。现有 `canAccessAuthorizationOverview`、`canAccessAuthorizationRisks`、`canAccessModule` 和 React Router 路由继续作为事实来源。
- **运行状态**：分组展开状态只存在当前 `WorkbenchShell` 挂载周期的 React state，不写入 `localStorage` 或 `sessionStorage`。
- **验证范围**：375px、850px、1440px 视口；键盘导航、可见焦点、Sheet 开关与焦点恢复、空分组隐藏、active 状态和权限过滤。
