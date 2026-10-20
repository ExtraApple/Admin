## Why

用户已裁定合法可选授权属性的显式空值应清空，而遗漏字段保持不变；当前字符串零值表示使多个入口无法区分两者。需要为已触及配置属性提供明确请求语义，而不是引入全平台PATCH协议。

## What Changes

- 角色description、权限group、菜单icon/component及合法path更新使用存在性感知的HTTP与Application输入。
- 遗漏保持原值，显式空字符串清空，非空替换；路径继续满足现有唯一性/类型等适用前提。
- 必填name/code的显式空值拒绝，不把不可清空字段或未提交字段算合法更新。
- 一次请求的属性/关系和版本按其既有事务契约提交；真实清空是配置变化，不提前实现no-op豁免。

**性质与来源**：已裁定、尚未发布的新目标；S1。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不扩展Identity邮箱/昵称空值政策、API/字典/组织全部可选字段，不裁定已绑定permission_code清空或S3自动码新规则，不新增通用PATCH抽象。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `rbac`: 角色说明与权限分组标签的显式清空。
- `menu-management`: 菜单可选展示/合法路径字段的存在性与清空。

## Impact

- 代码：`internal/authorization/adapters/http/roles.go`、`internal/authorization/adapters/http/permissions.go`及Routes说明、`internal/authorization/application/service.go`、`internal/navigation/routes.go`、`internal/navigation/types.go`、`internal/navigation/service.go`；`internal/apidoc/service.go`反射既有string binding长度约束以对齐本次PUT schema。
- 入口：PUT /api/admin/roles/:id、/permissions/:id、/menus/:id；现有权限码不变。
- 数据：`roles`、`permissions`、`menus`、`user_access_versions（保留对应配置事务语义）`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`fix-menu-api-code-consistency`、`enforce-authorization-reference-lifecycle`
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
