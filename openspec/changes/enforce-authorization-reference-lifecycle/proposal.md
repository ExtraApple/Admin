## Why

已确认的模式切换、角色删除和绑定/删除顺序政策还未成为完整生命周期契约。应保证成功提交不留下非法引用，同时允许合法的先绑定后删除和受控软删旧成员清理，而不是发明全局生命周期框架。

## What Changes

- 从custom切走时两个角色修改入口都清理旧custom关联；普通角色删除同事务清用户/权限/数据范围/菜单映射，不删共享资源。
- 仍有普通角色custom引用时拒绝组织删除，停用角色或无成员不豁免，不自动改成self/all。
- 所有现有新增关系写入与删除/停用共享目标行锁及当前读取协议，删除先完成则拒绝新引用；绑定先成功可随后合法删除并清理其新增引用和用户版本。
- 软删普通旧成员只允许在管理范围与保护前提下受控移除，不可重新加入，不删除版本，不因恢复身份复活被清除归属。
- 失败只回滚本请求，保留其他并发合法胜方；不承诺所有重叠请求必有一方失败。

**性质与来源**：已裁定、尚未发布的新目标；R4-R6 / R6.e1-e4,e6-e8。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不增加全局关系修复器、数据库级级联删除、S3多菜单传播策略、组织移动影响算法或Seed全轮事务，不预先宣称所有锁竞争都成功。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `rbac`: 普通角色删除、范围模式切换及关联对象生命周期。
- `menu-management`: 菜单/API关联提交前提与普通配置引用清理。
- `organization-management`: custom阻删与组织关联提交时一致性。
- `user-management`: 角色/组织关系与软删身份生命周期边界。

## Impact

- 代码：`internal/authorization/application`、`internal/authorization/adapters/gorm`、`internal/navigation`、`internal/organization`、`internal/identity/application`、`internal/identity/adapters/gorm`、`internal/app`。
- 入口：既有角色/权限/菜单/API/组织DELETE、角色与组织关联写入、数据范围及API启停接口；不新增路由。
- 数据：`users`、`roles`、`user_roles`、`role_permissions`、`role_data_scopes`、`role_menus`、`menus`、`apis`、`menu_apis`、`organizations`、`user_organizations`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`enforce-protected-authorization-assignments`、`fix-concurrent-organization-moves`、`fix-menu-api-code-consistency`、`fix-menu-create-status-preservation`；功能整合与同名Requirement发布基线的区别见design。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
