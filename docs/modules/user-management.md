# 用户管理

> 本文只保留用户、会话和头像的集成边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/user-management/spec.md`](../../openspec/specs/user-management/spec.md) 和 [`openspec/specs/auth/spec.md`](../../openspec/specs/auth/spec.md) 为准。

## 模块边界

`internal/identity` 负责用户身份资料、验证码、登录、JWT、Redis 登录会话、Cookie 单次续期、当前会话退出、密码和头像。角色、权限、数据范围与授权版本由 `internal/authorization` 拥有；普通退出不提升用户授权版本，也不再依赖 Access 黑名单。

## 功能入口

Swagger UI 的 `identity` 标签包含：

- 验证码、注册、登录和 Refresh Token。
- 当前用户资料、密码、头像、登出和初始化上下文。
- 管理员用户列表、修改、删除、状态切换和强制下线。
- 匿名只读的用户头像和默认头像。

## 用户与会话规则

- 注册需要唯一用户名、唯一邮箱、有效验证码和符合复杂度要求的密码。
- 个人资料页只允许修改昵称；邮箱仍由既有邮箱流程和接口管理，本文不新增邮箱换绑 UI。头像只能通过专用上传和恢复默认接口修改。
- 修改密码、真实用户身份／角色状态变化、删除用户、强制下线以及适用授权关系变化会提升授权版本；管理端纯昵称和合法全同值资料更新不创建或提升版本。
- 授权版本唯一存储在`user_access_versions`；非登录认证缺行时不补行、不回退旧字段：Access返回401 `AUTHN_TOKEN_INVALID`，Refresh保持401 `AUTHN_REFRESH_TOKEN_INVALID`且不签发Token。Access版本查询的真实数据库故障仍返回安全5xx，不伪装成认证无效，也不公开SQL或凭据；真实缺行、单表查询故障及既有错误边界见[C6验收记录](../reviews/evidence/c6-access-version-auth-fix-2026-10-17.json)。
- 用户软删除和授权版本提升在同一事务中完成，恢复用户不会让删除前 Token 重新生效。
- 管理端不能直接修改本人或受保护用户。保护依据用户与现存`roles.code=admin`的实际关联，不依据`users.role`或仅返回启用角色的运行时列表；admin角色停用后仍拒绝修改、删除、禁用、强制下线及归属变更，但不因此获得有效权限。软删管理事实通过只含ID／删除标志的独立读取获取，公开目录不扩大到软删用户；见[关联保护验收](../reviews/evidence/protected-authorization-assignments-2026-10-17.json)。
- 管理员资料更新不代改邮箱：省略或空字符串不提出邮箱变更；任何非空邮箱字符串（包括错误格式）均在业务层整次拒绝，其他字段及授权版本不部分生效。C5打通的入口与实际DTO Schema不变，历史修复验收见[去敏记录](../reviews/evidence/c5-admin-user-update-fix-2026-10-16.json)。
- 管理端PUT在锁定当前目标并通过保护／范围／邮箱检查后比较实际提交字段；纯昵称保存、合法全同值成功均不写授权版本，从未登录且无版本行的用户也不初始化。昵称／旧role字符串沿原值比较保存，不新增Trim或空值政策。真实已接受旧role字段／状态变化与资料同事务失效一次；旧`users.role`不成为RBAC身份来源，Kick、状态翻转及专用归属入口不套用资料豁免。[S2/N4/N6验收](../reviews/evidence/nickname-sessions-2026-10-18.json)已完成，delta待独立同步、未归档。
- 资料／版本写入失败的回滚，与已提交后回读失败分开判断；原程序受控回读故障中昵称已提交、版本未变，而现有映射返回401，恢复夹具后同一操作者Token仍有效。该边界单独取证，不把响应失败一律当作已回滚，也不在本目标重写既有错误类。
- 管理端资料修改、软删、状态切换和强制下线在同一事务中复核当前身份、实际保护关联与范围，再提交身份／版本写入；操作者和目标的实际角色依赖先统一升序锁定，不在JOIN中顺带锁一个不匹配的admin角色。普通只读Scope与管理current-read能力分开。
- 软删普通旧成员只有在实际移除、范围及保护前提满足时可从角色／组织集合受控清理；正常新增、保留或逐人修改软删目标仍拒绝，版本不删除。身份恢复不会重建已清关系，旧Access／Refresh仍拒绝；[R4-R6原程序证据](../reviews/evidence/authorization-reference-lifecycle-2026-10-18.json)包含这些边界。
- Seed现存身份和角色引用使用传入事务的当前目标锁；关联锁定记录查询失败继续传播，不视为“无关联”，有效管理员不重写密码。历史ID-SEED-COUNT证据保留原样，不把其旧COUNT路径改称本轮锁定查询覆盖。

## 头像安全边界

- 只接受静态 JPEG、PNG、WebP；完整解码后缩放并重新编码为 JPEG 或 PNG。
- 任一边不超过 8,192 像素，总像素不超过 40,000,000，输出最大 1,024×1,024。
- 自定义头像保存于私有 `image` bucket，外部只返回 `/api/avatars/:user_id` 或默认头像地址。
- 历史外部头像 URL 不被信任、代理或自动删除。
- 头像摘要针对标准化后的存储字节计算，不对外暴露，也不用于自动去重或恶意图片判定。
- 对象写入、数据库更新和旧对象清理遵循补偿边界；底层对象路径和存储错误不得返回客户端。

## 个人资料自助接口

- `GET /api/user/info` 和 `GET /api/user/context` 提供当前用户资料与授权上下文；`PUT /api/user/info` 的本次前端自助写入只提交 `nickname`。昵称要求非空、非全空白、最多 100 个 Unicode 字符，保留原始首尾空白和大小写；用户 ID、用户名、邮箱、角色、权限、菜单和启用状态只读。
- `GET /api/user/avatar-upload-policy` 需要有效 Access，返回 `data.max_bytes` 正整数和 `Cache-Control: no-store`。该值来自当前实际生效的 `file_upload.avatar_max_size_mb`，不返回配置细节；规则读取失败时客户端不能猜测默认上限。头像仍通过 `POST /api/user/avatar` 显式上传，`DELETE /api/user/avatar` 恢复默认。
- `PUT /api/user/password` 的新密码在 bcrypt 前执行至少 6 个 Unicode 字符、UTF-8 不超过 72 字节和至少 3 类字符校验；注册复用同一新设密码规则。密码写入与授权版本提升同一事务，成功不签发新 Token，前端随后复用现有 Cookie Logout。
- 前端密码输入只在页面内存中存在；取消、确认切换、真正离开、会话挂起／失效和身份变化都会清除敏感草稿。Access 仍有效的临时续期失败保留草稿，同 SID 手动恢复不回填密码；昵称与待保存头像预览按未保存修改规则保留。

## 前端注意事项

- 使用 `/api/user/context` 获取用户、角色、权限和菜单初始化数据。
- Access Token 只用于受保护接口，Refresh Token 只用于刷新。
- 头像可直接使用公开只读地址；管理文件下载仍需携带认证信息。
- 接口字段和状态码不要从本文复制，始终以运行中的 OpenAPI 文档为准。

## 工作台集成（已完成的个人资料自助）

本 change 已完成代码、自动化回归和隔离真实 HTTP／浏览器联合验收；脱敏证据见 [`complete-profile-self-service/evidence.md`](../../openspec/changes/complete-profile-self-service/evidence.md)，长期行为以主规格为准。

- 前端公开路由为 `/login`、`/register`；登录后有 `/profile`、`/no-management`。用户管理支持 `/users`、`/users/:id`、`/users/:id/roles`、`/users/:id/organizations`，入口同时受上下文可见菜单及权限码约束。
- `GET /api/admin/users?page=&size=&keyword=&status=` 对应 `admin.users.get`，关键词按用户名／昵称筛选，状态为 0／1，响应 `data={list,total,page,size}`，总数为范围内筛选后的结果。
- 列表项与 `GET /api/admin/users/:id`（`admin.users.id.get`）使用脱敏读模型：`id,username,nickname,avatar,email,status,roles,organizations,has_unmanaged_organizations,access_version`。`roles[]` 含 `id,code,name,status`，来自真实关系；`organizations[]` 含 `id,name`，只含操作者可管理归属。隐藏标志为 true 时，即使数组为空也不能称用户“未加入任何组织”；不得泄露隐藏组织 ID／名称。
- 角色编辑还需 `admin.roles.get`；组织编辑还需 `admin.organizations.tree.get`。状态、强制下线、删除分别受 `admin.users.id.status.put`、`admin.users.id.kick.put`、`admin.users.id.delete` 保护。本人及拥有 `admin` 角色的用户可按授权查看，但不可执行这些管理修改或归属修改。

### 用户定向写入

| 请求 | 权限码 | 必需请求数据 |
|---|---|---|
| `PUT /api/admin/users/:id/roles` | `admin.users.id.roles.put` | `{role_ids: number[], expected_access_version: number}` |
| `PUT /api/admin/users/:id/organizations` | `admin.users.id.organizations.put` | `{organization_ids: number[], expected_access_version: number}` |

数组必须显式提交，空数组表示清空对应可修改集合，不等于缺省或不修改。角色 PUT 只替换该用户角色；组织 PUT 提交该用户完整可管理组织集合并保留范围外关系与未移除关系的 `joined_at`。两者不影响其他用户，不互相覆盖，也不是跨维度原子保存。不能增删 `admin` 角色。

成功`data={access_version}`，下一维度使用返回的当前版本或重新读取详情。目标版本记录仍在同一事务按既有Ensure规则初始化／锁定并先比较；身份、保护、范围与全部引用前提满足后，合法等价关系返回current且不提升，真实关系变化才提升目标一次并使旧会话失效。缺行合法no-op可保留初始1，拒绝则本请求初始化一并回滚；这不是非登录认证补行，也不同于资料／配置no-op不创建版本。过期仍返回HTTP 409 `AUTHZ_CONFLICT`／`ORG_CONFLICT`，即使目标与当前相同也不能绕过CAS。两次合法no-op可成功，同旧版本最多一项真实修改提交；页面保留选择、重新读取核对，不静默重试。[成员差量验收](../reviews/evidence/membership-delta-2026-10-18.json)包含十对原程序MySQL交错、当前暂停点、精确返回码与最终关系，delta未同步、未归档。

纯成员写入与组织层级移动是不同失效来源：前者仅处理实际归属变化目标，后者按改变的`org_and_children`成员范围根识别子树内外依赖者，不能只处理移动节点成员，也不能以另一角色`all`或最终范围相同豁免。有限I4已完成[原程序定向验收](../reviews/evidence/organization-hierarchy-dependents-2026-10-18.json)，与用户定向CAS协议共同工作，不引入额外版本字段或请求级全局最小集合算法。

工作台只在页面内存保留 Access 与用户上下文；Refresh 仅通过 HttpOnly 会话 Cookie 传输。完整刷新先续期并读取真实上下文，确认前不展示受保护内容；退出失败保留跨刷新／标签的待确认标记。Client、浏览器能力与部署规则见 [README](../../README.md#管理工作台)，控制协议和故障处置见[登录安全策略](../runbooks/login-security.md)，架构取舍见 [ADR 0014](../adr/0014-browser-session-recovery.md)。
