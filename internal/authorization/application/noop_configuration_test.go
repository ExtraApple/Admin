package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	"admin/internal/organization"
)

// Equal configurations must not merely avoid version increments: they must not
// update the role or rebuild either relationship set.
type rejectingConfigurationWrites struct{ application.Repository }

func (repository rejectingConfigurationWrites) UpdateRole(context.Context, domain.Role) error {
	return errors.New("unexpected equal role write")
}

func (repository rejectingConfigurationWrites) ReplaceRoleDataScope(context.Context, uint, domain.DataScope, []uint) error {
	return errors.New("unexpected equal scope replacement")
}

func (repository rejectingConfigurationWrites) ReplaceRolePermissions(context.Context, uint, []uint) error {
	return errors.New("unexpected equal permission replacement")
}

func equalConfigurationService(fixture authorizationFixture) *application.Service {
	return application.NewService(rejectingConfigurationWrites{fixture.repository}, fixture.transactions, organizationScopeReaderForTest(fixture), fixture.users, fixture.versions, nil, fixture.users, fixture.roleMenus)
}

func TestEqualRoleConfigurationsDoNotWriteOrInitializeVersions(t *testing.T) {
	for _, configuration := range []string{"fields", "noncustom scope", "custom scope", "permissions", "empty permissions"} {
		t.Run(configuration, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role, err := fixture.service.CreateRole(ctx, application.CreateRoleRequest{Name: " Raw name ", Code: " raw-code ", Sort: -3, Status: 0, DataScope: "self"})
			if err != nil {
				t.Fatal(err)
			}
			// Persist disabled status explicitly; role creation's existing default
			// behavior is outside this configuration-update contract.
			if err := fixture.repository.UpdateRole(ctx, role); err != nil {
				t.Fatal(err)
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{10, 20}); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.versions.EnsureAndIncrement(ctx, 10); err != nil {
				t.Fatal(err)
			}
			service := equalConfigurationService(fixture)
			switch configuration {
			case "fields":
				actual, err := service.UpdateRole(ctx, role.ID, application.UpdateRoleRequest{Name: &role.Name, Code: &role.Code, Description: &role.Description, Sort: &role.Sort, Status: &role.Status, DataScope: "self"})
				if err != nil || actual != role {
					t.Fatalf("equal raw role = %#v, %v; want %#v", actual, err, role)
				}
			case "noncustom scope":
				if err := service.AssignRoleDataScope(ctx, role.ID, "self", nil); err != nil {
					t.Fatal(err)
				}
			case "custom scope":
				var ids []uint
				for _, code := range []string{"first", "second"} {
					unit := organization.Unit{Name: code, Code: code, Status: 1}
					if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
						t.Fatal(err)
					}
					ids = append(ids, unit.ID)
				}
				if err := fixture.repository.ReplaceRoleDataScope(ctx, role.ID, domain.DataScopeCustom, ids); err != nil {
					t.Fatal(err)
				}
				if err := service.AssignRoleDataScope(ctx, role.ID, "custom", []uint{ids[1], ids[0], ids[1]}); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				var ids []uint
				for _, code := range []string{"first.read", "second.read"} {
					permission, err := fixture.service.CreatePermission(ctx, application.CreatePermissionRequest{Name: code, Code: code})
					if err != nil {
						t.Fatal(err)
					}
					ids = append(ids, permission.ID)
				}
				if err := fixture.repository.ReplaceRolePermissions(ctx, role.ID, ids); err != nil {
					t.Fatal(err)
				}
				if err := service.AssignPermissionsToRole(ctx, role.ID, []uint{ids[1], ids[0], ids[1]}); err != nil {
					t.Fatal(err)
				}
			case "empty permissions":
				if err := service.AssignPermissionsToRole(ctx, role.ID, nil); err != nil {
					t.Fatal(err)
				}
			}
			if version, err := fixture.versions.Current(ctx, 10); err != nil || version != 2 {
				t.Fatalf("existing version = %d, %v; want 2", version, err)
			}
			if _, err := fixture.versions.Current(ctx, 20); !errors.Is(err, authgorm.ErrAccessVersionNotFound) {
				t.Fatalf("equal configuration initialized absent version: %v", err)
			}
		})
	}
}

func TestSameNoncustomScopeStillCleansResidualLinksAtomically(t *testing.T) {
	for _, entrypoint := range []string{"ordinary update", "scope assignment"} {
		for _, failVersion := range []bool{false, true} {
			name := entrypoint + "/commit"
			if failVersion {
				name = entrypoint + "/second version rollback"
			}
			t.Run(name, func(t *testing.T) {
				fixture := newAuthorizationFixture(t)
				ctx := context.Background()
				role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
				if err := fixture.db.Create(&authgorm.RoleDataScope{RoleID: role.ID, OrganizationID: 99}).Error; err != nil {
					t.Fatal(err)
				}
				if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{10, 20}); err != nil {
					t.Fatal(err)
				}
				for _, id := range []uint{10, 20} {
					if _, err := fixture.versions.Ensure(ctx, id); err != nil {
						t.Fatal(err)
					}
				}
				var versions application.AccessVersionStore = fixture.versions
				if failVersion {
					versions = failingLifecycleVersions{AccessVersionStore: versions, failUser: 20}
				}
				service := lifecycleService(fixture, fixture.transactions, versions, fixture.users)
				var err error
				if entrypoint == "ordinary update" {
					_, err = service.UpdateRole(ctx, role.ID, application.UpdateRoleRequest{DataScope: "self"})
				} else {
					err = service.AssignRoleDataScope(ctx, role.ID, "self", nil)
				}
				if (err != nil) != failVersion {
					t.Fatalf("residual cleanup error = %v, failure requested %v", err, failVersion)
				}
				ids, err := fixture.repository.CustomOrganizationIDs(ctx, role.ID)
				wantIDs := []uint{}
				wantVersion := 2
				if failVersion {
					wantIDs, wantVersion = []uint{99}, 1
				}
				if err != nil || !reflect.DeepEqual(ids, wantIDs) {
					t.Fatalf("cleanup relationships = %v, %v; want %v", ids, err, wantIDs)
				}
				if actual, err := fixture.repository.FindRole(ctx, role.ID); err != nil || actual != role {
					t.Fatalf("cleanup changed same-mode role = %#v, %v", actual, err)
				}
				for _, id := range []uint{10, 20} {
					if version, err := fixture.versions.Current(ctx, id); err != nil || version != wantVersion {
						t.Fatalf("user %d version = %d, %v; want %d", id, version, err, wantVersion)
					}
				}
			})
		}
	}
}

func TestRealRoleFieldChangesInvalidateAndRollbackOnSecondVersionFailure(t *testing.T) {
	for _, field := range []string{"name", "code", "description", "sort", "status", "scope"} {
		for _, failVersion := range []bool{false, true} {
			if failVersion && field != "description" {
				continue
			}
			name := field + "/commit"
			if failVersion {
				name = field + "/second version rollback"
			}
			t.Run(name, func(t *testing.T) {
				fixture := newAuthorizationFixture(t)
				ctx := context.Background()
				role, err := fixture.service.CreateRole(ctx, application.CreateRoleRequest{Name: " Original ", Code: " original ", Description: "clear me", Sort: 1, Status: 1, DataScope: "self"})
				if err != nil {
					t.Fatal(err)
				}
				if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{10, 20}); err != nil {
					t.Fatal(err)
				}
				for _, id := range []uint{10, 20} {
					if _, err := fixture.versions.Ensure(ctx, id); err != nil {
						t.Fatal(err)
					}
				}
				target := role
				request := application.UpdateRoleRequest{}
				switch field {
				case "name":
					target.Name = "Original"
					request.Name = &target.Name
				case "code":
					target.Code = "original"
					request.Code = &target.Code
				case "description":
					target.Description = ""
					request.Description = &target.Description
				case "sort":
					target.Sort = 0
					request.Sort = &target.Sort
				case "status":
					target.Status = 0
					request.Status = &target.Status
				case "scope":
					target.DataScope = domain.DataScopeAll
					request.DataScope = "all"
				}
				var versions application.AccessVersionStore = fixture.versions
				if failVersion {
					versions = failingLifecycleVersions{AccessVersionStore: versions, failUser: 20}
				}
				actual, err := lifecycleService(fixture, fixture.transactions, versions, fixture.users).UpdateRole(ctx, role.ID, request)
				if (err != nil) != failVersion {
					t.Fatalf("real field change error = %v, failure requested %v", err, failVersion)
				}
				want, wantVersion := target, 2
				if failVersion {
					want, wantVersion = role, 1
				} else if actual != target {
					t.Fatalf("returned role = %#v; want %#v", actual, target)
				}
				if stored, err := fixture.repository.FindRole(ctx, role.ID); err != nil || stored != want {
					t.Fatalf("stored role = %#v, %v; want %#v", stored, err, want)
				}
				for _, id := range []uint{10, 20} {
					if version, err := fixture.versions.Current(ctx, id); err != nil || version != wantVersion {
						t.Fatalf("user %d version = %d, %v; want %d", id, version, err, wantVersion)
					}
				}
			})
		}
	}
}

func TestPermissionReplacementComparesRawConfiguredReferencesAndRollsBack(t *testing.T) {
	for _, failVersion := range []bool{false, true} {
		name := "commit"
		if failVersion {
			name = "second version rollback"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
			live, err := fixture.service.CreatePermission(ctx, application.CreatePermissionRequest{Name: "Live", Code: "live.read"})
			if err != nil {
				t.Fatal(err)
			}
			deleted, err := fixture.service.CreatePermission(ctx, application.CreatePermissionRequest{Name: "Deleted", Code: "deleted.read"})
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.repository.ReplaceRolePermissions(ctx, role.ID, []uint{live.ID, deleted.ID}); err != nil {
				t.Fatal(err)
			}
			// A filtered PermissionsByRole read would miss this actual configured link.
			if err := fixture.db.Delete(&authgorm.Permission{}, deleted.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{10, 20}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uint{10, 20} {
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			err = equalConfigurationService(fixture).AssignPermissionsToRole(ctx, role.ID, []uint{live.ID, deleted.ID})
			if code, ok := application.CodeOf(err); !ok || code != application.CodeValidationInvalid {
				t.Fatalf("same configured set containing deleted permission error = %v", err)
			}
			var versions application.AccessVersionStore = fixture.versions
			if failVersion {
				versions = failingLifecycleVersions{AccessVersionStore: versions, failUser: 20}
			}
			err = lifecycleService(fixture, fixture.transactions, versions, fixture.users).AssignPermissionsToRole(ctx, role.ID, []uint{live.ID, live.ID})
			if (err != nil) != failVersion {
				t.Fatalf("raw replacement error = %v, failure requested %v", err, failVersion)
			}
			wantIDs, wantVersion := []uint{live.ID}, 2
			if failVersion {
				wantIDs, wantVersion = []uint{live.ID, deleted.ID}, 1
			}
			var ids []uint
			if err := fixture.db.Model(&authgorm.RolePermission{}).Where("role_id = ?", role.ID).Order("permission_id asc").Pluck("permission_id", &ids).Error; err != nil || !reflect.DeepEqual(ids, wantIDs) {
				t.Fatalf("raw permission references = %v, %v; want %v", ids, err, wantIDs)
			}
			for _, id := range []uint{10, 20} {
				if version, err := fixture.versions.Current(ctx, id); err != nil || version != wantVersion {
					t.Fatalf("user %d version = %d, %v; want %d", id, version, err, wantVersion)
				}
			}
		})
	}
}

func TestRoleNoopCandidatesRetainEmptyProtectedAndScopeGuards(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	ordinary := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
	for _, test := range []struct {
		name    string
		role    domain.Role
		request application.UpdateRoleRequest
		want    application.ErrorCode
	}{
		{"empty update", ordinary, application.UpdateRoleRequest{}, application.CodeValidationInvalid},
		{"protected same name", admin, application.UpdateRoleRequest{Name: &admin.Name}, application.CodeConflict},
		{"invalid scope beside same name", ordinary, application.UpdateRoleRequest{Name: &ordinary.Name, DataScope: "invalid"}, application.CodeValidationInvalid},
		{"custom requires dedicated endpoint", ordinary, application.UpdateRoleRequest{Name: &ordinary.Name, DataScope: "custom"}, application.CodeValidationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := setFixtureRoleUsers(ctx, fixture.repository, test.role.ID, []uint{10}); err != nil {
				t.Fatal(err)
			}
			_, err := equalConfigurationService(fixture).UpdateRole(ctx, test.role.ID, test.request)
			if code, ok := application.CodeOf(err); !ok || code != test.want {
				t.Fatalf("same-like update error = %v; want %s", err, test.want)
			}
			if actual, err := fixture.repository.FindRole(ctx, test.role.ID); err != nil || actual != test.role {
				t.Fatalf("rejected update changed role = %#v, %v", actual, err)
			}
			if _, err := fixture.versions.Current(ctx, 10); !errors.Is(err, authgorm.ErrAccessVersionNotFound) {
				t.Fatalf("rejected update initialized version: %v", err)
			}
		})
	}
}
