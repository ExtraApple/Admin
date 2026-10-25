## 1. 领域模型、DTO 与配置

- [ ] 1.1 按现行 `internal/identity` 模块边界定义登录会话、续期代、固定签发材料及最小 SessionStore port；区分活动／缺失／撤销／真实故障，修改导出接口前查询引用并迁移全部调用者，不恢复已退役的目录分层。
- [ ] 1.2 定义 Cookie-only 请求／响应 DTO：Login 返回 access_token／session_id／user，Refresh 返回 access_token／session_id，Logout 返回 null；Refresh／Logout 只接受空对象，明确拒绝旧 refresh_token 与未知字段，移除旧公开协议类型。
- [ ] 1.3 更新 JWT Adapter，强制明确用途与 sid／用户／授权版本绑定，Refresh 增加代和不可预测标识；支持固定签发时间与到期时间的结果重建，删除无用途旧 Token 推断，保留现有库而不重写密码学。
- [ ] 1.4 增加 `browser_session.origin` 与默认关闭的 `allow_loopback_http` 配置，验证规范 Origin、HTTPS／精确回环例外及正 JWT 时长；更新现有配置入口，非法配置启动失败，不信任任意转发 Header。

## 2. Redis 会话与原子续期

- [ ] 2.1 在现有 Redis Adapter 实现会话创建／读取和安全 key，使用 sid、凭据标识摘要及 TTL，不保存原始 JWT key、不新增 MySQL 表或扫描清理任务；登录替换只原子撤销同浏览器原有效会话。
- [ ] 2.2 使用现有 go-redis/v9 原子脚本／事务实现当前凭据比较、一次性消费、新代与固定结果同时提交，并让 Session TTL 只随新代推进。
- [ ] 2.3 实现固定 30 秒同尝试结果恢复，支持前代／已下发新代 Cookie、原凭据提交后到期、同一固定签发结果与不延长窗口／Token／Session TTL。
- [ ] 2.4 实现 superseded、异常重用、会话终态与撤销竞争：已确认结果被取代返回独立结果，异常旧凭据重用撤销当前 sid；缓存不得复活缺失／撤销会话或绕过用户／授权版本检查。

## 3. Identity Application 行为

- [ ] 3.1 迁移 Login，保留现有验证码、错误计数、锁定、用户启用和唯一授权版本初始化，成功时建立新会话，失败不发布新 Token／Cookie 或替换原有效会话。
- [ ] 3.2 迁移 Refresh，组合实时用户／Authorization 校验、SessionStore 原子结果与固定材料签发；真实依赖或签发故障为安全 500，明确无效为统一 401，不因故障重建会话或回退旧版本来源。
- [ ] 3.3 为 Access Authenticate 增加活动 sid 与用户／授权版本绑定检查，保留实时访问快照、缺版本拒绝、真实故障 500 和停用用户 403；正常 Refresh 轮换不拒绝尚未到期的同会话 Access。
- [ ] 3.4 将 Logout 改为当前 Cookie 会话撤销，支持无有效 Access、重复退出和无有效 Cookie 的幂等成功；真实撤销故障不能报告成功，普通退出不提升用户授权版本或影响其他独立会话。
- [ ] 3.5 删除旧 Access 黑名单运行路径及其失效依赖，迁移相关消费者／行为测试；保留现有授权事实变化和 no-op 规则，历史 Redis key 自然到期，不批量清库或提升版本。

## 4. HTTP Handler、Cookie 与 App 路由

- [ ] 4.1 为 Login／Refresh／Logout 复用 Go 1.25 CrossOriginProtection，并显式强制规范 Origin、X-Admin-Request 与 JSON 防护协议；防护失败在处理凭据／Cookie 副作用前拒绝，不引入宽松 CORS 或新 CSRF 引导路由。
- [ ] 4.2 实现 admin_refresh 的 Host-only／Path=/api／HttpOnly／SameSite=Strict／生产 Secure 设置与同范围删除，以及 no-store；Refresh 读取 Cookie 和尝试 Header，Logout 不依赖 Bearer，旧 JSON 通道返回 400。
- [ ] 4.3 增加 Identity 持有的 AUTHN_CSRF_INVALID 与 AUTHN_REFRESH_SUPERSEDED 安全定义并迁移错误映射：403 不清合法 Cookie，409 不回写旧 Cookie，真实 500 保留可重试 Cookie，普通业务 401 不修改 Cookie。
- [ ] 4.4 在组合根注入新依赖，并仅将精确 POST /api/user/logout Descriptor 改为 Public，由 Handler 执行 Cookie／来源检查；其余 /api/user 和 /api/admin 身份边界、唯一 Gin 注册和三个访问等级保持不变。

## 5. Route Catalog、OpenAPI 与日志安全

- [ ] 5.1 最小扩展 Route Descriptor operation、验证与 Snapshot 投影以表达 Cookie 安全方案、必需请求 Header 和响应 Header；缺失或非法描述在注册前失败，不在 APIDoc 硬编码认证 URL。
- [ ] 5.2 迁移 Identity Descriptor、OpenAPI 生成和相关消费者，使请求／响应 Schema、Cookie、no-store 与实际 400／401／403／409／500 契约一致；Cookie Logout 不声明不存在的 Bearer 前置依赖。
- [ ] 5.3 更新 API Metadata／Seed 受影响静态事实与行为断言，保留管理员配置字段，不回填历史 Public NeedAuth、不新增 Permission Code 或改变其他动态权限策略。
- [ ] 5.4 将 Cookie、Set-Cookie、Refresh JWT 与固定签发材料纳入现有日志／Audit／错误脱敏，保留原有消息和上传安全边界；不新增运行事件／字段白名单、遥测系统或历史审计迁移。

## 6. 前端 Client 与集中会话状态

- [ ] 6.1 迁移集中 API Client 和 TypeScript 契约，Cookie 控制请求带防护 Header、空对象和尝试标识，不从 JSON／Web Storage 获取 Refresh；保留同源 Bearer 业务请求、四字段错误处理与不自动重放业务请求。
- [ ] 6.2 将 `session-state.ts` 与 Provider 暴露为明确的 bootstrapping／anonymous／authenticated／recovery-required／logout-pending／unavailable 状态，保留页面 single-flight 和 generation 保护，并移除旧内存 Refresh 管理。
- [ ] 6.3 实现必要能力检查、退出标记优先、一次启动 Cookie 续期加真实 context 确认；路由守卫区分待恢复和匿名，保留安全原目的地址、原登录／注册页面及验证码／锁定语义。
- [ ] 6.4 实现主动续期和手动恢复：临时故障在 Access 有效时保留编辑，到期后屏蔽受保护内容及新操作但保持同会话编辑实例；恢复成功后读取最新上下文，同用户／sid 才继续旧编辑。
- [ ] 6.5 明确处理 401、普通权限 403、账户停用、409 与 500；旧业务响应不影响新身份，失败保存／删除不自动重放，未知恢复结果不无限循环或自动重交密码／验证码。

## 7. 跨标签协调与退出交互

- [ ] 7.1 实现 `admin-workbench.auth` Web Lock 及三类最小 localStorage 协调标记，所有登录／续期／退出覆盖真实 Cookie 响应完成，在锁内重读状态；不将仅忽略晚到 JS 结果当作阻止 Cookie 覆盖的措施。
- [ ] 7.2 实现同尝试未知结果重取、30 秒窗口与 superseded 手动恢复；标签不广播或持久化 Token，分别取得自己的 Access／context，并在 focus／认证操作时补读错过的事件。
- [ ] 7.3 在用户通过既有未保存修改保护后登记退出意图、暂停续期并清内存，失败保留 logout-pending；提供不依赖 Access 的重试退出和真实显式登录，普通启动／恢复不能清标记。
- [ ] 7.4 覆盖跨标签退出／换用户、旧标签重试、StrictMode 重复执行和晚到响应；新登录且 context 确认前不清退出意图，之后旧操作不能清新 Cookie、撤销新会话或将旧编辑交给新身份。
- [ ] 7.5 Cookie／协调存储／锁能力不可用时显示明确不可用状态；中途退出标记写失败仍允许尝试服务器撤销，但网络也失败时明确未完成；保持主题 key、导航挂载状态、头像菜单和无新增退出确认。

## 8. 回归与真实运行验收

- [ ] 8.1 迁移现有会话、JWT、Identity 与 HTTP 契约测试，保留能捕获用途／过期、授权版本、禁用、撤销、错误分类及旧协议拒绝的行为回归；删除只钉死旧文案／内存实现／转发副本的测试，不重新绑定实现细节。
- [ ] 8.2 使用真实 Redis、多个应用调用者及确定性交错验证同尝试唯一提交、30 秒内外、前代到期恢复、TTL 不延长、superseded、异常重用与退出竞争终态，不能用单线程 fake 结果冒充竞争证明。
- [ ] 8.3 使用真实 MySQL／Redis 和实际 HTTP 请求验证登录→续期→当前上下文→退出，确认 Cookie 属性、无 JSON Refresh、Access 已过期仍可退出、不同会话隔离、旧 Token 拒绝及安全日志／四字段信封。
- [ ] 8.4 在隔离环境制造真实依赖失败与响应丢失，分别验证 Redis 查询故障 500、会话丢失 401、签发／响应提交后恢复、不清 retry Cookie，以及认证／权限／no-op 的原有边界。
- [ ] 8.5 在真实浏览器与可信本机 HTTPS 站点验证刷新／直达／新标签恢复、确认前不展示保护内容、双标签续期与退出／换用户、Cookie Header／正文丢失、晚到响应及旧操作不覆盖新 Cookie；单独验证精确回环 HTTP 开发例外。
- [ ] 8.6 在真实浏览器验证断网／500 下有效 Access 保留编辑、到期后同会话恢复、失败业务不重放、退出待确认跨刷新与标签生效、阻塞存储／Cookie／锁的明确失败，以及主题／账号菜单／键盘／未保存修改保护未退化。
- [ ] 8.7 汇总迁移后运行必要后端测试、前端既有受影响测试、类型检查与构建门禁；记录实际执行命令、环境层级和行为结果，区分真实运行与库级测试，不保存凭据或把未执行场景标为通过。

## 9. 文档、规格同步与发布

- [ ] 9.1 在真实运行证明后更新 CONTEXT 的登录会话／授权版本边界，新增 ADR 记录 Cookie、单次轮换与协调取舍，并给 ADR 0004／0011 的被替代约束补充关联，不重写历史决策事实。
- [ ] 9.2 更新 README、配置／反向代理与认证运维说明及全部仓库内调用示例，说明生产 HTTPS、显式回环例外、Cookie／Header 协议、浏览器能力前提和安全 500／待确认退出限制。
- [ ] 9.3 补充前后端同批切换、旧 Token 全量重登、禁止混跑旧协议和普通回滚仍须校验 sid 的发布说明；不执行生产变更、签名密钥切换或历史数据操作来冒充本地验收。
- [ ] 9.4 实施验收完成后同步本 Change 的 auth／admin-workbench／route-catalog／logging 增量到对应主规格并严格校验；归档前确认不存在旧 Token 内存／JSON／黑名单契约冲突，不沿用其他已归档 Change 的 skip-specs 特例。
