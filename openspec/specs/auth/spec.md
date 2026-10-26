# auth Specification

## Purpose

认证模块负责后台系统的验证码、注册、登录、JWT 校验、可撤销登录会话与 Cookie 续期，以及登录防刷保护。它定义公开入口、登录安全约束、凭据签发内容和会话撤销后的拒绝行为。
## Requirements
### Requirement: 验证码生成
系统 SHALL 提供公开验证码接口，生成一次性数字图片验证码，并将验证码答案短期存入 Redis。

#### Scenario: 客户端请求验证码
- **WHEN** 客户端调用 `GET /api/captcha`
- **THEN** 系统返回验证码 ID 和 base64 图片内容
- **AND** 系统将验证码答案存入 Redis 的 `captcha:<id>`
- **AND** 验证码在 5 分钟后自动过期

### Requirement: 验证码校验
注册和登录 SHALL 提交有效验证码，并且验证码无论校验成功还是失败都必须被消费。

#### Scenario: 验证码正确
- **WHEN** 用户提交匹配的 `captcha_id` 和 `captcha_code`
- **THEN** 系统继续执行注册或登录流程
- **AND** 系统从 Redis 删除该验证码答案

#### Scenario: 验证码错误或过期
- **WHEN** 用户提交错误、已使用或已过期的验证码
- **THEN** 系统拒绝本次请求
- **AND** 系统返回 HTTP 422、`AUTHN_CAPTCHA_INVALID` 和安全英文消息
- **AND** `data` SHALL 为 `null`
- **AND** 用户必须重新获取验证码后才能重试

### Requirement: 登录锁定
系统 SHALL 基于用户名使用 Redis 记录登录凭据错误累计，并在达到五次阈值后进行渐进式锁定，同时不暴露用户名是否存在。有效验证码之后、未锁定且进入凭据校验时，用户名不存在或密码错误 SHALL 计入累计；锁定期间的重试 SHALL NOT 再次计入凭据错误。

本次失败计入后的累计次数少于 5 次 SHALL 不锁定，5～9 次 SHALL 锁定 1 分钟，10～14 次 SHALL 锁定 5 分钟，15～19 次 SHALL 锁定 15 分钟，20 次及以上 SHALL 锁定 1 小时。第 10 次失败和锁定到期 SHALL NOT 清零累计；每次记录凭据错误 SHALL 将累计记录的有效期续为 24 小时，累计记录到期自动清除，成功登录清除累计与锁定。

#### Scenario: 未达到锁定阈值时登录失败
- **WHEN** 用户提交有效验证码且未处于锁定状态，用户名不存在或启用用户密码错误，且本次错误计入后的累计失败次数少于 5 次
- **THEN** 系统递增 `fail:<username>`
- **AND** 系统返回 HTTP 401、`AUTHN_CREDENTIALS_INVALID` 和相同的模糊英文提示
- **AND** 系统 MAY 在 `data.remaining_attempts` 返回剩余尝试次数
- **AND** 响应 SHALL NOT 通过状态、错误码、消息或详情暴露用户名是否存在

#### Scenario: 登录失败达到锁定阈值
- **WHEN** 本次登录凭据错误使累计失败次数达到 5 次或以上
- **THEN** 系统在 Redis 中设置 `lock:<username>`，采用该累计次数对应的锁定档位
- **AND** 系统返回 HTTP 429 和 `AUTHN_LOGIN_LOCKED`
- **AND** 系统在 `data.retry_after_seconds` 和 `Retry-After` Header 返回锁定时间
- **AND** 锁定存在期间拒绝继续登录

#### Scenario: 账号处于锁定状态
- **WHEN** Redis 中存在 `lock:<username>`
- **THEN** 系统拒绝登录请求
- **AND** 系统返回 HTTP 429、`AUTHN_LOGIN_LOCKED`、`data.retry_after_seconds` 和 `Retry-After` Header

#### Scenario: 登录成功
- **WHEN** 验证码正确、账号启用、密码匹配
- **THEN** 系统清理 `fail:<username>` 和 `lock:<username>`
- **AND** 系统签发 access token 和 refresh token
- **AND** JSON 响应 SHALL 使用 `code=200`、空 `error_code`、`msg=success` 和登录结果 `data`

#### Scenario: 锁定到期保留凭据错误累计
- **WHEN** `lock:<username>` 到期，而 `fail:<username>` 尚未到期
- **THEN** 系统 SHALL 保留已有累计次数
- **AND** 下一次满足计入条件的凭据错误 SHALL 在已有次数上递增，而不是从第一次重新开始

#### Scenario: 第十次失败继续累计
- **WHEN** 已累计 9 次错误、对应锁定已到期，且累计记录仍有效，用户再次发生可计入的凭据错误
- **THEN** 系统 SHALL 将累计次数递增为 10 并锁定 5 分钟
- **AND** 系统 SHALL NOT 在该次锁定后重置累计

#### Scenario: 更高档位继续升级
- **WHEN** 用户未成功登录，累计记录仍有效，后续可计入的凭据错误达到 15 次或 20 次
- **THEN** 系统 SHALL 分别锁定 15 分钟或 1 小时，而不是因第 10 次重置而停留在较低档位

#### Scenario: 累计记录续期与到期
- **WHEN** 系统记录一次新的登录凭据错误
- **THEN** 系统 SHALL 将 `fail:<username>` 的有效期续为该次记录后的 24 小时
- **AND** 在无新计入错误且该记录到期后，系统 SHALL 不保留此前累计，后续凭据错误从新的累计开始

### Requirement: 认证入口使用稳定公开错误
公开登录入口、Cookie 会话控制入口和认证中间件 SHALL 使用稳定认证错误码，且不得把 Token 解析器、密码比较器、Redis 或数据库错误直接返回客户端。

#### Scenario: 缺少认证 Header
- **WHEN** 使用 Bearer Access 的 Authenticated 或 PermissionControlled 路由未携带 Authorization Header
- **THEN** 系统 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 响应 SHALL 使用四字段错误信封

#### Scenario: Authorization Header 格式错误
- **WHEN** Authorization Header 不是合法 Bearer Token 格式
- **THEN** 系统 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 系统 SHALL NOT 回显 Header

#### Scenario: Token 解析失败
- **WHEN** Bearer Token 签名、有效期、用途、登录会话或授权版本校验失败
- **THEN** 系统 SHALL 返回 HTTP 401 和稳定认证错误码
- **AND** 安全英文消息 SHALL NOT 包含解析器或存储内部错误

#### Scenario: Cookie 控制请求防护失败
- **WHEN** 登录、续期或退出请求的 Origin、必需浏览器防护 Header 或 JSON 防护协议不符合声明
- **THEN** 系统 SHALL 返回 HTTP 403、`AUTHN_CSRF_INVALID`、固定安全消息 `browser request verification failed` 和 null data
- **AND** 系统 SHALL NOT 回显来源或凭据、消费续期凭据、清除合法 Cookie 或撤销其他会话

#### Scenario: 旧请求体或非法续期尝试标识
- **WHEN** 续期或退出提交旧 `refresh_token` 字段、损坏 JSON、不允许的请求字段，或续期缺少合法的 `X-Refresh-Attempt-ID`
- **THEN** 系统 SHALL 返回 HTTP 400 和 `HTTP_REQUEST_INVALID`
- **AND** 系统 SHALL NOT 通过忽略旧字段保留旧 Token 请求通道

### Requirement: 账号状态拒绝登录
已知账号未启用时，系统 SHALL 在不泄漏内部状态细节的前提下拒绝签发 Token。

#### Scenario: 禁用账号使用正确凭据登录
- **WHEN** 用户提交有效验证码和正确密码但账号状态不是启用
- **THEN** 系统 SHALL 返回 HTTP 403 和稳定 `IDENTITY_*` 错误码
- **AND** 系统 SHALL NOT 签发 Access Token 或 Refresh Token

### Requirement: JWT 会话
系统 SHALL 在登录成功后建立独立登录会话，并签发包含用户 ID、授权版本、明确令牌用途和会话标识的 JWT。Access Token SHALL 保持角色码和权限码快照且只供 Bearer 认证；Refresh Token SHALL 绑定会话续期代及不可预测凭据标识，仅通过服务端 HttpOnly 会话 Cookie 传递，且 SHALL NOT 复制角色和权限快照。

#### Scenario: 登录成功后返回令牌
- **WHEN** 用户通过现有验证码、登录锁定、密码、启用状态和授权版本检查并成功建立 Redis 登录会话
- **THEN** 系统 SHALL 设置 `admin_refresh` Cookie，并以四字段成功信封返回 data 中的 `access_token`、非授权性的 `session_id` 和脱敏后的 `user`
- **AND** 响应 JSON SHALL NOT 包含 `refresh_token`
- **AND** Access 用途 SHALL 为 `access`，Refresh 用途 SHALL 为 `refresh`，两者 SHALL 绑定同一会话和授权版本
- **AND** Access SHALL 可用于认证受保护的 `/api/user` 和 `/api/admin` 请求，Cookie 本身 SHALL NOT 直接认证这些业务请求

#### Scenario: 大权限集合不扩大续期 Cookie
- **WHEN** 用户拥有较多角色或权限并成功登录或续期
- **THEN** 完整 Refresh Cookie SHALL 小于浏览器 4 KiB 容量，载荷 SHALL NOT 随角色／权限数量增长
- **AND** Refresh SHALL 保留全部会话安全绑定，Access 的授权快照 SHALL 保持完整
- **AND** 续期 SHALL 读取实时授权快照，不以 Cookie 内嵌权限替代当前授权事实

#### Scenario: 拒绝旧版或缺少会话绑定的令牌
- **WHEN** Token 没有新会话绑定、没有明确 `token_type` 或不能匹配新会话事实
- **THEN** 系统 SHALL 拒绝该 Token 并要求重新登录
- **AND** 系统 SHALL NOT 根据 `iat`／`exp` 间隔推断旧用途、回退旧协议或把有效签名当成足够的登录证明

#### Scenario: 时长与浏览器记忆边界
- **WHEN** 新登录或合法新一代续期签发 Token
- **THEN** 系统 SHALL 使用现有经验证的 JWT 时长配置，仓库默认 Access 为 15 分钟、Refresh 为 10080 分钟
- **AND** 新 Refresh 与服务端会话 SHALL 从该次签发滑动到期，不新增空闲超时或首次登录绝对上限
- **AND** 系统 SHALL NOT 提供长期记住我承诺或把关闭浏览器当作可靠的强制退出

### Requirement: 密码复杂度
系统 SHALL 在注册和修改密码时校验密码复杂度。

#### Scenario: 密码满足规则
- **WHEN** 密码长度不少于 6 位
- **AND** 密码包含大写字母、小写字母、数字、特殊符号中的至少 3 类
- **THEN** 系统允许继续后续流程

#### Scenario: 密码不满足规则
- **WHEN** 密码长度少于 6 位，或字符类型少于 3 类
- **THEN** 系统拒绝请求并返回校验错误

### Requirement: Token 实时失效
系统 SHALL 使用 Authorization 持有的用户授权版本使权限和用户状态变更后的旧 Token 失效，并通过稳定认证错误契约拒绝无效会话。Access 和 Refresh 还 SHALL 匹配活动登录会话，不得仅凭 JWT `exp` 决定可用性；正常 Refresh 轮换 SHALL NOT 单独使同一会话尚未到期的其他 Access 失效。

Access认证 SHALL 将“唯一授权版本行缺失”分类为不可验证会话，而非内部故障；该分类映射为认证拒绝。明确确认的用户不存在身份继续按既有认证拒绝契约处理。数据库连接、查询、表不可用或其他真实基础设施失败 SHALL 继续返回 HTTP 500、`IDENTITY_INTERNAL_ERROR`，响应 `data` SHALL 为 `null`，不得泄露底层信息、伪装成凭据无效或成功。
需要使现有会话失效的跨入口操作 SHALL 只在授权、身份状态或组织范围事实真实变化时提升受影响用户的授权版本；合法等价 no-op SHALL 不提升版本。业务事实、授权版本和适用的并发版本检查 SHALL 保持同一事务语义，任一步失败 SHALL 不留下部分提交。

#### Scenario: Token 版本一致
- **WHEN** 已认证请求携带用途为 Access Token 的有效 JWT
- **AND** JWT 中的 `token_version` 等于 `user_access_versions.version`
- **AND** 用户状态为启用且绑定的登录会话仍活动
- **THEN** JWT 中间件 SHALL 允许请求继续，并使用实时访问快照而不是 JWT 内嵌权限作为当前授权事实

#### Scenario: Token 版本不一致
- **WHEN** Access Token 或 Refresh Token 中的 `token_version` 与 `user_access_versions.version` 不一致，或令牌用途不符合请求场景
- **THEN** 系统拒绝该 Token
- **AND** Access Token 请求 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 用户 SHALL 需要重新登录，不能经续期把失效凭据升级成新授权

#### Scenario: 用户被禁用或删除
- **WHEN** Access Token 对应用户不存在或状态不是启用
- **THEN** JWT 中间件拒绝请求
- **AND** 不存在或不可验证身份 SHALL 返回 HTTP 401 和稳定认证错误码
- **AND** 已建立身份但账号被禁用 SHALL 返回 HTTP 403 和稳定 `IDENTITY_*` 错误码

#### Scenario: Access 身份查询真实基础设施失败
- **WHEN** Access 认证查询用户身份时发生数据库连接、查询、表不可用或其他真实基础设施故障
- **AND** 故障不是已确认的用户不存在
- **THEN** JWT 中间件 SHALL 返回 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** `data` SHALL 为 `null`
- **AND** 响应不得包含底层数据库、驱动或连接错误
- **AND** 系统不得将该故障伪装为 `AUTHN_TOKEN_INVALID` 或签发新 Token


#### Scenario: 登录初始化新用户版本
- **WHEN** 用户通过身份和密码校验但 `user_access_versions` 不存在
- **THEN** 系统幂等创建版本 1
- **AND** 系统重新读取当前版本
- **AND** 系统 SHALL 签发包含该版本并绑定新登录会话的 Access 和 Refresh Token

#### Scenario: 登录初始化失败
- **WHEN** 系统无法创建或读取用户授权版本或建立登录会话
- **THEN** 系统拒绝登录
- **AND** 系统 SHALL NOT 发布新 Cookie 或 Token
- **AND** 响应 SHALL 使用 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** `data` SHALL 为 `null`
- **AND** 响应 SHALL NOT 包含底层数据库或 Redis 错误

#### Scenario: 使用 Refresh Token 刷新
- **WHEN** 客户端向 `POST /api/refresh` 发送可信浏览器请求、空对象、合法尝试 Header，以及用途、签名、有效期、用户状态、会话和授权版本均有效的 Refresh Cookie
- **THEN** 系统 SHALL 原子消费当前凭据并设置新代 Refresh Cookie
- **AND** 新 Token SHALL 使用 `user_access_versions` 中的当前版本并携带明确用途和同一会话绑定
- **AND** 成功响应 SHALL 使用 code=200、空 error_code、msg=success，data 只包含 `access_token` 和 `session_id`
- **AND** 系统 SHALL NOT 要求同时持有有效 Access 或返回 JSON Refresh Token

#### Scenario: Refresh Token 刷新失败
- **WHEN** Refresh Token 缺失、无效、过期、用途错误、用户不存在或被禁用、授权版本缺行／不一致，或会话已撤销／丢失
- **THEN** `POST /api/refresh` SHALL 返回 HTTP 401 和 `AUTHN_REFRESH_TOKEN_INVALID`
- **AND** 安全英文消息 SHALL NOT 区分具体拒绝原因
- **AND** `data` SHALL 为 `null`
- **AND** 系统 SHALL NOT 签发新 Token 或重建会话
- **AND** 系统 SHALL NOT 回退其他授权版本存储

#### Scenario: Refresh 真实基础设施失败与凭据拒绝分离
- **WHEN** Refresh 校验、会话原子操作或签发发生数据库、Redis、连接、查询或其他真实基础设施故障
- **THEN** 系统 SHALL 返回 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** `data` SHALL 为 `null`
- **AND** 系统 SHALL NOT 伪装成 `AUTHN_REFRESH_TOKEN_INVALID`、回显原始错误或清除仍可用于有限恢复的 Cookie
- **AND** 已提交续期的恢复 SHALL 遵守同尝试有限结果恢复契约，不回滚成可任意复用的旧凭据


#### Scenario: 认证时授权版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到 `user_access_versions`
- **THEN** 系统拒绝该 Token
- **AND** Access 返回 401 / `AUTHN_TOKEN_INVALID`，Refresh 返回 401 / `AUTHN_REFRESH_TOKEN_INVALID`
- **AND** 系统 SHALL NOT 跳过授权版本检查或自动补行

#### Scenario: 权限相关变更提升版本
- **WHEN** 用户密码、用户状态、角色分配、角色权限、角色菜单、菜单记录或组织成员归属发生影响登录态的真实变更
- **THEN** 系统通过 `EnsureAndIncrement` 提升受影响用户的授权版本
- **AND** 这些用户已持有的旧 Access Token 和 Refresh Token 在下一次使用时失效

#### Scenario: 真实授权变化只使受影响用户失效
- **WHEN** 角色、权限、用户状态或组织范围关系的规范化事实真实变化
- **THEN** 系统只提升实际受影响用户的授权版本
- **AND** 每个受影响用户在同一业务提交中最多提升一次
- **AND** 不得因最终有效权限相同而豁免真实授权事实变化

#### Scenario: 合法授权 no-op 不提升版本
- **WHEN** 当前授权版本有效，提交的规范化角色、权限、状态或组织范围事实与当前事实等价
- **THEN** 系统 SHALL 成功返回当前授权版本
- **AND** 系统 SHALL NOT 重建等价关系或提升授权版本
- **AND** 现有有效且未撤销的 Access Token 和 Refresh Token SHALL 继续可用

#### Scenario: 授权变更失败保持原事实
- **WHEN** 授权关系、身份状态、组织范围或授权版本初始化／锁定／提升任一步失败
- **THEN** 系统 SHALL 回滚同一事务中的全部业务事实和授权版本变化
- **AND** 系统 SHALL NOT 以部分提交或成功响应结束请求


#### Scenario: Access缺版本不补行
- **WHEN** 用途、签名、有效期、会话、用户状态合法的 Access JWT 对应 `user_access_versions` 行不存在
- **THEN** 系统 SHALL 返回HTTP 401和AUTHN_TOKEN_INVALID
- **AND** 非登录认证不得Ensure、跳过版本检查、读取旧users.token_version或签发新Token

#### Scenario: Access读取真实基础设施失败
- **WHEN** Access认证读取版本发生数据库连接、查询、表不可用或其他真实基础设施故障而不是已确认缺行
- **THEN** 系统 SHALL 返回 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`
- **AND** `data` SHALL 为 `null`
- **AND** 不得回退其他存储、创建版本或签发新 Token
- **AND** 响应不得包含底层错误

#### Scenario: Refresh缺行保持既有结果
- **WHEN** 有效Refresh JWT的唯一授权版本行缺失
- **THEN** 系统 SHALL 继续返回HTTP 401与AUTHN_REFRESH_TOKEN_INVALID，且不签发Token

#### Scenario: 正常续期不撤销未到期的 Access
- **WHEN** 同一活动会话成功轮换 Refresh，其他标签仍持有同会话未到期的 Access
- **THEN** 系统 SHALL 继续按会话、用户和授权版本验证这些 Access
- **AND** 系统 SHALL NOT 仅因续期代变化拒绝这些 Access

### Requirement: 独立登录会话事实
Identity SHALL 为每次成功登录建立独立 Redis 会话。Access 与 Refresh SHALL 同时绑定该会话、用户和授权版本，且会话存在与活动状态 SHALL 是认证必要条件；系统 SHALL NOT 新建 MySQL 会话表、从 JWT 补建缺失会话或通过提升用户授权版本实现普通退出。

#### Scenario: 不同浏览器独立登录
- **WHEN** 同一用户在两个独立浏览器配置中分别成功登录
- **THEN** 系统 SHALL 建立两个不同会话，撤销其中一个不影响另一个

#### Scenario: Redis 真实故障与状态丢失
- **WHEN** 会话查询发生真实 Redis 故障
- **THEN** 系统 SHALL 返回安全的 500 / `IDENTITY_INTERNAL_ERROR`
- **AND** Redis 成功返回会话缺失时 SHALL 拒绝会话，不从有效 JWT 重建

#### Scenario: 成功续期才推进期限
- **WHEN** 新代续期原子成功
- **THEN** 服务端会话期限 SHALL 跟随该代 Refresh 到期时间推进
- **AND** 幂等重取、失败或状态读取 SHALL NOT 推进期限

### Requirement: 浏览器 Cookie 凭据与请求防护
系统 SHALL 只通过 Host-only、HttpOnly、SameSite=Strict、Path=/api 的 `admin_refresh` 会话 Cookie 传递 Refresh Token，HTTPS 下 SHALL 设置 Secure。Cookie 设置 SHALL NOT 使用长期 Expires／Max-Age；清除 SHALL 使用同范围的删除属性。Login／Refresh／Logout SHALL 校验可信 Origin、`X-Admin-Request: 1` 和 JSON 防护协议，且 SHALL NOT 开启跨域凭据 CORS 或回退 JSON Refresh 通道。

#### Scenario: Cookie 不向 JavaScript 暴露续期凭据
- **WHEN** 登录或续期成功
- **THEN** Refresh SHALL 仅经 Set-Cookie 传递，JSON SHALL NOT 包含 Refresh
- **AND** 控制响应 SHALL 使用 no-store，其他业务请求仍必须携带 Bearer Access

#### Scenario: 拒绝伪造来源和简单表单
- **WHEN** Cookie 控制请求的 Origin 缺失、为 null、不是配置的精确来源，或请求缺少防护 Header／不符合 JSON 防护协议
- **THEN** 系统 SHALL 以 `AUTHN_CSRF_INVALID` 拒绝，且不改变有效会话或 Cookie
- **AND** 系统 SHALL NOT 从任意 Host、Forwarded Header 或匹配前缀推导可信来源

#### Scenario: 原始凭据不得进入新状态 key 或公开证据
- **WHEN** 系统保存会话／有限幂等状态或记录请求
- **THEN** 新 Redis key SHALL NOT 包含原始 JWT，长期会话事实 SHALL 使用必要的凭据标识摘要
- **AND** JWT、Cookie 和签发材料 SHALL NOT 出现在日志、错误响应或仓库证据中

### Requirement: 一次性续期与有限结果恢复
系统 SHALL 对登录会话执行原子的一次性 Refresh 消费，并将同一尝试的固定签发结果保留 30 秒用于有限恢复。新代、尝试结果与期限 SHALL 以同一提交语义生效；任何重取 SHALL NOT 再次签发不同代、延长原结果期限或越过撤销／授权版本检查。

#### Scenario: 同尝试并发只有一个新代
- **WHEN** 多个调用者以同一会话、凭据和尝试 ID 同时续期
- **THEN** 系统 SHALL 只推进一次续期代
- **AND** 其余合法调用 SHALL 在窗口内取得同一代与固定到期时间

#### Scenario: 响应丢失后重取
- **WHEN** 续期已提交但 Cookie Header 或响应正文丢失，客户端在 30 秒内使用同一尝试重试
- **THEN** 系统 SHALL 接受匹配的前代凭据或该尝试已经产生的新代凭据，并在会话仍活动且未被更高代取代时返回同一结果
- **AND** 系统 SHALL NOT 延长幂等窗口、Token 到期时间或 Redis 会话 TTL

#### Scenario: 前代在提交后到期
- **WHEN** 前代凭据在首次提交时有效，提交后才自然到期，而合法结果仍处于 30 秒恢复窗口
- **THEN** 系统 SHALL 仅在签名、尝试绑定、活动会话、当前代及用户授权仍匹配时恢复已提交且仍有效的结果
- **AND** 系统 SHALL NOT 使用过期凭据创建新的续期代

#### Scenario: 已有结果被更高代取代
- **WHEN** 系统确认该尝试曾合法提交，但当前会话已推进到更高续期代
- **THEN** 系统 SHALL 返回 409 / `AUTHN_REFRESH_SUPERSEDED`、固定安全消息 `refresh attempt was superseded` 和 null data
- **AND** 系统 SHALL NOT 回写旧 Cookie、延长结果或撤销当前有效会话

#### Scenario: 异常旧凭据重用
- **WHEN** 旧 Refresh 凭据在恢复窗口外或不匹配的尝试中再次使用，且不属于已确认的合法结果被取代
- **THEN** 系统 SHALL 撤销该登录会话并返回统一的 Refresh 认证拒绝
- **AND** 其他独立登录会话 SHALL NOT 受影响

#### Scenario: 撤销或授权变化优先于结果缓存
- **WHEN** 尝试结果仍在 30 秒内，但会话已撤销／丢失、用户停用或授权版本不再匹配
- **THEN** 系统 SHALL 拒绝结果重取
- **AND** 系统 SHALL NOT 从短期缓存重建会话或绕过实时授权

#### Scenario: 已提交后签发失败
- **WHEN** 续期原子提交完成后签名／响应发生真实故障
- **THEN** 系统 SHALL 返回安全 500，并允许同尝试在有效窗口内恢复固定签发材料
- **AND** 系统 SHALL NOT 回滚为可任意复用旧凭据或签发额外新代

### Requirement: 当前登录会话撤销
`POST /api/user/logout` SHALL 使用 Cookie 会话控制协议，不依赖有效 Access；成功退出 SHALL 撤销当前登录会话的全部 Access／Refresh 可用性并清除同范围 Cookie，普通退出 SHALL NOT 提升用户授权版本或撤销其他独立登录。

#### Scenario: 当前会话退出
- **WHEN** 合法浏览器请求退出当前会话
- **THEN** 该会话尚未自然到期的所有 Access 和 Refresh SHALL 在后续使用时被拒绝
- **AND** 系统 SHALL 清除 Cookie 并返回四字段成功信封与 null data

#### Scenario: Access 已过期仍可退出
- **WHEN** 当前 Access 已过期或客户端内存已清空，但仍有有效 Refresh Cookie
- **THEN** 合法退出请求 SHALL 能撤销该 Cookie 对应会话，不因缺少 Bearer Access 而拒绝

#### Scenario: 重复或已无有效会话的退出
- **WHEN** 退出请求重复、Cookie 缺失／无效或对应会话已经撤销／到期
- **THEN** 系统 SHALL 幂等清除该 Cookie 并返回成功
- **AND** 系统 SHALL NOT 根据未验证 claim 删除其他会话

#### Scenario: 撤销基础设施失败
- **WHEN** Redis 撤销发生真实故障
- **THEN** 系统 SHALL 返回 500 / `IDENTITY_INTERNAL_ERROR`，不声称退出成功
- **AND** 系统 SHALL 保留用于重试的 Cookie，不能假装完成服务端撤销

#### Scenario: 退出与续期交错
- **WHEN** 退出与续期或结果重取并发
- **THEN** 活动会话状态 SHALL 决定唯一线性化结果
- **AND** 撤销生效后任何晚到续期结果 SHALL NOT 恢复可用会话

#### Scenario: 同浏览器显式重新登录
- **WHEN** 用户在同浏览器显式提交真实凭据并成功完成新登录
- **THEN** 系统 SHALL 建立新会话并撤销该浏览器原有效 Cookie 指向的旧会话
- **AND** 新登录失败 SHALL NOT 替换原有效会话，其他浏览器的独立登录 SHALL 保持不变

### Requirement: Cookie 会话部署配置
系统 SHALL 显式配置浏览器可见规范 Origin，生产 SHALL 仅允许同源 HTTPS。仅在显式开启开发例外且主机为 localhost 或 127.0.0.1 时，系统 SHALL 允许本机 HTTP 与相应非 Secure 开发 Cookie；系统 SHALL NOT 为局域网、公网、通配符或任意转发 Header 自动降低安全要求。

#### Scenario: 生产配置有效
- **WHEN** 配置为合法 HTTPS Origin
- **THEN** 系统 SHALL 精确校验来源并设置 Secure Cookie，不以代理上游 HTTP 地址替代浏览器来源

#### Scenario: 显式本机开发例外
- **WHEN** HTTP Origin 为允许的回环主机且开发例外已明确开启
- **THEN** 系统 SHALL 允许本机 Vite 代理联调
- **AND** 同一例外 SHALL NOT 允许任意局域网或公网 HTTP

#### Scenario: 配置不满足前提
- **WHEN** Origin 缺失／无效、含路径或通配符，HTTP 未获回环例外，或 JWT 时长非正数
- **THEN** 系统 SHALL 在启动时明确拒绝该配置，不静默补成宽松策略

