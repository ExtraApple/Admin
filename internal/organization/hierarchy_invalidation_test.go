package organization_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
)

// Every member in this projection has an applicable org_and_children role.
// Memberships and version updates remain real persisted module-owned facts.
type hierarchyRoleMemberProjection struct {
	members organization.ScopeReader
}

func (projection hierarchyRoleMemberProjection) HierarchyDependentUserIDs(ctx context.Context, roots []uint) ([]uint, error) {
	return projection.members.CurrentMemberUserIDsByOrganizations(ctx, roots)
}

type unavailableHierarchyDependencies struct{}

func (unavailableHierarchyDependencies) HierarchyDependentUserIDs(context.Context, []uint) ([]uint, error) {
	return nil, errors.New("hierarchy dependencies unavailable")
}

func (fixture topologyFixture) hierarchyService(dependencies organization.HierarchyDependencyReader) *organization.Service {
	return organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, noCustomOrganizationReferences{}, dependencies)
}

func TestHierarchyMoveInvalidatesOnlyChangedAncestorRootMembers(t *testing.T) {
	for _, test := range []struct {
		name     string
		moveRoot bool
		parent   string
		changed  map[uint]bool
	}{
		{"shared ancestor", false, "sibling", map[uint]bool{20: true, 30: true}},
		{"cross tree", false, "outside", map[uint]bool{10: true, 20: true, 40: true}},
		{"move to root zero", false, "zero", map[uint]bool{10: true, 20: true}},
		{"move from root zero", true, "child", map[uint]bool{10: true, 20: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			sibling := organization.Unit{ParentID: fixture.root.ID, Name: "Sibling", Code: "sibling", Status: 1}
			if err := fixture.repository.CreateUnit(ctx, &sibling); err != nil {
				t.Fatal(err)
			}
			memberships := []organization.Membership{
				{UserID: 10, OrganizationID: fixture.root.ID},
				{UserID: 20, OrganizationID: fixture.child.ID},
				{UserID: 30, OrganizationID: sibling.ID},
				{UserID: 40, OrganizationID: fixture.outside.ID},
				{UserID: 50, OrganizationID: fixture.leaf.ID},
				// A reader depending on both changed roots still advances once.
				{UserID: 20, OrganizationID: sibling.ID},
			}
			if err := fixture.repository.CreateMemberships(ctx, memberships); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uint{10, 20, 30, 40, 50} {
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			var beforeMembers []organization.Membership
			if err := fixture.db.Order("user_id asc, organization_id asc").Find(&beforeMembers).Error; err != nil {
				t.Fatal(err)
			}
			parent := map[string]uint{"sibling": sibling.ID, "outside": fixture.outside.ID, "child": fixture.child.ID, "zero": 0}[test.parent]
			target := fixture.leaf.ID
			if test.moveRoot {
				target = fixture.outside.ID
			}
			service := fixture.hierarchyService(hierarchyRoleMemberProjection{organization.NewScopeReader(fixture.repository, organization.NewHierarchy(fixture.repository))})
			if _, err := service.UpdateUnit(ctx, 7, target, organization.UpdateUnitRequest{ParentID: &parent}); err != nil {
				t.Fatal(err)
			}
			stored, err := fixture.repository.FindUnitByID(ctx, target)
			if err != nil || stored.ParentID != parent {
				t.Fatalf("stored parent = %d, %v; want %d", stored.ParentID, err, parent)
			}
			for _, id := range []uint{10, 20, 30, 40, 50} {
				want := 1
				if test.changed[id] {
					want = 2
				}
				got, err := fixture.versions.Current(ctx, id)
				if err != nil || got != want {
					t.Fatalf("user %d version = %d, %v; want %d", id, got, err, want)
				}
			}
			var afterMembers []organization.Membership
			if err := fixture.db.Order("user_id asc, organization_id asc").Find(&afterMembers).Error; err != nil || !reflect.DeepEqual(afterMembers, beforeMembers) {
				t.Fatalf("move altered memberships: %#v, %v", afterMembers, err)
			}
		})
	}
}

func TestHierarchyNonStructuralUpdatesAndMembershipWritesDoNotReadDependencies(t *testing.T) {
	fixture := newTopologyFixture(t)
	ctx := context.Background()
	if err := fixture.repository.CreateMemberships(ctx, []organization.Membership{
		{UserID: 20, OrganizationID: fixture.child.ID},
		{UserID: 30, OrganizationID: fixture.leaf.ID},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{20, 30, 40} {
		if _, err := fixture.versions.Ensure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	service := fixture.hierarchyService(unavailableHierarchyDependencies{})
	if _, err := service.UpdateUnit(ctx, 7, fixture.leaf.ID, organization.UpdateUnitRequest{ParentID: &fixture.child.ID, Name: "Same parent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateUnit(ctx, 7, fixture.leaf.ID, organization.UpdateUnitRequest{Name: "Display only"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{20, 30, 40} {
		got, err := fixture.versions.Current(ctx, id)
		if err != nil || got != 1 {
			t.Fatalf("nonstructural update changed user %d version: %d, %v", id, got, err)
		}
	}
	if err := service.SetUsers(ctx, 7, fixture.leaf.ID, []uint{30, 40}); err != nil {
		t.Fatal(err)
	}
	stored, err := fixture.repository.FindUnitByID(ctx, fixture.leaf.ID)
	if err != nil || stored.ParentID != fixture.child.ID || stored.Name != "Display only" {
		t.Fatalf("stored nonstructural updates = %#v, %v", stored, err)
	}
	members, err := fixture.repository.MemberUserIDs(ctx, fixture.leaf.ID)
	if err != nil || !reflect.DeepEqual(members, []uint{30, 40}) {
		t.Fatalf("stored members = %v, %v", members, err)
	}
	for _, id := range []uint{20, 30, 40} {
		want := 1
		if id == 40 {
			want = 2
		}
		got, err := fixture.versions.Current(ctx, id)
		if err != nil || got != want {
			t.Fatalf("membership-only user %d version = %d, %v; want %d", id, got, err, want)
		}
	}
}

func TestHierarchyMoveDependencyAndSecondVersionFailureRollBackAllFacts(t *testing.T) {
	for _, failure := range []string{"dependency read", "second version"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			if err := fixture.repository.CreateMemberships(ctx, []organization.Membership{
				{UserID: 20, OrganizationID: fixture.root.ID},
				{UserID: 30, OrganizationID: fixture.outside.ID},
				{UserID: 40, OrganizationID: fixture.leaf.ID},
			}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uint{20, 30, 40} {
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			before, err := fixture.repository.ListAllUnits(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var beforeMembers []organization.Membership
			if err := fixture.db.Order("user_id asc, organization_id asc").Find(&beforeMembers).Error; err != nil {
				t.Fatal(err)
			}
			var dependencies organization.HierarchyDependencyReader = unavailableHierarchyDependencies{}
			if failure == "second version" {
				dependencies = hierarchyRoleMemberProjection{organization.NewScopeReader(fixture.repository, organization.NewHierarchy(fixture.repository))}
				if err := fixture.db.Exec("CREATE TRIGGER reject_second_hierarchy_version BEFORE UPDATE ON user_access_versions WHEN OLD.user_id = 30 AND (SELECT version FROM user_access_versions WHERE user_id = 20) = 2 BEGIN SELECT RAISE(ABORT, 'second hierarchy version failed'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			service := fixture.hierarchyService(dependencies)
			_, err = service.UpdateUnit(ctx, 7, fixture.leaf.ID, organization.UpdateUnitRequest{ParentID: &fixture.outside.ID, Name: "Must roll back"})
			if code, _ := organization.CodeOf(err); code != organization.CodeInternalError {
				t.Fatalf("move error = %v; want internal error", err)
			}
			after, err := fixture.repository.ListAllUnits(ctx)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("tree changed on failed move: %#v, %v", after, err)
			}
			var afterMembers []organization.Membership
			if err := fixture.db.Order("user_id asc, organization_id asc").Find(&afterMembers).Error; err != nil || !reflect.DeepEqual(afterMembers, beforeMembers) {
				t.Fatalf("memberships changed on failed move: %#v, %v", afterMembers, err)
			}
			for _, id := range []uint{20, 30, 40} {
				got, err := fixture.versions.Current(ctx, id)
				if err != nil || got != 1 {
					t.Fatalf("user %d version after rollback = %d, %v; want 1", id, got, err)
				}
			}
		})
	}
}
