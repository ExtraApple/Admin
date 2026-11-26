# admin-workbench Specification Delta

## ADDED Requirements

### Requirement: 已软删除用户回收站（恢复与彻底注销）
工作台 SHALL 在用户管理模块提供受 `admin.deleted-users.get` 权限控制的独立静态子路由 `/users/deleted`（面包屑「工作台 / 用户管理 / 已删除用户」，归属用户管理模块页签），对接 `GET /api/admin/deleted-users`、`POST /api/admin/users/:id/restore` 与 `POST /api/admin/users/:id/anonymize`，为全量数据范围管理员提供已软删除用户的检索、恢复与高风险受控匿名化（彻底注销）闭环。

#### Scenario: 浏览与搜索已软删除用户列表
- **WHEN** 具备 `admin.deleted-users.get` 权限的管理员从 `/users` 列表进入或直达 `/users/deleted`，并输入关键词或切换分页
- **THEN** 页面 SHALL 通过 URL 查询参数同步 `keyword`、`page`、`size` 并向 `GET /api/admin/deleted-users` 请求真实分页结果
- **AND** 桌面视口 SHALL 以表格、窄屏视口（含 375px / 850px）SHALL 以卡片展示用户名、昵称、邮箱、删除时间、保留角色/组织摘要及受保护标识，不产生页面级横向溢出
- **AND** 若服务端因非全量数据范围返回 `403 IDENTITY_PERMISSION_DENIED`，页面 SHALL 明确提示该视图仅限全量数据范围管理员访问，不得伪报为空列表

#### Scenario: 确认恢复软删除用户
- **WHEN** 具备 `admin.users.id.restore.post` 权限的管理员在 `/users/deleted` 对非保护的已软删除用户点击「恢复用户」
- **THEN** 页面 SHALL 打开确认弹窗，说明恢复后用户将回到正常用户列表、删除前登录会话与邮箱验证链接仍保持失效、且软删期间已清理的角色或组织归属不会自动恢复
- **AND** 确认后 SHALL 调用 `POST /api/admin/users/:id/restore`；成功后刷新列表（尾页删空且 `page > 1` 时回退上一页），并在具备 `admin.users.id.get` 权限时提供前往已恢复用户详情的反馈入口
- **AND** 失败（如 `409 IDENTITY_CONFLICT` 或 `403 IDENTITY_PERMISSION_DENIED`）时 SHALL 展示本地化错误且不显示成功提示

#### Scenario: 手输用户名二次确认彻底注销（受控匿名化）
- **WHEN** 具备 `admin.users.id.anonymize.post` 权限的管理员在 `/users/deleted` 对非保护的已软删除用户点击「彻底注销」
- **THEN** 页面 SHALL 打开高风险二次确认弹窗，明确说明该操作不可撤销：将改写用户名与邮箱并立即释放原用户名与邮箱、清除密码/昵称/待验证邮箱/可信自定义头像以及全部角色与组织归属，同时保留用户 ID 墓碑以维持历史审计日志、文件与消息引用完整（且不改写历史审计日志）
- **AND** 提交按钮 SHALL 在管理员手动输入与目标当前 `username` 精确一致的确认文本前保持禁用
- **AND** 确认后 SHALL 调用 `POST /api/admin/users/:id/anonymize` 并携带 `{ confirm_username }`；成功后刷新列表并在当前页清空且 `page > 1` 时回退上一页
- **AND** 若服务端返回 `422`（确认用户名不匹配）、`409`（状态已变化）或 `403`，页面 SHALL 保留错误反馈且不得假报成功

#### Scenario: 受保护的已软删除用户不提供恢复或彻底注销按钮
- **WHEN** 回收站列表中某条记录标记为 `protected = true`（关联 `admin` 角色）
- **THEN** 页面 SHALL 标明受保护身份并说明由系统初始化规则托管，SHALL NOT 提供可执行的「恢复用户」或「彻底注销」动作

## MODIFIED Requirements

### Requirement: 用户管理动作与快捷浮层
工作台 SHALL 使用现有单用户操作完成启用／停用、强制下线和删除；状态、确认及操作结果 SHALL 以真实服务器响应为准。`/users` 列表页在管理员具备 `admin.deleted-users.get` 权限时 SHALL 提供进入 `/users/deleted` 的「已删除用户」入口。

#### Scenario: 确认用户管理动作
- **WHEN** 管理员对非本人、非 `admin` 角色的可管理用户选择切换状态、强制下线或删除
- **THEN** 页面 SHALL 说明对象与动作影响，确认后才提交实际操作，成功后更新显示的用户状态或列表
- **AND** 强制下线 SHALL 不被展示为资料或状态变化，软删除 SHALL 不被展示为仅将用户停用，且软删除确认文案 SHALL 准确说明账号将移入已删除用户回收站、在恢复或彻底注销前仍保留用户名与邮箱占用并立即使旧会话与待验证邮箱失效

#### Scenario: 用户管理动作失败
- **WHEN** 服务端拒绝状态切换、强制下线或删除，或请求失败
- **THEN** 页面 SHALL 保持与服务端确认结果一致的列表／详情，显示错误且不展示成功提示

#### Scenario: 展开用户快捷操作
- **WHEN** 管理员在桌面行或移动端卡片点击一位用户的操作按钮
- **THEN** 快捷操作 SHALL 以该按钮下方的不撑高行／卡片的实色浮层展开（桌面表格右对齐、窄屏卡片左对齐），并在视口边界自动防溢出翻转
- **AND** 浮层 SHALL 只包含该用户当前获准的动作，并清晰区分危险删除动作

#### Scenario: 收起用户快捷操作
- **WHEN** 管理员点击浮层以外区域、按 Escape、打开另一位用户的操作菜单、滚动列表或页面，或离开当前视图
- **THEN** 原浮层 SHALL 收起且不触发未选择的用户操作
