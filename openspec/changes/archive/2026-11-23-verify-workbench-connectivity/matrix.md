# 工作台前后端集成验收矩阵

## 1. 环境与角色

| 组件 | 地址／端口 |
|---|---|
| 后端 | `http://127.0.0.1:18080` |
| Vite | `http://localhost:15173` |
| MySQL 8.0 | `127.0.0.1:13306`，库 `admin_workbench_acceptance` |
| Redis 7 | `127.0.0.1:16379` |
| MinIO | `127.0.0.1:19000/19001` |
| Mailpit | `127.0.0.1:11025/18025` |
| RabbitMQ | `127.0.0.1:15672/15673` |

| Actor | 角色与权限 | 数据范围 | 预期导航 |
|---|---|---|---|
| `admin` | `admin`，全部权限 | 全局 | 授权核对、身份与组织、配置管理、审计 |
| `manager` | `manager`：`admin.users.get`、`admin.users.id.get`、`admin.users.id.roles.put`、`admin.users.id.organizations.put`、`admin.roles.get`、`admin.roles.id.get`、`admin.organizations.get`、`admin.organizations.tree.get`、`admin.organizations.id.put`、`admin.organizations.id.users.get`、`admin.organizations.id.users.post` | 自定义组织 A | 角色、用户、组织；无授权核对、菜单、接口、审计 |
| `viewer` | `user`，无管理权限 | 仅本人 | 无管理页；可进入个人资料 |
| `disabled` | `user`，状态停用 | 无 | 登录拒绝；原会话后续请求失效 |

组织 A 内目标用户 `user-a`；组织 B 内目标用户 `user-b`；manager 只能管理 A。

## 2. 页面矩阵

所有页面公共断言：无未解释控制台错误；受保护页面不闪现未授权内容；直接访问未授权路由被页面守卫拦截；网络请求使用统一信封（原生响应除外）；每行记录截图或网络证据。

| ID | 页面／路由 | Actor | 关键调用 | 读取／渲染期望 | 写操作深度 |
|---|---|---|---|---|---|
| P-01 | `/login` | 匿名 | `GET /api/captcha`、`POST /api/login`、`POST /api/refresh` | 成功登录；Access 只在内存，Refresh 只在 HttpOnly Cookie；错误验证码／密码显示稳定错误 | 错误密码、错误验证码、锁定／限流 |
| P-02 | `/register` | 匿名 | `GET /api/captcha`、`POST /api/register` | 成功注册；重复用户名／邮箱 409；字段错误 422 | 一次成功＋重复／非法输入 |
| P-03 | `/` 工作台壳 | admin | `POST /api/refresh`、`GET /api/user/context`、`GET /api/avatars/:id` | 恢复会话后再显示受保护内容；导航按权限分组；空分组隐藏 | 刷新恢复、退出、旧会话失效 |
| P-04 | `/profile` | admin／viewer | `GET /api/user/info`、`GET /api/user/avatar-upload-policy`、`POST/DELETE /api/user/avatar`、`GET /api/user/email-verification-policy`、`PUT /api/user/info`、`POST /api/user/email-verifications*`、`PUT /api/user/password` | 当前资料、头像、邮箱验证状态真实；未启用 SMTP 时换绑／重发禁用，确认／取消按契约 | 昵称、头像上传／恢复、邮箱 smoke、改密退出 |
| P-05 | `/authorization-overview` | admin | `GET /api/admin/authorization-overview` | 只展示当前数据范围内总览；manager 无导航，直接访问被拒绝 | 不写 |
| P-06 | `/authorization-risks` | admin | `GET /api/admin/authorization-risks?page&size&kind&resource&keyword` | 筛选、分页、空结果、错误态；manager 403／守卫 | 不写 |
| P-07 | `/roles`、`/roles/:id`、权限／菜单／数据范围编辑 | admin | `GET/DELETE/PUT /api/admin/roles*`、`GET/POST /api/admin/roles/:id/permissions`、`GET/POST /api/admin/roles/:id/menus`、`GET /api/admin/menus`、`GET /api/admin/permissions`、`GET /api/admin/organizations/tree` | 列表、详情、分页、权限／菜单／范围回显 | 编辑权限、菜单、数据范围各一次；manager 只读列表／详情，写 403 |
| P-08 | `/users`、`/users/:id`、角色／组织编辑 | admin／manager | `GET /api/admin/users`、`GET/PUT /api/admin/users/:id`、`PUT /api/admin/users/:id/roles`、`PUT /api/admin/users/:id/organizations`、`PUT /api/admin/users/:id/status`、`PUT /api/admin/users/:id/kick`、`GET /api/admin/roles`、`GET /api/admin/organizations/tree` | admin 全部；manager 只见 A 内用户；B 用户不可见或 403 | admin 角色／组织／启停／踢下线；manager A 内角色／组织写，B 403 |
| P-09 | `/organizations`、`/organizations/:id`、移动 | admin／manager | `GET /api/admin/organizations/tree`、`GET /api/admin/organizations`、`GET/PUT/DELETE /api/admin/organizations/:id`、`GET/POST /api/admin/organizations/:id/users` | 树、列表、详情、成员；manager 只见 A；B 不可见／403 | admin 移动／编辑；manager A 内移动／成员写，B 403 |
| P-10 | `/menus` | admin | `GET/POST/PUT/DELETE /api/admin/menus*`、`POST /api/admin/menus/impact-preview`、`GET/POST /api/admin/menus/:id/apis`、`POST /api/admin/menus/sync` | 树、详情、类型字段、绑定关系、同步预览；manager 无入口／API 403 | 创建／修改／删除预览＋fingerprint、绑定／清空、同步 |
| P-11 | `/apis` | admin | `GET/PUT/DELETE /api/admin/apis*`、`POST /api/admin/apis/impact-preview`、`GET /api/admin/api-groups`、`GET /api/admin/api-methods`、`POST /api/admin/apis/sync`、`POST /api/admin/apis/sync-permissions`、`POST /api/admin/apis/:id/menu-button` | 分页、筛选、RouteState、只读路由字段；manager 无入口／API 403 | 元数据编辑、删除预览、同步、生成按钮菜单 |
| P-12 | `/audit-logs` | admin | `GET /api/admin/audit-logs`、`login-logs`、`operation-logs`、`permission-logs`、`data-access-logs` | 分类、分页、时间／方法／状态／路径／用户筛选；manager 无入口／API 403 | 不写；空结果、非法筛选 422 |
| P-13 | `/no-management`、`*` | viewer／匿名 | `GET /api/user/context` | viewer 只显示访问说明和个人资料；未知路由 404 | 不写 |
| P-14 | `/docs`、`/docs/openapi.json`、头像／下载 | public／admin | `GET /docs`、`GET /docs/openapi.json`、`GET /api/avatars/:id`、`GET /api/files/:id/download` | HTML／JSON／blob 原生响应；错误仍为错误信封；公开头像不加 Bearer | 下载、缺失文件、未授权 |

## 3. API 契约矩阵

| 层 | 范围 | 期望 |
|---|---|---|
| 前端引用 API | 约 59 个路径 | 每个至少一次成功；统一信封键为 `code,data,error_code,msg`；`code` 等于 HTTP 状态；分页／筛选字段一致 |
| Route Descriptor | 约 94 个 | Method／Path／Access／Permission／Audit／OpenAPI 元数据完整；Gin 注册与 Catalog 一致 |
| 认证 | `/api/login`、`/api/refresh`、`/api/user/logout`、`/api/user/context` | 精确空对象、Origin、`X-Admin-Request`、Content-Type；Refresh 仅 Cookie；Access 只在响应／内存 |
| 错误 | 401、403、404、405、409、422、429、503 | 稳定 `error_code`；不泄露密码、token、SQL、堆栈；字段错误结构正确 |
| 原生 | `/docs`、`/docs/openapi.json`、头像、文件下载 | 不按 JSON 信封解析；缺失／未授权仍为信封错误 |

代表错误样例：

| 场景 | 请求 | 预期 |
|---|---|---|
| 缺少 Access | `GET /api/admin/users` | `401 AUTHN_UNAUTHORIZED` 或既有认证错误 |
| Access 过期／Refresh 失败 | 业务请求或 `/api/refresh` | 受控 401；页面回到登录，不显示伪造成功 |
| manager 访问菜单／接口／审计 | 直接访问 `/menus` 或对应 API | 页面守卫；API `403` 权限错误 |
| 非法字段 | 用户／角色／组织更新 | `422`＋字段错误；原状态不变 |
| 冲突／过期 fingerprint | 菜单／API 影响预览或删除 | `409`／冲突错误；不自动重放 |
| 登录／邮箱限流 | 连续错误登录或签发 | `429`＋`Retry-After`／安全数据 |
| SMTP 关闭 | 邮箱换绑／重发 | `503 IDENTITY_EMAIL_VERIFICATION_DISABLED`；确认／取消仍可用 |
| 不存在或错误方法 | 随机 API／错误 Method | `404`／`405`；不得回退为 SPA HTML |

## 4. 失败与弱网矩阵

| ID | 注入 | 操作 | 期望 |
|---|---|---|---|
| F-01 | `Offline` 首屏 | 打开登录／工作台 | 明确网络失败；不显示已登录或受保护内容；可恢复重试 |
| F-02 | `Offline` 保存 | 资料／角色／用户编辑保存 | 不显示成功；保留草稿；恢复后重试一次 |
| F-03 | `Slow 3G` 登录 | 提交登录 | 加载态、按钮防重复；成功／失败只出现一次 |
| F-04 | `Slow 3G` 保存＋重复点击 | 角色／用户／菜单提交 | 不重复创建；旧响应不覆盖新状态 |
| F-05 | Access 过期 | 页面内发起业务请求 | 受控刷新或回登录；不误报成功；新上下文生效 |
| F-06 | Refresh 无效 | 刷新／恢复 | 清除会话，显示登录；不闪现受保护数据 |
| F-07 | manager 访问未授权 | 直接 URL＋API | 页面守卫＋API 403；导航不显示入口 |
| F-08 | 422 字段错误 | 用户／角色／组织表单 | 字段错误定位；草稿保留；无成功状态 |
| F-09 | 409 冲突 | 菜单／API 预览、stale 更新 | 冲突提示；不自动重放；重新读取后重试 |
| F-10 | 后端断连 | 任意读取／写入 | 网络错误；不伪报成功；恢复后重试 |
| F-11 | Redis 停止 | 登录／刷新／会话恢复 | 不把依赖故障当凭据无效；保留重试路径；恢复后验证 |
| F-12 | 停用用户 | 登录／已登录请求 | 登录拒绝；旧 Access 后续请求失效；不保留管理内容 |

## 5. WebSocket 矩阵

| ID | 操作 | 期望 |
|---|---|---|
| W-01 | 认证用户 `POST /api/user/messages/ws-ticket` | `200`，ticket 绑定用户且一次性 |
| W-02 | 使用有效 ticket 升级 `/api/user/messages/ws` | `101` WebSocket 协议升级；非 JSON 信封 |
| W-03 | 缺失／重复／过期 ticket | 升级前拒绝；`401`；不建立连接 |
| W-04 | 未认证直接升级 | `401`；无协议升级 |
| W-05 | 断线后按 cursor 重连 | 恢复；不重复投递已确认事件 |
| W-06 | 创建／更新消息后刷新 | 刷新事件到达；无 RabbitMQ 时标记该行未执行 |

## 6. 可访问性与性能矩阵

| ID | 页面 | 检查 | 门禁 |
|---|---|---|---|
| N-01 | `/login` | Lighthouse a11y、trace 冷加载 | critical／serious＝0；记录 LCP／INP／CLS 基线 |
| N-02 | `/` | Lighthouse a11y、trace 冷加载 | 同上；导航／账号菜单键盘可操作 |
| N-03 | `/users` | Lighthouse a11y、列表／空态 | critical／serious＝0 |
| N-04 | `/users/:id` | Lighthouse a11y、表单／Dialog | critical／serious＝0；焦点返回 |
| N-05 | `/profile` | Lighthouse a11y、表单／头像 | critical／serious＝0 |
| N-06 | 所有页面 | 控制台／未处理 Promise | 无未解释错误 |
| N-07 | 桌面／窄屏、亮／暗、减少动态 | 视口／主题 emulation | 无横向溢出；焦点可见；浮层可用 |
| N-08 | 慢网／离线 | 交互反馈 | 见 F-01～F-04；不因性能测试改变业务断言 |

性能只记录首轮基线，不设置未经验证的数字阈值；a11y critical／serious 为硬门禁。

## 7. Evidence 字段

每个矩阵行至少记录：

- ID、Actor、路由／API、前置夹具。
- 实际 HTTP 状态、`error_code`、关键响应字段。
- 网络请求名称、失败类型、耗时（失败行）。
- 控制台错误／未处理 Promise。
- UI 关键文本或截图路径（脱敏）。
- DB／Redis 状态（涉及写入、会话、ticket、限流时）。
- 预期／实际／通过或未执行。
- 环境、镜像、端口、清理结果。
