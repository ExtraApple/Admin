## 1. 契约基线与端到端启动

- [ ] 1.1 对照 proposal、design、五份 delta spec、ADR 0011／0012 和 `preview.html` 固化首期页面及现有路由／权限码清单，确认 `standardize-api-response-contract` 后端已验收但尚未联合发布
- [ ] 1.2 为新读写接口及扩展的列表／菜单／组织树响应明确 DTO、稳定错误码、权限码与 OpenAPI 形状；为成员写入确认 `access_version` 并发前置条件和范围外关系保留规则
- [ ] 1.3 明确本地真实联调所需的 Go 服务、MySQL／Redis／MinIO、开发代理和一个可用管理员／范围受限用户的数据准备步骤；不把演示账号作为产品数据

## 2. 后端 model／repository／dto

- [ ] 2.1 在 Authorization 仓储与 DTO 增加角色按 ID 读取、按用户读取／替换真实角色 ID 关系及分页批量角色摘要的最小能力；避免用 `users.role` 作为角色来源
- [ ] 2.2 在 Identity 用户查询仓储与 DTO 扩展按用户名／昵称关键词和状态的范围内分页筛选、管理员按 ID 详情、真实 `roles[]`／范围内 `organizations[]`／`has_unmanaged_organizations`／`access_version` 响应
- [ ] 2.3 在 Organization 仓储与 DTO 增加按用户批量读取成员关系、区分范围内／范围外组织和逐人差量修改所需的最小能力，保留既有关系 `joined_at`
- [ ] 2.4 为组织树 DTO 增加 `manageable`，为角色菜单管理员 DTO／查询准备已分配菜单平铺列表（含停用项、`parent_id`、`status`），不改变用户上下文菜单 DTO
- [ ] 2.5 在授权版本仓储增加事务内按目标用户锁定版本并检查期望值的最小接口，缺少版本行时按现有 Ensure 规则初始化；不增加新表或历史数据迁移

## 3. 后端 application／service

- [ ] 3.1 在 Authorization service 实现按 ID 读取角色及逐人角色归属替换：验证目标用户范围和受保护身份、角色存在及 `admin` 角色禁止改派，事务内检查版本、修改单个用户关系并提升版本
- [ ] 3.2 在 Identity service 通过 Authorization／Organization 最小查询端口批量组装当前页及详情的真实归属、范围外组织提示和授权版本；筛选与总数均只对可见用户生效，无 N+1 查询
- [ ] 3.3 在 Organization service 实现树节点可管理标志、逐人组织归属差量写入：校验目标用户和所有候选组织，范围外已有关系与其他用户关系不变，加入时间规则和版本更新在同一事务内成立
- [ ] 3.4 在 Navigation service 将管理员角色菜单读取改为按排序／ID 返回全部显式分配的平铺菜单（含停用、缺父级的子菜单），保持 `/api/user/context` 的原有启用／权限过滤
- [ ] 3.5 增加后端 service／repository 行为测试：分页条件与可见范围、真实多角色和隐藏组织标志、本人／受保护用户拒绝、无效角色／组织拒绝、缺父级菜单、并发版本冲突与只一次版本提升、未改关系的 `joined_at` 保留

## 4. 后端 handler／router／OpenAPI

- [ ] 4.1 在拥有模块的 HTTP adapter 注册 `GET /api/admin/roles/:id` (`admin.roles.id.get`) 与 `GET /api/admin/users/:id` (`admin.users.id.get`)，扩展 `GET /api/admin/users` 查询参数和响应
- [ ] 4.2 注册 `PUT /api/admin/users/:id/roles` (`admin.users.id.roles.put`) 与 `PUT /api/admin/users/:id/organizations` (`admin.users.id.organizations.put`)；校验请求中的显式数组及 `expected_access_version`，对过期写入返回 409／`AUTHZ_CONFLICT` 或 `ORG_CONFLICT`
- [ ] 4.3 更新组织树／管理员角色菜单路由响应 Schema、Route Catalog、API 元数据／权限码同步与 OpenAPI；保持既有角色或组织整组分配路由行为不变
- [ ] 4.4 增加 HTTP／Route Catalog 测试，覆盖新路由 401／403、404、422、409、200 的真实行为、输出四字段信封、范围外组织不泄露、菜单已停用与缺父级场景、DTO／OpenAPI 与路由权限码一致

## 5. 前端工程和共享 API 契约

- [ ] 5.1 在 `web/` 初始化 npm 锁文件、Vite + React + TypeScript + Tailwind CSS；按 ADR 0011 显式选用 Radix 基元的 shadcn/ui 按需组件，配置 `/api`、头像和 `/docs` 开发代理与可直达页面的 SPA 回退
- [ ] 5.2 在 `web/` 只建立一套集中 API Client：四字段信封解包、HTTP/status 一致性、稳定 `error_code` 本地化、安全 fallback、422 字段级错误与原生头像／下载／OpenAPI 路径；对应完成 `standardize-api-response-contract` 的 8.1–8.5、8.7–8.8
- [ ] 5.3 实现会话内存存储、登录刷新退出及 401／403 边界；用真实 `/api/captcha` 一次性验证码和注册、锁定倒计时与手动重试完成认证表单及字段错误，对应完成该 change 的 8.6
- [ ] 5.4 增加共享 API Client、错误映射、字段提示、登录锁定／未知码／原生资源行为测试，执行该 change 的 Frontend Acceptance 9.1–9.4 并记录同一份验证证据

## 6. 前端授权与页面纵切

- [ ] 6.1 实现正式路由、直接访问／后退、登录后目的地址、当前用户资料、无可用管理菜单说明页；仅按真实菜单与权限显示支持的管理入口和操作，保留服务器最终授权裁决
- [ ] 6.2 实现角色服务端分页列表、按 ID 详情和权限／已分配菜单／数据范围并列剖面；对 `admin` 角色禁用写入，对停用或权限缺失的已分配菜单只标配置风险
- [ ] 6.3 实现角色权限、菜单、自定义数据范围三个独立编辑页：从真实分配关系预选、单节点勾选、摘要和旧会话影响确认、分别保存、失败保留选择与未保存离页确认
- [ ] 6.4 实现用户服务端关键词／状态筛选与分页、真实多角色和可见组织展示、隐藏关系提示、按 ID 详情，以及受保护用户不可修改的操作状态
- [ ] 6.5 实现用户逐人角色／组织独立编辑及启停／下线／删除确认；使用目标用户版本提交各自 PUT，409 先重载核对，绝不调用整组成员覆盖接口模拟逐人操作
- [ ] 6.6 实现组织树及仅供定位祖先、组织详情／成员空态、修改上级路径确认和逐人成员加入／移除；成员修改复用用户定向组织 PUT，失败不伪造成功
- [ ] 6.7 将样稿的冷白／深海青、桌面宽松用户行、平板／手机卡片和树详情切换落入实际页面；操作浮层出现在按钮右侧、不撑高布局、不挡下一人按钮，点击外部、切换用户或滚动自动关闭
- [ ] 6.8 补齐可访问标签、键盘焦点、确认弹窗、窄屏无横向溢出及真实空／错误／加载状态；正式应用不包含演示按钮、模拟失败或硬编码业务数据

## 7. 文档、联合验收和发布门槛

- [ ] 7.1 更新 README、前端启动／构建／同源部署与 API Client 说明，并同步 ADR 0011／0012 的实施状态及新路由 OpenAPI／受影响长期规格导航；保留 `preview.html` 仅作参考
- [ ] 7.2 运行 `go test ./... -count=1`、前端类型检查／完整测试／生产构建，验证新增后端权限、事务及页面真实数据，确认原菜单与整组分配调用者未被破坏
- [ ] 7.3 运行真实 Go + `web/` 浏览器联调：验证码错误／登录锁定／公开注册／无管理菜单、角色三维授权、用户多角色与逐人并发冲突、范围外祖先与组织成员、用户菜单弹层的桌面／平板／手机行为
- [ ] 7.4 使用真实响应和共享契约 Fixture 核对信封／字段错误／未知码、头像／下载／原始 OpenAPI 例外；回填 `standardize-api-response-contract` Integration Acceptance 10.1–10.7，只有两份 change 的相关验收全部通过才可标记可发布／归档
