package application_test

import (
	"context"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	"admin/internal/organization"
)

func TestCustomOrganizationReferenceGuardCountsOnlyExistingOrdinaryCustomRoles(t *testing.T) {
	for _, test := range []struct {
		name    string
		code    string
		scope   domain.DataScope
		status  int
		deleted bool
		want    bool
	}{
		{name: "active memberless", code: "ordinary", scope: domain.DataScopeCustom, status: 1, want: true},
		{name: "inactive memberless", code: "ordinary", scope: domain.DataScopeCustom, status: 0, want: true},
		{name: "noncustom residual", code: "ordinary", scope: domain.DataScopeSelf, status: 1},
		{name: "protected residual", code: "admin", scope: domain.DataScopeCustom, status: 0},
		{name: "deleted residual", code: "ordinary", scope: domain.DataScopeCustom, status: 1, deleted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role := createAssignmentRole(t, fixture, test.code, test.status, test.scope)
			if err := fixture.db.Create(&authgorm.RoleDataScope{RoleID: role.ID, OrganizationID: 99}).Error; err != nil {
				t.Fatal(err)
			}
			if test.deleted {
				if err := fixture.db.Delete(&authgorm.Role{}, role.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if actual, err := fixture.service.HasCustomOrganizationReferences(ctx, 99); err != nil || actual != test.want {
				t.Fatalf("custom reference = %v, %v; want %v", actual, err, test.want)
			}
		})
	}
}

func TestBothScopeExitEntrypointsRollbackReferencesAndVersionOnFailure(t *testing.T) {
	for _, ordinaryUpdate := range []bool{false, true} {
		name := "scope assignment"
		if ordinaryUpdate {
			name = "ordinary update"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
			unit := organization.Unit{Name: "Scope", Code: "scope", Status: 1}
			if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
				t.Fatal(err)
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{20}); err != nil {
				t.Fatal(err)
			}
			if err := fixture.service.AssignRoleDataScope(ctx, role.ID, "custom", []uint{unit.ID}); err != nil {
				t.Fatal(err)
			}
			versions := failingLifecycleVersions{AccessVersionStore: fixture.versions, failUser: 20}
			service := lifecycleService(fixture, fixture.transactions, versions, fixture.users)
			var err error
			if ordinaryUpdate {
				_, err = service.UpdateRole(ctx, role.ID, application.UpdateRoleRequest{DataScope: "all"})
			} else {
				err = service.AssignRoleDataScope(ctx, role.ID, "all", nil)
			}
			if err == nil {
				t.Fatal("scope exit committed despite failed version")
			}
			actual, err := fixture.service.RoleDataScope(ctx, role.ID)
			if err != nil || actual.DataScope != domain.DataScopeCustom || len(actual.OrganizationIDs) != 1 || actual.OrganizationIDs[0] != unit.ID {
				t.Fatalf("scope exit partially committed = %#v, %v", actual, err)
			}
			if referenced, err := fixture.service.HasCustomOrganizationReferences(ctx, unit.ID); err != nil || !referenced {
				t.Fatalf("rollback released reference = %v, %v", referenced, err)
			}
			if version, err := fixture.versions.Current(ctx, 20); err != nil || version != 2 {
				t.Fatalf("rollback version = %d, %v", version, err)
			}
		})
	}
}

func TestCustomOrganizationSetChangeInvalidatesAndRollsBackAtomically(t *testing.T) {
	for _, failVersion := range []bool{false, true} {
		name := "commit"
		if failVersion {
			name = "second version rollback"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeCustom)
			var ids []uint
			for _, code := range []string{"old", "new"} {
				unit := organization.Unit{Name: code, Code: code, Status: 1}
				if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, unit.ID)
			}
			if err := fixture.repository.ReplaceRoleDataScope(ctx, role.ID, domain.DataScopeCustom, ids[:1]); err != nil {
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
			err := lifecycleService(fixture, fixture.transactions, versions, fixture.users).AssignRoleDataScope(ctx, role.ID, "custom", []uint{ids[1], ids[1]})
			if (err != nil) != failVersion {
				t.Fatalf("custom set change error = %v, failure requested %v", err, failVersion)
			}
			wantID, wantVersion := ids[1], 2
			if failVersion {
				wantID, wantVersion = ids[0], 1
			}
			actual, err := fixture.service.RoleDataScope(ctx, role.ID)
			if err != nil || actual.DataScope != domain.DataScopeCustom || len(actual.OrganizationIDs) != 1 || actual.OrganizationIDs[0] != wantID {
				t.Fatalf("custom set after replacement = %#v, %v; want organization %d", actual, err, wantID)
			}
			for _, id := range []uint{10, 20} {
				if version, err := fixture.versions.Current(ctx, id); err != nil || version != wantVersion {
					t.Fatalf("user %d version = %d, %v; want %d", id, version, err, wantVersion)
				}
			}
		})
	}
}
