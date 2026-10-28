## 1. 数据契约与请求层

- [ ] 1.1 在 `web/src/lib/models.ts` 增加菜单、API Metadata、RouteState、影响预览、同步摘要、分页和 API 错误相关的 TypeScript 类型。
- [ ] 1.2 在 `web/src/lib/api.ts` 增加菜单/API 列表、详情、CRUD、绑定/清空、同步、按钮生成和 impact preview 请求方法，复用统一认证、错误信封和 fingerprint header/body 规则。
- [ ] 1.3 为接口筛选、分页和菜单对象选择实现 URL query 编码/解码，确保刷新、直接访问和浏览器前进后退可以恢复页面状态。

## 2. 工作台路由与导航

- [ ] 2.1 在 `web/src/lib/workbench-navigation.ts` 增加“配置管理”分组、`/menus`、`/apis` 入口和对应查询权限过滤。
- [ ] 2.2 在正式 React Router 中注册菜单管理和接口管理页面，并让 Breadcrumb、直接访问、刷新和未授权地址遵守现有工作台守卫。
- [ ] 2.3 保持桌面侧栏、移动 Sheet、主题选择、账号菜单和主内容焦点恢复行为不变，补充配置管理导航的 active/折叠状态测试。

## 3. 共享管理交互

- [ ] 3.1 实现影响预览确认组件，展示受影响菜单/API/角色/用户、版本变化、同步新增/跳过/冲突和安全说明。
- [ ] 3.2 实现高影响操作状态机和 `NAV_CONFLICT`/`API_META_CONFLICT` 处理，禁止自动重放并保留未提交草稿。
- [ ] 3.3 实现管理页统一加载、空结果、403、字段错误、网络失败、提交失败和手动重试状态。
- [ ] 3.4 实现未保存修改保护、Dialog/Sheet 焦点返回、键盘操作和亮暗主题语义状态样式，复用现有管理样式与 Radix/shadcn 组件。

## 4. 菜单管理页面

- [ ] 4.1 实现菜单树、节点选择、类型图标、状态展示、URL `menu_id` 恢复和桌面双栏/窄屏单栏布局。
- [ ] 4.2 实现目录、页面、按钮共用表单，按类型控制 path/component/parent 字段，并保留服务端字段错误和交叉校验错误。
- [ ] 4.3 实现菜单创建、修改、删除的预览→确认→fingerprint 提交流程，成功后刷新树和详情，失败时保留草稿。
- [ ] 4.4 实现按钮 API 已绑定列表和候选 API 选择器，支持关键词/Method/Path/status/NeedAuth 筛选、合法集合去重和权限码分离展示。
- [ ] 4.5 实现 `clear=true` 显式清空绑定流程，禁止空数组解绑，并覆盖清空预览、确认、冲突和成功刷新。
- [ ] 4.6 实现菜单路由同步预览和提交，展示新增/跳过/未知父级/重复路径/循环/类型冲突并在冲突时禁止提交。

## 5. 接口管理页面

- [ ] 5.1 实现服务端分页 API 列表、关键词/分组/Method/status/NeedAuth/NeedAudit/RouteState 筛选和移动端卡片视图。
- [ ] 5.2 实现 API 详情与编辑面板，Method、Path、NeedAuth、API ID、RouteState 只读；名称、分组、权限码、备注、排序、status、need_audit 可编辑。
- [ ] 5.3 实现 RouteState 标签、漂移说明和 `matched`/`metadata_only`/`catalog_only`/`access_mismatch` 对应操作入口。
- [ ] 5.4 实现 API 元数据修改、删除的预览→确认→fingerprint 提交流程，不提供手工创建 API，不自动修改 Method/Path/NeedAuth。
- [ ] 5.5 实现 API Route Catalog 同步预览和提交，展示新增/跳过/漂移/冲突摘要，处理 `API_META_CONFLICT` 并禁止展示部分成功结果。
- [ ] 5.6 实现从 API 生成按钮菜单的父级选择、名称/排序表单、预览摘要、fingerprint 提交和成功后菜单/API 刷新。

## 6. 测试、浏览器验收与 TODO

- [ ] 6.1 增加请求层和数据模型测试，覆盖 URL query、分页参数、统一错误信封、fingerprint body/header 和冲突错误映射。
- [ ] 6.2 增加菜单管理组件测试，覆盖类型字段、树选择、绑定/清空、同步冲突、未保存保护和无权限状态。
- [ ] 6.3 增加接口管理组件测试，覆盖分页筛选、RouteState、只读字段、元数据编辑、同步、删除和按钮生成。
- [ ] 6.4 使用真实后端和浏览器完成 `/menus`、`/apis` 的预览→提交、刷新恢复、陈旧 fingerprint、绑定边界、403、亮暗主题、375px/850px/1440px 和键盘验收。
- [ ] 6.5 全部实现和验收通过后，将 `TODO.md` 中菜单管理页面和接口管理页面两项从 `[ ]` 更新为 `[x]`，并记录实际验证命令与结果。
