## Why

用户已选择合法语义无变化的配置请求不使旧会话失效，但真正角色/菜单展示变化仍属于配置变化。这是比现行宽泛版本触发更精细的新产品行为，需要基于提交时事实而非RowsAffected实现。

## What Changes

- 角色/菜单字段及角色权限/菜单/custom/API绑定等配置集合，在完成权限、保护、引用、范围和现有并发前提后比较规范化的目标事实。
- 无字段请求仍拒绝；真正同值或等价配置成功且不提升版本；派生关系仍需变更时不得假no-op。
- 真实角色/菜单展示变化仍使现行依赖集合失效；不以另一角色重复授权或最终权限未变豁免。
- 同步正常skip且没有任何派生写入时不额外失效；不改变未知parent_path处理。

**性质与来源**：已裁定、尚未发布的新目标；N1(config) / N3 / N6(config)。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不覆盖成员关系差量或Identity资料入口，不新增全局最小失效/最终权限diff，不豁免真实展示变化、Kick或状态切换。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `rbac`: 角色字段、权限和数据范围配置无变化豁免及真实展示变化。
- `menu-management`: 菜单字段、角色菜单、API绑定与菜单同步的无变化规则。

## Impact

- 代码：`internal/authorization/application/service.go`、`internal/navigation/service.go`、`internal/authorization/adapters/gorm/repository.go`、`internal/navigation/types.go`。
- 入口：PUT /api/admin/roles/:id、/menus/:id；POST角色权限/数据范围/菜单、菜单API绑定及既有菜单同步。
- 数据：`roles`、`role_data_scopes`、`role_permissions`、`menus`、`role_menus`、`menu_apis`、`permissions`、`apis`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`enforce-authorization-reference-lifecycle`、`enforce-protected-authorization-assignments`、`fix-menu-api-code-consistency`、`support-explicit-authorization-field-clearing`；功能整合与同名Requirement发布基线的区别见design。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
