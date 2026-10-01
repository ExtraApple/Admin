# 用户管理

> 本文只保留用户、会话和头像的集成边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/user-management/spec.md`](../../openspec/specs/user-management/spec.md) 和 [`openspec/specs/auth/spec.md`](../../openspec/specs/auth/spec.md) 为准。

## 模块边界

`internal/identity` 负责用户身份资料、验证码、登录、JWT、Refresh Token、黑名单、密码和头像。角色、权限、数据范围与授权版本由 `internal/authorization` 拥有。

## 功能入口

Swagger UI 的 `identity` 标签包含：

- 验证码、注册、登录和 Refresh Token。
- 当前用户资料、密码、头像、登出和初始化上下文。
- 管理员用户列表、修改、删除、状态切换和强制下线。
- 匿名只读的用户头像和默认头像。

## 用户与会话规则

- 注册需要唯一用户名、唯一邮箱、有效验证码和符合复杂度要求的密码。
- 个人资料只允许修改昵称和邮箱；头像只能通过专用上传和恢复默认接口修改。
- 修改密码、修改用户状态、删除用户、强制下线以及授权关系变化会提升授权版本。
- 授权版本唯一存储在 `user_access_versions`；非登录认证缺少版本记录时拒绝 Token。
- 用户软删除和授权版本提升在同一事务中完成，恢复用户不会让删除前 Token 重新生效。
- 超级管理员不能通过普通管理接口修改、删除、禁用或强制下线自己及其他超级管理员。

## 头像安全边界

- 只接受静态 JPEG、PNG、WebP；完整解码后缩放并重新编码为 JPEG 或 PNG。
- 任一边不超过 8,192 像素，总像素不超过 40,000,000，输出最大 1,024×1,024。
- 自定义头像保存于私有 `image` bucket，外部只返回 `/api/avatars/:user_id` 或默认头像地址。
- 历史外部头像 URL 不被信任、代理或自动删除。
- 头像摘要针对标准化后的存储字节计算，不对外暴露，也不用于自动去重或恶意图片判定。
- 对象写入、数据库更新和旧对象清理遵循补偿边界；底层对象路径和存储错误不得返回客户端。

## 前端注意事项

- 使用 `/api/user/context` 获取用户、角色、权限和菜单初始化数据。
- Access Token 只用于受保护接口，Refresh Token 只用于刷新。
- 头像可直接使用公开只读地址；管理文件下载仍需携带认证信息。
- 接口字段和状态码不要从本文复制，始终以运行中的 OpenAPI 文档为准。

## 工作台集成（实施中的 change）

以下为现有代码的接入说明；长期规格尚未同步的部分见 [`build-admin-workbench` 用户 delta](../../openspec/changes/build-admin-workbench/specs/user-management/spec.md)，不表示联合验收已经通过。

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

成功 `data={access_version}`，下一维度须使用新版本或重新读取详情。目标版本记录在同一事务锁定、检查并提升；过期返回 HTTP 409 `AUTHZ_CONFLICT`／`ORG_CONFLICT`，无部分提交。页面保留选择，要求重新读取、展示最新归属并核对后再手动提交，不静默重试。成功会使目标用户旧会话在后续请求失效。

工作台只保留内存 Access／Refresh Token，完整刷新需重新登录；Client、原生资源入口及部署规则见 [README](../../README.md#管理工作台)。
