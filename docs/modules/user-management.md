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
