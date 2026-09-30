## Context

根目录 `web/` 目前只有不联网的 `preview.html`。ADR 0011 已决定 React + TypeScript + Vite、Tailwind CSS、按需引入 shadcn/ui 并显式选择 Radix UI；ADR 0012 确认用户列表必须呈现真实多角色与组织归属，逐人编辑不能调用角色／组织的整组成员覆盖接口。后端已有 JWT、`GET /api/user/context`、分页角色与用户列表、角色的权限／菜单／数据范围写入、组织树与成员列表；但没有按 ID 读取角色／管理员用户、逐人归属写入，组织树祖先节点也没有可操作标识。`standardize-api-response-contract` 已通过后端验收，前端及联合验收尚未完成；本 change 与其共享前端接入成果而不复制响应协议。

## Goals / Non-Goals

**Goals:**
- 将样稿明确的认证、授权导航、角色／用户／组织管理及响应式交互接入真实后端；支持可直达的列表、详情和编辑页。
- 补全用户归属和角色详情所需的后端契约；逐人写入具备权限、范围、受保护用户和并发安全保障。
- 在同一真实前后端链路完成可验证的认证、管理操作、错误状态与响应契约验收。

**Non-Goals:**
- 管理员创建用户，管理其他资源模块或把 `preview.html` 改成生产入口；不复刻样稿里的假数据、假验证码、预览按钮和模拟失败开关。
- 重写现有角色／组织整组分配接口、引入第二套 API 信封或让 `users.role` 代表实际角色。
- 增加数据库表、迁移存量关系、引入新 Redis key／MinIO bucket，或在本 change 单独发布已变更的后端协议。

## Decisions

### 1. 前端边界、路由与视觉

在 `web/` 初始化 npm 管理的 Vite React/TypeScript 应用。保留 `preview.html` 单独可打开作为视觉参考；把配色、字阶、宽松用户行、卡片断点和右侧不撑行的操作浮层落到主题变量及按需组件。正式页面用真实状态／权限，不复制样稿 DOM 或演示用 JavaScript。页面按职责拆成认证与公共个人资料、角色、用户和组织模块，详情／编辑使用 URL 参数，可刷新和直接打开；浏览器后退与未保存确认一致。选择 npm 和最少必需的路由／组件依赖；版本在实施时按项目锁文件固定。不同于整套后台模板，组件只是基本交互基元，避免盖掉视觉基线。

开发环境 Vite 代理 `/api`、头像及 `/docs` 到现有 Go 服务，生产环境由静态站点与 API 共源的反向代理提供 SPA 回退；不把前端强制塞入 Go handler，也不要求跨域持久凭据。Access/Refresh Token 仅保存在页面运行内存，不写本地持久存储；刷新页面重新登录，并保留原目的页用于登录成功后跳转。退出调用现有 `/api/user/logout`，清空客户端状态。角色／用户／组织页面只暴露用户上下文菜单允许且有对应权限码的功能；后端仍是授权最终裁决者。没有本工作台支持的管理菜单时显示专用说明页，仍允许 `/api/user/info` 资料入口和退出，不虚构审批流程。

### 2. 单一 HTTP 客户端与既有 change 的责任

前端请求只通过 `standardize-api-response-contract` 的集中 Client：成功解包四字段信封，按稳定 `error_code` 映射中文提示及字段错误，保留安全英文 fallback；原生头像／下载／OpenAPI 走明确非信封入口。验证码图片使用后端返回的 ID 与 base64 图像，登录或注册失败后按验证码一次性语义重新获取；429 使用 `Retry-After`／安全详情展示剩余时间，不自动提交重试。401／授权版本变化清空令牌并返回登录，403 保留当前页面并显示无权限。构建 Client 与错误处理时同步完成既有 change 的 Frontend Part 8.x、Frontend Acceptance 9.x，并在真实联调时完成其 Integration Acceptance 10.x；同一份实现和验收证据回填两份任务表，不创建平行 Client。

### 3. 读模型和路由所有权

`GET /api/admin/roles/:id` 由 Authorization 提供单角色身份／状态／数据范围详情，受 `admin.roles.id.get` 保护。Identity 扩展 `GET /api/admin/users?page=&size=&keyword=&status=` 的列表项为真实 `roles[]`（稳定 ID／编码／名称）、可管理的 `organizations[]`（稳定 ID／名称）以及 `has_unmanaged_organizations`；当目标用户还有范围外归属时该标志为 true，页面须明确显示有其他不可见归属，不能把当前数组称作完整归属。服务器按用户名／昵称和状态在数据范围内分页筛选，`total` 对应筛选结果；`GET /api/admin/users/:id` 返回同样的归属投影与该用户的 `access_version`，受 `admin.users.id.get` 保护并检查目标是否可见。列表读取批量收集当前页用户 ID，分别经 Authorization／Organization 的只读端口读取归属，避免逐用户查询和跨模块直接读表。所有写入仍由拥有关系的模块处理；路由定义归入各自 Route Catalog，OpenAPI 描述明确新旧字段和错误码。

`GET /api/admin/organizations/tree` 给每个返回节点加入 `manageable: boolean`：范围外祖先为 false，仅用于定位；范围内节点为 true。组织详情继续使用现有组织列表／成员端点，不允许用祖先占位节点触发详情、移动或成员写入。管理员 `GET /api/admin/roles/:id/menus` 返回按 `sort asc, id asc` 排序的平铺已分配菜单项（包括已停用者及 `parent_id`／`status`）；子菜单即使未分配父级也不能在树组装时丢失。编辑器用 `/api/admin/menus` 的完整菜单树显示层级，并仅按角色菜单响应中的 ID 预选，不将仅作定位的父级自动计为已分配；`GET /api/user/context` 继续只返回用户实际可见的已启用菜单。

### 4. 逐人归属写入和并发边界

Authorization 提供 `PUT /api/admin/users/:id/roles`（权限码 `admin.users.id.roles.put`），请求 `{role_ids: number[], expected_access_version: number}`；Organization 提供 `PUT /api/admin/users/:id/organizations`（权限码 `admin.users.id.organizations.put`），请求 `{organization_ids: number[], expected_access_version: number}`。逐人 PUT 只替换目标用户在相应维度的归属，空数组为显式清空；请求需要完整该维度的可管理集合。成功响应返回新的 `access_version`，便于随后编辑另一个维度；每次提交只使该用户旧会话在后续请求失效。组织详情的逐人加入／移除复用同一组织 PUT：先读取目标用户详情、变更其可管理组织集合，然后提交，绝不读取整组成员再覆盖整个组织。

两种写入都在事务中锁定目标用户的 `user_access_versions` 行，比较 `expected_access_version` 后才更新关系并提升授权版本；过期返回 HTTP 409 和模块所有的 `AUTHZ_CONFLICT`／`ORG_CONFLICT`，不部分提交。验证目标仍在操作者可管理用户范围，拒绝修改本人及已有 `admin` 角色的受保护用户；拒绝管理端通过逐人写入增删 `admin` 角色。组织写入验证所有新增目标为实际可管理节点，保留目标用户范围外的现有组织关系及其 `joined_at`，仅对可管理关系做差量调整；保留未移除关系的 `joined_at`，重新加入生成新时间。角色写入校验角色存在且仅改变该用户的 `user_roles`。无新表和迁移；已有版本行在首次读取时按既有 Ensure 逻辑初始化，版本检查和写入同一事务，避免读后覆盖。

### 5. 页面行为与失败边界

角色详情并列呈现权限码、已分配菜单和数据范围；分开的编辑页分别调用现有权限／菜单／数据范围接口，各有变更摘要和旧会话失效确认，失败保留选择并允许重试。自定义范围只选明确可管理的组织节点，勾选父级不隐式勾选下级；缺少权限码的已分配菜单仅标记配置风险，不显示角色级“用户可见菜单数”。用户列表使用真实服务端搜索／状态筛选与分页，受保护用户可查看但不可修改；移动端为卡片，快捷操作右侧浮层点击外部、切换用户或滚动收起且不改变行高。用户归属两个编辑页分别保存；409 须先重新读取、呈现冲突，不自动覆盖。组织树与详情按桌面分栏／手机树详情切换，展示成员空态，移动组织时显示新旧路径并由服务端拒绝循环。请求失败不得显示成功；离开未保存编辑页需二次确认。组件保持可读标签、键盘访问与窄屏无横向溢出。

## Risks / Trade-offs

- [两个 change 共享前端验收] → 以 `standardize-api-response-contract` 为唯一信封契约；实施时同步两套任务勾选，联合验收未过不得标记两者可发布。
- [管理者范围内可见用户可能同时属于不可管理组织] → 读写只编辑可管理关系，服务端保留隐藏关系与加入时间；禁止前端全量覆盖或将不可见误认为空。
- [并发修改与角色改动引起版本变化] → 版本在事务中锁定比较，返回 409 并要求重新加载；不静默重试。
- [样稿的假入口与现有后端路由不一致] → 仅把布局与状态作为参照；真实菜单、权限、验证码、错误和写入结果来自 API。
- [内存令牌使整页刷新后需要重新登录] → 优先避免持久化敏感令牌；保存原目的 URL，成功登录后继续，并在操作文档说明。

## Migration Plan

1. 先扩充各拥有模块的只读／逐人写入契约、权限码及 Route Catalog/OpenAPI，用现有数据表和授权版本验证；保留原整组接口供其他调用者使用。
2. 初始化 `web/` 并完成共用 API Client、认证与页面纵切片；替换所有演示数据与预览动作，不替换 `preview.html` 参考文件。
3. 完成后端、前端和真实联调验证；回填两个 change 的相关验收任务。上线时同步部署 SPA 和同源 `/api` 代理，旧后端不得作为已完成统一响应契约单独发布；回滚前端需与已发布的后端响应版本保持一致。

## Open Questions

无阻塞性未决项。界面视觉以当前 `web/preview.html` 和 ADR 0011／0012 为准；依赖的具体包版本在初始化锁文件时固定。
