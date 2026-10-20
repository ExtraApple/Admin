package application_test

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/organization"
)

type managementFacts map[uint]application.ManagedUserFact

func (facts managementFacts) ManagementUsersByIDs(_ context.Context, ids []uint) ([]application.ManagedUserFact, error) {
	result := make([]application.ManagedUserFact, 0, len(ids))
	for _, id := range ids {
		if fact, ok := facts[id]; ok {
			result = append(result, fact)
		}
	}
	return result, nil
}

func managementService(fixture authorizationFixture, facts managementFacts) *application.Service {
	return application.NewService(fixture.repository, fixture.transactions, organizationScopeReaderForTest(fixture), fixture.users, fixture.versions, nil, facts, fixture.roleMenus)
}

func createAssignmentRole(t *testing.T, fixture authorizationFixture, code string, status int, scope domain.DataScope) domain.Role {
	t.Helper()
	role := domain.Role{Name: code, Code: code, Status: status, DataScope: scope}
	if err := fixture.repository.CreateRole(context.Background(), &role); err != nil {
		t.Fatal(err)
	}
	// Establish the stored disabled state; creation retains its current default.
	if status == 0 {
		if err := fixture.repository.UpdateRole(context.Background(), role); err != nil {
			t.Fatal(err)
		}
	}
	return role
}

func TestRoleUserReplacementRejectsWholeProtectedOrInvalidWrites(t *testing.T) {
	for _, test := range []struct {
		name              string
		oldIDs, targetIDs []uint
		operatorID        uint
		scoped, adminRole bool
		want              application.ErrorCode
	}{
		{name: "raw zero", oldIDs: []uint{20}, targetIDs: []uint{21, 0}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "raw zero otherwise equal", oldIDs: []uint{20}, targetIDs: []uint{20, 0, 20}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "raw missing", oldIDs: []uint{20}, targetIDs: []uint{21, 99}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "raw deleted", oldIDs: []uint{20}, targetIDs: []uint{21, 50}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "raw self", oldIDs: []uint{20}, targetIDs: []uint{21, 10}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "raw disabled admin", oldIDs: []uint{20}, targetIDs: []uint{21, 40}, operatorID: 10, want: application.CodeConflict},
		{name: "retained self", oldIDs: []uint{10, 20}, targetIDs: []uint{10, 21}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "equal self", oldIDs: []uint{10, 20}, targetIDs: []uint{20, 10, 20}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "retained disabled admin", oldIDs: []uint{20, 40}, targetIDs: []uint{21, 40}, operatorID: 10, want: application.CodeConflict},
		{name: "equal disabled admin", oldIDs: []uint{20, 40}, targetIDs: []uint{40, 20, 40}, operatorID: 10, want: application.CodeConflict},
		{name: "clear removes self", oldIDs: []uint{10, 20}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "clear removes disabled admin", oldIDs: []uint{20, 40}, operatorID: 10, want: application.CodeConflict},
		{name: "clear removes missing", oldIDs: []uint{20, 99}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "retain deleted", oldIDs: []uint{20, 50}, targetIDs: []uint{50}, operatorID: 10, want: application.CodeInvalidUser},
		{name: "raw outside scope", targetIDs: []uint{20}, operatorID: 10, scoped: true, want: application.CodeInvalidUser},
		{name: "clear removes outside scope", oldIDs: []uint{20}, operatorID: 10, scoped: true, want: application.CodeInvalidUser},
		{name: "equal outside scope", oldIDs: []uint{20}, targetIDs: []uint{20, 20}, operatorID: 10, scoped: true, want: application.CodeInvalidUser},
		{name: "admin empty", operatorID: 10, adminRole: true, want: application.CodeConflict},
		{name: "admin unchanged", oldIDs: []uint{40}, targetIDs: []uint{40}, operatorID: 10, adminRole: true, want: application.CodeConflict},
		{name: "zero operator", oldIDs: []uint{20}, targetIDs: []uint{21}, want: application.CodeInvalidUser},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			managerScope := domain.DataScopeAll
			if test.scoped {
				managerScope = domain.DataScopeSelf
			}
			manager := createAssignmentRole(t, fixture, "manager", 1, managerScope)
			ordinary := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
			admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
			if err := setFixtureRoleUsers(ctx, fixture.repository, manager.ID, []uint{10}); err != nil {
				t.Fatal(err)
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, admin.ID, []uint{40}); err != nil {
				t.Fatal(err)
			}
			roleID := ordinary.ID
			if test.adminRole {
				roleID = admin.ID
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, roleID, test.oldIDs); err != nil {
				t.Fatal(err)
			}
			facts := managementFacts{10: {ID: 10}, 20: {ID: 20}, 21: {ID: 21}, 40: {ID: 40}, 50: {ID: 50, Deleted: true}}
			for _, id := range []uint{10, 20, 21, 40, 50, 99} {
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			before, err := fixture.repository.UserIDsByRole(ctx, roleID)
			if err != nil {
				t.Fatal(err)
			}
			err = managementService(fixture, facts).AssignUsersToRole(ctx, test.operatorID, roleID, test.targetIDs)
			if code, ok := application.CodeOf(err); !ok || code != test.want {
				t.Fatalf("error = %v, want code %s", err, test.want)
			}
			after, err := fixture.repository.UserIDsByRole(ctx, roleID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("relations changed: %v -> %v (%v)", before, after, err)
			}
			for _, id := range []uint{10, 20, 21, 40, 50, 99} {
				if version, err := fixture.versions.Current(ctx, id); err != nil || version != 1 {
					t.Fatalf("user %d version = %d, %v", id, version, err)
				}
			}
		})
	}
}

func TestDisabledOrdinaryRoleAndUserAllowFullReplacementAndClear(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	manager := createAssignmentRole(t, fixture, "manager", 1, domain.DataScopeAll)
	ordinary := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	if err := setFixtureRoleUsers(ctx, fixture.repository, manager.ID, []uint{10}); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureRoleUsers(ctx, fixture.repository, ordinary.ID, []uint{20}); err != nil {
		t.Fatal(err)
	}
	fixture.users[20] = identitydomain.DirectoryUser{ID: 20, Status: 0}
	fixture.users[21] = identitydomain.DirectoryUser{ID: 21, Status: 0}
	for _, id := range []uint{10, 20, 21} {
		if _, err := fixture.versions.Ensure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.service.AssignUsersToRole(ctx, 10, ordinary.ID, []uint{21, 21}); err != nil {
		t.Fatal(err)
	}
	ids, err := fixture.repository.UserIDsByRole(ctx, ordinary.ID)
	if err != nil || !reflect.DeepEqual(ids, []uint{21}) {
		t.Fatalf("replacement = %v, %v", ids, err)
	}
	for _, id := range []uint{20, 21} {
		if version, err := fixture.versions.Current(ctx, id); err != nil || version != 2 {
			t.Fatalf("version %d = %d, %v", id, version, err)
		}
	}
	if version, err := fixture.versions.Current(ctx, 10); err != nil || version != 1 {
		t.Fatalf("operator version = %d, %v", version, err)
	}
	snapshot, err := fixture.service.Snapshot(ctx, domain.Principal{UserID: 21})
	if err != nil || len(snapshot.Roles) != 0 || len(snapshot.Permissions) != 0 {
		t.Fatalf("runtime snapshot = %#v, %v", snapshot, err)
	}
	if err := fixture.service.AssignUsersToRole(ctx, 10, ordinary.ID, nil); err != nil {
		t.Fatal(err)
	}
	ids, err = fixture.repository.UserIDsByRole(ctx, ordinary.ID)
	if err != nil || len(ids) != 0 {
		t.Fatalf("clear = %v, %v", ids, err)
	}
	if version, err := fixture.versions.Current(ctx, 21); err != nil || version != 3 {
		t.Fatalf("cleared user version = %d, %v", version, err)
	}
}

func TestSetUserRolesUsesDeletedFactsAndDisabledAdminProtection(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	manager := createAssignmentRole(t, fixture, "manager", 1, domain.DataScopeAll)
	admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
	ordinary := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	if err := setFixtureRoleUsers(ctx, fixture.repository, manager.ID, []uint{10}); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureRoleUsers(ctx, fixture.repository, admin.ID, []uint{40}); err != nil {
		t.Fatal(err)
	}
	facts := managementFacts{20: {ID: 20}, 40: {ID: 40}, 50: {ID: 50, Deleted: true}}
	service := managementService(fixture, facts)
	for _, test := range []struct {
		id   uint
		want application.ErrorCode
	}{{40, application.CodeConflict}, {50, application.CodeInvalidUser}} {
		if _, err := fixture.versions.Ensure(ctx, test.id); err != nil {
			t.Fatal(err)
		}
		before, err := fixture.repository.RoleIDsByUser(ctx, test.id)
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.SetUserRoles(ctx, 10, test.id, application.UpdateUserRolesRequest{RoleIDs: []uint{ordinary.ID}, ExpectedAccessVersion: 1})
		if code, ok := application.CodeOf(err); !ok || code != test.want {
			t.Fatalf("target %d: %v", test.id, err)
		}
		after, err := fixture.repository.RoleIDsByUser(ctx, test.id)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("target %d roles changed: %v -> %v, %v", test.id, before, after, err)
		}
		if version, err := fixture.versions.Current(ctx, test.id); err != nil || version != 1 {
			t.Fatalf("target %d version = %d, %v", test.id, version, err)
		}
	}
	if version, err := service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{ordinary.ID}, ExpectedAccessVersion: 1}); err != nil || version != 2 {
		t.Fatalf("disabled ordinary role assignment = %d, %v", version, err)
	}
	for _, id := range []uint{20, 40} {
		snapshot, err := service.Snapshot(ctx, domain.Principal{UserID: id})
		if err != nil || len(snapshot.Roles) != 0 || len(snapshot.Permissions) != 0 {
			t.Fatalf("disabled runtime user %d = %#v, %v", id, snapshot, err)
		}
	}
}

func TestRolePermissionsValidateRawIDsAndProtectAdminWithoutBlockingSharedConfiguration(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
	ordinary := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	code, err := domain.NewPermissionCode("ordinary.read")
	if err != nil {
		t.Fatal(err)
	}
	permission := domain.Permission{Name: "Read", Code: code}
	if err := fixture.repository.CreatePermission(ctx, &permission); err != nil {
		t.Fatal(err)
	}
	for _, roleID := range []uint{admin.ID, ordinary.ID} {
		if err := setFixtureRoleUsers(ctx, fixture.repository, roleID, []uint{10, 20}); err != nil {
			t.Fatal(err)
		}
		if err := fixture.repository.ReplaceRolePermissions(ctx, roleID, []uint{permission.ID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uint{10, 20} {
		if _, err := fixture.versions.Ensure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name   string
		roleID uint
		ids    []uint
		want   application.ErrorCode
	}{
		{"raw zero", ordinary.ID, []uint{permission.ID, 0}, application.CodeValidationInvalid},
		{"missing permission", ordinary.ID, []uint{permission.ID, 999}, application.CodeValidationInvalid},
		{"admin unchanged", admin.ID, []uint{permission.ID}, application.CodeConflict},
		{"admin clear", admin.ID, nil, application.CodeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := fixture.repository.PermissionsByRole(ctx, test.roleID)
			if err != nil {
				t.Fatal(err)
			}
			err = fixture.service.AssignPermissionsToRole(ctx, test.roleID, test.ids)
			if code, ok := application.CodeOf(err); !ok || code != test.want {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
			after, err := fixture.repository.PermissionsByRole(ctx, test.roleID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("permissions changed: %v -> %v, %v", before, after, err)
			}
			for _, id := range []uint{10, 20} {
				if version, err := fixture.versions.Current(ctx, id); err != nil || version != 1 {
					t.Fatalf("version %d = %d, %v", id, version, err)
				}
			}
		})
	}
	// Equal disabled-role configuration is legal without invalidating either member.
	if err := fixture.service.AssignPermissionsToRole(ctx, ordinary.ID, []uint{permission.ID, permission.ID}); err != nil {
		t.Fatal(err)
	}
	permissions, err := fixture.repository.PermissionsByRole(ctx, ordinary.ID)
	if err != nil || len(permissions) != 1 || permissions[0].ID != permission.ID {
		t.Fatalf("deduplicated permissions = %v, %v", permissions, err)
	}
	for _, id := range []uint{10, 20} {
		if version, err := fixture.versions.Current(ctx, id); err != nil || version != 1 {
			t.Fatalf("version %d = %d, %v", id, version, err)
		}
	}
	snapshot, err := fixture.service.Snapshot(ctx, domain.Principal{UserID: 10})
	if err != nil || len(snapshot.Roles) != 0 || len(snapshot.Permissions) != 0 {
		t.Fatalf("disabled configuration authorized runtime: %#v, %v", snapshot, err)
	}
	// A real shared configuration change still affects protected members indirectly.
	if err := fixture.service.AssignPermissionsToRole(ctx, ordinary.ID, nil); err != nil {
		t.Fatal(err)
	}
	permissions, err = fixture.repository.PermissionsByRole(ctx, ordinary.ID)
	if err != nil || len(permissions) != 0 {
		t.Fatalf("clear permissions = %v, %v", permissions, err)
	}
	for _, id := range []uint{10, 20} {
		if version, err := fixture.versions.Current(ctx, id); err != nil || version != 2 {
			t.Fatalf("clear version %d = %d, %v", id, version, err)
		}
	}
}

func TestRoleDataScopeRejectsInapplicableRawInputWithoutWrites(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	ordinary := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
	unit := organization.Unit{Name: "Scope", Code: "scope", Status: 0}
	if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
		t.Fatal(err)
	}
	if err := fixture.organizations.UpdateUnit(ctx, unit.ID, map[string]any{"status": 0}); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureRoleUsers(ctx, fixture.repository, ordinary.ID, []uint{10}); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureRoleUsers(ctx, fixture.repository, admin.ID, []uint{20}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.ReplaceRoleDataScope(ctx, ordinary.ID, domain.DataScopeCustom, []uint{unit.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{10, 20} {
		if _, err := fixture.versions.Ensure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name, scope string
		roleID      uint
		ids         []uint
		want        application.ErrorCode
	}{
		{"custom empty", "custom", ordinary.ID, nil, application.CodeValidationInvalid},
		{"custom mixed zero", "custom", ordinary.ID, []uint{unit.ID, 0}, application.CodeValidationInvalid},
		{"custom missing", "custom", ordinary.ID, []uint{unit.ID, 999}, application.CodeValidationInvalid},
		{"all zero", "all", ordinary.ID, []uint{0}, application.CodeValidationInvalid},
		{"self organization", "self", ordinary.ID, []uint{unit.ID}, application.CodeValidationInvalid},
		{"org organization", "org", ordinary.ID, []uint{unit.ID}, application.CodeValidationInvalid},
		{"org children zero", "org_and_children", ordinary.ID, []uint{0}, application.CodeValidationInvalid},
		{"admin data scope", "self", admin.ID, nil, application.CodeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := fixture.service.RoleDataScope(ctx, test.roleID)
			if err != nil {
				t.Fatal(err)
			}
			err = fixture.service.AssignRoleDataScope(ctx, test.roleID, test.scope, test.ids)
			if code, ok := application.CodeOf(err); !ok || code != test.want {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
			after, err := fixture.service.RoleDataScope(ctx, test.roleID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("data scope changed: %v -> %v, %v", before, after, err)
			}
			for _, id := range []uint{10, 20} {
				if version, err := fixture.versions.Current(ctx, id); err != nil || version != 1 {
					t.Fatalf("version %d = %d, %v", id, version, err)
				}
			}
		})
	}
	if err := fixture.service.AssignRoleDataScope(ctx, ordinary.ID, "custom", []uint{unit.ID, unit.ID}); err != nil {
		t.Fatal(err)
	}
	if version, err := fixture.versions.Current(ctx, 10); err != nil || version != 1 {
		t.Fatalf("custom version = %d, %v", version, err)
	}
	if err := fixture.service.AssignRoleDataScope(ctx, ordinary.ID, "all", nil); err != nil {
		t.Fatal(err)
	}
	actual, err := fixture.service.RoleDataScope(ctx, ordinary.ID)
	if err != nil || actual.DataScope != domain.DataScopeAll || len(actual.OrganizationIDs) != 0 {
		t.Fatalf("all data scope = %#v, %v", actual, err)
	}
	if version, err := fixture.versions.Current(ctx, 10); err != nil || version != 2 {
		t.Fatalf("all version = %d, %v", version, err)
	}
}
