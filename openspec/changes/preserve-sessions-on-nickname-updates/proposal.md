## Why

用户已经明确纯昵称属于资料变化而不是授权变化，当前接受的管理端更新却无条件提升版本。该新目标需要在C5合法HTTP入口可用后实施，不能把旧场景的条件语句反推为已确认缺陷。

## What Changes

- 管理端只改昵称时保存并返回脱敏资料，不创建/提升授权版本，旧有效Access/Refresh保持可用。
- 合法已提交字段全部同值时成功不提升；完全无可更新字段仍拒绝。
- 实际角色/身份状态变化仍按当前登录态规则提升版本，混合昵称与状态真实变化只处理一次。
- 本人/受保护身份/范围/邮箱禁止照常先校验；Kick仍主动失效，状态切换仍真正翻转。

**性质与来源**：已裁定、尚未发布的新目标；S2 / N4 / N6(identity)。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不开放管理员邮箱编辑，不改变受保护目标规则，不把昵称政策推广成所有角色/菜单展示字段不失效，不退役旧字段或重做角色分配。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `user-management`: 管理员纯昵称及合法同值资料更新不失效，与真实身份变化区分。

## Impact

- 代码：`internal/identity/application/user_service.go`、`internal/identity/application/contracts.go`、`internal/identity/adapters/gorm/repository.go`、`internal/identity/dto.go`。
- 入口：PUT /api/admin/users/:id；PUT /api/admin/users/:id/status、/kick只作不豁免对照。
- 数据：`users`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`fix-admin-user-update-validation`
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
