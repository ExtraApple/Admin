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

func hierarchyDependencyService(fixture authorizationFixture, repository application.Repository, organizations application.OrganizationScopeReader) *application.Service {
	return application.NewService(repository, fixture.transactions, organizations, fixture.users, fixture.versions, nil, fixture.users, fixture.roleMenus)
}

func hierarchyDependencyUnit(t *testing.T, fixture authorizationFixture, code string, parentID uint) organization.Unit {
	t.Helper()
	unit := organization.Unit{Name: code, Code: code, ParentID: parentID, Status: 1}
	if err := fixture.organizations.CreateUnit(context.Background(), &unit); err != nil {
		t.Fatal(err)
	}
	return unit
}

// This owner-level regression applies the returned IDs to the real version store.
// Organization's topology/HTTP tests own the move and token consequences.
func TestHierarchyDependenciesPreserveIndividualRoleFactsAndPersistOnlyDependentVersions(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	common := hierarchyDependencyUnit(t, fixture, "common", 0)
	oldRoot := hierarchyDependencyUnit(t, fixture, "old", common.ID)
	newRoot := hierarchyDependencyUnit(t, fixture, "new", common.ID)
	moved := hierarchyDependencyUnit(t, fixture, "moved", oldRoot.ID)
	unrelated := hierarchyDependencyUnit(t, fixture, "unrelated", 0)
	child := createAssignmentRole(t, fixture, "children", 1, domain.DataScopeOrgAndChildren)
	duplicate := createAssignmentRole(t, fixture, "duplicate-children", 1, domain.DataScopeOrgAndChildren)
	disabled := createAssignmentRole(t, fixture, "disabled-children", 0, domain.DataScopeOrgAndChildren)
	deleted := createAssignmentRole(t, fixture, "deleted-children", 1, domain.DataScopeOrgAndChildren)
	missing := createAssignmentRole(t, fixture, "missing-children", 1, domain.DataScopeOrgAndChildren)
	fixed := createAssignmentRole(t, fixture, "fixed-org", 1, domain.DataScopeOrg)
	self := createAssignmentRole(t, fixture, "self-only", 1, domain.DataScopeSelf)
	custom := createAssignmentRole(t, fixture, "custom-org", 1, domain.DataScopeCustom)
	all := createAssignmentRole(t, fixture, "all-orgs", 1, domain.DataScopeAll)
	admin := createAssignmentRole(t, fixture, "admin", 1, domain.DataScopeOrgAndChildren)
	if err := fixture.repository.ReplaceRoleDataScope(ctx, custom.ID, domain.DataScopeCustom, []uint{oldRoot.ID}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		userID uint
		roots  []uint
		roles  []uint
		want   bool
	}{
		{20, []uint{oldRoot.ID}, []uint{child.ID}, true},
		{21, []uint{newRoot.ID}, []uint{child.ID}, true},
		{22, []uint{oldRoot.ID}, []uint{all.ID, child.ID}, true},
		{23, []uint{oldRoot.ID}, []uint{child.ID, duplicate.ID}, true},
		{24, []uint{oldRoot.ID, newRoot.ID}, []uint{child.ID, duplicate.ID}, true},
		{25, []uint{common.ID}, []uint{child.ID}, false},
		{26, []uint{unrelated.ID}, []uint{child.ID}, false},
		{27, []uint{oldRoot.ID}, []uint{disabled.ID}, false},
		{28, []uint{oldRoot.ID}, []uint{deleted.ID}, false},
		{29, []uint{oldRoot.ID}, []uint{fixed.ID}, false},
		{30, []uint{oldRoot.ID}, []uint{self.ID}, false},
		{31, []uint{oldRoot.ID}, []uint{custom.ID}, false},
		{32, []uint{oldRoot.ID}, []uint{all.ID}, false},
		{33, []uint{oldRoot.ID}, []uint{admin.ID}, false},
		{34, []uint{oldRoot.ID}, []uint{admin.ID, child.ID}, true},
		{35, []uint{moved.ID}, []uint{child.ID}, false},
		{36, []uint{oldRoot.ID}, []uint{child.ID}, true},
		{37, []uint{oldRoot.ID}, []uint{missing.ID}, false},
		{38, []uint{oldRoot.ID}, nil, false},
	}
	wantIDs := []uint{}
	for _, test := range tests {
		memberships := make([]organization.Membership, len(test.roots))
		for index, root := range test.roots {
			memberships[index] = organization.Membership{UserID: test.userID, OrganizationID: root}
		}
		if err := fixture.organizations.CreateMemberships(ctx, memberships); err != nil {
			t.Fatal(err)
		}
		if err := setFixtureUserRoles(ctx, fixture.repository, test.userID, test.roles); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.versions.Ensure(ctx, test.userID); err != nil {
			t.Fatal(err)
		}
		if test.want {
			wantIDs = append(wantIDs, test.userID)
		}
	}
	// Keep historical references deliberately: absent/deleted metadata cannot
	// become a phantom dependency, even when old relationship rows survive.
	if err := fixture.db.Delete(&authgorm.Role{}, deleted.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Unscoped().Delete(&authgorm.Role{}, missing.ID).Error; err != nil {
		t.Fatal(err)
	}
	beforeRows := membershipRows(t, fixture)
	service := hierarchyDependencyService(fixture, fixture.repository, organizationScopeReaderForTest(fixture))
	err := fixture.transactions.Run(ctx, func(tx context.Context) error {
		return organizationScopeReaderForTest(fixture).WithLockedHierarchy(tx, func(tx context.Context) error {
			ids, err := service.HierarchyDependentUserIDs(tx, []uint{newRoot.ID, oldRoot.ID, 0, oldRoot.ID})
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(ids, wantIDs) {
				t.Fatalf("dependent IDs = %v, want %v", ids, wantIDs)
			}
			for _, id := range ids {
				if _, err := fixture.versions.EnsureAndIncrement(tx, id); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		want := 1
		if test.want {
			want = 2
		}
		if got, err := fixture.versions.Current(ctx, test.userID); err != nil || got != want {
			t.Fatalf("user %d persisted version = %d, %v; want %d", test.userID, got, err, want)
		}
	}
	if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
		t.Fatalf("dependency read changed role relations: %v -> %v", beforeRows, rows)
	}
}

// These deterministic storage mutations represent a writer that won before the
// metadata barrier, not proof of SQLite/MySQL concurrent locking semantics.
type beforeHierarchyRoleLock struct {
	application.Repository
	before func(context.Context) error
}

func (repository beforeHierarchyRoleLock) LockRoles(ctx context.Context, ids []uint) ([]domain.Role, error) {
	if err := repository.before(ctx); err != nil {
		return nil, err
	}
	return repository.Repository.LockRoles(ctx, ids)
}

func TestHierarchyDependenciesUseWinningMetadataAndIgnoreDeletedRoleReferences(t *testing.T) {
	for _, mutation := range []string{"enable", "scope to children", "disable", "scope to fixed", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			root := hierarchyDependencyUnit(t, fixture, "root", 0)
			status, scope := 1, domain.DataScopeOrgAndChildren
			if mutation == "enable" {
				status = 0
			}
			if mutation == "scope to children" {
				scope = domain.DataScopeOrg
			}
			role := createAssignmentRole(t, fixture, "candidate", status, scope)
			if err := fixture.organizations.CreateMemberships(ctx, []organization.Membership{{UserID: 20, OrganizationID: root.ID}}); err != nil {
				t.Fatal(err)
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, role.ID, []uint{20}); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
				t.Fatal(err)
			}
			repository := beforeHierarchyRoleLock{Repository: fixture.repository, before: func(tx context.Context) error {
				switch mutation {
				case "enable":
					role.Status = 1
				case "scope to children":
					role.DataScope = domain.DataScopeOrgAndChildren
				case "disable":
					role.Status = 0
				case "scope to fixed":
					role.DataScope = domain.DataScopeOrg
				case "delete":
					return fixture.repository.DeleteRole(tx, role.ID)
				}
				return fixture.repository.UpdateRole(tx, role)
			}}
			service := hierarchyDependencyService(fixture, repository, organizationScopeReaderForTest(fixture))
			err := fixture.transactions.Run(ctx, func(tx context.Context) error {
				return organizationScopeReaderForTest(fixture).WithLockedHierarchy(tx, func(tx context.Context) error {
					ids, err := service.HierarchyDependentUserIDs(tx, []uint{root.ID})
					if err != nil {
						return err
					}
					want := []uint{}
					if mutation == "enable" || mutation == "scope to children" {
						want = []uint{20}
					}
					if !reflect.DeepEqual(ids, want) {
						t.Fatalf("winning %s dependencies = %v, want %v", mutation, ids, want)
					}
					for _, id := range ids {
						if _, err := fixture.versions.EnsureAndIncrement(tx, id); err != nil {
							return err
						}
					}
					return nil
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			wantVersion := 1
			if mutation == "enable" || mutation == "scope to children" {
				wantVersion = 2
			}
			if version, err := fixture.versions.Current(ctx, 20); err != nil || version != wantVersion {
				t.Fatalf("winning %s version = %d, %v; want %d", mutation, version, err, wantVersion)
			}
			if mutation == "delete" {
				if _, err := fixture.repository.FindRole(ctx, role.ID); !errors.Is(err, application.ErrNotFound) {
					t.Fatalf("winning deletion did not persist: %v", err)
				}
				if ids, err := fixture.repository.RoleIDsByUser(ctx, 20); err != nil || len(ids) != 0 {
					t.Fatalf("winning deletion left references: %v, %v", ids, err)
				}
			} else if stored, err := fixture.repository.FindRole(ctx, role.ID); err != nil || stored.Status != role.Status || stored.DataScope != role.DataScope {
				t.Fatalf("winning metadata did not persist: %#v, %v", stored, err)
			}
		})
	}
}

type failingHierarchyMembers struct {
	application.OrganizationScopeReader
	failure error
}

func (reader failingHierarchyMembers) CurrentMemberUserIDsByOrganizations(context.Context, []uint) ([]uint, error) {
	return nil, reader.failure
}

type failingHierarchyFacts struct {
	application.Repository
	stage   string
	failure error
	locked  bool
}

func (repository *failingHierarchyFacts) CurrentUserRoleReferences(ctx context.Context, ids []uint) ([]application.UserRoleReference, error) {
	if repository.stage == "references" || repository.stage == "recheck" && repository.locked {
		return nil, repository.failure
	}
	return repository.Repository.CurrentUserRoleReferences(ctx, ids)
}

func (repository *failingHierarchyFacts) LockRoles(ctx context.Context, ids []uint) ([]domain.Role, error) {
	if repository.stage == "metadata" {
		return nil, repository.failure
	}
	roles, err := repository.Repository.LockRoles(ctx, ids)
	repository.locked = true
	return roles, err
}

func TestHierarchyDependencyFactFailuresRollBackCallerRelationsAndVersions(t *testing.T) {
	for _, stage := range []string{"members", "references", "metadata", "recheck"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			root := hierarchyDependencyUnit(t, fixture, "root", 0)
			child := createAssignmentRole(t, fixture, "child", 1, domain.DataScopeOrgAndChildren)
			extra := createAssignmentRole(t, fixture, "extra", 1, domain.DataScopeOrg)
			if err := fixture.organizations.CreateMemberships(ctx, []organization.Membership{{UserID: 20, OrganizationID: root.ID}}); err != nil {
				t.Fatal(err)
			}
			if err := setFixtureRoleUsers(ctx, fixture.repository, child.ID, []uint{20}); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
				t.Fatal(err)
			}
			beforeRows := membershipRows(t, fixture)
			beforeVersions := membershipVersionRows(t, fixture)
			failure := errors.New("hierarchy dependency " + stage + " failure")
			repository := &failingHierarchyFacts{Repository: fixture.repository, stage: stage, failure: failure}
			organizations := organizationScopeReaderForTest(fixture)
			if stage == "members" {
				organizations = failingHierarchyMembers{OrganizationScopeReader: organizations, failure: failure}
			}
			service := hierarchyDependencyService(fixture, repository, organizations)
			err := fixture.transactions.Run(ctx, func(tx context.Context) error {
				if err := fixture.repository.ApplyUserRoleChanges(tx, 20, []uint{extra.ID}, nil); err != nil {
					return err
				}
				if _, err := fixture.versions.EnsureAndIncrement(tx, 20); err != nil {
					return err
				}
				return organizations.WithLockedHierarchy(tx, func(tx context.Context) error {
					_, err := service.HierarchyDependentUserIDs(tx, []uint{root.ID})
					return err
				})
			})
			if code, ok := application.CodeOf(err); !ok || code != application.CodeInternalError || !errors.Is(err, failure) {
				t.Fatalf("%s failure = %v, want wrapped internal error with original cause", stage, err)
			}
			if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
				t.Fatalf("%s failure committed caller relations: %v -> %v", stage, beforeRows, rows)
			}
			if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
				t.Fatalf("%s failure committed caller versions: %v -> %v", stage, beforeVersions, versions)
			}
		})
	}
}

type afterHierarchyRoleLock struct {
	application.Repository
	after func(context.Context) error
}

func (repository afterHierarchyRoleLock) LockRoles(ctx context.Context, ids []uint) ([]domain.Role, error) {
	roles, err := repository.Repository.LockRoles(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := repository.after(ctx); err != nil {
		return nil, err
	}
	return roles, nil
}

func TestHierarchyDependencyReferenceRecheckRejectsNewLowerRoleAndRollsBack(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	root := hierarchyDependencyUnit(t, fixture, "root", 0)
	lower := createAssignmentRole(t, fixture, "lower", 1, domain.DataScopeOrgAndChildren)
	candidate := createAssignmentRole(t, fixture, "candidate", 1, domain.DataScopeOrgAndChildren)
	if err := fixture.organizations.CreateMemberships(ctx, []organization.Membership{{UserID: 20, OrganizationID: root.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := setFixtureRoleUsers(ctx, fixture.repository, candidate.ID, []uint{20}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	beforeRows := membershipRows(t, fixture)
	beforeVersions := membershipVersionRows(t, fixture)
	repository := afterHierarchyRoleLock{Repository: fixture.repository, after: func(tx context.Context) error {
		if err := fixture.repository.ApplyUserRoleChanges(tx, 20, []uint{lower.ID}, nil); err != nil {
			return err
		}
		_, err := fixture.versions.EnsureAndIncrement(tx, 20)
		return err
	}}
	service := hierarchyDependencyService(fixture, repository, organizationScopeReaderForTest(fixture))
	err := fixture.transactions.Run(ctx, func(tx context.Context) error {
		return organizationScopeReaderForTest(fixture).WithLockedHierarchy(tx, func(tx context.Context) error {
			_, err := service.HierarchyDependentUserIDs(tx, []uint{root.ID})
			return err
		})
	})
	if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
		t.Fatalf("new lower-role dependency = %v, want conflict", err)
	}
	if rows := membershipRows(t, fixture); !reflect.DeepEqual(rows, beforeRows) {
		t.Fatalf("reference conflict committed relations: %v -> %v", beforeRows, rows)
	}
	if versions := membershipVersionRows(t, fixture); !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("reference conflict committed versions: %v -> %v", beforeVersions, versions)
	}
}

func TestHierarchyDependencyEmptyRootSetDoesNotInvalidateVersions(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	reader := failingHierarchyMembers{OrganizationScopeReader: organizationScopeReaderForTest(fixture), failure: errors.New("no changed root may query membership")}
	service := hierarchyDependencyService(fixture, fixture.repository, reader)
	for _, roots := range [][]uint{nil, {}, {0, 0}} {
		ids, err := service.HierarchyDependentUserIDs(ctx, roots)
		if err != nil || !reflect.DeepEqual(ids, []uint{}) {
			t.Fatalf("empty roots %v dependencies = %v, %v", roots, ids, err)
		}
	}
	if version, err := fixture.versions.Current(ctx, 20); err != nil || version != 1 {
		t.Fatalf("empty roots changed version: %d, %v", version, err)
	}
}
