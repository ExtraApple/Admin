# menu-api-management-ui Specification

## ADDED Requirements

### Requirement: 配置管理工作台入口
正式 React 工作台 SHALL 在现有导航壳层中提供“配置管理”分组，并按当前用户上下文权限显示菜单管理和接口管理入口。

- 菜单管理入口 SHALL 使用 `/menus`。
- 接口管理入口 SHALL 使用 `/apis`。
- 入口显示 SHALL 复用现有 `admin.menus.get` 和 `admin.apis.get` 查询权限语义。
- 直接访问无权限地址时，页面 SHALL 保留统一 401/403 处理，不得依靠隐藏导航绕过后端权限。
- 页面刷新或直接打开地址 SHALL 根据 URL 和真实 API 重新读取数据，不依赖此前页面内存选择。

#### Scenario: 有权限用户看到配置管理入口
- **WHEN** 用户上下文允许查询菜单或接口管理
- **THEN** 工作台在“配置管理”分组中显示对应入口
- **AND** 入口指向稳定的 `/menus` 或 `/apis` 地址

#### Scenario: 无查询权限用户看不到入口
- **WHEN** 用户上下文不包含对应查询权限
- **THEN** 工作台不显示对应入口
- **AND** 直接访问该页面时仍由后端权限返回受控的无权限状态

#### Scenario: 刷新直接地址
- **WHEN** 用户刷新 `/menus`、`/apis` 或带对象/筛选 query 的地址
- **THEN** 页面按地址重新读取真实对象、列表和筛选条件
- **AND** 页面不使用过期的本地菜单、权限或 API Metadata 缓存恢复事实

### Requirement: 菜单树管理页面
菜单管理页面 SHALL 以树形菜单和详情编辑区维护真实菜单，并严格表达目录、页面和按钮的后端类型约束。

- 桌面端 SHALL 展示左侧菜单树和右侧详情/编辑区；窄屏 SHALL 纵向排列。
- 菜单节点 SHALL 显示名称、类型和状态，并按后端返回的层级和排序展示。
- 页面 SHALL 支持创建根菜单、创建子菜单、编辑和删除；删除前 SHALL 先获取影响预览。
- 目录 SHALL 不展示可填写的 path/component；页面 SHALL 要求非空 path；按钮 SHALL 不展示 path/component，并只允许目录或页面作为父级。
- 表单 SHALL 支持名称、类型、父级、状态、排序、图标和菜单自身权限码等字段。
- 不满足后端类型、路径、父级或权限码约束时，页面 SHALL 保留输入并展示字段或表单级错误。

#### Scenario: 维护菜单树
- **WHEN** 用户打开菜单管理页面并具有查询权限
- **THEN** 页面显示真实菜单树和节点状态
- **AND** 选择节点后右侧显示该节点真实详情

#### Scenario: 创建合法页面菜单
- **WHEN** 用户选择页面类型、合法父级和唯一非空 path 后提交
- **THEN** 页面请求菜单创建影响预览
- **AND** 用户确认后使用预览 fingerprint 提交创建
- **AND** 成功后刷新树并选中新菜单

#### Scenario: 类型字段动态约束
- **WHEN** 用户切换目录、页面或按钮类型
- **THEN** 页面只显示满足该类型的字段和父级选择
- **AND** 最终保存仍以服务端验证结果为准

#### Scenario: 菜单删除冲突
- **WHEN** 用户删除仍有子菜单或 fingerprint 已过期的菜单
- **THEN** 页面显示 `NAV_CONFLICT` 的可理解错误
- **AND** 不显示删除成功，不清除用户当前草稿

### Requirement: 菜单 API 绑定和清空
按钮菜单详情 SHALL 提供真实 API 绑定管理，并将绑定、替换和显式清空作为独立的高影响操作。

- 目录和页面 SHALL 不提供可执行的 API 绑定控件，并说明只有按钮可绑定 API。
- 按钮 SHALL 显示已绑定 API 和可搜索候选 API。
- 候选 SHALL 支持关键词、Method、Path、状态和认证要求筛选；停用、公开或不存在的 API 不得作为新绑定目标。
- 绑定 SHALL 提交去重后的非空 API ID 集合。
- 清空 SHALL 使用 `clear=true` 和当前 fingerprint；空数组不得被当作解绑。
- 菜单自身 permission_code SHALL 与 API 绑定的最终权限码分开展示。

#### Scenario: 绑定 API
- **WHEN** 用户为按钮选择合法启用且需要认证的 API 集合
- **THEN** 页面展示集合差异和影响预览
- **AND** 用户确认后使用 fingerprint 提交绑定
- **AND** 成功后刷新绑定列表和菜单权限摘要

#### Scenario: 目录或页面无绑定操作
- **WHEN** 用户选择目录或页面菜单
- **THEN** 页面不提供可执行的绑定或清空按钮
- **AND** 页面说明该类型不能绑定 API

#### Scenario: 显式清空绑定
- **WHEN** 用户确认清空按钮的全部 API 绑定
- **THEN** 页面提交 `clear=true` 和预览 fingerprint
- **AND** 成功后显示空绑定列表
- **AND** 不把菜单自身 permission_code 误显示为已清除

#### Scenario: 绑定提交陈旧
- **WHEN** 绑定预览后相关菜单、API 或权限事实发生变化
- **THEN** 页面显示 `NAV_CONFLICT`
- **AND** 保留当前选择供用户比较，不自动重放旧提交

### Requirement: API 元数据管理页面
接口管理页面 SHALL 使用服务端分页列表和详情编辑区维护允许编辑的 API Metadata 字段，并只读展示真实路由字段和 RouteState。

- 列表 SHALL 支持关键词、分组、HTTP Method、status、NeedAuth、NeedAudit、RouteState 和分页筛选。
- 筛选条件和页码 SHALL 可通过 URL query 恢复。
- Method、Path、NeedAuth、API ID 和 RouteState SHALL 只读。
- 页面 SHALL 不提供手工创建 API 按钮。
- 页面可编辑名称、分组、permission_code、备注、排序、status 和 need_audit；高影响字段 SHALL 走预览和 fingerprint 提交。
- 列表和详情 SHALL 显示 `matched`、`metadata_only`、`catalog_only`、`access_mismatch` 的文字标签和说明。

#### Scenario: 分页筛选 API
- **WHEN** 用户设置关键词或筛选条件并翻页
- **THEN** 页面向真实 API 列表接口请求当前条件和页码
- **AND** 页面显示服务端返回的总数、当前页和数据
- **AND** 条件变化时页码重置为第一页

#### Scenario: 编辑 API 元数据
- **WHEN** 用户修改名称、分组、权限码、备注、排序、status 或 need_audit
- **THEN** 页面先请求 API 影响预览
- **AND** 用户确认后使用 fingerprint 提交
- **AND** 成功后刷新当前详情和列表行

#### Scenario: 路由字段只读
- **WHEN** 用户查看 API 详情
- **THEN** Method、Path 和 NeedAuth 以只读信息展示
- **AND** 页面不提供手工创建 API 或修改这些字段的表单
- **AND** 页面提供 Route Catalog 同步或漂移诊断入口

#### Scenario: RouteState 诊断
- **WHEN** API 的 RouteState 为 `metadata_only`、`catalog_only` 或 `access_mismatch`
- **THEN** 页面显示对应文字、原因和允许的下一步操作
- **AND** 页面不把数据库 Metadata 当作真实 Gin 路由来源

### Requirement: Route Catalog 同步和按钮生成
页面 SHALL 将菜单路由同步、API 路由同步和 API 生成按钮操作呈现为先预览后提交的差异工作流。

- 菜单和 API 同步 SHALL 展示新增、跳过、漂移、父级/路径冲突和访问策略冲突。
- 存在不可提交冲突时，确认提交按钮 SHALL 禁用。
- API 详情 SHALL 支持选择目录或页面父级、按钮名称和排序后生成按钮菜单。
- 生成按钮 SHALL 使用 API impact preview 和 fingerprint；按钮父级非法时不得创建任何事实。
- 同步和生成按钮成功后 SHALL 刷新受影响列表、树、RouteState 和权限摘要。

#### Scenario: 预览菜单路由同步
- **WHEN** 用户点击菜单同步并具有同步权限
- **THEN** 页面请求菜单同步影响预览
- **AND** 页面展示新增、跳过和冲突摘要
- **AND** 预览本身不改变菜单、权限或授权版本

#### Scenario: 预览并提交 API 路由同步
- **WHEN** 用户请求 API Route Catalog 同步且预览无冲突
- **THEN** 页面展示待创建和漂移项
- **AND** 用户确认后提交当前 fingerprint
- **AND** 成功后显示最终新增数量并刷新列表

#### Scenario: 从 API 生成按钮
- **WHEN** 用户为启用且需要认证的 API 选择目录或页面父级并输入名称、排序
- **THEN** 页面展示按钮、API 绑定、权限码和用户影响摘要
- **AND** 用户确认后提交 fingerprint 创建按钮

#### Scenario: 同步 fingerprint 过期
- **WHEN** 用户确认前 Route Catalog 或 API Metadata 发生变化
- **THEN** 页面显示 `API_META_CONFLICT`
- **AND** 要求重新获取预览，不显示部分成功结果或自动重试

### Requirement: 统一管理页面状态和可访问性
菜单和接口管理页面 SHALL 在真实数据和高影响操作流程中保持可恢复、可访问和响应式。

- 页面 SHALL 提供加载、空结果、失败、403、字段错误、预览冲突和提交失败状态。
- 保存失败 SHALL 保留未保存输入；未保存输入存在时切换对象、离开页面或刷新 SHALL 触发保护。
- 状态 SHALL 同时使用文字、语义颜色、图标和可访问名称表达，不得只依赖颜色。
- 页面 SHALL 在 375px、850px、1440px 视口和亮暗主题下可操作，不能产生页面级横向滚动。
- 树节点、筛选、表格/卡片、抽屉、Dialog、按钮和焦点返回 SHALL 支持键盘操作。

#### Scenario: API 请求失败
- **WHEN** 列表、详情、预览或提交请求失败
- **THEN** 页面显示安全错误信息和手动重试入口
- **AND** 页面不把本地草稿标记为已保存

#### Scenario: 403 权限变化
- **WHEN** 页面请求返回 403
- **THEN** 页面保留当前位置并说明没有权限
- **AND** 页面不显示成功状态，不通过本地权限绕过后端限制

#### Scenario: 窄屏和主题切换
- **WHEN** 用户在窄屏或切换亮暗主题时使用菜单/API 页面
- **THEN** 页面保持输入、当前对象、弹窗、焦点和未保存保护
- **AND** 所有内容仍可阅读和操作
