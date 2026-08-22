package httpadapter

import (
	"net/http"
	"reflect"
	"testing"

	"admin/internal/routecatalog"
)

func TestRoutesPreserveOrderedDescriptorContract(t *testing.T) {
	t.Parallel()

	expected := []routeContract{
		{method: http.MethodGet, path: "/api/admin/roles", name: "List Roles", permission: "admin.roles.get", responseSchema: reflect.TypeOf(roleListResponse{})},
		{method: http.MethodPost, path: "/api/admin/roles", name: "Create Role", permission: "admin.roles.post", requestSchema: reflect.TypeOf(createRoleRequest{}), responseSchema: reflect.TypeOf(roleResponse{})},
		{method: http.MethodPut, path: "/api/admin/roles/:id", name: "Update Role", permission: "admin.roles.id.put", requestSchema: reflect.TypeOf(updateRoleRequest{}), responseSchema: reflect.TypeOf(roleResponse{})},
		{method: http.MethodDelete, path: "/api/admin/roles/:id", name: "Delete Role", permission: "admin.roles.id.delete", responseSchema: nil},
		{method: http.MethodPost, path: "/api/admin/roles/:id/users", name: "Assign Role Users", permission: "admin.roles.id.users.post", requestSchema: reflect.TypeOf(assignUsersRequest{}), responseSchema: nil},
		{method: http.MethodGet, path: "/api/admin/roles/:id/users", name: "List Role Users", permission: "admin.roles.id.users.get", responseSchema: reflect.TypeOf(userListResponse{})},
		{method: http.MethodPost, path: "/api/admin/roles/:id/data-scope", name: "Assign Role Data Scope", permission: "admin.roles.id.data-scope.post", requestSchema: reflect.TypeOf(assignDataScopeRequest{}), responseSchema: nil},
		{method: http.MethodGet, path: "/api/admin/roles/:id/data-scope", name: "Get Role Data Scope", permission: "admin.roles.id.data-scope.get", responseSchema: reflect.TypeOf(dataScopeResponse{})},
		{method: http.MethodGet, path: "/api/admin/permissions", name: "List Permissions", permission: "admin.permissions.get", responseSchema: reflect.TypeOf(permissionListResponse{})},
		{method: http.MethodPost, path: "/api/admin/permissions", name: "Create Permission", permission: "admin.permissions.post", requestSchema: reflect.TypeOf(createPermissionRequest{}), responseSchema: reflect.TypeOf(permissionResponse{})},
		{method: http.MethodPut, path: "/api/admin/permissions/:id", name: "Update Permission", permission: "admin.permissions.id.put", requestSchema: reflect.TypeOf(updatePermissionRequest{}), responseSchema: reflect.TypeOf(permissionResponse{})},
		{method: http.MethodDelete, path: "/api/admin/permissions/:id", name: "Delete Permission", permission: "admin.permissions.id.delete", responseSchema: nil},
		{method: http.MethodGet, path: "/api/admin/permission-codes", name: "List Permission Codes", permission: "admin.permission-codes.get", responseSchema: reflect.TypeOf(codesResponse{})},
		{method: http.MethodPost, path: "/api/admin/permissions/sync", name: "Sync Route Permissions", permission: "admin.permissions.sync.post", responseSchema: reflect.TypeOf(syncResponse{})},
		{method: http.MethodPost, path: "/api/admin/roles/:id/permissions", name: "Assign Role Permissions", permission: "admin.roles.id.permissions.post", requestSchema: reflect.TypeOf(assignPermissionsRequest{}), responseSchema: nil},
		{method: http.MethodGet, path: "/api/admin/roles/:id/permissions", name: "List Role Permissions", permission: "admin.roles.id.permissions.get", responseSchema: reflect.TypeOf(permissionListEnvelope{})},
		{method: http.MethodGet, path: "/api/admin/permission-groups", name: "List Permission Groups", permission: "admin.permission-groups.get", responseSchema: reflect.TypeOf(groupListResponse{})},
		{method: http.MethodPost, path: "/api/admin/permission-groups", name: "Create Permission Group", permission: "admin.permission-groups.post", requestSchema: reflect.TypeOf(createGroupRequest{}), responseSchema: reflect.TypeOf(groupResponse{})},
		{method: http.MethodPut, path: "/api/admin/permission-groups/:id", name: "Update Permission Group", permission: "admin.permission-groups.id.put", requestSchema: reflect.TypeOf(updateGroupRequest{}), responseSchema: reflect.TypeOf(groupResponse{})},
		{method: http.MethodDelete, path: "/api/admin/permission-groups/:id", name: "Delete Permission Group", permission: "admin.permission-groups.id.delete", responseSchema: nil},
	}

	routes := Routes(nil, nil)
	if len(routes) != len(expected) {
		t.Fatalf("route count = %d, want %d", len(routes), len(expected))
	}

	for index, want := range expected {
		descriptor := routes[index]
		if descriptor.Handler == nil {
			t.Fatalf("route %d (%s %s) has nil Handler", index, want.method, want.path)
		}
		if descriptor.Method != want.method || descriptor.Path != want.path || descriptor.Access != routecatalog.PermissionControlled || descriptor.Name != want.name || descriptor.Group != "authorization" || descriptor.DefaultPermissionCode != want.permission || descriptor.DefaultAuditCategory != "authorization" {
			t.Fatalf("route %d descriptor = %#v, want method=%q path=%q access=%v name=%q group=%q permission=%q audit=%q", index, descriptor, want.method, want.path, routecatalog.PermissionControlled, want.name, "authorization", want.permission, "authorization")
		}
		if descriptor.OpenAPI.Summary != want.name {
			t.Fatalf("route %d OpenAPI summary = %q, want %q", index, descriptor.OpenAPI.Summary, want.name)
		}
		assertRequestContract(t, index, descriptor.OpenAPI.Request, want.requestSchema)
		assertResponseContract(t, index, descriptor.OpenAPI.Responses, want.responseSchema)
	}
}

type routeContract struct {
	method         string
	path           string
	name           string
	permission     string
	requestSchema  reflect.Type
	responseSchema reflect.Type
}

func assertRequestContract(t *testing.T, index int, request routecatalog.RequestBody, wantSchema reflect.Type) {
	t.Helper()

	if wantSchema == nil {
		if request.Kind != routecatalog.NoBody || request.Schema != nil || request.Required {
			t.Fatalf("route %d request = %#v, want no body", index, request)
		}
		return
	}
	if request.Kind != routecatalog.JSONBody || request.Schema != wantSchema || !request.Required {
		t.Fatalf("route %d request = %#v, want required JSON schema %v", index, request, wantSchema)
	}
}

func assertResponseContract(t *testing.T, index int, responses map[int]routecatalog.Response, wantSuccessSchema reflect.Type) {
	t.Helper()

	if len(responses) != 6 {
		t.Fatalf("route %d response count = %d, want 6: %#v", index, len(responses), responses)
	}
	if response, ok := responses[http.StatusOK]; !ok || response.Description != "success" || response.Kind != routecatalog.JSONBody || response.Schema == nil || (wantSuccessSchema == nil && response.DataSchema != nil) || (wantSuccessSchema != nil && response.DataSchema == nil) {
		t.Fatalf("route %d success response = %#v, want envelope data presence for %v", index, response, wantSuccessSchema)
	}
	if response, ok := responses[http.StatusBadRequest]; !ok || response.Description != "bad request" || response.Kind != routecatalog.JSONBody || len(response.Errors) == 0 || response.Schema == nil {
		t.Fatalf("route %d bad request response = %#v, want error envelope with definitions", index, response)
	}
}
