## Why

已裁定的集合写入和保护身份规则尚未完整发布，当前各入口前提不一致，容易把完整替换、可管理部分替换和运行时启停混为一谈。本change把明确的新写入政策落成按事实owner可验证的契约，不追认成旧缺陷。

## What Changes

- 完整验证原始目标以及实际新增/移除的旧关系对象，任一非法、越界或受保护变更整次拒绝。
- 直接编辑admin角色的用户/权限/菜单关联拒绝；现存admin关联形成保护身份，不因角色停用解除。
- 组织整组成员允许原样保留受保护关系，实际增删本人/受保护归属拒绝；删除组织不得连带移除这类归属。
- 保留停用普通用户/角色/菜单预配置；custom及菜单API集合仍非空，非custom非空organization_ids拒绝。
- 角色完整替换与逐人组织可管理部分替换分开，Seed和普通对象生命周期引用清理按各自明确例外执行。

**性质与来源**：已裁定、尚未发布的新目标；P2-P6 / R1-R3。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不把新前提全称旧漏洞，不增加Permission状态或组织范围之外另一套配置管理范围，不把共享非保护配置对操作者的间接影响当成直接本人写入。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `rbac`: 角色关联、原始集合、数据范围输入与保护身份前提。
- `menu-management`: 角色菜单与菜单API的保护、存在和集合前提。
- `organization-management`: 组织整组成员及删除的保护与范围边界。

## Impact

- 代码：`internal/authorization/application`、`internal/authorization/adapters/http`、`internal/navigation`、`internal/organization`、`internal/identity/application`、`internal/app/identity.go`、`internal/app/organization.go`。
- 入口：POST /api/admin/roles/:id/users、/permissions、/data-scope、/menus；POST /api/admin/organizations/:id/users；DELETE /api/admin/organizations/:id；PUT /api/admin/users/:id/roles、/organizations及既有Identity管理写入。
- 数据：`users`、`roles`、`user_roles`、`permissions`、`role_permissions`、`role_data_scopes`、`menus`、`role_menus`、`organizations`、`user_organizations`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`fix-menu-api-binding-id-validation`、`fix-menu-api-code-consistency`；功能整合与同名Requirement发布基线的区别见design。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
