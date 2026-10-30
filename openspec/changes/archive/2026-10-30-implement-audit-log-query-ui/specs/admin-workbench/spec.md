# admin-workbench Specification Delta

## MODIFIED Requirements

### Requirement: 根据实际授权显示管理入口
工作台 SHALL 以 `GET /api/user/context` 的实际可见菜单和当前权限码决定导航及操作入口；页面隐藏操作不替代后端权限检查。除既有授权核对、身份与组织入口外，工作台 SHALL 支持配置管理展示层分组，包含菜单管理和接口管理入口；并 SHALL 支持审计展示层分组，包含审计日志查询入口。审计日志入口 SHALL 由五个审计读权限中的任意一个决定，不要求额外菜单记录。

#### Scenario: 有管理入口的用户
- **WHEN** 登录用户的上下文提供工作台支持的可见管理菜单及相应权限
- **THEN** 页面 SHALL 只呈现获准的角色、用户、组织、菜单、接口或审计日志入口以及其有权限执行的查看／修改操作
- **AND** 不展示被停用或最终不可见的菜单

#### Scenario: 配置管理入口按查询权限显示
- **WHEN** 用户具有 `admin.menus.get` 或 `admin.apis.get` 对应的真实查询权限
- **THEN** 工作台在“配置管理”分组中显示对应的 `/menus` 或 `/apis` 入口
- **AND** 写操作按钮 SHALL 继续按对应写权限控制

#### Scenario: 审计入口按读权限显示
- **WHEN** 用户的权限码包含 `admin.audit-logs.get`、`admin.login-logs.get`、`admin.operation-logs.get`、`admin.permission-logs.get` 或 `admin.data-access-logs.get` 中的任意一个
- **THEN** 工作台在“审计”分组中显示 `/audit-logs` 入口
- **AND** 该入口 SHALL NOT 要求用户同时拥有额外菜单记录
- **AND** 用户没有任何审计读权限时 SHALL NOT 显示该入口

#### Scenario: 没有可用管理菜单
- **WHEN** 已登录用户没有本工作台可用的管理菜单
- **THEN** 页面 SHALL 显示“当前没有可用的管理功能”的专用说明，仍提供真实个人资料和退出入口
- **AND** 页面 SHALL NOT 显示空白工作台或声称注册用户处于待审核流程

#### Scenario: 被服务端拒绝访问
- **WHEN** 当前页面上的管理请求返回 403
- **THEN** 页面 SHALL 保留当前位置，说明没有权限且不显示成功状态；直接访问未授权地址也 SHALL NOT 绕过限制

### Requirement: 工作台导航分组与响应式壳层
工作台 SHALL 使用基于 shadcn/ui 与 Radix 的导航壳层，在不改变后端菜单、权限码和路由事实的前提下，按展示层分组呈现当前操作者可访问的工作台入口。展示层分组 SHALL 包含“授权核对”“身份与组织”“配置管理”和“审计”；配置管理只承载菜单管理与接口管理入口；审计只承载审计日志查询入口。

#### Scenario: 按权限显示导航分组
- **WHEN** 已认证用户打开工作台壳层
- **THEN** 页面将可访问的侧栏入口归入“授权核对”“身份与组织”“配置管理”“审计”展示层分组
- **AND** “授权核对”只包含用户有权限访问的授权总览和风险清单入口
- **AND** “身份与组织”只包含用户有权限访问的角色、用户和组织入口
- **AND** “配置管理”只包含用户有权限访问的菜单管理和接口管理入口
- **AND** “审计”只包含用户有权限访问的 `/audit-logs` 入口
- **AND** 页面继续使用现有权限判断，不因展示分组新增任何管理权限

#### Scenario: 隐藏空分组
- **WHEN** 某个展示层分组没有任何当前用户可访问的入口
- **THEN** 工作台不渲染该分组的标题、折叠控制或空菜单
- **AND** 页面不得用隐藏分组暗示用户拥有不可访问的功能

#### Scenario: 顶部账号菜单
- **WHEN** 已认证用户点击顶部头像或昵称入口
- **THEN** 页面展开包含“个人资料”和“退出登录”的下拉菜单，不立即导航或退出
- **AND** 选择个人资料后导航到原 `/profile` 页面，保留个人资料 Breadcrumb 名称，不依赖管理菜单或管理权限
- **AND** 选择退出登录后沿用现有登出接口、内存会话清除、登录页跳转及失败提示，不新增退出二次确认
- **AND** 个人资料和退出导航仍遵守当前页面的未保存修改保护；取消离开时保留原页面、会话和未保存选择
- **AND** 顶部不再单独渲染退出按钮，移动导航 Sheet 不重复承载账号菜单操作
- **AND** 桌面与窄屏菜单均支持键盘操作和点击外部关闭，Escape 关闭后焦点返回账号菜单触发器

#### Scenario: 初始化分组状态
- **WHEN** WorkbenchShell 首次挂载且当前地址属于一个可见展示层分组
- **THEN** 当前地址所属分组默认展开
- **AND** 其他可见分组默认折叠
- **AND** 分组状态只保留在当前工作台挂载周期内，不写入 localStorage、sessionStorage 或服务端

#### Scenario: 用户控制分组状态
- **WHEN** 用户点击展示层分组的展开或折叠控制
- **THEN** 对应分组切换展开状态并暴露正确的 `aria-expanded` 状态
- **AND** 用户可以折叠当前地址所属分组
- **AND** 路由切换不得自动改写用户已经设置的分组状态

#### Scenario: 桌面导航
- **WHEN** 视口宽度大于 850px
- **THEN** 工作台显示宽度为 228px 的侧边导航和主内容区域
- **AND** 侧边导航不提供整体折叠为图标栏的模式
- **AND** 当前地址对应的导航入口在可见时保留 active 状态和键盘焦点样式
- **AND** 顶部保留两级“工作台 / 当前页面”Breadcrumb、用户身份账号菜单和外观选择入口

#### Scenario: 移动导航 Sheet
- **WHEN** 视口宽度小于或等于 850px
- **THEN** 工作台使用 Sheet 承载与桌面端相同的权限过滤导航分组
- **AND** 顶部提供可访问的导航菜单触发按钮、两级 Breadcrumb、用户身份账号菜单和外观选择入口
- **AND** Sheet 不重复承载退出操作
- **AND** Sheet 与桌面端共享分组展开状态

#### Scenario: 选择移动导航入口
- **WHEN** 用户在移动 Sheet 中选择一个可访问导航入口
- **THEN** 工作台导航到该入口对应的 React Router 地址
- **AND** Sheet 关闭
- **AND** 焦点恢复到打开 Sheet 的菜单触发按钮
- **AND** 路由切换后主要内容继续获得现有的页面焦点恢复行为

#### Scenario: 关闭移动导航
- **WHEN** 用户按下 Escape、点击关闭按钮或点击 Sheet 遮罩
- **THEN** Sheet 关闭
- **AND** 焦点返回菜单触发按钮或 Radix Dialog 定义的安全返回目标
- **AND** 页面不得产生无法关闭的导航遮罩

#### Scenario: 窄屏壳层可操作
- **WHEN** 用户在 375px、850px 或 1440px 视口使用键盘或触摸操作工作台壳层
- **THEN** 导航分组、导航入口、菜单按钮、Sheet 关闭路径和退出入口均可访问
- **AND** 页面不得产生页面级横向滚动
- **AND** 壳层替换不得绕过现有认证、权限过滤或 React Router 路由守卫

## ADDED Requirements

### Requirement: 审计日志查询页面
工作台 SHALL 提供 `/audit-logs` 只读审计日志查询页面，使用真实审计查询接口，按“全部”“登录”“操作”“权限变更”“数据访问”页签展示日志；页面 SHALL NOT 提供修改、删除或导出能力。页签 SHALL 按当前用户权限裁剪；“全部”不可用时 SHALL 默认选中第一个有权限的页签。查询、分页与页签状态 SHALL 写入 URL，刷新、直接访问和浏览器前进后退 SHALL 恢复相同查询。页面 SHALL 只查询热表，并明确提示结果不包含已归档数据。

#### Scenario: 页签与权限裁剪
- **WHEN** 用户打开 `/audit-logs`
- **THEN** 页面 SHALL 只显示该用户拥有对应读权限的页签
- **AND** “全部”页签 SHALL 要求 `admin.audit-logs.get`
- **AND** 登录、操作、权限变更、数据访问页签 SHALL 分别要求 `admin.login-logs.get`、`admin.operation-logs.get`、`admin.permission-logs.get`、`admin.data-access-logs.get`
- **AND** 用户没有任何审计读权限时 SHALL 显示权限说明而不是空表格

#### Scenario: 默认页签
- **WHEN** 用户打开 `/audit-logs`，或 URL 中的 `type` 没有对应用户权限
- **THEN** 页面 SHALL 选中“全部”，当且仅当用户有 `admin.audit-logs.get`
- **AND** 否则 SHALL 选中第一个有权限的页签，并按该页签请求日志

#### Scenario: 刷新、直达和前进后退恢复查询
- **WHEN** 用户设置页签、时间范围、筛选项或分页后刷新页面，或直接打开带查询参数的地址，或使用浏览器前进后退
- **THEN** 页面 SHALL 使用 URL 中的页签、筛选和分页重新请求真实数据
- **AND** 页面 SHALL NOT 依赖此前点击产生的内存状态

#### Scenario: 默认时间窗口与预设
- **WHEN** 用户首次打开页面且 URL 没有时间范围
- **THEN** 页面 SHALL 查询最近 24 小时
- **AND** 时间范围 SHALL 以绝对起止时间写入 URL，刷新后不随当前时间漂移
- **AND** 预设 1 小时、24 小时、7 天、30 天和 90 天 SHALL 只作为快捷写入绝对时间的方式
- **AND** 单次查询范围 SHALL NOT 超过 90 天；超出或开始时间晚于结束时间时 SHALL 显示字段错误且不发起查询
- **AND** 时间输入与显示 SHALL 使用与审计日志时间一致的服务器本地时间语义

#### Scenario: 列表与分页
- **WHEN** 查询返回日志
- **THEN** 桌面视口 SHALL 使用表格，窄屏 SHALL 使用卡片列表
- **AND** 分页 SHALL 使用服务端总数与页码，每页 10、20 或 50 条，默认 20
- **AND** 翻页和切换每页条数 SHALL 更新 URL 并重新请求

#### Scenario: 列表列
- **WHEN** 页面展示日志行
- **THEN** 每行 SHALL 展示时间、分类、用户（username 与 ID）、Method、Path、状态、耗时和客户端 IP
- **AND** query、body、metadata 与 user_agent SHALL NOT 出现在列表中，只在详情中展示

#### Scenario: 日志详情
- **WHEN** 用户打开一条日志的详情
- **THEN** 页面 SHALL 在 Sheet 中展示 query、body、metadata 和 user_agent
- **AND** 内容 SHALL 按纯文本或格式化 JSON 渲染，长字段默认折叠并可展开、复制
- **AND** 页面 SHALL NOT 将日志内容作为 HTML 执行，也不展示未脱敏原文

#### Scenario: 查询失败和空结果
- **WHEN** 查询返回空结果、加载失败、422 字段错误或 403
- **THEN** 页面 SHALL 分别显示空结果、可重试失败、字段错误或权限说明
- **AND** 页面 SHALL 保留当前筛选和页签，不把失败显示为成功或空数据

#### Scenario: 热表边界
- **WHEN** 用户查询审计日志
- **THEN** 页面 SHALL 只查询热表，并明确提示结果不包含已归档数据
- **AND** 页面 SHALL NOT 声称覆盖冷表历史

#### Scenario: 审计查询自身的记录
- **WHEN** 用户查看“数据访问”页签
- **THEN** 审计查询接口自身产生的 `data_access` 记录 SHALL 作为普通日志出现
- **AND** 页面 SHALL NOT 自动排除这些记录

#### Scenario: 响应式与可访问性
- **WHEN** 用户在 375px、850px 或 1440px 视口使用键盘或触摸操作
- **THEN** 页签、筛选、表格或移动卡片、分页和详情 Sheet SHALL 可操作
- **AND** 页面 SHALL NOT 产生页面级横向滚动
- **AND** 状态 SHALL 同时使用文字、语义颜色和可访问名称
