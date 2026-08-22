package httpadapter

import (
	"net/http"

	"admin/internal/authorization/application"
	"admin/internal/routecatalog"
)

// Routes returns the complete Authorization-owned RBAC HTTP surface.
func Routes(service *application.Service, routeSource application.RoutePermissionSource) []routecatalog.Descriptor {
	handler := &Handler{service: service, routes: routeSource}
	return []routecatalog.Descriptor{
		authRoute(http.MethodGet, "/api/admin/roles", "List Roles", "admin.roles.get", handler.listRoles, nil, roleListResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles", "Create Role", "admin.roles.post", handler.createRole, createRoleRequest{}, roleResponse{}),
		authRoute(http.MethodPut, "/api/admin/roles/:id", "Update Role", "admin.roles.id.put", handler.updateRole, updateRoleRequest{}, roleResponse{}),
		authRoute(http.MethodDelete, "/api/admin/roles/:id", "Delete Role", "admin.roles.id.delete", handler.deleteRole, nil, nil),
		authRoute(http.MethodPost, "/api/admin/roles/:id/users", "Assign Role Users", "admin.roles.id.users.post", handler.assignRoleUsers, assignUsersRequest{}, nil),
		authRoute(http.MethodGet, "/api/admin/roles/:id/users", "List Role Users", "admin.roles.id.users.get", handler.listRoleUsers, nil, userListResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles/:id/data-scope", "Assign Role Data Scope", "admin.roles.id.data-scope.post", handler.assignDataScope, assignDataScopeRequest{}, nil),
		authRoute(http.MethodGet, "/api/admin/roles/:id/data-scope", "Get Role Data Scope", "admin.roles.id.data-scope.get", handler.getDataScope, nil, dataScopeResponse{}),
		authRoute(http.MethodGet, "/api/admin/permissions", "List Permissions", "admin.permissions.get", handler.listPermissions, nil, permissionListResponse{}),
		authRoute(http.MethodPost, "/api/admin/permissions", "Create Permission", "admin.permissions.post", handler.createPermission, createPermissionRequest{}, permissionResponse{}),
		authRoute(http.MethodPut, "/api/admin/permissions/:id", "Update Permission", "admin.permissions.id.put", handler.updatePermission, updatePermissionRequest{}, permissionResponse{}),
		authRoute(http.MethodDelete, "/api/admin/permissions/:id", "Delete Permission", "admin.permissions.id.delete", handler.deletePermission, nil, nil),
		authRoute(http.MethodGet, "/api/admin/permission-codes", "List Permission Codes", "admin.permission-codes.get", handler.permissionCodes, nil, codesResponse{}),
		authRoute(http.MethodPost, "/api/admin/permissions/sync", "Sync Route Permissions", "admin.permissions.sync.post", handler.syncPermissions, nil, syncResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles/:id/permissions", "Assign Role Permissions", "admin.roles.id.permissions.post", handler.assignPermissions, assignPermissionsRequest{}, nil),
		authRoute(http.MethodGet, "/api/admin/roles/:id/permissions", "List Role Permissions", "admin.roles.id.permissions.get", handler.rolePermissions, nil, permissionListEnvelope{}),
		authRoute(http.MethodGet, "/api/admin/permission-groups", "List Permission Groups", "admin.permission-groups.get", handler.listGroups, nil, groupListResponse{}),
		authRoute(http.MethodPost, "/api/admin/permission-groups", "Create Permission Group", "admin.permission-groups.post", handler.createGroup, createGroupRequest{}, groupResponse{}),
		authRoute(http.MethodPut, "/api/admin/permission-groups/:id", "Update Permission Group", "admin.permission-groups.id.put", handler.updateGroup, updateGroupRequest{}, groupResponse{}),
		authRoute(http.MethodDelete, "/api/admin/permission-groups/:id", "Delete Permission Group", "admin.permission-groups.id.delete", handler.deleteGroup, nil, nil),
	}
}
