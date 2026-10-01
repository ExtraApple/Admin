package gormadapter_test

import (
	"context"
	"errors"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	authapp "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

func TestRepositoryPersistsAuthorizationRelationsAndScopes(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(authgorm.Models()...); err != nil {
		t.Fatalf("migrate authorization models: %v", err)
	}
	repository := authgorm.NewRepository(db)
	ctx := context.Background()

	role := authdomain.Role{Name: "Operator", Code: "operator", Status: 1, DataScope: authdomain.DataScopeCustom}
	if err := repository.CreateRole(ctx, &role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	code, err := authdomain.NewPermissionCode("operator.read")
	if err != nil {
		t.Fatalf("create permission code: %v", err)
	}
	permission := authdomain.Permission{Name: "Read", Code: code}
	if err := repository.CreatePermission(ctx, &permission); err != nil {
		t.Fatalf("create permission: %v", err)
	}
	secondCode, _ := authdomain.NewPermissionCode("operator.write")
	secondPermission := authdomain.Permission{Name: "Write", Code: secondCode}
	if err := repository.CreatePermission(ctx, &secondPermission); err != nil {
		t.Fatalf("create second permission: %v", err)
	}
	page, total, err := repository.ListPermissions(ctx, 0, 1)
	if err != nil || len(page) != 1 || total != 2 {
		t.Fatalf("permission page = %#v, total = %d, err = %v; want one of two", page, total, err)
	}
	if err := repository.ReplaceRoleUsers(ctx, role.ID, []uint{7, 7, 8}); err != nil {
		t.Fatalf("replace role users: %v", err)
	}
	if err := repository.ReplaceRolePermissions(ctx, role.ID, []uint{permission.ID, permission.ID}); err != nil {
		t.Fatalf("replace role permissions: %v", err)
	}
	if err := repository.ReplaceRoleDataScope(ctx, role.ID, authdomain.DataScopeCustom, []uint{11, 11}); err != nil {
		t.Fatalf("replace role scope: %v", err)
	}

	userIDs, err := repository.UserIDsByRole(ctx, role.ID)
	if err != nil || len(userIDs) != 2 || userIDs[0] != 7 || userIDs[1] != 8 {
		t.Fatalf("role user IDs = %#v, %v", userIDs, err)
	}
	permissions, err := repository.PermissionsByRole(ctx, role.ID)
	if err != nil || len(permissions) != 1 || permissions[0].Code.String() != "operator.read" {
		t.Fatalf("role permissions = %#v, %v", permissions, err)
	}
	scope, err := repository.RoleDataScope(ctx, role.ID)
	if err != nil || scope.DataScope != authdomain.DataScopeCustom || len(scope.OrganizationIDs) != 1 || scope.OrganizationIDs[0] != 11 {
		t.Fatalf("role scope = %#v, %v", scope, err)
	}

	runner := platformdatabase.NewTransactionRunner(db)
	rollbackRole := authdomain.Role{Name: "Rollback", Code: "rollback", Status: 1, DataScope: authdomain.DataScopeAll}
	if err := runner.Run(ctx, func(tx context.Context) error {
		if err := repository.CreateRole(tx, &rollbackRole); err != nil {
			return err
		}
		return errors.New("rollback")
	}); err == nil {
		t.Fatal("transaction rollback should return callback error")
	}
	if _, err := repository.FindRole(ctx, rollbackRole.ID); !errors.Is(err, authapp.ErrNotFound) {
		t.Fatalf("rolled back role lookup error = %v, want ErrNotFound", err)
	}
}
func TestRepositoryListsAllAssignedRoleSummariesAndReplacesOnlyOneUser(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(authgorm.Models()...); err != nil {
		t.Fatalf("migrate authorization models: %v", err)
	}
	repository := authgorm.NewRepository(db)
	ctx := context.Background()
	first := authdomain.Role{Name: "First", Code: "first", Sort: 1, Status: 1, DataScope: authdomain.DataScopeSelf}
	second := authdomain.Role{Name: "Second", Code: "second", Sort: 2, Status: 1, DataScope: authdomain.DataScopeSelf}
	disabled := authdomain.Role{Name: "Disabled", Code: "disabled", Sort: 3, Status: 0, DataScope: authdomain.DataScopeSelf}
	replacement := authdomain.Role{Name: "Replacement", Code: "replacement", Sort: 4, Status: 1, DataScope: authdomain.DataScopeSelf}
	for _, role := range []*authdomain.Role{&first, &second, &disabled, &replacement} {
		if err := repository.CreateRole(ctx, role); err != nil {
			t.Fatalf("create role %q: %v", role.Code, err)
		}
	}
	disabled.Status = 0
	if err := repository.UpdateRole(ctx, disabled); err != nil {
		t.Fatalf("disable role: %v", err)
	}
	if err := repository.ReplaceRoleUsers(ctx, first.ID, []uint{7}); err != nil {
		t.Fatalf("assign first role: %v", err)
	}
	if err := repository.ReplaceRoleUsers(ctx, second.ID, []uint{7, 8}); err != nil {
		t.Fatalf("assign second role: %v", err)
	}
	if err := repository.ReplaceRoleUsers(ctx, disabled.ID, []uint{7}); err != nil {
		t.Fatalf("assign disabled role: %v", err)
	}

	summaries, err := repository.UserRoleSummaries(ctx, []uint{8, 7, 7})
	if err != nil {
		t.Fatalf("list role summaries: %v", err)
	}
	want := []authapp.UserRoleSummary{
		{UserID: 7, RoleID: first.ID, Code: first.Code, Name: first.Name, Status: first.Status},
		{UserID: 7, RoleID: second.ID, Code: second.Code, Name: second.Name, Status: second.Status},
		{UserID: 7, RoleID: disabled.ID, Code: disabled.Code, Name: disabled.Name, Status: disabled.Status},
		{UserID: 8, RoleID: second.ID, Code: second.Code, Name: second.Name, Status: second.Status},
	}
	if len(summaries) != len(want) {
		t.Fatalf("role summaries = %#v; want %d rows including disabled assignments", summaries, len(want))
	}
	for index := range want {
		if summaries[index] != want[index] {
			t.Fatalf("role summary[%d] = %#v; want %#v", index, summaries[index], want[index])
		}
	}

	if err := repository.ReplaceUserRoles(ctx, 7, []uint{replacement.ID, replacement.ID}); err != nil {
		t.Fatalf("replace user 7 roles: %v", err)
	}
	firstUserRoles, err := repository.RoleIDsByUser(ctx, 7)
	if err != nil || len(firstUserRoles) != 1 || firstUserRoles[0] != replacement.ID {
		t.Fatalf("user 7 role IDs = %#v, %v; want replacement only", firstUserRoles, err)
	}
	secondUserRoles, err := repository.RoleIDsByUser(ctx, 8)
	if err != nil || len(secondUserRoles) != 1 || secondUserRoles[0] != second.ID {
		t.Fatalf("user 8 role IDs = %#v, %v; want second role unchanged", secondUserRoles, err)
	}
}

type noUsers struct{}

func (noUsers) ListUsersByIDs(context.Context, []uint) ([]identitydomain.DirectoryUser, error) {
	return []identitydomain.DirectoryUser{}, nil
}
