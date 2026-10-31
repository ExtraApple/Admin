# admin-workbench Specification Delta

## RENAMED Requirements

- FROM: `### Requirement: 个人资料三个独立编辑区`
- TO: `### Requirement: 个人资料四个独立编辑区`

- FROM: `### Requirement: 昵称编辑与身份字段只读`
- TO: `### Requirement: 昵称与邮箱编辑边界`

## MODIFIED Requirements

### Requirement: 运行中续期失败保留编辑
工作台 SHALL 在临时续期故障下保留同一会话的页面与未保存选择；当前 Access 到期后 SHALL 暂停新的受保护操作并屏蔽受保护内容，直到明确恢复或失效。恢复 SHALL NOT 自动重放失败的业务请求。个人资料昵称、邮箱候选和本地待保存头像 SHALL 遵循同会话草稿保护；邮箱当前密码、验证 token 和密码区密码在会话挂起或失效时清空，SHALL NOT 作为恢复草稿保留。

#### Scenario: Access 尚有效时续期失败
- **WHEN** 已认证用户正在编辑，主动续期因网络或 500 暂时失败，而当前 Access 尚有效
- **THEN** 页面 SHALL 保留编辑与当前有效身份并提示续期暂不可用
- **AND** 系统 SHALL NOT 将临时故障当成退出或自动重提交编辑

#### Scenario: 到期后恢复同一身份
- **WHEN** Access 到期，用户手动恢复并经服务端确认仍为同一用户和登录会话
- **THEN** 页面 SHALL 在重新读取上下文后恢复同一编辑实例
- **AND** 确认前 SHALL 阻止受保护操作，不向新身份移交旧编辑

#### Scenario: 业务请求认证失败
- **WHEN** 普通业务请求返回认证拒绝或真实权限拒绝
- **THEN** Client SHALL 按所属会话与错误类别更新状态
- **AND** Client SHALL NOT 自动续期后重放保存、删除或其他原始业务请求

#### Scenario: 个人资料会话挂起后的敏感草稿例外
- **WHEN** 个人资料正在编辑且 Access 到期导致会话挂起，或服务端确认会话失效
- **THEN** 页面 SHALL 立即清空旧会话的全部密码输入和邮箱验证 token 并阻止其恢复，SHALL NOT 持久化密码或 token
- **AND** 昵称、邮箱候选和待保存头像 SHALL 仅可在重新确认同一 userID 和 SID 后恢复；新身份 SHALL NOT 接收旧草稿

#### Scenario: 资料读取临时故障不冒充身份失效
- **WHEN** 个人资料读取或保存后的回读返回安全的 `500 / IDENTITY_INTERNAL_ERROR` 而非明确认证终态
- **THEN** 页面 SHALL 呈现读取失败与可操作的重试入口，保留仍有效的会话与非敏感草稿，SHALL NOT 将真实存储故障伪装为身份不存在并登出
- **AND** 若写请求未取得确认成功，页面 SHALL 说明结果未确认，SHALL NOT 根据一般 500 推断写入未提交、宣称回滚或自动重发写请求

#### Scenario: 写入已确认但显示重读失败
- **WHEN** 客户端已收到写入成功的 HTTP 200，而随后为更新展示读取 context 或受控头像失败
- **THEN** 页面 SHALL 保留已确认的保存成功状态，并单独说明显示更新未完成
- **AND** 重试 SHALL 仅重新读取显示，SHALL NOT 再次提交已经确认的写请求

### Requirement: 个人资料四个独立编辑区
现有 `/profile` SHALL 保持既有视觉、主题和账号菜单，使用真实资料提供昵称、头像、邮箱和密码四个独立提交区，同一时间 SHALL 仅有一个页内编辑区展开，SHALL NOT 提供暗示四个区域原子提交的统一保存。每个区域提交期间 SHALL 防止重复提交与冲突切换；邮箱区 SHALL 拥有独立 dirty 状态，不与昵称合并提交。

#### Scenario: 独立展开与提交
- **WHEN** 用户打开个人资料、展开昵称区并保存有效修改
- **THEN** 页面 SHALL 仅提交昵称区请求，SHALL NOT 连带上传头像、修改邮箱或修改密码；头像、邮箱和密码区 SHALL 保持收起且各自已保存状态不变

#### Scenario: 邮箱独立展开与提交
- **WHEN** 用户展开邮箱区并保存有效的新邮箱与当前密码
- **THEN** 页面 SHALL 仅提交邮箱换绑请求，SHALL NOT 连带修改昵称、上传头像或修改密码
- **AND** 成功后 SHALL 更新邮箱卡片并保留其他区域已保存状态

#### Scenario: 提交期间重复操作
- **WHEN** 任一区域请求尚未确定结果而用户再次点击保存、恢复默认或尝试切换区域
- **THEN** 页面 SHALL 阻止重复写入和冲突切换，明确显示当前提交状态，SHALL NOT 并行发出同一区域的重复请求

### Requirement: 昵称与邮箱编辑边界
个人资料中昵称和邮箱可编辑，但 SHALL 由各自独立编辑器提交。昵称 SHALL 非空、非全空白且最多 100 个 Unicode 字符；页面 SHALL NOT 自动 Trim、更改大小写或提供清空昵称。邮箱 SHALL 仅提交 `email` 与 `current_password`，只清理首尾空白并保留大小写；用户 ID、用户名、角色和启用状态 SHALL 只读；邮箱验证状态和待验证邮箱 SHALL 由服务端返回，客户端 SHALL NOT 直接写入。

#### Scenario: 昵称边界与原值提交
- **WHEN** 用户输入空字符串、全空白字符串或超过 100 个 Unicode 字符的昵称并保存
- **THEN** 页面 SHALL 显示昵称字段错误并阻止请求；字符数 SHALL NOT 以 UTF-16 码元或 UTF-8 字节数代替 Unicode 字符计数
- **AND** 当用户输入恰为 100 个 Unicode 字符且非全空白的昵称（包括补充平面字符），页面 SHALL 通过此项校验，按原有首尾空白及大小写发送，不自动规范化

#### Scenario: 只读身份不被混入写请求
- **WHEN** 用户查看个人资料并保存昵称或换绑邮箱
- **THEN** 用户 ID、用户名、角色和启用状态 SHALL 仅展示真实已保存值
- **AND** 昵称提交体 SHALL NOT 含邮箱、验证状态或其他只读字段；邮箱提交体 SHALL 只含邮箱与当前密码，SHALL NOT 混入昵称、验证状态或待验证邮箱
- **AND** 页面 SHALL NOT 允许通过当前界面清空昵称或直接设置验证状态

#### Scenario: 邮箱格式与提交边界
- **WHEN** 用户提交空邮箱或格式无效的邮箱
- **THEN** 页面 SHALL 显示邮箱字段错误并阻止请求
- **AND** 页面 SHALL NOT 自动小写化、删除中间字符或把待验证邮箱当作当前邮箱提交
- **AND** 服务端邮箱格式校验和当前／待验证唯一性检查仍是最终判据

#### Scenario: 邮箱验证状态由服务器决定
- **WHEN** 换绑、确认或取消成功
- **THEN** 页面 SHALL 使用服务端返回的当前邮箱、待验证邮箱和验证状态更新显示
- **AND** 页面 SHALL NOT 根据本地请求内容自行推断验证成功或切换通知资格

### Requirement: 个人资料当前标签同步与并发边界
昵称、头像和邮箱状态保存、确认或取消成功后，工作台 SHALL 同步当前标签个人页与顶栏账号显示，并重新读取受控头像内容和邮箱状态。其他标签 SHALL 在重新进入个人资料或刷新时读取最新资料；SHALL NOT 新增跨标签资料广播、资料持久化缓存、CAS、资料版本或编辑锁。纯昵称、头像或邮箱状态修改 SHALL NOT 被当成授权版本变化要求重新登录。

#### Scenario: 当前标签同步真实新头像与邮箱状态
- **WHEN** 昵称保存、头像上传、恢复默认、邮箱换绑、确认或取消成功
- **THEN** 当前标签的个人页和顶栏 SHALL 更新已保存昵称、头像和邮箱状态；头像 SHALL 发起能取得新内容的受控读取，SHALL NOT 继续依赖旧缓存或把本地预览作为保存成功结果
- **AND** 邮箱状态 SHALL 重新读取服务端用户资料，不把客户端候选值当作已验证事实
- **AND** 前端 SHALL NOT 要求服务端暴露头像 SHA-256、object key 或存储配置作为显示版本

#### Scenario: 其他标签与最后实际写入
- **WHEN** 另一标签修改资料，或多个客户端并发保存同一区域
- **THEN** 当前标签 SHALL 不依赖新增资料广播即时更新，重新进入个人资料或刷新时 SHALL 读取最新服务端显示
- **AND** 最终保存值 SHALL 以数据库最后实际成功写入为准，页面 SHALL NOT 引入 CAS、编辑锁或以客户端响应抵达顺序宣称全局最终值

## ADDED Requirements

### Requirement: 个人资料邮箱换绑与验证界面
个人资料 SHALL 在现有邮箱卡片内提供内联换绑与验证界面，展示当前邮箱（已验证／未验证）、待验证邮箱和部署策略；SHALL NOT 新增独立路由，SHALL NOT 在 Web Storage 保存 token、邮箱状态或验证凭据。邮箱卡片 SHALL 支持换绑、验证当前邮箱、验证待验证邮箱、重发和取消待验证。

#### Scenario: 策略与邮箱状态展示
- **WHEN** 用户打开个人资料邮箱卡片
- **THEN** 页面 SHALL 读取 `GET /api/user/email-verification-policy` 并展示当前邮箱、验证状态和待验证邮箱
- **AND** 策略不可用时 SHALL 说明当前部署未启用邮件服务，禁用换绑和重发，但保留验证码确认与取消待验证
- **AND** 策略读取失败时 SHALL 按不可发送处理，保留取消和确认入口

#### Scenario: 换绑已验证邮箱
- **WHEN** 当前邮箱已验证，用户输入新邮箱和正确当前密码并保存
- **THEN** 页面 SHALL 提示候选邮箱将进入待验证、当前已验证邮箱继续有效
- **AND** 成功后 SHALL 展示待验证邮箱并提供重发或取消
- **AND** 页面 SHALL NOT 使当前会话失效或宣称新邮箱已验证

#### Scenario: 换绑未验证邮箱
- **WHEN** 当前邮箱未验证，用户输入新邮箱和正确当前密码并保存
- **THEN** 页面 SHALL 明确警告保存后将立即替换当前邮箱、原未验证邮箱不再保留
- **AND** 成功后 SHALL 展示新的未验证当前邮箱，而不是待验证邮箱

#### Scenario: 验证链接自动确认
- **WHEN** 用户打开 `/profile#email_token=...`
- **THEN** 页面 SHALL 在会话确认后自动提交一次验证请求，并立即清除地址栏 fragment
- **AND** 匿名打开时 SHALL 沿用现有登录 `from`（含 hash）回跳，登录后继续自动确认
- **AND** 失败时 SHALL 保留 token 供核对并显示错误，不得自动重试
- **AND** 页面 SHALL NOT 新增独立验证路由或在 Web Storage 保存 token

#### Scenario: 验证当前或待验证邮箱
- **WHEN** 用户输入验证 token 并确认
- **THEN** 页面 SHALL 调用 `POST /api/user/email-verifications/confirm` 并展示服务端返回的当前邮箱、待验证邮箱和验证状态
- **AND** 无效、过期或已使用 token SHALL 显示可恢复错误；重发 SHALL 明确目标地址
- **AND** 429 SHALL 显示通用“请求过于频繁，请稍后再试”，不展示精确倒计时
- **AND** 503 SHALL 重新读取用户资料并展示真实状态与重发入口

#### Scenario: 取消待验证邮箱
- **WHEN** 存在待验证邮箱且用户选择取消
- **THEN** 页面 SHALL 提交当前展示的 `pending_email` 作为预期值，不要求再次输入密码
- **AND** 成功后 SHALL 清空待验证展示并保留当前已验证邮箱
- **AND** 409 `IDENTITY_EMAIL_PENDING_STALE` SHALL 重新读取资料并提示待验证邮箱已变化

#### Scenario: 邮箱错误与草稿边界
- **WHEN** 换绑返回 422 当前密码或邮箱字段错误、409 邮箱占用、429 限流或 503 发送不可用
- **THEN** 页面 SHALL 分别显示字段错误、邮箱占用、限流或邮件服务不可用，并保留邮箱草稿
- **AND** 邮箱草稿 SHALL 与昵称草稿独立，切换编辑区 SHALL 遵守既有未保存修改保护
- **AND** 邮箱当前密码和验证 token SHALL NOT 作为可恢复草稿持久化

#### Scenario: 响应式与可访问性
- **WHEN** 用户在 375px、850px 或 1440px 视口使用键盘或触摸操作
- **THEN** 邮箱编辑器、待验证邮箱、取消、重发和验证码输入 SHALL 可操作
- **AND** 页面 SHALL NOT 产生页面级横向滚动
- **AND** 验证状态 SHALL 同时使用文字、语义颜色和可访问名称
