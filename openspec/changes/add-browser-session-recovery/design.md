## Context

当前 `web/src/lib/session-state.ts` 在模块内保存两种 Token 和用户上下文，完整刷新即丢失；已经具有主动续期、单页单飞和防止晚到续期复活本地状态的逻辑。`internal/identity/application/service.go` 的 Refresh 重新签发一对 JWT，却不消费旧 Refresh；Logout 仅把当前 Access 写入黑名单。JWT 没有登录会话标识，因此仅保存 Token 不能实现安全恢复与当前会话撤销。

当前主规格的三项约定将被显式替换：`admin-workbench` 要求两种 Token 只驻内存并在刷新后登录；`auth` 允许无用途标记的旧 Token、JSON Refresh Token 和 Access 黑名单退出，并把 Refresh 存储故障映射为 401；`route-catalog` 将所有 `/api/user/**` 归为 Authenticated。统一响应的四字段信封、实时授权版本事实和其他业务路由最低身份边界保持不变。

按 ADR 0004/0006/0011 的现行模块与集中 Client 实施。`openspec/config.yaml` 的旧技术目录描述不作为恢复退役 handler/service/router 路径的依据。所有原子会话状态操作在 Identity 的 Redis Adapter 中，Authorization 继续独占 `user_access_versions`；不创建第二套授权版本或跨模块数据库访问。

本设计落实已逐项确认的 Q1—Q17；本轮只记录未来目标，不把静态调查、既有测试或已有部署当成新行为的验收证据。

## Goals / Non-Goals

**Goals:**
- 跨刷新恢复真实会话；身份与权限重新确认前，不闪现登录成功或受保护内容。
- Access 仅驻内存，Refresh 仅经 HttpOnly Cookie 传递；服务端对每次登录有独立、可撤销的事实。
- 一次性续期、30 秒同尝试幂等恢复、跨标签协调、退出防复活和故障分类形成一个完整协议。
- 保留未保存编辑、主题、顶部账号菜单及既有权限失效规则；只有同一会话恢复成功才继续原操作环境。
- 原协议一次切换，真实浏览器与真实 MySQL／Redis／HTTP 故障和竞争场景提供实施证据。

**Non-Goals:**
- 长期记住我、跨域部署、多标签不同用户、全设备普通退出、闲置超时或绝对会话上限。
- 新业务模块、MySQL 会话表、运维会话大盘、历史数据回填、额外维护任务或业务请求自动重放。
- JWT 密码学重写、引入另一套通用认证框架、修改当前用户／角色／组织模型或其他路由权限。

## Decisions

### 1. Identity 拥有登录会话，Authorization 仍拥有授权版本

登录会话是一次成功登录形成的可撤销身份连续性，不是页面实例，也不是授权版本。使用服务端生成的高熵 `sid`；JWT 明确携带 `sid`、用户 ID、`token_version` 和 `token_type`，Refresh 另带不可预测的凭据标识及单调续期代。`session_id` 可作为非授权性的关联信息返回，知道它不能认证、续期或撤销任意会话。

Access JWT 保持既有角色／权限快照字段；Refresh JWT 只承载用户、授权版本、用途、SID、续期代、凭据标识及签发／到期时间，不复制角色／权限集合。Refresh 从实时授权快照签发新的 Access，不能使用 Cookie 内嵌权限授权。完整 Refresh Cookie 须低于浏览器 4 KiB 容量，不能随用户权限数量增长。真实 Chrome 已证明原完整快照会触发 `NameValuePairExceedsMaxSize`；此修正由用户确认，不采用 Cookie 分片或修改 Access 载荷。

Identity Application 定义最小 SessionStore interface，集中完成创建、读取活动会话、原子续期与撤销。Redis Adapter 实现原子状态变化；JWT Adapter 只做现有算法的签名与验证；HTTP Adapter 持有 Cookie／Origin／Header 规则；App 注入依赖。不要让 Handler 或页面自行组合 GET 后 SET，也不把 Redis client、Gin Context 或 MySQL 事务泄漏进 interface。

所有 Access 认证仍验证签名、用途、有效期、用户启用状态和 Authorization 实时快照，并增加活动 `sid` 与用户／授权版本绑定检查。Access 不要求等于最新续期代：正常轮换不得使其他标签仍未到期的 Access 失效；退出、会话缺失或授权版本变化则使该会话凭据失效。JWT 内嵌角色／权限不替代本次实时访问快照。

不采用提升用户授权版本实现普通退出：这会错误地下线其他独立登录。新登录会话也不替代现有强制下线、密码／身份状态及授权变化的版本提升语义。

### 2. Redis 会话事实与有限幂等状态

- `auth:session:<sid>`：用户 ID、登录时授权版本、活动续期代、当前 Refresh 标识摘要、到期时间及必要的当前尝试关联。TTL 跟随实际 Refresh 到期；只有成功签发新代时推进，幂等重取不续 TTL。
- `auth:refresh-attempt:<sid>:<attempt-id>`：已消费凭据摘要、产生的新代、固定签发时间／到期时间、足以重建同一签发结果的材料及 30 秒截止时间。TTL 由首次提交决定，重取不延长。
- 不将原始 JWT、Cookie 或密码放入新 Redis key；Session 只需凭据标识摘要，幂等结果保存固定签发材料而非把可用 Token 长期归档。继续使用现有 JWT 库签发，不能自行实现密码学或在恢复时生成不同时间的新代。
- 创建只发生于真实登录；Refresh 不能在记录缺失时从 JWT 重建 Session。撤销删除活动事实或设置等效终态；任何剩余短期结果都必须先通过活动会话检查，不能借缓存复活。
- 使用现有 go-redis/v9 的原子脚本／事务能力实现比较并推进、结果登记、TTL 及撤销排他，不新增 Redis 部署或 MySQL 表，不通过 SCAN、后台清理或无限历史尝试集合维持正确性。

仓库默认 Access 15 分钟、Refresh 10080 分钟；仍读取现有部署时长并校验为正数。新代的两种到期时间从该次签发计算，Session 同步滑动；没有新增首次登录满 7 天必退的上限。Redis 真实故障为安全 500；成功读取后缺失／终态为认证拒绝，不能退回只验 JWT。

### 3. Cookie-only HTTP 协议，一次切换

| 路由 | 请求 | 成功 data | Cookie／身份规则 |
|---|---|---|---|
| `POST /api/login` | 原用户名、密码、6 字符验证码 JSON；可信 Origin 与 `X-Admin-Request: 1` | `access_token`、`session_id`、脱敏 `user` | 建立新 Session，设置 Refresh Cookie；失败不替换原有效会话 |
| `POST /api/refresh` | 空对象 `{}`；Cookie、上述防护及 `X-Refresh-Attempt-ID` | `access_token`、`session_id` | 消费当前凭据或重取同尝试结果，设置新代 Cookie，不要求 Access |
| `POST /api/user/logout` | 空对象 `{}`；Cookie 与上述防护 | `null` | 不要求有效 Access；撤销当前 Cookie 绑定的会话并清 Cookie，重复退出幂等 |
| `GET /api/user/context` | 原 Bearer Access | 原 `user / roles / permissions / menus` | 完整 Access 认证和实时上下文，不使用 Cookie 直接放行业务请求 |

Refresh／Logout 不接受旧 `refresh_token` 字段或其他来源的 Refresh 凭据。旧 JSON 通道、响应字段及 Token 用途兼容分支删除，禁止用未知字段被忽略来保留隐式兼容；旧协议或损坏传输请求返回 `400 / HTTP_REQUEST_INVALID`。普通业务 Client 继续只向同源路径发送 Bearer。

Refresh Cookie 固定为 `admin_refresh`，Host-only、不设置 Domain、Path 为 `/api`、HttpOnly、SameSite=Strict；HTTPS 时必须 Secure。成功设置时不含长期 Expires／Max-Age；清除时使用相同 Name／Path／Domain 范围及过期删除属性。Cookie 的浏览器寿命不是服务端会话的到期事实，浏览器会话恢复可能保留 Cookie，不能宣称关浏览器就是可靠登出。认证控制响应使用 `Cache-Control: no-store`。

Logout 对没有有效 Cookie、已撤销或已过期的会话仍可幂等清除该 Cookie 并返回成功，不据未验证 claim 删除任何其他会话。真实 Redis 撤销失败返回 500，不清除仍用于重试的 Cookie，不声称服务端退出成功。新的显式登录成功后原子建立新 Session 并撤销该浏览器原有效 Cookie 所指的旧 Session；不得把一个用户的其他浏览器会话都撤销。

### 4. 使用现有标准库防护与严格同源配置

新增 `browser_session.origin`（面向浏览器的规范 Origin）和 `browser_session.allow_loopback_http`（默认 false）。配置不允许路径、query、fragment、userinfo、通配符或 `null`；生产 Origin 必须 HTTPS。只有显式允许且主机为 `localhost` 或 `127.0.0.1` 的 HTTP Origin 才可使用开发 Cookie 例外；不信任任意 Host／X-Forwarded-* 决定白名单或降级 Secure。Origin 表示 Vite／反向代理对外地址，不是代理上游 8080 地址。

对 Login／Refresh／Logout 在处理凭据或改变 Cookie 前执行：精确匹配已配置 Origin、要求 `X-Admin-Request: 1` 和 JSON Content-Type，并复用 Go 1.25 `net/http.CrossOriginProtection.Check`。配置的规范 Origin 用 `AddTrustedOrigin` 明确登记，以适应本机 Vite／生产反向代理上游 Host 与页面地址不同的情况；无不安全 bypass。标准库会允许部分无 Origin 请求，因此必须保留本协议对缺失／null Origin 的显式拒绝。

不增加跨域凭据 CORS 或 CSRF 引导路由，不使用随机 CSRF Token 的自制签名协议。自定义非简单 Header、严格 Origin、拒绝跨域凭据及 SameSite 共同保护 SPA JSON 控制入口。防护失败统一 `403 / AUTHN_CSRF_INVALID`，data=null，不回显 Origin、Cookie 或 Header 值；不清除合法 Cookie，也不因为被拒绝请求撤销其他会话。SameSite 或 HttpOnly 不能被描述为消除了 XSS。

### 5. 续期单次消费与 30 秒结果恢复

每个真实续期由浏览器在发送前保存一个不可预测但不能单独授权的尝试 ID。服务端以 Session、凭据摘要、尝试 ID、代和版本作为比较条件，原子提交新代与固定签发材料。唯一提交者推进一次；其他同尝试请求只能取回已提交结果，不再次调用一个会重新取当前时间的签发流程。

有效重取同时要求：相同尝试、匹配的前代凭据或该尝试已产生的新代凭据、首次提交后的 30 秒内、Session 仍活动且仍为该结果对应代、用户及授权版本仍有效。新 Cookie Header 已到但正文丢失的重试，也能用同尝试的新代凭据重取。若原凭据在提交时有效、随后才到期，只允许在上述窗口内验证其签名／绑定并恢复已经提交的仍有效结果；不得因此用过期凭据新建续期。

以下结果必须区分：
- 当前凭据、全新尝试：提交一代，推进 Session TTL。
- 合法同尝试重取：返回同一代和原到期时间，既不推进代也不延长窗口／TTL。
- 确认的同尝试结果已被其他成功尝试推进到更高代：`409 / AUTHN_REFRESH_SUPERSEDED`，data=null，不设置旧 Cookie、不撤销现有会话。浏览器废弃旧协调信息并提供按新状态手动恢复，不能无限自动循环。
- 不匹配／窗口外的旧凭据重用：撤销当前 Session 并返回统一 401，不影响其他独立登录。
- Session 已撤销／丢失、用户停用、授权版本不一致：统一 401，不得返回缓存成功。
- Redis、数据库、签发真实失败：安全 500。若提交完成但签发／响应失败，同尝试可在窗口内恢复；不是通过回滚到已消费旧代或宽松接受任意旧 Token 来恢复。

前端协调减少合法竞争，但服务端原子性仍需覆盖绕过浏览器锁、多个应用实例和晚到请求。退出与续期必须围绕同一活动 Session 状态线性化；撤销胜出后不能签发可再次使用的凭据。

### 6. 一个前端会话 module，而不是多个页面各自恢复

集中会话 module 对外提供状态订阅及显式登录、启动恢复、手动恢复、退出／重试退出和上下文读取。状态至少区分 `bootstrapping`、`anonymous`、`authenticated`、`recovery-required`、`logout-pending` 与 `unavailable`。保留单页 single-flight 和 generation 保护，但不把 null 同时解释成“未恢复”和“未登录”。

App 挂载先检查所需浏览器能力，再在没有退出标记时执行一次 Cookie 续期与真实 `/api/user/context`；全部成功才发布 authenticated。路由守卫在启动未结束时显示恢复状态，不闪现旧数据或提前跳转登录。匿名／明确失效落到原登录入口并保留安全的原目的地址；有效会话访问登录／注册页按原规则进入工作台。验证码与密码不得自动重提交。

Access 到期信息只用于安排主动续期、在后台标签恢复时检查期限或暂停请求，不作为授权事实。启动恢复的网络／超时／500 保持身份未确认状态，显示手动重试与重新登录入口。运行中相同故障且 Access 尚有效时保留页面、上下文与编辑并提示；到期后屏蔽受保护内容与新的受保护操作，但保持同一会话的编辑实例，恢复并重新校验同一身份后再启用。身份／Session 改变则丢弃旧编辑，不交给新身份。

普通权限 403 保留页面；`IDENTITY_ACCOUNT_DISABLED` 和明确认证拒绝走会话失效。既有失败业务请求不自动刷新后重放，尤其保存／删除；会话控制重取不是业务重放。所有晚到响应必须比较操作 generation 与当前浏览器事件，不能重建已退出或被替换的状态。

### 7. 跨标签认证操作串行化，协调状态不携带凭据

复用浏览器标准能力：同源 `navigator.locks` 的 `admin-workbench.auth` 锁覆盖 Login／Refresh／Logout 的请求与 Cookie 响应完成；本地 module 保留 single-flight。每个操作取得锁后重新读取退出标记、浏览器事件与待续期尝试，不依赖等待前的内存副本。锁不是永久租约，标签关闭应释放。

实施时经用户确认，增加 `admin-workbench.auth.state` 短状态锁，原子保护浏览器协调状态的读写／清除，绝不跨网络等待。退出先单独取得状态锁持久化意图、清理本地状态并释放，再等待认证请求锁；其他需要两个锁的流程一律先认证请求锁、再短状态锁。不得持有状态锁等待认证请求锁，以避免死锁；不能用锁外比较后删除标记，因为另一标签的新退出意图可能在比较与删除之间丢失。

仅增加下列 localStorage 状态，不保存 Token、密码、验证码、用户资料、权限或菜单：
- `admin-workbench.auth.refresh-attempt`：尝试 ID、开始时间、操作 epoch；未知响应结果在有效窗口内复用，不能立即换 ID 把合法重试当成重用攻击。
- `admin-workbench.auth.logout-pending`：退出操作 ID、目标 Session 非授权关联与 epoch。
- `admin-workbench.auth.event`：递增 epoch、事件种类与非授权 Session 关联，触发其他标签清理旧状态。该值不能直接授予登录或权限。

主题 key 保持原样，导航状态仍不持久化。不把 Token 放进跨标签事件；等待标签在锁内按最新 Cookie 有序取得自己的内存 Access 并独立验证上下文。必要存储读写必须有结果，不能沿用主题偏好“写失败不影响业务”的宽松策略来伪称认证协调成功。

退出意图先写标记并广播，使在途续期结果即使晚到也不能恢复 UI；完成撤销或新的显式登录成功且上下文已确认后，才在锁内清除对应标记。其他标签收到退出即清旧上下文；换用户成功后提示身份变化，不默默恢复旧编辑。事件丢失的标签在获得焦点或开始认证操作时重读状态，服务端 Session／授权版本检查仍是最终保护。

### 8. 退出待确认不是“已完全退出”

现有账号菜单及未保存修改确认保持；没有新增退出二次确认。用户真正允许离开后，才登记退出意图、暂停主动续期、清理内存并执行 Cookie Logout。有效 Access 已过期或内存已清空时，手动重试退出仍能工作。

网络／500 时保留退出标记和 Cookie，展示“服务端退出未确认”与“重试退出”。自动启动／普通续期不得穿过标记。新的显式登录必须是用户重新提交真实凭据并完成验证，不能把一次普通恢复按钮当作解除退出意图；新登录失败仍保留标记。重复退出、开发 StrictMode 重复挂载、晚到响应及旧标签重试都不能覆盖新登录的 Cookie 或撤销新 Session。

Cookie、localStorage、Web Locks 等必要能力不可用时进入 unavailable，提供明确设置说明，不降级到旧内存模式或持久 Token。会话中途标记写入失败必须明确报告本地防恢复未能建立，并仍允许尝试服务器撤销；在无法持久保存意图且网络也失败时，不可能承诺跨刷新离线退出，必须显示未完成而非成功。阻止自动恢复，不把可用 Cookie 当作绕过能力前提的理由。

### 9. 错误 interface 与 Cookie 副作用

| 情况 | HTTP／公开码 | 凭据副作用 |
|---|---|---|
| 旧请求协议、损坏 JSON、非法尝试 Header | `400 / HTTP_REQUEST_INVALID` | 不消费有效凭据，不改变其他会话 |
| Origin／防护 Header 拒绝 | `403 / AUTHN_CSRF_INVALID` | 不清 Cookie、不撤销会话、不回显来源 |
| Refresh 明确失效 | `401 / AUTHN_REFRESH_TOKEN_INVALID` | 清本 Cookie；异常重用同时撤销绑定 Session |
| Access 明确失效或旧 Token | `401 / AUTHN_TOKEN_INVALID` | 不从普通业务响应直接修改 Cookie，避免晚到响应清除新登录 |
| 用户停用 | 保留既有 `403 / IDENTITY_ACCOUNT_DISABLED` | 前端清旧身份并进入明确失效流程，不当作普通权限 403 |
| 已确认的幂等结果被取代 | `409 / AUTHN_REFRESH_SUPERSEDED` | 不回写旧 Cookie；手动协调，不自动重放业务 |
| 真实依赖／签发故障 | `500 / IDENTITY_INTERNAL_ERROR` | 保留用于有限恢复或重试退出的 Cookie；不伪装成凭据无效 |

新公开码由 Identity HTTP Adapter 的认证错误定义唯一持有：`AUTHN_CSRF_INVALID` 固定安全消息 `browser request verification failed`，`AUTHN_REFRESH_SUPERSEDED` 固定消息 `refresh attempt was superseded`，data 均为 null。四字段信封、错误所有权和本地化仍走现有集中路径。凭据缺失／过期／撤销／版本不符的具体原因不公开区分。

### 10. Route Catalog 仍是唯一静态事实来源

Logout 的 Access Level 从 Authenticated 改为 Public，是取消 Access 前置依赖而不是允许匿名撤销任意身份；Cookie 证明与 CSRF 属于 Handler 的实际契约，类似既有 Public Refresh 的凭据校验。其余用户与管理路由访问等级不变，不新增枚举，不让 API Metadata 开关影响 Cookie 控制端点安全。

在 Route Descriptor 的 operation 中增加最小可选 Cookie／请求 Header／响应 Header 描述，交给现有验证、快照复制与 OpenAPI 生成统一消费。Cookie 会话控制需要的真实 400／401／403／409／500 由 Identity 描述；不把不存在的 Bearer 中间件错误拼到 Logout。文档声明 Cookie security scheme、`Set-Cookie`、`Cache-Control`、必要 Header 及新 data Schema，不能依靠 APIDoc 按 Path 硬编码第二份认证路由表。

现有 API Metadata 的管理员可配置字段继续保留；新 Descriptor 的默认值与 API Doc 静态访问事实随 Snapshot 更新。此变更不回填历史 Public `need_auth`、不新增 Permission Code、不改变其他动态权限策略。需要修改的现有路由文档、组合根、Seed 断言和消费者必须一起迁移。

### 11. 日志与文档同属交付

Cookie、Set-Cookie、Authorization、Refresh JWT 及续期签发材料不得进入运行日志、Audit Body／Query、错误响应、截图说明或仓库证据；新协调标识也不自动加入 RuntimeLogger 字段白名单。保持已有事件名，不加会话遥测系统；用现有安全错误码表达失败。保留既有消息正文省略、Query 白名单与上传安全 metadata，不回写历史审计。

实施时在 CONTEXT 添加“登录会话”与“授权版本”的区别，记录此项有实质取舍的 ADR，并更新 ADR 0004／0011 的相关当前约束、README、配置和认证运行说明。当前只在 change-local 描述目标，主规格在验收后同步，不触碰已归档审计。

### 12. 验证 interface 与真实联合门禁

现有 JWT／会话／HTTP 契约测试迁移到新语义；不把旧内存实现、固定文案或 forwarding 断言重新钉死。保留能捕获消费方错误的回归：过期、版本失效、会话撤销、Cookie／CSRF 边界、原子续期、幂等不延长、合法 superseded 与异常重用、退出并发终态和晚到结果不复活。

Redis 原子竞争必须用真实 Redis、多个应用调用者及确定性屏障证明，不以 fake 或单线程顺序执行代替。身份／授权版本与上下文使用真实 MySQL；真实 HTTP 验证信封、Header／Cookie 和 Access 边界。真实浏览器覆盖刷新／直达、Cookie 属性、双标签、30 秒结果丢失、500 与断网、退出待确认、阻塞存储、同用户恢复保留编辑、换用户不带编辑，以及主题／菜单／键盘行为。生产安全策略用可信本机 HTTPS 站点验证，HTTP 开发例外单独标记；不拿关闭 Cookie 安全标志的截图充当 HTTPS 验收。

证据区分库级、真实 Redis、真实 MySQL、真实 HTTP 和浏览器层次，记录行为和安全摘要，不保存真实密码、JWT、Cookie 或原始基础设施错误；临时环境与脚本不提交。

## Risks / Trade-offs

- [HttpOnly 不是 XSS 的完整防护] → 不暴露 Refresh 给 JS，但同源恶意脚本仍可能使用活跃页面；保留现有内容安全，不声称此变更消除 XSS。
- [一次性轮换会把丢响应误判为盗用] → 同尝试 30 秒有界恢复、浏览器串行协调、缓存活动／代检查；窗口外仍可能要求重新登录，不能扩成无限旧凭据兼容。
- [晚到结果可能覆盖 Cookie 或新身份] → 全部认证副作用共用跨标签锁与 epoch；服务端先检查活动 Session，不从缓存复活；通过受控交错而非顺序测试证明。
- [Redis 成为会话可用性依赖] → 真实故障返回可重试的安全 500，状态丢失要求重新登录，不能仅凭 JWT 重建；复用现有 Redis 运行配置，不承诺重启永不掉登录。
- [同一浏览器共享 Cookie 不支持多用户并行] → 明确共享语义，身份变化停止旧操作并提示；不同浏览器配置保留独立会话。
- [关闭浏览器不必然删除会话 Cookie] → 不提供记住我承诺，也不把窗口关闭当强制退出；服务端期限、明确退出和撤销决定安全状态。
- [离线且禁止持久状态时无法可靠记录退出意图] → 能力失败显式阻止恢复并提示未完成；不能以清空内存冒充全端撤销。
- [协议破坏与当前规格冲突] → 同批发布、旧 Token 拒绝、全部调用者迁移和主规格同步，禁止半切换或用兼容分支掩盖遗漏。

## Migration Plan

1. 实施和验证新 interface，更新仓库内登录／刷新／退出调用者、DTO、Route Catalog、OpenAPI、安全错误、本地化及门禁脚本；删除旧 JSON Refresh／legacy 用途与黑名单退出路径，保持未受影响能力。
2. 配置可信对外 Origin／HTTPS 或显式回环开发例外，核实反向代理转发与浏览器属性。没有真实部署 Origin 时不填通配符或猜测域名；验收使用明确隔离的本机站点。
3. 在新 change 记录真实联合验收，完成受影响主规格同步和用户重登发布说明；不需要 MySQL 迁移、授权版本批量提升或 Redis 清库。
4. 同一切换窗口停止旧实例／旧前端入口，发布前后端新版本并刷新静态入口；旧 Token 因缺少新会话绑定而拒绝。已有验证码／限制／旧黑名单 key 自然到期，新登录创建新 Session。
5. 普通回滚只能回到实现相同会话协议且仍校验 SID 的已验收版本。旧实现不检查 SID，直接回滚可能使已撤销新 Access 再次有效，因此不是允许的常规回退；异常恢复需单独批准停流及签名密钥切换等全量重登录措施，不能自动恢复旧 JSON 通道或旧版本读取。

## Open Questions

无待用户裁决的产品分支。实际部署 Origin 由环境提供并经配置验证；本次本地联合门禁结果见下方，不代表生产环境已发布或完成生产验收。

## References

- 现行事实：`openspec/specs/auth/spec.md`、`admin-workbench/spec.md`、`route-catalog/spec.md`、`logging/spec.md`；ADR 0004、0006、0011。
- 架构规格一致性：经用户确认，`modular-layered-architecture` 同步两处黑名单旧约束为活动登录会话与撤销，分层、Principal 和授权版本归属不变。
- 已核对 Go 1.25 标准库 `net/http.CrossOriginProtection`、`Check`、`AddTrustedOrigin` 文档；标准库无 Origin 时的允许行为不能替代本协议的显式 Origin 要求。
- [OWASP CSRF：SPA 自定义请求 Header 与严格 Origin／CORS 约束](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html#employing-custom-request-headers-for-ajaxapi)。
- [MDN Set-Cookie：HttpOnly、Secure、SameSite 与浏览器会话恢复限制](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie)。

## Implementation Verification

### 已执行门禁

| 层级 | 实际命令／环境 | 结果 |
|---|---|---|
| 后端完整回归 | `go test ./... -count=1` | 43 个测试包通过；包含修正后的最小 Refresh JWT 与会话行为回归 |
| 真实 MySQL | `go test -tags=mysql_integration ./... -count=1`；通过环境变量注入隔离数据库 DSN | 43 个测试包通过；未把 DSN 或凭据写入证据 |
| 真实 Redis | `go test -tags=redis_integration ./internal/identity/adapters/redis -count=1 -timeout=2m` | 通过；多个独立客户端和屏障覆盖竞争、30 秒边界、前代到期恢复、TTL 不延长、superseded、重用撤销与退出交错 |
| Cookie 容量回归 | `go test ./internal/identity/adapters/jwt -count=1` | 大权限快照下 Cookie 容量回归通过，Access 授权快照保持完整 |
| 前端现有测试 | `npm test`，目录 `web/` | 6 个文件、94 个行为测试通过 |
| 类型与生产构建 | `npm run build`，目录 `web/` | `tsc --noEmit` 与 Vite 构建通过；仍有既有单包大于 500 kB 的构建提示，未为本次变更扩大拆包范围 |
| OpenSpec 严格校验 | `openspec validate add-browser-session-recovery --type change --strict`；`openspec validate auth --type spec --strict`，其余四份主规格同参数逐项执行 | Change 与 auth／admin-workbench／route-catalog／logging／modular-layered-architecture 五份主规格全部通过；旧内存／JSON Refresh／黑名单契约冲突已消除 |

### 真实 HTTP 与浏览器

验收使用真实 MySQL、Redis、MinIO、RabbitMQ 和 `app.New` 组合根，不以 fake 会话存储或替代后端冒充运行。临时入口仅把 Access 缩短为 1 分钟以观察实际到期；仓库默认 15 分钟未改变。Chrome 使用本次本机证书的 SPKI 白名单信任 HTTPS，不全局忽略 TLS 错误、不修改系统信任库；回环 HTTP 例外使用另一明确开启开发例外的实例和独立浏览器配置。

- **协议与身份**：真实验证码登录、Cookie 续期、上下文、过期 Access 退出均成功；控制响应为四字段信封和 no-store，JSON 无 Refresh。有效签名但缺少 SID 的旧格式 JWT 返回 401；仅有 Cookie 的业务请求返回 401。普通权限拒绝返回 403，等价角色 PUT 保持授权版本和原 Access 可用。
- **Cookie**：HTTPS Chrome 确认 Host-only、`/api`、HttpOnly、Secure、Strict、session Cookie。实际 104 项权限时完整 Set-Cookie 为 428 字节。修正前 Chrome 明确拒收超大凭据，修正后登录与刷新恢复成功；只有显式回环 HTTP 实例省略 Secure。独立浏览器配置产生不同 SID，退出一个仍可用另一个访问真实上下文。
- **故障分类**：隔离 Redis ACL 实际拒绝查询、脚本与撤销后，Access／Refresh／Logout 返回安全 500、null data，不设置删除 Cookie；恢复依赖后可续期。主动删除该测试 SID 后返回 401，不从 JWT 重建。没有通过关闭 Cookie 安全标志取得生产 HTTPS 证据。
- **未知结果恢复**：请求阶段丢包代理将真实请求交给后端并丢弃提交后的全部响应，证明旧 Cookie 保留；另以真实后端响应 Header 加空正文证明 Cookie 已更新但结果未知。两种情况均在窗口内复用原尝试、恢复完全相同的 Access 和 SID，仅推进一代。响应阶段中断本身可能已处理 Set-Cookie，因此不把它误记为 Header 丢失证明。
- **真实独立标签**：用 Chromium 新页面及明确目标绑定确认两个不同页面，各自发出一次续期并读取自己的有效上下文。普通刷新、直达编辑页、新标签恢复成功；延迟真实 context 时仅显示确认状态，不展示登录表单或保护内容。
- **晚到副作用**：真实后端已提交续期但 Cookie／正文仍由丢包代理扣留时，另一标签先持久化退出意图，网络退出等待同一认证锁。释放晚到响应后旧续期被代际检查拒绝，退出撤销 SID 并清 Cookie，两个标签均无保护上下文，旧编辑被卸载。
- **编辑与业务请求**：真实权限编辑器产生未保存勾选；浏览器断网和真实 500 时有效 Access 保留同一 DOM 与选择。实际到期后隐藏并 inert 原工作台，集中 Client 在网络前拒绝保护操作；手动恢复同一 SID 后仍为同一编辑实例和选择，失败保存没有自动重放。
- **退出待确认与换用户**：真实撤销 500 保留 Cookie 和标记；完整刷新与新标签不发自动续期。UI 重试不含 Bearer 并成功撤销。真实错误密码登录保留退出意图；新的显式登录在 context 被扣留期间仍不清标记，确认后才清。旧标签在登录锁后等待的退出重试不请求服务器、不撤销新 SID；普通用户的真实 403 不清新身份。
- **能力与交互**：原生禁用 localStorage、原生阻止 Cookie，以及受控移除 Web Locks API 均显示不可用且不发登录／续期。中途 Storage 写入拒绝故障配合真实离线，不伪称已保存退出标记或已撤销；网络恢复后仍可请求真实服务器退出。主题在写入拒绝时仅改变当前外观、不弱化认证不可用；既有账号菜单 Enter／Escape 焦点、取消未保存退出、外观保留均通过，320 px 能力错误界面无横向溢出。
- **脱敏**：检查真实审计表 464 条记录及隔离运行日志，未发现 JWT、生成的密码／签名密钥或原始 Redis 故障文本；未归档请求 Header、Cookie、响应 JWT 或签发材料。

### 环境清理与边界

本次创建的 10 个浏览器句柄已释放，两个验收实例和 Vite 已停止，4 个自建容器及其所属匿名卷已删除；临时 Go 入口、可执行文件与浏览器配置目录已移除。未提交临时脚本、真实凭据或原始错误，未改用户 `.env`、系统证书信任或生产环境；本地验证不等于生产发布，也不承诺 Redis 状态丢失后继续登录。

