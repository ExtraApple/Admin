package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/navigation"
	"admin/internal/organization"
)

type failingLifecycleVersions struct {
	application.AccessVersionStore
	failUser uint
}

func (versions failingLifecycleVersions) EnsureAndIncrement(ctx context.Context, userID uint) (int, error) {
	version, err := versions.AccessVersionStore.EnsureAndIncrement(ctx, userID)
	if err == nil && userID == versions.failUser {
		return 0, errors.New("injected second user version failure")
	}
	return version, err
}

func lifecycleService(fixture authorizationFixture, transactions application.TransactionRunner, versions application.AccessVersionStore, facts application.UserManagementReader) *application.Service {
	return application.NewService(fixture.repository, transactions, organizationScopeReaderForTest(fixture), fixture.users, versions, nil, facts, fixture.roleMenus)
}

func TestRoleDeletionAtomicallyClearsAllMappingsAndPreservesSharedResources(t *testing.T) {
	for _, failVersion := range []bool{false, true} {
		name := "commit"
		if failVersion {
			name = "second version failure"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
			unit := organization.Unit{Name: "Scope", Code: "scope", Status: 1}
			if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
				t.Fatal(err)
			}
			if err := fixture.service.AssignRoleDataScope(ctx, role.ID, "custom", []uint{unit.ID}); err != nil {
				t.Fatal(err)
			}
			code, _ := domain.NewPermissionCode("shared.read")
			permission := domain.Permission{Name: "Shared", Code: code}
			if err := fixture.repository.CreatePermission(ctx, &permission); err != nil {
				t.Fatal(err)
			}
			menu := navigation.Menu{Name: "Shared", Type: 3, Status: 1}
			if err := fixture.roleMenus.CreateMenu(ctx, &menu); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uint{20, 21} {
				fixture.users[id] = identitydomain.DirectoryUser{ID: id}
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{21, 20}); err != nil {
				t.Fatal(err)
			}
			if err := fixture.repository.ReplaceRolePermissions(ctx, role.ID, []uint{permission.ID}); err != nil {
				t.Fatal(err)
			}
			if err := fixture.roleMenus.ReplaceRoleMenus(ctx, role.ID, []uint{menu.ID}); err != nil {
				t.Fatal(err)
			}
			var versions application.AccessVersionStore = fixture.versions
			if failVersion {
				versions = failingLifecycleVersions{AccessVersionStore: versions, failUser: 21}
			}
			err := lifecycleService(fixture, fixture.transactions, versions, fixture.users).DeleteRole(ctx, role.ID)
			if (err != nil) != failVersion {
				t.Fatalf("DeleteRole error = %v, failure requested %v", err, failVersion)
			}
			_, err = fixture.repository.FindRole(ctx, role.ID)
			if failVersion && err != nil || !failVersion && !errors.Is(err, application.ErrNotFound) {
				t.Fatalf("role after deletion = %v", err)
			}
			for _, model := range []any{&authgorm.UserRole{}, &authgorm.RolePermission{}, &authgorm.RoleDataScope{}, &navigation.RoleMenuModel{}} {
				var count int64
				if err := fixture.db.Model(model).Where("role_id = ?", role.ID).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				want := int64(0)
				if failVersion {
					want = 1
					if _, ok := model.(*authgorm.UserRole); ok {
						want = 2
					}
				}
				if count != want {
					t.Fatalf("%T relation count = %d, want %d", model, count, want)
				}
			}
			if _, err := fixture.roleMenus.FindMenu(ctx, menu.ID); err != nil {
				t.Fatalf("shared menu lost: %v", err)
			}
			if _, err := fixture.repository.FindPermission(ctx, permission.ID); err != nil {
				t.Fatalf("shared permission lost: %v", err)
			}
			for _, id := range []uint{20, 21} {
				want := 2
				if failVersion {
					want = 1
				}
				if version, err := fixture.versions.Current(ctx, id); err != nil || version != want {
					t.Fatalf("user %d version = %d, %v; want %d", id, version, err, want)
				}
			}
		})
	}
}

func TestBothRoleScopeEntrypointsReleaseInactiveMemberlessCustomReferences(t *testing.T) {
	for _, ordinaryUpdate := range []bool{false, true} {
		for _, scope := range []string{"all", "self", "org", "org_and_children"} {
			name := "scope/" + scope
			if ordinaryUpdate {
				name = "update/" + scope
			}
			t.Run(name, func(t *testing.T) {
				fixture := newAuthorizationFixture(t)
				ctx := context.Background()
				role := createAssignmentRole(t, fixture, "inactive", 0, domain.DataScopeSelf)
				unit := organization.Unit{Name: "Scope", Code: "scope", Status: 1}
				if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
					t.Fatal(err)
				}
				if err := fixture.service.AssignRoleDataScope(ctx, role.ID, "custom", []uint{unit.ID}); err != nil {
					t.Fatal(err)
				}
				if referenced, err := fixture.service.HasCustomOrganizationReferences(ctx, unit.ID); err != nil || !referenced {
					t.Fatalf("inactive memberless custom reference = %v, %v", referenced, err)
				}
				if ordinaryUpdate {
					if _, err := fixture.service.UpdateRole(ctx, role.ID, application.UpdateRoleRequest{DataScope: scope}); err != nil {
						t.Fatal(err)
					}
				} else if err := fixture.service.AssignRoleDataScope(ctx, role.ID, scope, nil); err != nil {
					t.Fatal(err)
				}
				actual, err := fixture.service.RoleDataScope(ctx, role.ID)
				if err != nil || string(actual.DataScope) != scope || len(actual.OrganizationIDs) != 0 {
					t.Fatalf("scope after exit = %#v, %v", actual, err)
				}
				if referenced, err := fixture.service.HasCustomOrganizationReferences(ctx, unit.ID); err != nil || referenced {
					t.Fatalf("released reference = %v, %v", referenced, err)
				}
				if err := fixture.service.AssignRoleDataScope(ctx, role.ID, "custom", nil); err == nil {
					t.Fatal("custom reentry reused removed configuration")
				}
			})
		}
	}
}

func TestDeletedOrdinaryOldRoleMemberCanOnlyBeActuallyRemoved(t *testing.T) {
	for _, blocked := range []string{"", "outside scope", "protected", "self", "missing identity"} {
		name := blocked
		if name == "" {
			name = "allowed removal"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			managerScope := domain.DataScopeAll
			if blocked == "outside scope" {
				managerScope = domain.DataScopeSelf
			}
			manager := createAssignmentRole(t, fixture, "manager", 1, managerScope)
			role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
			if err := setFixtureRoleUsers(ctx, fixture.repository, manager.ID, []uint{10}); err != nil {
				t.Fatal(err)
			}
			target := uint(20)
			if blocked == "self" {
				target = 10
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{target}); err != nil {
				t.Fatal(err)
			}
			if blocked == "protected" {
				admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
				if err := setFixtureRoleUsers(ctx, fixture.repository, admin.ID, []uint{target}); err != nil {
					t.Fatal(err)
				}
			}
			facts := managementFacts{target: {ID: target, Deleted: true}}
			if blocked == "missing identity" {
				delete(facts, target)
			}
			if _, err := fixture.versions.Ensure(ctx, target); err != nil {
				t.Fatal(err)
			}
			err := managementService(fixture, facts).AssignUsersToRole(ctx, 10, role.ID, nil)
			if (err != nil) != (blocked != "") {
				t.Fatalf("cleanup error = %v", err)
			}
			wantIDs, wantVersion := []uint{}, 2
			if blocked != "" {
				wantIDs, wantVersion = []uint{target}, 1
			}
			ids, err := fixture.repository.UserIDsByRole(ctx, role.ID)
			if err != nil || !reflect.DeepEqual(ids, wantIDs) {
				t.Fatalf("cleanup members = %v, %v; want %v", ids, err, wantIDs)
			}
			if version, err := fixture.versions.Current(ctx, target); err != nil || version != wantVersion {
				t.Fatalf("preserved version = %d, %v; want %d", version, err, wantVersion)
			}
			if blocked == "" {
				facts[target] = application.ManagedUserFact{ID: target}
				fixture.users[target] = identitydomain.DirectoryUser{ID: target}
				if roles, err := fixture.repository.RoleIDsByUser(ctx, target); err != nil || len(roles) != 0 {
					t.Fatalf("restored identity revived role = %v, %v", roles, err)
				}
			}
		})
	}
}

func TestRoleBindingThenDeletionIncludesNewMemberAndReverseOrderRejects(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	manager := createAssignmentRole(t, fixture, "manager", 1, domain.DataScopeAll)
	role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	if err := setFixtureRoleUsers(ctx, fixture.repository, manager.ID, []uint{10}); err != nil {
		t.Fatal(err)
	}
	fixture.users[20] = identitydomain.DirectoryUser{ID: 20}
	if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	transactions := revokingScopeTransaction{base: fixture.transactions, revoke: func(ctx context.Context) error {
		return fixture.service.AssignUsersToRole(ctx, 10, role.ID, []uint{20})
	}}
	if err := lifecycleService(fixture, transactions, fixture.versions, fixture.users).DeleteRole(ctx, role.ID); err != nil {
		t.Fatal(err)
	}
	if version, err := fixture.versions.Current(ctx, 20); err != nil || version != 3 {
		t.Fatalf("new member deletion version = %d, %v", version, err)
	}
	if err := fixture.service.AssignUsersToRole(ctx, 10, role.ID, []uint{20}); err == nil {
		t.Fatal("member assignment recreated a deleted role reference")
	}
	if _, err := fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{role.ID}, ExpectedAccessVersion: 3}); err == nil {
		t.Fatal("per-user assignment recreated a deleted role reference")
	}
	if ids, err := fixture.repository.RoleIDsByUser(ctx, 20); err != nil || len(ids) != 0 {
		t.Fatalf("failed binding left references = %v, %v", ids, err)
	}
	if version, err := fixture.versions.Current(ctx, 20); err != nil || version != 3 {
		t.Fatalf("rejected binding version = %d, %v", version, err)
	}
}

func TestDeletedPermissionCannotBeBoundAndDeletionInvalidatesCurrentDependents(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	code, _ := domain.NewPermissionCode("ordinary.read")
	permission := domain.Permission{Name: "Read", Code: code}
	if err := fixture.repository.CreatePermission(ctx, &permission); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{20}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	transactions := revokingScopeTransaction{base: fixture.transactions, revoke: func(ctx context.Context) error {
		return fixture.service.AssignPermissionsToRole(ctx, role.ID, []uint{permission.ID})
	}}
	if err := lifecycleService(fixture, transactions, fixture.versions, fixture.users).DeletePermission(ctx, permission.ID); err != nil {
		t.Fatal(err)
	}
	if version, err := fixture.versions.Current(ctx, 20); err != nil || version != 3 {
		t.Fatalf("permission dependent version = %d, %v", version, err)
	}
	if err := fixture.service.AssignPermissionsToRole(ctx, role.ID, []uint{permission.ID}); err == nil {
		t.Fatal("assignment recreated deleted permission reference")
	}
	if permissions, err := fixture.service.RolePermissions(ctx, role.ID); err != nil || len(permissions) != 0 {
		t.Fatalf("failed binding left permissions = %v, %v", permissions, err)
	}
	if version, err := fixture.versions.Current(ctx, 20); err != nil || version != 3 {
		t.Fatalf("rejected permission binding version = %d, %v", version, err)
	}
}
