package httpadapter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHTTPAdapterSeparatesRoutesFromCapabilityDeclarations(t *testing.T) {
	t.Parallel()

	packageDirectory := authorizationHTTPPackageDirectory(t)
	assertFileDeclarations(t, filepath.Join(packageDirectory, "routes.go"), map[string]struct{}{"Routes": {}})
	assertFileDeclarations(t, filepath.Join(packageDirectory, "shared.go"), setOf(
		"Handler", "authRoute", "queryPage", "pathID", "bindJSON", "badRequest", "success",
	))
	assertFileDeclarations(t, filepath.Join(packageDirectory, "roles.go"), setOf(
		"createRoleRequest", "updateRoleRequest", "assignUsersRequest", "assignDataScopeRequest",
		"roleResponse", "roleListResponse", "roleList", "roleInfo", "userListResponse", "userInfo", "dataScopeResponse", "dataScopeInfo",
		"listRoles", "createRole", "updateRole", "deleteRole", "assignRoleUsers", "listRoleUsers", "assignDataScope", "getDataScope", "roleInfoOf",
	))
	assertFileDeclarations(t, filepath.Join(packageDirectory, "permissions.go"), setOf(
		"createPermissionRequest", "updatePermissionRequest", "assignPermissionsRequest",
		"permissionResponse", "permissionListResponse", "permissionListEnvelope", "permissionList", "permissionInfo", "codesResponse", "syncResponse", "syncData",
		"listPermissions", "createPermission", "updatePermission", "deletePermission", "permissionCodes", "syncPermissions", "assignPermissions", "rolePermissions", "permissionInfoOf",
	))
	assertFileDeclarations(t, filepath.Join(packageDirectory, "permission_groups.go"), setOf(
		"createGroupRequest", "updateGroupRequest", "groupResponse", "groupListResponse", "groupList", "groupInfo",
		"listGroups", "createGroup", "updateGroup", "deleteGroup",
	))
}

func authorizationHTTPPackageDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("find structure test source path")
	}
	return filepath.Dir(file)
}

func assertFileDeclarations(t *testing.T, path string, want map[string]struct{}) {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filepath.Base(path), err)
	}
	got := declarationNames(parsed)
	if len(got) != len(want) {
		t.Fatalf("%s declarations = %v, want %v", filepath.Base(path), got, sortedSet(want))
	}
	for _, name := range got {
		if _, ok := want[name]; !ok {
			t.Fatalf("%s contains unexpected declaration %q; got %v, want %v", filepath.Base(path), name, got, sortedSet(want))
		}
	}
	for name := range want {
		if !contains(got, name) {
			t.Fatalf("%s does not contain required declaration %q; got %v", filepath.Base(path), name, got)
		}
	}
}

func declarationNames(file *ast.File) []string {
	names := make([]string, 0, len(file.Decls))
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			names = append(names, declaration.Name.Name)
		case *ast.GenDecl:
			for _, specification := range declaration.Specs {
				if typeSpec, ok := specification.(*ast.TypeSpec); ok {
					names = append(names, typeSpec.Name.Name)
				}
			}
		}
	}
	return names
}

func setOf(names ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	for index := 1; index < len(result); index++ {
		for previous := index; previous > 0 && result[previous] < result[previous-1]; previous-- {
			result[previous], result[previous-1] = result[previous-1], result[previous]
		}
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
