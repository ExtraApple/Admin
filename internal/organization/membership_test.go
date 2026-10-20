package organization_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

// These fixtures contain no role data. Lifecycle integration uses the real
// Authorization capability; membership policy cases isolate member invariants.
type noCustomOrganizationReferences struct{}

func (noCustomOrganizationReferences) LockOrganizationReferences(context.Context, uint, uint, []uint) (bool, error) {
	return false, nil
}

type noHierarchyDependencies struct{}

func (noHierarchyDependencies) HierarchyDependentUserIDs(context.Context, []uint) ([]uint, error) {
	return []uint{}, nil
}

func TestApplyMembershipDeltaPreservesExistingJoinedAtAndCreatesNewMembershipTime(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(organization.Models()...); err != nil {
		t.Fatalf("migrate organization models: %v", err)
	}
	joinedAt := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	if err := db.Create(&organization.Membership{UserID: 7, OrganizationID: 10, CreatedAt: joinedAt}).Error; err != nil {
		t.Fatalf("create existing membership: %v", err)
	}
	repository := organization.NewGORMRepository(db)
	if err := repository.ApplyMembershipDelta(context.Background(), 10, []uint{8}, nil); err != nil {
		t.Fatalf("ApplyMembershipDelta() = %v", err)
	}
	memberships, err := repository.MemberOrganizationMemberships(context.Background(), 7)
	if err != nil || len(memberships) != 1 || memberships[0].OrganizationID != 10 || !memberships[0].JoinedAt.Equal(joinedAt) {
		t.Fatalf("MemberOrganizationMemberships() = %#v, %v", memberships, err)
	}
	var added organization.Membership
	if err := db.First(&added, "user_id = ? AND organization_id = ?", 8, 10).Error; err != nil {
		t.Fatalf("load added membership: %v", err)
	}
	if added.CreatedAt.IsZero() || !added.CreatedAt.After(joinedAt) {
		t.Fatalf("added membership CreatedAt = %s, want after %s", added.CreatedAt, joinedAt)
	}
}

type organizationMemberPolicy map[uint]organization.ManagedUser

func (policy organizationMemberPolicy) ManagedUsers(context.Context, uint, []uint) (map[uint]organization.ManagedUser, error) {
	return policy, nil
}

func TestOrganizationSetUsersRetainsProtectedAndSelfRowsWhileMaintainingOrdinaryMembers(t *testing.T) {
	fixture := newTopologyFixture(t)
	ctx := context.Background()
	joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	for _, id := range []uint{7, 20, 30} {
		if err := fixture.db.Create(&organization.Membership{UserID: id, OrganizationID: fixture.leaf.ID, CreatedAt: joined}).Error; err != nil {
			t.Fatal(err)
		}
	}
	policy := organizationMemberPolicy{
		7:  {Exists: true, Visible: true},
		20: {Exists: true, Visible: true, Protected: true},
		30: {Exists: true, Visible: true},
		40: {Exists: true, Visible: true},
	}
	service := organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), policy, noCustomOrganizationReferences{}, noHierarchyDependencies{})
	if err := service.SetUsers(ctx, 7, fixture.leaf.ID, []uint{7, 20, 40, 40}); err != nil {
		t.Fatal(err)
	}
	ids, err := fixture.repository.MemberUserIDs(ctx, fixture.leaf.ID)
	if err != nil || !reflect.DeepEqual(ids, []uint{7, 20, 40}) {
		t.Fatalf("members = %v, %v", ids, err)
	}
	for _, id := range []uint{7, 20} {
		rows, err := fixture.repository.MemberOrganizationMemberships(ctx, id)
		if err != nil || len(rows) != 1 || !rows[0].JoinedAt.Equal(joined) {
			t.Fatalf("retained user %d = %#v, %v", id, rows, err)
		}
	}
}

func TestOrganizationMemberRejectionsLeaveStoredMembersUnitsAndVersionsUnchanged(t *testing.T) {
	normal := organization.ManagedUser{Exists: true, Visible: true}
	protected := organization.ManagedUser{Exists: true, Visible: true, Protected: true}
	for _, test := range []struct {
		name       string
		old        uint
		selected   []uint
		facts      organizationMemberPolicy
		deleteUnit bool
		code       organization.ErrorCode
	}{
		{"raw zero", 30, []uint{30, 0}, organizationMemberPolicy{30: normal}, false, organization.CodeValidationInvalid},
		{"missing target", 30, []uint{30, 99}, organizationMemberPolicy{30: normal}, false, organization.CodeValidationInvalid},
		{"deleted target", 30, []uint{30, 40}, organizationMemberPolicy{30: normal, 40: {Exists: true, Deleted: true, Visible: true}}, false, organization.CodeValidationInvalid},
		{"outside target", 30, []uint{30, 40}, organizationMemberPolicy{30: normal, 40: {Exists: true}}, false, organization.CodePermissionDenied},
		{"add self", 30, []uint{30, 7}, organizationMemberPolicy{30: normal, 7: normal}, false, organization.CodeValidationInvalid},
		{"add protected", 30, []uint{30, 20}, organizationMemberPolicy{30: normal, 20: protected}, false, organization.CodeConflict},
		{"clear self", 7, nil, organizationMemberPolicy{7: normal}, false, organization.CodeValidationInvalid},
		{"clear protected", 20, nil, organizationMemberPolicy{20: protected}, false, organization.CodeConflict},
		{"clear outside", 30, nil, organizationMemberPolicy{30: {Exists: true}}, false, organization.CodePermissionDenied},
		{"clear missing", 30, nil, organizationMemberPolicy{}, false, organization.CodeValidationInvalid},
		{"clear deleted self", 7, nil, organizationMemberPolicy{7: {Exists: true, Deleted: true, Visible: true}}, false, organization.CodeValidationInvalid},
		{"clear deleted protected", 20, nil, organizationMemberPolicy{20: {Exists: true, Deleted: true, Visible: true, Protected: true}}, false, organization.CodeConflict},
		{"clear deleted outside", 30, nil, organizationMemberPolicy{30: {Exists: true, Deleted: true}}, false, organization.CodePermissionDenied},
		{"retain deleted", 30, []uint{30, 40}, organizationMemberPolicy{30: {Exists: true, Deleted: true, Visible: true}, 40: normal}, false, organization.CodeValidationInvalid},
		{"retain missing", 30, []uint{30, 40}, organizationMemberPolicy{40: normal}, false, organization.CodeValidationInvalid},
		{"retain outside protected", 20, []uint{20, 40}, organizationMemberPolicy{20: {Exists: true, Protected: true}, 40: normal}, false, organization.CodePermissionDenied},
		{"delete self organization", 7, nil, organizationMemberPolicy{7: normal}, true, organization.CodeValidationInvalid},
		{"delete protected organization", 20, nil, organizationMemberPolicy{20: protected}, true, organization.CodeConflict},
		{"delete outside organization member", 30, nil, organizationMemberPolicy{30: {Exists: true}}, true, organization.CodePermissionDenied},
		{"delete missing organization member", 30, nil, organizationMemberPolicy{}, true, organization.CodeValidationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
			if err := fixture.db.Create(&organization.Membership{UserID: test.old, OrganizationID: fixture.leaf.ID, CreatedAt: joined}).Error; err != nil {
				t.Fatal(err)
			}
			versions := []authgorm.UserAccessVersion{}
			for _, id := range []uint{7, 20, 30, 40, 99} {
				if _, err := fixture.versions.Ensure(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			if err := fixture.db.Order("user_id").Find(&versions).Error; err != nil {
				t.Fatal(err)
			}
			service := organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), test.facts, noCustomOrganizationReferences{}, noHierarchyDependencies{})
			var err error
			if test.deleteUnit {
				err = service.DeleteUnit(ctx, 7, fixture.leaf.ID)
			} else {
				err = service.SetUsers(ctx, 7, fixture.leaf.ID, test.selected)
			}
			if code, _ := organization.CodeOf(err); code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			if _, err := fixture.repository.FindUnitByID(ctx, fixture.leaf.ID); err != nil {
				t.Fatalf("unit changed: %v", err)
			}
			rows, err := fixture.repository.MemberOrganizationMemberships(ctx, test.old)
			if err != nil || len(rows) != 1 || rows[0].OrganizationID != fixture.leaf.ID || !rows[0].JoinedAt.Equal(joined) {
				t.Fatalf("memberships changed = %#v, %v", rows, err)
			}
			ids, err := fixture.repository.MemberUserIDs(ctx, fixture.leaf.ID)
			if err != nil || !reflect.DeepEqual(ids, []uint{test.old}) {
				t.Fatalf("members changed = %v, %v", ids, err)
			}
			var after []authgorm.UserAccessVersion
			if err := fixture.db.Order("user_id").Find(&after).Error; err != nil || !reflect.DeepEqual(after, versions) {
				t.Fatalf("versions changed = %#v, %v", after, err)
			}
		})
	}
}

func TestOrganizationSetUsersRollsBackReplacementWhenInvalidationFails(t *testing.T) {
	fixture := newTopologyFixture(t)
	ctx := context.Background()
	joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	if err := fixture.db.Create(&organization.Membership{UserID: 30, OrganizationID: fixture.leaf.ID, CreatedAt: joined}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{30, 40} {
		if _, err := fixture.versions.Ensure(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.db.Exec("CREATE TRIGGER reject_membership_version BEFORE UPDATE ON user_access_versions WHEN OLD.user_id = 40 BEGIN SELECT RAISE(ABORT, 'membership invalidation failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	policy := organizationMemberPolicy{30: {Exists: true, Visible: true}, 40: {Exists: true, Visible: true}}
	service := organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), policy, noCustomOrganizationReferences{}, noHierarchyDependencies{})
	if err := service.SetUsers(ctx, 7, fixture.leaf.ID, []uint{40}); err == nil {
		t.Fatal("replacement succeeded despite invalidation failure")
	} else if code, _ := organization.CodeOf(err); code != organization.CodeInternalError {
		t.Fatalf("storage error = %v", err)
	}
	ids, err := fixture.repository.MemberUserIDs(ctx, fixture.leaf.ID)
	if err != nil || !reflect.DeepEqual(ids, []uint{30}) {
		t.Fatalf("members changed = %v, %v", ids, err)
	}
	rows, err := fixture.repository.MemberOrganizationMemberships(ctx, 30)
	if err != nil || len(rows) != 1 || !rows[0].JoinedAt.Equal(joined) {
		t.Fatalf("joined_at changed = %#v, %v", rows, err)
	}
	for _, id := range []uint{30, 40} {
		version, err := fixture.versions.Current(ctx, id)
		if err != nil || version != 1 {
			t.Fatalf("version user %d = %d, %v", id, version, err)
		}
	}
}

func TestOrganizationSetUsersInvalidatesOnlyChangedMembersAndPreservesEquality(t *testing.T) {
	fixture := newTopologyFixture(t)
	ctx := context.Background()
	joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	if err := fixture.db.Create(&[]organization.Membership{
		{UserID: 30, OrganizationID: fixture.leaf.ID, CreatedAt: joined},
		{UserID: 40, OrganizationID: fixture.leaf.ID, CreatedAt: joined},
		{UserID: 60, OrganizationID: fixture.child.ID, CreatedAt: joined},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&[]authgorm.UserAccessVersion{
		{UserID: 30, Version: 7},
		{UserID: 40, Version: 11},
		{UserID: 50, Version: 13},
		{UserID: 60, Version: 17},
	}).Error; err != nil {
		t.Fatal(err)
	}
	policy := organizationMemberPolicy{
		30: {Exists: true, Visible: true},
		40: {Exists: true, Visible: true},
		50: {Exists: true, Visible: true},
	}
	service := organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), policy, noCustomOrganizationReferences{}, noHierarchyDependencies{})
	if err := service.SetUsers(ctx, 7, fixture.leaf.ID, []uint{50, 40, 50}); err != nil {
		t.Fatal(err)
	}
	ids, err := fixture.repository.MemberUserIDs(ctx, fixture.leaf.ID)
	if err != nil || !reflect.DeepEqual(ids, []uint{40, 50}) {
		t.Fatalf("members = %v, %v", ids, err)
	}
	retained, err := fixture.repository.MemberOrganizationMemberships(ctx, 40)
	if err != nil || len(retained) != 1 || !retained[0].JoinedAt.Equal(joined) {
		t.Fatalf("retained membership = %#v, %v", retained, err)
	}
	added, err := fixture.repository.MemberOrganizationMemberships(ctx, 50)
	if err != nil || len(added) != 1 || !added[0].JoinedAt.After(joined) {
		t.Fatalf("added membership = %#v, %v", added, err)
	}
	for id, expected := range map[uint]int{30: 8, 40: 11, 50: 14, 60: 17} {
		current, err := fixture.versions.Current(ctx, id)
		if err != nil || current != expected {
			t.Fatalf("version for %d = %d, %v; want %d", id, current, err, expected)
		}
	}
	for _, selected := range [][]uint{{40, 50, 40}, {50, 40}} {
		if err := service.SetUsers(ctx, 7, fixture.leaf.ID, selected); err != nil {
			t.Fatal(err)
		}
	}
	for id, expected := range map[uint]int{30: 8, 40: 11, 50: 14, 60: 17} {
		current, err := fixture.versions.Current(ctx, id)
		if err != nil || current != expected {
			t.Fatalf("equal replacement version for %d = %d, %v; want %d", id, current, err, expected)
		}
	}
	for id, expected := range map[uint][]organization.MembershipFact{40: retained, 50: added} {
		current, err := fixture.repository.MemberOrganizationMemberships(ctx, id)
		if err != nil || !reflect.DeepEqual(current, expected) {
			t.Fatalf("equal replacement memberships for %d = %#v, %v", id, current, err)
		}
	}
	observers, err := fixture.repository.MemberOrganizationMemberships(ctx, 60)
	if err != nil || len(observers) != 1 || observers[0].OrganizationID != fixture.child.ID || !observers[0].JoinedAt.Equal(joined) {
		t.Fatalf("observer membership = %#v, %v", observers, err)
	}
}

func TestOrganizationSetUsersEqualDoesNotInitializeAnyUserVersion(t *testing.T) {
	for _, retained := range []bool{false, true} {
		name := "empty members"
		if retained {
			name = "retained self and protected members"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
			rows := []organization.Membership{{UserID: 60, OrganizationID: fixture.child.ID, CreatedAt: joined}}
			selected := []uint{}
			if retained {
				rows = append(rows,
					organization.Membership{UserID: 7, OrganizationID: fixture.leaf.ID, CreatedAt: joined},
					organization.Membership{UserID: 20, OrganizationID: fixture.leaf.ID, CreatedAt: joined},
				)
				selected = []uint{20, 7, 20}
			}
			if err := fixture.db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			var before []organization.Membership
			if err := fixture.db.Order("user_id, organization_id").Find(&before).Error; err != nil {
				t.Fatal(err)
			}
			policy := organizationMemberPolicy{
				7:  {Exists: true, Visible: true},
				20: {Exists: true, Visible: true, Protected: true},
			}
			service := organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), policy, noCustomOrganizationReferences{}, noHierarchyDependencies{})
			if err := service.SetUsers(ctx, 7, fixture.leaf.ID, selected); err != nil {
				t.Fatal(err)
			}
			var after []organization.Membership
			if err := fixture.db.Order("user_id, organization_id").Find(&after).Error; err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("equal members = %#v, %v; want %#v", after, err, before)
			}
			var count int64
			if err := fixture.db.Model(&authgorm.UserAccessVersion{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("equal bulk request initialized %d versions, %v", count, err)
			}
		})
	}
}
