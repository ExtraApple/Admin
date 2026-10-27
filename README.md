# Admin

本项目包含 Go + Gin + GORM 后端及 `web/` 中的 React + TypeScript 管理工作台。已发布行为以 `openspec/specs/` 为准，实施中的变化与验收门禁以 `openspec/changes/` 为准；代码存在不等于联合验收通过。

## 代码结构

项目采用业务模块优先的分层单体。`main.go` 只启动 `internal/app`；App 是唯一组合根、Gin 业务路由注册点和 AutoMigrate/Seed 编排入口。配置、日志、数据库、Redis 和 MinIO 构造集中在 `internal/platform`。

业务实现位于固定的 `internal/identity`、`authorization`、`navigation`、`apimetadata`、`organization`、`dictionary`、`files`、`audit`、`routecatalog`、`apidoc` 和 `uploadsecurity` 模块。复杂模块按 `adapters -> application -> domain` 依赖，跨模块调用使用调用方声明的最小 Contract。

详细所有权、推荐阅读入口和架构验证命令见 [业务模块代码导航](docs/module-navigation.md)。完整文档入口见 [docs 使用说明](docs/README.md)，领域术语见 [CONTEXT.md](CONTEXT.md)，长期架构决策见 [docs/adr/](docs/adr/)，当前行为规格见 [openspec/specs/](openspec/specs/)。

## 配置

项目使用 `config.yaml + .env` 混合配置：

- `config.yaml` 保存配置结构和非敏感默认值。
- `.env` 保存密码、密钥等敏感值。
- `.env.example` 是模板，可以提交到 Git。
- `.env` 已加入 `.gitignore`，不要提交真实密码。

首次本地启动前复制 `.env.example` 为 `.env`，并填写下列配置；已有 `.env` 时只补充缺少的项，不覆盖现有凭据：

```env
MYSQL_PASSWORD=
REDIS_PASSWORD=
MINIO_USERNAME=minioadmin
MINIO_PASSWORD=minioadmin
JWT_SECRET=
ADMIN_PASSWORD=
```

邮件服务默认关闭，`config.yaml` 中的 `smtp.enabled: false` 不读取 SMTP 凭据，也不创建邮件发送器，不影响正常启动和登录。需要发送邮箱验证邮件时，将开关改为 `true`，配置真实 SMTP 地址／端口，并在 `.env` 或部署 Secret 中提供非空的 `SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM`；启用后缺少凭据或配置无效仍会阻止启动，不自动降级。已有 SMTP 部署也必须显式设置 `enabled: true`，仅填写 `host` 不再启用邮件。

Linux、Docker、Kubernetes 部署时不必依赖 `.env` 文件，可以直接注入同名环境变量，详细示例见 [docs/runbooks/configuration.md](docs/runbooks/configuration.md)。

超级管理员会在启动时按新 RBAC 体系自动兜底创建，管理员身份只看：

```text
users -> user_roles -> roles.code = "admin"
```

## 启动

```bash
go run .\main.go
```

本地 API 文档：

```text
http://localhost:8080/docs
http://localhost:8080/docs/openapi.json
```

MinIO 本地启动示例：

```bash
minio.exe server D:\WORK\minio\data --console-address ":9001"
```

## 管理工作台

正式入口为 `web/index.html`；`web/preview.html` 只是静态视觉参考，不是登录或业务数据入口。应用使用 React、TypeScript、Vite、Tailwind CSS 和基于 Radix UI 的按需 shadcn/ui 组件，npm 版本与依赖以 `web/package.json`、锁文件为准。

工作台侧栏入口按“授权核对”“身份与组织”分组，只显示当前用户可访问的入口，空分组不显示。桌面侧栏固定 228px；850px 及以下使用导航 Sheet。首次挂载只展开当前路由所属分组，后续路由切换不重置用户选择，也不持久化分组状态。未保存编辑的导航确认取消后保留编辑与 Sheet；确认离开后关闭导航并恢复菜单按钮焦点。

右上角头像／昵称用于展开账号菜单，提供“个人资料”和“退出登录”；不再单独显示顶部退出按钮或侧栏“账户”分组。个人资料保留原页面与 Breadcrumb 名称，没有管理权限的用户仍可通过账号菜单访问。退出不新增二次确认，遵守页面原有未保存修改保护；允许离开后先记录退出意图并清理内存，真实 Cookie 登出失败则显示“服务端退出未确认”和重试入口，不宣称退出成功。菜单支持键盘操作、Escape 和点击外部关闭，窄屏保留可访问的头像触发器。
个人资料页 `/profile` 的昵称、头像和密码保存彼此独立：昵称只提交 `nickname`，要求非空、非全空白且最多 100 个 Unicode 字符，并保留原始首尾空白和大小写；头像先本地预览，读取实际生效的 `GET /api/user/avatar-upload-policy` 后才启用选图与上传，规则不可用时不采用默认上限，恢复默认仍可用；密码新值在 bcrypt 前按不少于 6 个 Unicode 字符、UTF-8 不超过 72 字节和至少 3 类字符校验。密码成功响应先清除敏感输入，再复用现有 Cookie Logout；Logout 未确认不会宣称成功，待确认标记跨刷新保留，只有退出成功或显式重新登录并完成 context 后才清除。

本次个人资料自助的真实 HTTP、Redis／MinIO、浏览器多标签和故障恢复证据见 [`complete-profile-self-service/evidence.md`](openspec/changes/complete-profile-self-service/evidence.md)。

正式前端在HTML内容和React挂载前同步加载同源`/theme.js`，提供“亮色”“暗色”“跟随系统”三种外观模式。所选模式仅按当前浏览器、同一站点保存在`admin-workbench.theme`；它不绑定用户、不保存Token、不改变导航分组或业务数据。系统模式跟随`prefers-color-scheme`，固定模式不受系统变化覆盖；同源其他标签通过浏览器`storage`事件同步，不刷新页面、不重建业务页面。

在 `web/` 目录执行：

```powershell
npm ci
npm run dev
npm run typecheck
npm test
npm run build
npm run preview
```

Radix primitives 使用同一兼容发布系列，Sheet、确认框、Select 等浮层需要共享 FocusScope 和 DismissableLayer。升级依赖后先停止旧的开发服务器，再执行 `npm run dev -- --force` 刷新 Vite 预构建缓存；旧缓存仍可能保留重复的浮层核心，仅安装成功或 HMR 刷新不足以验证嵌套浮层。完整刷新后通过 Cookie 与真实上下文恢复会话，明确失效才要求重新登录。

`dev` 启动真实应用，`typecheck` 检查类型，`test` 运行 Vitest，`build` 输出 `web/dist/`，`preview` 仅本地预览构建产物，不是生产服务器。开发代理默认转发 `/api`、`/avatars`、`/docs` 到 `http://127.0.0.1:8080`。后端端口不同时，在启动 Vite 前设置：
构建产物必须同时包含可直接获取的`/theme.js`静态脚本和SPA资源；`/theme.js`不是业务API或权限路由。主题切换的真实页面、Portal、原生控件和编辑状态验收由对应OpenSpec change记录，不以构建成功替代浏览器验收。

```powershell
$env:ADMIN_API_TARGET = 'http://127.0.0.1:18080'
npm run dev
```

该变量配置开发代理目标，不把跨域后端地址写入浏览器客户端。生产同源反向代理必须优先把 `/api`、`/api/*`、`/docs`、`/docs/*` 及头像资源转发给 Go 服务，再为前端路由提供 `index.html` SPA 回退；API 404/405 和文档错误不得回退成 HTML。部署 `web/dist/`，不发布样稿或把 Vite 开发服务器用于生产。

Access Token 和用户上下文只保存在页面内存；Refresh 仅由 Host-only、`Path=/api`、HttpOnly、SameSite=Strict 的 `admin_refresh` 会话 Cookie 持有，HTTPS 必须 Secure，不设置长期 Expires／Max-Age。Refresh 不在 JSON 或 JavaScript／Web Storage 中。启动先 Cookie Refresh，再获取真实用户上下文，完成前只显示确认状态，不闪现登录或受保护内容。关闭浏览器不保证服务端退出，浏览器恢复可能保留 Cookie。

Login／Refresh／Logout 都要求精确的可信页面 Origin、JSON 和 `X-Admin-Request: 1`；Refresh 另需不可预测的 `X-Refresh-Attempt-ID`。Refresh 与 Logout body 均为 `{}`，不需要 Access；业务请求仍使用 Bearer，不自动重放。登录 data 为 `access_token / session_id / user`，Refresh 为 `access_token / session_id`，退出为 `null`。协议、Cookie jar 示例和 401／403／409／500 处置见[登录安全策略](docs/runbooks/login-security.md)。

工作台要求安全上下文、原生 Web Locks 和可用 localStorage，无能力时明确不可用，不降级为仅内存会话。三个 `admin-workbench.auth.*` 存储 key 只协调非凭据事件、续期尝试和退出待确认，不持久化用户或权限。退出失败标记跨刷新／标签保留并阻止自动登录；只在确认退出或用户真实重新登录且 context 成功后清除。Access 仍有效的续期故障保留编辑，到期挂起不卸载；手动恢复需同 SID／上下文，新身份清除旧编辑。403 权限拒绝仍保留页面，注册不自动获得管理权限，无菜单用户可查看个人资料并退出。

根配置 `browser_session.origin: http://localhost:5173`、`allow_loopback_http: true` 仅用于 Vite 回环开发，不是上游 8080 Origin。`ADMIN_API_TARGET` 不改变可信页面 Origin；换页面主机／端口须同步配置。生产设置实际对外 HTTPS Origin、关闭 HTTP 例外，遵守[配置与同窗口切换规则](docs/runbooks/configuration.md#浏览器会话与反向代理)：前后端一次切换、旧 Token 全部重新登录、禁止混跑旧实例；回滚仍必须校验 SID，不能直接回到旧后端。切换不清 Redis、不批量提升授权版本、不覆盖管理员 API Metadata 或回填历史 Public 配置。

### API Client 与发布边界

所有业务 JSON 请求使用 `web/src/lib/api.ts` 的 `api.get/post/put/delete`：只接受 `{code,error_code,msg,data}`，校验 HTTP status 与 `code` 一致，成功要求空 `error_code` 和 `msg=success`，只向页面返回 `data`。无业务值为 `null`，空集合为 `[]`；不兼容旧信封、字段别名或英文消息分支。

`ApiError` 保留 HTTP 状态、稳定 `errorCode`、安全英文 fallback、字段错误和白名单安全详情。中文映射在 `web/src/lib/errors.ts`；未知码只使用经过安全处理的 fallback。422 的 `data.fields[]` 按 JSON 字段名映射，可展示同字段多项错误。客户端只接纳批准的登录剩余次数、锁定／邮箱验证等待秒数，并读取 `Retry-After`；不展示任意错误对象、内部 cause、堆栈、密码、Token 或被拒绝值，不自动重交认证表单。

授权总览使用同一 Client 的 `api.authorizationOverview()` 与 `api.authorizationRisks(query)`：入口为 `/authorization-overview`，完整风险清单为 `/authorization-risks`；后端接口分别要求 `admin.authorization.overview.get` 与 `admin.authorization.risks.get`。页面只读，风险详情仅按当前会话已有的角色／用户读取权限提供链接，不在总览或风险清单执行修复写入。发布时先同步 Route Catalog、权限码和后端 OpenAPI，再发布前端 Client 与页面。

原生成功响应使用显式入口：下载走 `api.blob`，头像走 `api.image`（公开头像也可使用同源只读地址），原始 `/docs/openapi.json` 走 `api.raw`；Swagger `/docs` 直接由浏览器打开 HTML。原生成功不经过信封解析，提交前的错误仍按错误信封处理，已提交的流不能追加 JSON。Client 只接受同源路径，公开认证、头像和文档入口不附加 Bearer Token。

统一响应是 breaking contract。`standardize-api-response-contract` 仅达到 Backend Accepted 时仍为 Release Blocked；必须与 `build-admin-workbench` 共用同一 Client，并完成前端及真实联合验收后才可视为可发布，不单独上线已切换协议的后端。路由／权限／读模型及归属编辑边界见 [用户管理](docs/modules/user-management.md)、[角色管理](docs/modules/role-management.md)、[组织管理](docs/modules/organization-management.md)；隔离联调环境见 [配置手册](docs/runbooks/configuration.md)。

2026-09-30 两份 change 的后端、前端与真实联合验收已通过，记录见[联合验收证据](openspec/changes/archive/2026-10-01-standardize-api-response-contract/evidence/frontend-integration-acceptance-2026-09-30.txt)。[响应基线](openspec/changes/archive/2026-10-01-standardize-api-response-contract/evidence/openapi-2026-09-30.json)按完整 Response 去重保存，`routes.responses` 指向 `responseSchemas` 索引；[错误矩阵](openspec/changes/archive/2026-10-01-standardize-api-response-contract/evidence/error-matrix-2026-09-30.json)记录错误所有者、状态、fallback、安全数据类型与 JSON 字段码。窄屏个人资料的真实长内容截图见[320px 验收截图](openspec/changes/archive/2026-10-01-build-admin-workbench/evidence/profile-320.png)。
个人资料自助的真实联合验收记录见 [`complete-profile-self-service/evidence.md`](openspec/changes/complete-profile-self-service/evidence.md)；该记录只包含已执行且脱敏的命令、状态和观察，不替代 OpenSpec 主规格。
