# 浏览器会话恢复与独立登录会话撤销

## 状态

已接受。本文记录 `add-browser-session-recovery` 的架构决策，不作为测试、浏览器验收或发布通过声明；验收范围与结果由该 change 的任务和证据记录负责。

本决策局部替换 [ADR 0004](0004-authorization-owns-access-version-storage.md) 的 Redis 黑名单退出职责、[ADR 0011](0011-react-frontend-and-customizable-ui-stack.md) 初期两种 Token 仅驻内存、刷新后重新登录的约束，并记录 [ADR 0006](0006-modular-layered-monolith-contracts-and-composition-root.md) 认证控制接口的已批准破坏性协议切换。原记录的历史迁移、分层、模块所有权和前端技术路线继续有效。

## 背景

整页刷新会清空内存凭据与上下文，但将 Refresh 放进 WebStorage 会把可持续续期的凭据直接暴露给页面脚本。仅把 Refresh 移入 Cookie 也不足以支持安全恢复：旧协议不能独立撤销一次登录，重复使用 Refresh 不受限制，响应丢失、并行标签与晚到 Set-Cookie 都可能使合法操作失败或复活已退出状态。

授权版本解决的是用户授权及身份安全变化后的全局失效，不是某个浏览器的退出。以提升版本代替普通退出，会下线其他独立登录。需要在保持实时授权校验的同时，明确登录会话、单次续期、浏览器协调与服务端撤销的不同边界。

## 决策

### 模块所有权与凭据边界

系统保持 `internal/` 下按领域分包的模块化单体，不恢复已退役的 handler/service/router 分层目录。Identity Application 定义最小会话存储接口，Redis Adapter 实现会话原子操作，JWT Adapter 负责签名与验证，HTTP Adapter 负责 Cookie 和请求防护，组合根注入依赖。Redis client、Gin Context 和 MySQL 事务不泄漏进会话接口。

每次真实登录建立独立高熵 SID，Identity 拥有其活动事实。Authorization 继续独占 MySQL `user_access_versions`，不新增另一份授权版本。普通退出撤销当前 Cookie 证明的 SID，不提升版本；密码、用户状态或授权变化仍按既有规则提升版本，使该用户所有旧版本凭据失效。

Access 明确绑定用途、用户、SID 与授权版本，保留原有角色／权限快照语义；每次受保护请求仍核验有效期、用户状态、活动会话及实时授权事实。正常续期不使其他标签尚未到期的 Access 失效，Access 不要求匹配最新续期代。

Refresh 是最小签名会话绑定，包含用户、用途、SID、授权版本、不可预测的凭据标识、单调续期代和签发／到期时间，不复制角色或权限集合。新的 Access 来自实时授权快照，而不是 Cookie 内嵌权限。最小载荷使 Cookie 不随权限数量增长，避免浏览器单 Cookie 容量限制；不采用 Cookie 分片，也不改变 Access 的授权载荷语义。旧用途推断、JSON Refresh 和黑名单退出路径一并移除。

### Redis 活动事实与有界单次续期

`auth:session:<sid>` 保存活动会话、用户／版本绑定、当前续期代、凭据标识摘要与到期事实。Refresh 不能在记录缺失时仅凭 JWT 重建会话。`auth:refresh-attempt:<sid>:<attempt>` 保存一次成功提交的固定签发材料，期限为首次提交后 30 秒；不保存原始 JWT 或 Cookie。

Redis 原子 Rotate 同时比较会话、凭据、尝试及代，推进新代并登记结果。相同尝试在窗口内可携带已消费前代或本次产生的新代凭据恢复同一签发结果，覆盖 Set-Cookie 已处理但响应正文丢失的情况。重取不改变签发时间、到期时间、会话 TTL 或恢复窗口。恢复仍要求会话活动、结果仍是当前代、用户及授权版本有效；不能借缓存复活退出会话。

已确认的同尝试结果被后续成功续期取代时返回 `409 / AUTHN_REFRESH_SUPERSEDED`，不写旧 Cookie、不撤销现有会话。窗口外或不匹配的旧凭据重用返回统一 401 并撤销该 SID，而不是该用户所有登录。撤销与轮换围绕同一活动事实线性化。

默认 Access 为 15 分钟、Refresh 为 10080 分钟，配置必须为正数。成功新代按当次签发滑动服务端期限，没有新增首次登录起算的绝对上限；浏览器 Cookie 生命周期不是服务端期限。

### Cookie-only 控制入口与部署前提

Refresh 仅通过 `admin_refresh` Cookie 传输：Host-only、Path=/api、HttpOnly、SameSite=Strict，HTTPS 下 Secure。设置成功时不带 Expires／Max-Age；删除使用相同范围的过期属性。浏览器可能恢复会话 Cookie，关闭窗口不能被当作可靠退出。Refresh 不进入 JSON、JavaScript 存储或跨标签消息；Access 和用户上下文只驻内存。

Login、Refresh 和 Logout 必须是 JSON 请求，精确匹配配置的 `browser_session.origin`，携带 `X-Admin-Request: 1`，并执行标准库跨源防护；Refresh 另需不可预测的 16..128 字符 `X-Refresh-Attempt-ID`。缺失或 null Origin 不接受，不用 Host 或 X-Forwarded-* 猜测可信来源。生产必须 HTTPS；`allow_loopback_http` 默认 false，仅显式允许 localhost 或 127.0.0.1 的 HTTP 开发例外。部署者必须配置浏览器看到的规范 Origin，并正确部署 TLS／反向代理；不提供跨域凭据 CORS 或通配来源。

Login 成功只在真实会话提交后返回 `access_token/session_id/user`，替换该浏览器原有效 Cookie 绑定的会话，不撤销其他浏览器登录。Refresh 使用 `{}` 并返回 `access_token/session_id`。`POST /api/user/logout` 使用 `{}`、无需 Access，Route Catalog 为 Public，但仍严格校验 Cookie 和来源防护，成功 data 为 null。它不能据未验证 claim 撤销任意 SID。所有控制响应使用 `Cache-Control: no-store`；普通业务继续使用 Bearer，Cookie 不直接授权业务操作。

### 串行 Cookie 副作用与持久退出意图

集中前端会话模块使用原生 Web Lock `admin-workbench.auth` 覆盖 Login／Refresh／Logout 的真实 fetch、响应正文消费与 Cookie 副作用完成。仅丢弃晚到 JavaScript 结果不能撤回浏览器已经处理的 Set-Cookie，因此不能以单页 single-flight 或 generation 替代跨标签串行化。

短锁 `admin-workbench.auth.state` 只保护协调状态读写，不跨网络等待。需要两个锁时只能 auth → state。退出先单独取得 state 锁，写入退出意图并释放，再等待 auth 锁；禁止持有 state 等待 auth。该额外锁保证比较与清除标记之间不会丢失另一标签的新退出意图。

localStorage 仅保存三个非凭据键：`.event`（epoch、事件种类、SID 关联）、`.logout-pending`（随机退出 ID、SID、epoch）、`.refresh-attempt`（随机尝试 ID、时间、epoch），均使用 `admin-workbench.auth` 前缀。不存用户资料、权限、菜单或 Token；SID 和尝试 ID 本身不授予权限。取得锁后重新读取状态，不能依赖等待前的副本。

保留原有未保存编辑离开保护，不新增退出确认。用户允许离开后先持久化退出意图、清内存并广播，再请求撤销。失败、刷新或另一标签不能解除该意图；只有确认服务端退出，或用户真正重新提交凭据、显式登录成功且真实上下文确认后，才能清除对应标记。不能用普通恢复代替显式登录解除退出意图。旧标签和晚到操作必须校验 epoch／SID，不能清新标记、覆盖新 Cookie 或撤销新登录。

### 恢复与失败语义

启动先检查必要能力，在没有退出意图时 Cookie 续期，再读取真实用户上下文；两者完成前只显示身份确认状态，不闪现登录页或受保护内容。运行中暂时续期失败且 Access 有效时保留编辑；到期后暂停内容与受保护操作但不卸载编辑，仅同 SID 且上下文重新确认才能继续。身份变化清除旧编辑。失败业务请求不自动重放。

| 情况 | 对外语义与副作用 |
|---|---|
| 请求结构或旧 JSON 协议错误 | 400，不消费有效凭据 |
| 来源／防护失败 | 403，不清合法 Cookie，不撤销会话 |
| 会话缺失、撤销、过期或版本失效 | 认证拒绝，不能回退到仅验 JWT |
| 合法恢复结果已被取代 | 409，无 Set-Cookie；丢弃旧协调信息，提供手动恢复，不自动循环 |
| Redis／数据库／签发真实故障 | 安全 500，不伪装为无效凭据；保留可有限恢复或重试退出的 Cookie |
| Logout 已撤销、过期或无有效 Cookie | 幂等成功并清 Cookie，不影响其他会话 |
| Logout 真实撤销故障 | 500，不清重试 Cookie，保留退出意图并显示“服务端退出未确认” |
| Cookie／localStorage／Web Locks 必要能力失败 | 明确 unavailable，不降级到旧内存认证或持久 Token |

若持久退出意图写入失败，仍允许尝试服务器撤销，但必须说明本地防恢复未建立；若同时离线，不能承诺跨刷新退出成功。HttpOnly 降低 Refresh 被脚本读取的风险，但同源恶意脚本仍可能操作活跃会话，SameSite 和来源校验也不消除 XSS。

## 替代方案

- **维持纯内存与刷新后登录**：简单且不增加恢复状态，但不满足跨刷新恢复，也不能解决单次登录撤销。
- **将 Refresh 放 WebStorage**：易实现恢复，却把续期凭据暴露给脚本及存储读取路径；不采用。
- **无状态 Refresh 或继续黑名单 Access 退出**：减少活动会话读取，但无法立即撤销该登录的其他 Access／Refresh，且可能从旧凭据重建已退出状态；不采用。
- **普通退出提升授权版本**：复用已有全局失效机制，却错误影响其他独立登录；仅保留其原有安全及授权变化职责。
- **严格一次消费、不提供恢复窗口**：状态较少，但成功提交后丢响应会把合法浏览器当作重用攻击；选择固定 30 秒同尝试恢复，而非无限旧代宽限。
- **只做单标签锁或仅忽略晚到响应**：无法约束跨标签 Cookie 副作用；选择原生跨标签认证锁及短状态锁，不自制超时租约锁或无锁比较删除。
- **MySQL 新建会话表**：可采用不同持久化恢复策略，但增加迁移及跨存储设计；本次复用既有 Redis，接受明确的可用性成本。

## 运行与安全成本

Redis 从辅助能力变为每次会话认证和续期的安全依赖：故障时拒绝放行并返回可重试故障，活动事实丢失要求重新登录，不能承诺 Redis 重启永不掉登录。运维需按现有 Redis 配置管理可用性与状态恢复；MySQL 授权版本仍为唯一持久化授权事实。原子脚本与有界结果状态增加实现复杂度，但不依赖扫描、无限历史或后台清理维持正确性。

浏览器必须允许所需 Cookie、存储和 Web Locks；同源共享 Cookie 意味着同一浏览器配置中的标签不支持同时登录不同用户。恢复窗口之外的未知结果可能需要重登，这是避免无限重放与旧凭据复活的代价。Cookie 安全、精确 Origin 与反向代理配置是发布前提，不是可在故障时静默降低的选项。

日志、审计、错误及证据不得记录 Authorization、Cookie、Set-Cookie、JWT 或固定签发材料；不新增会话遥测系统。凭据缺失／撤销／版本不符不公开细分原因。真实存储故障不能被吞掉而声称退出完成。

前后端同窗口切换，旧 Token 缺少新绑定必须重新登录；不需要 Redis 清库、MySQL 会话迁移或批量提升授权版本。常规回滚只能使用仍实现相同协议并检查 SID 的版本。旧后端不检查 SID，可能接受已经撤销的新 Access，不能作为普通回滚；旧协议恢复需另行批准的事件处置与签名密钥切换等全量重登录方案。

## 关联记录

- [领域语言](../../CONTEXT.md)：登录会话、授权版本、访问凭据与续期凭据。
- [ADR 0004](0004-authorization-owns-access-version-storage.md)：授权版本存储所有权及历史迁移。
- [ADR 0006](0006-modular-layered-monolith-contracts-and-composition-root.md)：分层、最小 Contract 与唯一组合根继续有效。
- [ADR 0011](0011-react-frontend-and-customizable-ui-stack.md)：前端选型与集中 Client。
- [变更设计](../../openspec/changes/add-browser-session-recovery/design.md)：协议、错误、安全边界与验证要求。
