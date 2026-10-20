## Why

现行菜单绑定要求提交API全部合法，但原始列表中的零ID在校验前被过滤，适用真实请求已经写入关系。C1需要在实际业务入口恢复整次拒绝，而不是只增加DTO外观校验。

## What Changes

- 对原始api_ids逐项拒绝零ID，再对合法值去重；内部Application调用与HTTP遵守同一规则。
- 保留非空集合及API存在、启用、需要认证的既有前提。
- 无效输入不修改Menu/API/关系/Permission/角色授权/任何用户版本；合法重复ID仍可规范化。

**性质与来源**：现行承诺下已核实缺陷；C1 / R1.a。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不修改所有unique ID helper的全局语义，不将根parent_id=0判非法，不借此新增空数组解绑或所有批量入口政策。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `menu-management`: 明确菜单API原始列表的整次校验及失败状态不变。

## Impact

- 代码：`internal/navigation/routes.go`、`internal/navigation/service.go`、`internal/navigation/repository.go`。
- 入口：POST /api/admin/menus/:id/apis；admin.menus.id.apis.post不变。
- 数据：`menus`、`apis`、`menu_apis`、`permissions`、`role_permissions`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：无外部change前置；与其他独立修复可分别实施。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
