package application_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
)

func membershipFixture(t *testing.T) (authorizationFixture, domain.Role) {
	t.Helper()
	fixture := newAuthorizationFixture(t)
	manager := createAssignmentRole(t, fixture, "manager", 1, domain.DataScopeAll)
	role := createAssignmentRole(t, fixture, "ordinary", 0, domain.DataScopeSelf)
	if err := setFixtureRoleUsers(context.Background(), fixture.repository, manager.ID, []uint{10, 30}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{10, 20, 21, 22, 30} {
		fixture.users[id] = identitydomain.DirectoryUser{ID: id}
	}
	return fixture, role
}

func membershipRows(t *testing.T, fixture authorizationFixture) []authgorm.UserRole {
	t.Helper()
	var rows []authgorm.UserRole
	if err := fixture.db.Order("user_id asc, role_id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func membershipVersionRows(t *testing.T, fixture authorizationFixture) []authgorm.UserAccessVersion {
	t.Helper()
	var rows []authgorm.UserAccessVersion
	if err := fixture.db.Order("user_id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func rejectRetainedMembershipWrites(t *testing.T, fixture authorizationFixture, userID, roleID uint) {
	t.Helper()
	for _, operation := range []struct{ event, row string }{{"DELETE", "OLD"}, {"INSERT", "NEW"}} {
		statement := fmt.Sprintf("CREATE TRIGGER preserve_retained_%s BEFORE %s ON user_roles WHEN %s.user_id = %d AND %s.role_id = %d BEGIN SELECT RAISE(ABORT, 'retained relationship must survive'); END", operation.event, operation.event, operation.row, userID, operation.row, roleID)
		if err := fixture.db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func rejectMembershipVersionWrites(t *testing.T, fixture authorizationFixture) {
	t.Helper()
	for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
		statement := fmt.Sprintf("CREATE TRIGGER preserve_versions_%s BEFORE %s ON user_access_versions BEGIN SELECT RAISE(ABORT, 'no-op must preserve versions'); END", event, event)
		if err := fixture.db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoleMembershipDeltaChangesOnlyAddedAndRemovedUsers(t *testing.T) {
	for _, failSecondVersion := range []bool{false, true} {
		name := "commit"
		if failSecondVersion {
			name = "second changed user rolls back"
		}
		t.Run(name, func(t *testing.T) {
			fixture, role := membershipFixture(t)
			ctx := context.Background()
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{20, 21}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uint{10, 20, 21, 22, 30} {
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			beforeRows, beforeVersions := membershipRows(t, fixture), membershipVersionRows(t, fixture)
			rejectRetainedMembershipWrites(t, fixture, 21, role.ID)
			if failSecondVersion {
				if err := fixture.db.Exec("CREATE TRIGGER fail_second_changed_version BEFORE UPDATE ON user_access_versions WHEN OLD.user_id = 22 BEGIN SELECT RAISE(ABORT, 'second actual changed user'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			err := fixture.service.AssignUsersToRole(ctx, 10, role.ID, []uint{22, 21, 22})
			if failSecondVersion {
				if code, ok := application.CodeOf(err); !ok || code != application.CodeInternalError {
					t.Fatalf("second changed version error = %v", err)
				}
				if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
					t.Fatalf("failed delta changed relationships: %v -> %v", beforeRows, rows)
				}
				if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
					t.Fatalf("failed delta changed versions: %v -> %v", beforeVersions, versions)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ids, err := fixture.repository.UserIDsByRole(ctx, role.ID); err != nil || !reflect.DeepEqual(ids, []uint{21, 22}) {
				t.Fatalf("complete replacement members = %v, %v", ids, err)
			}
			wantVersions := append([]authgorm.UserAccessVersion(nil), beforeVersions...)
			for index := range wantVersions {
				if wantVersions[index].UserID == 20 || wantVersions[index].UserID == 22 {
					wantVersions[index].Version++
				}
			}
			if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, wantVersions) {
				t.Fatalf("symmetric-difference versions = %v; want %v", versions, wantVersions)
			}
			if readerIDs, err := fixture.repository.RoleIDsByUser(ctx, 30); err != nil || len(readerIDs) != 1 {
				t.Fatalf("unrelated all-scope reader relationships = %v, %v", readerIDs, err)
			}
		})
	}
}

func TestEquivalentBulkRoleMembershipsDoNotWriteOrInitializeVersions(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated=%t", populated), func(t *testing.T) {
			fixture, role := membershipFixture(t)
			ctx := context.Background()
			var current, target []uint
			if populated {
				current, target = []uint{20, 21}, []uint{21, 20, 21, 20}
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, current); err != nil {
				t.Fatal(err)
			}
			// One retained user has a version; another deliberately does not.
			if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
				t.Fatal(err)
			}
			beforeRows, beforeVersions := membershipRows(t, fixture), membershipVersionRows(t, fixture)
			if populated {
				rejectRetainedMembershipWrites(t, fixture, 20, role.ID)
			}
			rejectMembershipVersionWrites(t, fixture)
			for range 2 {
				if err := fixture.service.AssignUsersToRole(ctx, 10, role.ID, target); err != nil {
					t.Fatal(err)
				}
			}
			if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
				t.Fatalf("equal target changed rows: %v -> %v", beforeRows, rows)
			}
			if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
				t.Fatalf("equal target wrote or initialized versions: %v -> %v", beforeVersions, versions)
			}
		})
	}
}

func TestUserRoleDeltaPreservesRetainedLinksAndConflictsOnStaleEqualTarget(t *testing.T) {
	fixture, retained := membershipFixture(t)
	ctx := context.Background()
	removed := createAssignmentRole(t, fixture, "removed", 1, domain.DataScopeSelf)
	added := createAssignmentRole(t, fixture, "added", 1, domain.DataScopeSelf)
	if err := setFixtureUserRoles(ctx, fixture.repository, 20, []uint{retained.ID, removed.ID}); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureUserRoles(ctx, fixture.repository, 21, []uint{removed.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{20, 21, 30} {
		if _, err := fixture.versions.Ensure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	rejectRetainedMembershipWrites(t, fixture, 20, retained.ID)
	version, err := fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{added.ID, retained.ID, added.ID}, ExpectedAccessVersion: 1})
	if err != nil || version != 2 {
		t.Fatalf("real role delta = %d, %v", version, err)
	}
	wantIDs := []uint{retained.ID, added.ID}
	if ids, err := fixture.repository.RoleIDsByUser(ctx, 20); err != nil || !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("changed target roles = %v, %v; want %v", ids, err, wantIDs)
	}
	if ids, err := fixture.repository.RoleIDsByUser(ctx, 21); err != nil || !reflect.DeepEqual(ids, []uint{removed.ID}) {
		t.Fatalf("other user's roles = %v, %v", ids, err)
	}
	for _, id := range []uint{21, 30} {
		if current, err := fixture.versions.Current(ctx, id); err != nil || current != 1 {
			t.Fatalf("unrelated user %d version = %d, %v", id, current, err)
		}
	}
	beforeRows, beforeVersions := membershipRows(t, fixture), membershipVersionRows(t, fixture)
	rejectMembershipVersionWrites(t, fixture)
	for _, expected := range []int{1, 2, 2} {
		version, err = fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{added.ID, retained.ID, retained.ID}, ExpectedAccessVersion: expected})
		if expected == 1 {
			if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
				t.Fatalf("stale equal target = %d, %v", version, err)
			}
		} else if err != nil || version != 2 {
			t.Fatalf("current equal target = %d, %v", version, err)
		}
	}
	if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
		t.Fatalf("equal or stale target changed rows: %v -> %v", beforeRows, rows)
	}
	if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("equal or stale target changed versions: %v -> %v", beforeVersions, versions)
	}
}

func TestUserRoleNoopInitializesMissingVersionAtOneAndPreservesIt(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated=%t", populated), func(t *testing.T) {
			fixture, role := membershipFixture(t)
			ctx := context.Background()
			var ids []uint
			if populated {
				ids = []uint{role.ID, role.ID}
			}
			if err := setFixtureUserRoles(ctx, fixture.repository, 20, ids); err != nil {
				t.Fatal(err)
			}
			beforeRows := membershipRows(t, fixture)
			if populated {
				rejectRetainedMembershipWrites(t, fixture, 20, role.ID)
			}
			version, err := fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: ids, ExpectedAccessVersion: 1})
			if err != nil || version != 1 {
				t.Fatalf("initial equal target = %d, %v", version, err)
			}
			versions := membershipVersionRows(t, fixture)
			if len(versions) != 1 || versions[0].UserID != 20 || versions[0].Version != 1 {
				t.Fatalf("initialization must persist only target at one: %v", versions)
			}
			rejectMembershipVersionWrites(t, fixture)
			version, err = fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: ids, ExpectedAccessVersion: 1})
			if err != nil || version != 1 {
				t.Fatalf("second equal target = %d, %v", version, err)
			}
			if after := membershipVersionRows(t, fixture); !reflect.DeepEqual(after, versions) {
				t.Fatalf("equal target changed version row: %v -> %v", versions, after)
			}
			if after := membershipRows(t, fixture); !reflect.DeepEqual(after, beforeRows) {
				t.Fatalf("equal target rebuilt relationships: %v -> %v", beforeRows, after)
			}
		})
	}
}

func TestRejectedUserRoleNoopCandidatesRollbackMissingVersionInitialization(t *testing.T) {
	for _, reason := range []string{"stale", "zero role", "missing role", "deleted role", "missing user", "deleted user", "protected user", "protected role", "outside scope"} {
		t.Run(reason, func(t *testing.T) {
			fixture, role := membershipFixture(t)
			ctx := context.Background()
			if err := setFixtureUserRoles(ctx, fixture.repository, 20, []uint{role.ID}); err != nil {
				t.Fatal(err)
			}
			request := application.UpdateUserRolesRequest{RoleIDs: []uint{role.ID, role.ID}, ExpectedAccessVersion: 1}
			want := application.CodeValidationInvalid
			facts := managementFacts{10: {ID: 10}, 20: {ID: 20}, 30: {ID: 30}}
			switch reason {
			case "stale":
				request.ExpectedAccessVersion, want = 2, application.CodeConflict
			case "zero role":
				request.RoleIDs = append(request.RoleIDs, 0)
			case "missing role":
				request.RoleIDs = append(request.RoleIDs, 999999)
			case "deleted role":
				if err := fixture.db.Delete(&authgorm.Role{}, role.ID).Error; err != nil {
					t.Fatal(err)
				}
			case "missing user":
				delete(facts, 20)
				want = application.CodeInvalidUser
			case "deleted user":
				facts[20], want = application.ManagedUserFact{ID: 20, Deleted: true}, application.CodeInvalidUser
			case "protected user", "protected role":
				admin := createAssignmentRole(t, fixture, "admin", 0, domain.DataScopeAll)
				want = application.CodeConflict
				if reason == "protected user" {
					if err := setFixtureRoleUsers(ctx, fixture.repository, admin.ID, []uint{20}); err != nil {
						t.Fatal(err)
					}
				} else {
					request.RoleIDs = append(request.RoleIDs, admin.ID)
				}
			case "outside scope":
				managerIDs, err := fixture.repository.RoleIDsByUser(ctx, 10)
				if err != nil {
					t.Fatal(err)
				}
				manager, err := fixture.repository.FindRole(ctx, managerIDs[0])
				if err != nil {
					t.Fatal(err)
				}
				manager.DataScope = domain.DataScopeSelf
				if err := fixture.repository.UpdateRole(ctx, manager); err != nil {
					t.Fatal(err)
				}
				want = application.CodeInvalidUser
			}
			beforeRows, beforeVersions := membershipRows(t, fixture), membershipVersionRows(t, fixture)
			_, err := managementService(fixture, facts).SetUserRoles(ctx, 10, 20, request)
			if code, ok := application.CodeOf(err); !ok || code != want {
				t.Fatalf("rejected candidate error = %v; want %s", err, want)
			}
			if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
				t.Fatalf("rejection changed relationships: %v -> %v", beforeRows, rows)
			}
			if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
				t.Fatalf("rejection retained initialization: %v -> %v", beforeVersions, versions)
			}
		})
	}
}

func TestUserRoleNoopsLeaveVersionAvailableForRealChangeAndRollbackFailure(t *testing.T) {
	for _, failVersion := range []bool{false, true} {
		t.Run(fmt.Sprintf("failVersion=%t", failVersion), func(t *testing.T) {
			fixture, retained := membershipFixture(t)
			ctx := context.Background()
			removed := createAssignmentRole(t, fixture, "removed", 1, domain.DataScopeSelf)
			added := createAssignmentRole(t, fixture, "added", 1, domain.DataScopeSelf)
			if err := setFixtureUserRoles(ctx, fixture.repository, 20, []uint{retained.ID, removed.ID}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				version, err := fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{removed.ID, retained.ID, removed.ID}, ExpectedAccessVersion: 1})
				if err != nil || version != 1 {
					t.Fatalf("preceding equal target = %d, %v", version, err)
				}
			}
			rejectRetainedMembershipWrites(t, fixture, 20, retained.ID)
			beforeRows, beforeVersions := membershipRows(t, fixture), membershipVersionRows(t, fixture)
			if failVersion {
				if err := fixture.db.Exec("CREATE TRIGGER fail_target_version BEFORE UPDATE ON user_access_versions WHEN OLD.user_id = 20 BEGIN SELECT RAISE(ABORT, 'target increment failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			version, err := fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{retained.ID, added.ID}, ExpectedAccessVersion: 1})
			if failVersion {
				if code, ok := application.CodeOf(err); !ok || code != application.CodeInternalError || version != 0 {
					t.Fatalf("failed real change = %d, %v", version, err)
				}
				if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
					t.Fatalf("failed increment retained relation changes: %v -> %v", beforeRows, rows)
				}
				if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
					t.Fatalf("failed increment changed versions: %v -> %v", beforeVersions, versions)
				}
				return
			}
			if err != nil || version != 2 {
				t.Fatalf("real change after two no-ops = %d, %v", version, err)
			}
			if ids, err := fixture.repository.RoleIDsByUser(ctx, 20); err != nil || !reflect.DeepEqual(ids, []uint{retained.ID, added.ID}) {
				t.Fatalf("real change persisted roles = %v, %v", ids, err)
			}
			versions := membershipVersionRows(t, fixture)
			if len(versions) != 1 || versions[0].UserID != 20 || versions[0].Version != 2 {
				t.Fatalf("real change must advance target only once: %v", versions)
			}
		})
	}
}
