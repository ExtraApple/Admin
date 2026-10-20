package httpadapter

import (
	"net/http"
	"reflect"

	"admin/internal/authorization/application"
	"admin/internal/routecatalog"
)

// Routes returns the complete Authorization-owned RBAC HTTP surface.
func Routes(service *application.Service, routeSource application.RoutePermissionSource) []routecatalog.Descriptor {
	handler := &Handler{service: service, routes: routeSource}
	overview := authRoute(http.MethodGet, "/api/admin/authorization-overview", "Get Authorization Overview", "admin.authorization.overview.get", handler.getAuthorizationOverview, nil, authorizationOverviewResponse{})
	risks := authRoute(http.MethodGet, "/api/admin/authorization-risks", "List Authorization Risks", "admin.authorization.risks.get", handler.listAuthorizationRisks, nil, authorizationRiskPageResponse{})
	risks.OpenAPI.QuerySchema = reflect.TypeOf(authorizationRiskListQuery{})
	roleUpdate := authRoute(http.MethodPut, "/api/admin/roles/:id", "Update Role", "admin.roles.id.put", handler.updateRole, updateRoleRequest{}, roleResponse{})
	roleUpdate.OpenAPI.Description = "未提交的字段保持原值；显式空字符串清空 description，name、code 不得为空。JSON null 沿既有解析表示未提交。完成保护、唯一性及当前事实校验后，合法同值不提升授权版本；真实角色展示或派生配置变化仍原子失效，空更新仍拒绝。"
	permissionUpdate := authRoute(http.MethodPut, "/api/admin/permissions/:id", "Update Permission", "admin.permissions.id.put", handler.updatePermission, updatePermissionRequest{}, permissionResponse{})
	permissionUpdate.OpenAPI.Description = "未提交的字段保持原值；显式空字符串清空 group 标签，不删除权限或分组资源。name 不得为空，权限码仍不可修改。JSON null 沿既有解析表示未提交。"
	roleScope := authRoute(http.MethodPost, "/api/admin/roles/:id/data-scope", "Assign Role Data Scope", "admin.roles.id.data-scope.post", handler.assignDataScope, assignDataScopeRequest{}, nil)
	roleScope.OpenAPI.Description = "先校验角色保护、模式和全部原始组织ID；模式与适用组织集合相同则成功且不提升版本。非custom残留关联仍须清理并作为真实变化原子失效。"
	rolePermissions := authRoute(http.MethodPost, "/api/admin/roles/:id/permissions", "Assign Role Permissions", "admin.roles.id.permissions.post", handler.assignPermissions, assignPermissionsRequest{}, nil)
	rolePermissions.OpenAPI.Description = "先校验角色保护及全部原始权限ID，再去重比较实际配置；等价集合成功且不重建关联、不提升版本。真实关系变化仍在同一事务处理角色用户版本。"
	userRoles := authRoute(http.MethodPut, "/api/admin/users/:id/roles", "Set User Roles", "admin.users.id.roles.put", handler.setUserRoles, updateUserRolesRequest{}, accessVersionResponse{})
	userRoles.OpenAPI.Description = "先按既有Ensure规则锁定／初始化共享授权版本并比较expected_access_version，再完成身份、保护、范围和角色引用校验。合法等价集合返回当前access_version，不重建保留关系、不提升版本；真实关系变化仅提升目标一次。同旧版本最多一项真实修改提交；真实修改后旧版本请求即使集合相同仍返回409 AUTHZ_CONFLICT。"
	roleUsers := authRoute(http.MethodPost, "/api/admin/roles/:id/users", "Assign Role Users", "admin.roles.id.users.post", handler.assignRoleUsers, assignUsersRequest{}, nil)
	roleUsers.OpenAPI.Description = "完整替换该角色的用户目标，先验证全部原始目标及实际移除对象；只增删差量并使新增／移除用户失效，保留成员关系及版本。合法等价集合不提升版本；不以最终有效权限重复豁免真实归属变化。"
	return []routecatalog.Descriptor{
		overview,
		risks,
		authRoute(http.MethodGet, "/api/admin/roles", "List Roles", "admin.roles.get", handler.listRoles, nil, roleListResponse{}),
		authRoute(http.MethodGet, "/api/admin/roles/:id", "Get Role", "admin.roles.id.get", handler.getRole, nil, roleResponse{}),
		userRoles,
		authRoute(http.MethodPost, "/api/admin/roles", "Create Role", "admin.roles.post", handler.createRole, createRoleRequest{}, roleResponse{}),
		roleUpdate,
		authRoute(http.MethodDelete, "/api/admin/roles/:id", "Delete Role", "admin.roles.id.delete", handler.deleteRole, nil, nil),
		roleUsers,
		authRoute(http.MethodGet, "/api/admin/roles/:id/users", "List Role Users", "admin.roles.id.users.get", handler.listRoleUsers, nil, userListResponse{}),
		roleScope,
		authRoute(http.MethodGet, "/api/admin/roles/:id/data-scope", "Get Role Data Scope", "admin.roles.id.data-scope.get", handler.getDataScope, nil, dataScopeResponse{}),
		authRoute(http.MethodGet, "/api/admin/permissions", "List Permissions", "admin.permissions.get", handler.listPermissions, nil, permissionListResponse{}),
		authRoute(http.MethodPost, "/api/admin/permissions", "Create Permission", "admin.permissions.post", handler.createPermission, createPermissionRequest{}, permissionResponse{}),
		permissionUpdate,
		authRoute(http.MethodDelete, "/api/admin/permissions/:id", "Delete Permission", "admin.permissions.id.delete", handler.deletePermission, nil, nil),
		authRoute(http.MethodGet, "/api/admin/permission-codes", "List Permission Codes", "admin.permission-codes.get", handler.permissionCodes, nil, codesResponse{}),
		authRoute(http.MethodPost, "/api/admin/permissions/sync", "Sync Route Permissions", "admin.permissions.sync.post", handler.syncPermissions, nil, syncResponse{}),
		rolePermissions,
		authRoute(http.MethodGet, "/api/admin/roles/:id/permissions", "List Role Permissions", "admin.roles.id.permissions.get", handler.rolePermissions, nil, permissionListEnvelope{}),
		authRoute(http.MethodGet, "/api/admin/permission-groups", "List Permission Groups", "admin.permission-groups.get", handler.listGroups, nil, groupListResponse{}),
		authRoute(http.MethodPost, "/api/admin/permission-groups", "Create Permission Group", "admin.permission-groups.post", handler.createGroup, createGroupRequest{}, groupResponse{}),
		authRoute(http.MethodPut, "/api/admin/permission-groups/:id", "Update Permission Group", "admin.permission-groups.id.put", handler.updateGroup, updateGroupRequest{}, groupResponse{}),
		authRoute(http.MethodDelete, "/api/admin/permission-groups/:id", "Delete Permission Group", "admin.permission-groups.id.delete", handler.deleteGroup, nil, nil),
	}
}
