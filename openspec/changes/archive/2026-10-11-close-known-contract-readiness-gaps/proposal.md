## Why

已确认的管理员邮箱边界、登录锁定说明及登出黑名单 TTL 文档仍存在矛盾，OpenAPI 生成失败缺少 HTTP 层回归保护，归档后的验收链接也已失效。需要闭环这些已知遗留，避免继续按旧承诺操作或把文档修正误当成新增功能。

## What Changes

- 保留管理员不能代改邮箱、用户本人经密码校验与邮箱验证更正的现有边界；修正旧用户管理规格，删除 `UpdateByAdmin` 中不可达的邮箱更新分支。
- 在认证规格和运维说明中明确现有累计升级锁定：5～9 次为 1 分钟，10～14 次为 5 分钟，15～19 次为 15 分钟，20 次及以上为 1 小时；第 10 次和锁定到期不清零，每次记录失败续期 24 小时，成功登录清除计数。
- 按已发布认证契约修正登出 TTL 运维说明，保留 `jwt.expire` 配置固定时长，不改成 Token 剩余有效期。
- 明确 OpenAPI 生成失败的稳定 `HTTP_INTERNAL_ERROR`，增加实际 HTTP handler 的错误信封与内部错误不泄露回归，并以 HTTP 冒烟验证。
- 修正 README、模块导航和前端 ADR 中指向已归档 change 的失效链接，保留历史验收内容与结论。

### Non-Goals

- 不实施剩余 C6b 的全量逐条审计，不把本批完成视为 C6b 完成。
- 不新增个人资料自助流程、其他资源管理页面、管理员协助变更邮箱、手机验证、多语言字典或日志月分表。
- 不重新设计锁定策略、登出语义、角色及状态写入、响应信封或外部通知资格。
- 不重写历史验收工件，不清理无关旧代码，不迁移或恢复已退役的技术目录。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `user-management`：管理员更新普通用户的允许字段不再包含邮箱；明确携带非空邮箱更新时拒绝整次更新，保持邮箱所有权边界。
- `auth`：补齐登录锁定的现有档位、累计保留及清除规则；登出 TTL 已有正确主规格，不提交重复 delta。
- `api-management`：将 OpenAPI 生成失败的稳定技术错误码明确为现有 `HTTP_INTERNAL_ERROR`，保留四字段信封和泄漏防护要求。

## Impact

- 生产代码仅删除 `internal/identity/application/user_service.go` 的不可达邮箱分支，保留前置拒绝和其他可达更新语义。
- 验证涉及 Identity 既有行为测试、`internal/apidoc/service_test.go` 的 HTTP 失败回归及一次实际 HTTP 冒烟。
- 文档涉及 `docs/runbooks/login-security.md`、`README.md`、`docs/module-navigation.md`、`docs/adr/0011-react-frontend-and-customizable-ui-stack.md` 和 `docs/reviews/contract-readiness.md`。
- 既有 `PUT /api/admin/users/:id`、`POST /api/login`、`POST /api/user/logout`、`GET /docs/openapi.json` 的路由、访问等级、权限码及运行时请求／响应格式保持不变；不新增兼容路径。
- 不修改模型、数据库表、迁移、依赖、MinIO bucket 或配置。既有 `fail:<username>`、`lock:<username>`、`blacklist:<token>` 的 Redis key 与运行时策略保持不变。
