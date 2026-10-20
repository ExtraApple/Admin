## Why

用户已裁定管理端API权限同步整次请求全成全败，而当前实现逐API调用协调操作，可能保留先前项提交。需要把本次请求的数据库事实与成功attempt返回结果纳入同一事务，不把正常跳过当失败。

## What Changes

- API Metadata拥有整请求最外层事务，Navigation/Authorization通过Context加入，不独立提交。
- API权限码、权限、授权关联及全部受影响用户版本一起提交或回滚。
- created/created_count/updated_api基于已提交成功attempt；内部重试不累加已回滚attempt结果。
- 保留不需认证与已存在权限的正常skip以及现有受控公开错误。

**性质与来源**：已裁定、尚未发布的新目标；F2。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不合并Route Catalog权限同步、API路由同步和菜单同步，不重设计Seed，不新增重试次数/部分完成响应；旧业务计数问题未复现，不冒称已确认缺陷。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `api-management`: API权限同步整请求原子性、失败响应与成功attempt统计。

## Impact

- 代码：`internal/apimetadata/application/service.go`、`internal/apimetadata/application/contracts.go`、`internal/navigation/service.go`、`internal/platform/database/transaction.go`、`internal/app`。
- 入口：POST /api/admin/apis/sync-permissions；admin.apis.sync-permissions.post不变。
- 数据：`apis`、`menus`、`menu_apis`、`permissions`、`role_permissions`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`fix-menu-api-code-consistency`、`enforce-authorization-reference-lifecycle`；整轮事务继续满足已裁定的R6严格锁序，逐API版本触发不变。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
