## Why

普通菜单PUT改码会留下菜单与已绑定API不同码，C3已经观察真实提交；这违反现行统一权限码及菜单API一致性保证。需要让菜单侧写入消费既有绑定联动，而不是只修返回值。

## What Changes

- 菜单侧修改权限码时，在同一最外层事务复用既有绑定码解析、权限确保、授权合并及清理能力。
- 同步现有直接绑定API，保留关系集合；字段、权限授权和用户版本共同提交，失败整体回滚。
- 提取事务内共享编排，避免嵌套用例重复提升版本或单独提交；响应读取最终持久化事实。

**性质与来源**：现行承诺下已核实缺陷；C3。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不设计S3复杂多菜单图传播、旧API授权迁移新政策或method/path/NeedAuth与空code组合的新优先级；不做全库历史不一致修复或最小失效集合算法。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `menu-management`: 明确普通菜单改码同样受现有菜单API联动约束。

## Impact

- 代码：`internal/navigation/service.go`、`internal/navigation/contracts.go`、`internal/navigation/repository.go`、`internal/app/navigation.go`。
- 入口：PUT /api/admin/menus/:id、既有POST /api/admin/menus/:id/apis；权限码不新增。
- 数据：`menus`、`apis`、`menu_apis`、`permissions`、`role_permissions`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`fix-menu-api-binding-id-validation`
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
