## Context

身份模块已实现邮箱换绑、一次性验证凭据、重发和确认：`PUT /api/user/info` 接收 `email`／`current_password`，已验证当前邮箱进入 `pending_email`，未验证当前邮箱直接替换；`POST /api/user/email-verifications` 重发，`POST /api/user/email-verifications/confirm` 确认。默认 `smtp.enabled: false`，组合根只在启用时构造 sender 并注入注册和 `UserService`；关闭时换绑和重发返回通用 500。

前端个人资料页只读展示 `email`、`email_verified`、`pending_email`；`user-management` 规格明确禁止个人资料界面提供邮箱编辑、验证或重发入口。`CONTEXT.md` 已补充当前邮箱、邮箱换绑、邮箱验证和取消待验证术语。现有 `WorkbenchShell` 在匿名访问时把 `location.hash` 放入登录 `from`，登录后回跳保留 hash。

## Goals / Non-Goals

**Goals:**

- 在个人资料邮箱卡片内完成换绑、验证当前邮箱、验证待验证邮箱、重发和取消待验证。
- 增加邮件内验证链接和稳定的 SMTP 可用性信号，避免把未配置邮件服务呈现为通用故障。
- 取消待验证邮箱，解决候选地址长期占用；保留 pending 直到验证／取消／再次换绑。
- 保持现有密码再认证、唯一性、待验证语义、会话和授权版本行为。
- 用单元／HTTP／组件测试和 Mailpit 真实浏览器验收覆盖完整链路。

**Non-Goals:**

- 多邮箱、邮箱登录、管理员代改邮箱。
- 待验证邮箱自动过期清理、会话失效、异步／Outbox 邮件投递。
- 注册页“邮件未发送”提示、精确限流倒计时、短信或手机验证。
- 新权限码、数据库列／表、Redis key、MinIO bucket 或独立验证路由。

## Decisions

### 1. 采用前端 + 最小后端补齐

本期实现邮箱卡片，同时补齐邮件内链接、策略端点、取消待验证和稳定错误码。现有换绑／确认／重发协议不重写；不引入异步邮件投递或队列。

**Alternative rejected:** 仅前端 + token 粘贴。邮件只有 43 字符 token，复制粘贴体验差，且 pending 地址无法显式释放。

**Alternative rejected:** 完整后端重构（Outbox、自动过期、会话失效）。超出本次用户可感知的邮箱管理范围，且会牵动消息与授权模块。

### 2. 验证链接放入 fragment，不新增路由

邮件正文使用 `${browser_session.origin}/profile#email_token=...`。链接进入个人资料页后读取 hash、自动提交一次确认、立即用 `history.replaceState` 清除 fragment。token 不进入服务端访问日志；匿名访问沿用现有 `from`（含 hash）登录回跳。手动 token 输入保留作为邮件客户端不保留链接时的兜底。

**Alternative rejected:** query 参数 `?email_token=`。会被反向代理、访问日志和 Referer 记录，扩大凭据暴露面。

**Alternative rejected:** 新增 `/verify-email` 路由。现有个人资料页和登录回跳已能承载，新增路由会增加会话与回跳分支。

### 3. SMTP 可用性通过策略端点暴露

新增 `GET /api/user/email-verification-policy`，由组合根传入 `config.SMTP.Enabled`，返回 `{enabled}`。邮箱卡片在读取后决定是否允许换绑／重发；策略读取失败按不可发送处理。发送器未配置时，换绑和重发返回 503 `IDENTITY_EMAIL_VERIFICATION_DISABLED`，替换当前通用 500。确认与取消不依赖 SMTP，保持可用。

**Alternative rejected:** 前端始终展示操作、依赖 500 反馈。会把部署配置问题伪装成业务错误，且用户无法预先知道功能不可用。

**Alternative rejected:** 把 enabled 放进 `GET /api/user/info`。资料响应应保持用户事实，部署能力独立读取更清晰，也不扩大资料缓存语义。

### 4. 取消待验证使用预期值与用户行锁

新增 `POST /api/user/email-verifications/cancel`，请求包含当前展示的 `pending_email`。服务在用户行锁内比较；匹配则清空 `pending_email` 并失效该候选的未使用凭据；不匹配或已无 pending 返回 409 `IDENTITY_EMAIL_PENDING_STALE`。取消不需要密码。确认、再次换绑和取消都按 ADR 0010 的用户行锁线性化，避免旧页面覆盖新候选。

**Alternative rejected:** 无预期值的幂等清空。多标签场景下，旧页面会误清空另一标签刚设置的新候选。

**Alternative rejected:** 取消要求当前密码。pending 不是当前邮箱，取消是撤销候选而非建立新所有权；已认证会话足够。

### 5. 待验证邮箱不自动过期

token 15 分钟过期后保留 `pending_email`，继续占用地址唯一性，直到验证、取消或再次换绑。UI 明确提示并提供取消。这样“重发”在 token 过期后仍可用，不会静默丢弃用户意图。

**Alternative rejected:** 清理任务在凭据过期后自动释放 pending。会静默撤销候选，并让“重发”失去目标；如果未来要自动释放，应作为独立策略变更。

### 6. 保持未验证邮箱直接替换语义

当前邮箱未验证时，换绑直接替换当前邮箱并签发验证凭据，不进入 `pending_email`。UI 对此显示明确警告。待验证语义只保护已验证邮箱；未验证邮箱没有可保护的所有权事实。

**Alternative rejected:** 所有换绑都进入 pending。会让一个未验证地址继续占据当前邮箱，且与既有后端和规格语义冲突。

### 7. 邮箱变化不提升授权版本

换绑、确认、取消均不提升访问版本、不使现有会话失效。换绑已要求当前密码再认证；邮箱不是登录名或授权事实。密码、角色、权限和状态变化仍按既有规则失效。

**Alternative rejected:** 已验证邮箱切换时强制失效会话。收益有限，却会使用户在验证邮件后被迫重新登录，并扩大授权版本写入范围。

### 8. 前端邮箱编辑器与昵称完全分离

个人资料扩为四个独立编辑区：昵称、头像、邮箱、密码。邮箱区拥有独立 dirty 状态，只提交 `email`／`current_password`，只清理首尾空白并保留大小写；不与昵称合并提交，不把验证状态或待验证邮箱作为可写字段。成功后更新本地用户并刷新会话上下文；503 后重新读取 `GET /api/user/info`；429 只显示通用限流提示。邮箱当前密码和验证 token 不持久化，会话挂起时清空。

**Alternative rejected:** 把邮箱放进昵称表单一次保存。会让两个独立协议共享 dirty／错误／重试状态，容易把昵称写入和邮箱换绑互相牵连。

### 9. 邮件仍同步发送，验收使用 Mailpit

保持现有同步 SMTP 发送；投递失败时凭据失效并返回 503，但数据库中的 pending／未验证替换可能已提交，因此前端收到 503 后重新读取资料，展示真实状态并提供重发。仓库默认 `smtp.enabled: false`；隔离验收启用 Mailpit（SMTP 1025／HTTP 8025），不增加日志打印 token 或假发送模式。

**Alternative rejected:** 本期引入 Outbox／RabbitMQ 邮件投递。会扩大消息模块和失败重试契约；当前同步发送加 503 重读已能表达结果未知。

**Alternative rejected:** 开发环境把 token 打到日志。验证 token 是 bearer 凭据，日志泄露风险不可接受。

### 10. 领域术语与规格同步

`CONTEXT.md` 已定义当前邮箱、邮箱换绑、邮箱验证和取消待验证；`user-management` 增量规格新增完整邮箱换绑与验证要求，`admin-workbench` 更新个人资料编辑区、身份字段边界、续期草稿和跨标签同步。ADR 0010 继续覆盖并发线性化，本期不新增 ADR。

## Risks / Trade-offs

- [待验证邮箱长期占用地址] → 提供显式取消；UI 提示 pending 保留和占用；不自动清理。
- [同步 SMTP 在数据库提交后发送失败] → 返回 503，前端重新读取真实状态并提供重发；后续可用 Outbox 改进。
- [token 通过邮件和链接传播] → 15 分钟、一次性、绑定用户与目标邮箱；链接放 fragment；不在 Web Storage 保存；会话挂起清空。
- [策略读取失败时功能不可用] → fail-closed 禁用换绑／重发，保留确认和取消；错误信息可重试。
- [429 的 `retry_after_seconds` 是窗口长度而非精确剩余时间] → UI 只显示通用限流提示，不展示倒计时。
- [邮箱唯一性依赖数据库排序规则] → UI 保留大小写、只 Trim 首尾；服务端校验与唯一性仍是最终判据，验收覆盖大小写重复。
- [登录回跳依赖现有 hash 保留] → 复用既有 shell 的 `from` 行为并增加匿名链接验收；不新增路由。
- [默认 SMTP 关闭导致功能不可用] → 策略端点明确说明；部署手册记录启用方式；验收使用 Mailpit。
- [无会话失效] → 换绑要求当前密码；如未来安全策略要求失效，应独立评估，不在本期隐式改变。

## Migration Plan

1. 先部署后端：新增策略与取消路由、稳定错误码、邮件链接和 sender 配置。新路由为增量；SMTP 关闭时换绑／重发从通用 500 变为 503，属于明确的错误契约修正。
2. 再发布前端：邮箱编辑器读取策略；未启用 SMTP 的部署显示说明并禁用发送操作。
3. 无数据库迁移、无权限码、无菜单种子变更；取消只清空既有 `pending_email` 并失效凭据。
4. 回滚：回退前端即可恢复只读邮箱卡片；后端新增路由和错误码可保持；如需恢复旧 500 行为，回退后端发布。
5. 验收环境临时启用 Mailpit 和 SMTP 配置，验收后停止容器并恢复默认配置。

## Open Questions

无。自动过期、Outbox 投递、邮箱登录和会话失效均为后续独立变更。
