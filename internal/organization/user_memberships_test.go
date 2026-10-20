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

type membershipVisibility struct {
	scope organization.OrganizationScope
}

func (fake membershipVisibility) Scope(context.Context, uint) (organization.OrganizationScope, error) {
	return fake.scope, nil
}
func (fake membershipVisibility) CurrentScope(context.Context, uint) (organization.OrganizationScope, error) {
	return fake.scope, nil
}

type membershipTargetPolicy struct{ target organization.ManagedUser }

func (fake membershipTargetPolicy) ManagedUsers(_ context.Context, _ uint, ids []uint) (map[uint]organization.ManagedUser, error) {
	facts := make(map[uint]organization.ManagedUser, len(ids))
	for _, id := range ids {
		facts[id] = fake.target
	}
	return facts, nil
}

func TestUserMembershipsPreserveHiddenAndUnchangedJoinedAtAndUseOneVersion(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	models := append(organization.Models(), &authgorm.UserAccessVersion{})
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	repository := organization.NewGORMRepository(db)
	ctx := context.Background()
	a := organization.Unit{Name: "A", Code: "a", Status: 1}
	b := organization.Unit{Name: "B", Code: "b", Status: 1}
	hidden := organization.Unit{Name: "hidden", Code: "hidden", Status: 1}
	for _, unit := range []*organization.Unit{&a, &b, &hidden} {
		if err := repository.CreateUnit(ctx, unit); err != nil {
			t.Fatal(err)
		}
	}
	joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	if err := db.Create(&[]organization.Membership{{UserID: 20, OrganizationID: a.ID, CreatedAt: joined}, {UserID: 20, OrganizationID: hidden.ID, CreatedAt: joined}, {UserID: 30, OrganizationID: b.ID, CreatedAt: joined}}).Error; err != nil {
		t.Fatal(err)
	}
	versions := authgorm.NewAccessVersions(db)
	service := organization.NewUserMembershipService(repository, membershipVisibility{organization.OrganizationScope{OrganizationIDs: []uint{a.ID, b.ID}}}, membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, versions, platformdatabase.NewTransactionRunner(db))
	facts, err := service.UserOrganizations(ctx, 7, []uint{20, 30})
	if err != nil || len(facts) != 3 {
		t.Fatalf("user organizations = %#v, %v", facts, err)
	}
	hiddenCount := 0
	for _, fact := range facts {
		if !fact.Manageable {
			hiddenCount++
			if fact.Name != "" || fact.OrganizationID != 0 || fact.UserID != 20 {
				t.Fatalf("hidden identity leaked = %#v", fact)
			}
		}
	}
	if hiddenCount != 1 {
		t.Fatalf("hidden memberships = %d", hiddenCount)
	}
	version, err := service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{a.ID, b.ID, b.ID}, ExpectedAccessVersion: 1})
	if err != nil || version != 2 {
		t.Fatalf("first Set = %d, %v", version, err)
	}
	facts20, err := repository.MemberOrganizationMemberships(ctx, 20)
	if err != nil || len(facts20) != 3 {
		t.Fatalf("memberships = %#v, %v", facts20, err)
	}
	var firstAdded time.Time
	for _, fact := range facts20 {
		if fact.OrganizationID == b.ID {
			firstAdded = fact.JoinedAt
		}
	}
	for _, fact := range facts20 {
		if fact.OrganizationID == a.ID || fact.OrganizationID == hidden.ID {
			if !fact.JoinedAt.Equal(joined) {
				t.Fatalf("retained joined_at = %v", fact)
			}
		} else if fact.JoinedAt.IsZero() || !fact.JoinedAt.After(joined) {
			t.Fatalf("new joined_at = %v", fact)
		}
	}
	if _, err := service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{}, ExpectedAccessVersion: 1}); err == nil {
		t.Fatal("stale write succeeded")
	} else if code, _ := organization.CodeOf(err); code != organization.CodeConflict {
		t.Fatalf("stale code = %q", code)
	}
	unchanged, _ := repository.MemberOrganizationMemberships(ctx, 20)
	if !reflect.DeepEqual(unchanged, facts20) {
		t.Fatalf("stale write changed memberships = %#v", unchanged)
	}
	version, err = service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{}, ExpectedAccessVersion: 2})
	if err != nil || version != 3 {
		t.Fatalf("clear = %d, %v", version, err)
	}
	retained, _ := repository.MemberOrganizationMemberships(ctx, 20)
	if len(retained) != 1 || retained[0].OrganizationID != hidden.ID || !retained[0].JoinedAt.Equal(joined) {
		t.Fatalf("hidden membership = %#v", retained)
	}
	other, _ := repository.MemberOrganizationMemberships(ctx, 30)
	if len(other) != 1 || other[0].OrganizationID != b.ID || !other[0].JoinedAt.Equal(joined) {
		t.Fatalf("other user memberships = %#v", other)
	}
	version, err = service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{b.ID}, ExpectedAccessVersion: 3})
	if err != nil || version != 4 {
		t.Fatalf("rejoin = %d, %v", version, err)
	}
	rejoined, err := repository.MemberOrganizationMemberships(ctx, 20)
	if err != nil || len(rejoined) != 2 {
		t.Fatalf("rejoined memberships = %#v, %v", rejoined, err)
	}
	for _, fact := range rejoined {
		if fact.OrganizationID == b.ID && !fact.JoinedAt.After(firstAdded) {
			t.Fatalf("rejoined time = %s, original = %s", fact.JoinedAt, firstAdded)
		}
	}
}

func TestOrganizationTreeDistinguishesManageableNodesFromPositioningAncestors(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(organization.Models()...); err != nil {
		t.Fatal(err)
	}
	repository := organization.NewGORMRepository(db)
	ctx := context.Background()
	root := organization.Unit{Name: "定位祖先", Code: "root", Status: 1}
	if err := repository.CreateUnit(ctx, &root); err != nil {
		t.Fatal(err)
	}
	visible := organization.Unit{Name: "可管理", Code: "visible", ParentID: root.ID, Status: 1}
	other := organization.Unit{Name: "无关", Code: "other", ParentID: root.ID, Status: 1}
	for _, unit := range []*organization.Unit{&visible, &other} {
		if err := repository.CreateUnit(ctx, unit); err != nil {
			t.Fatal(err)
		}
	}
	service := organization.NewService(repository, organization.NewHierarchy(repository), membershipVisibility{organization.OrganizationScope{OrganizationIDs: []uint{visible.ID}}}, nil, nil, platformdatabase.NewTransactionRunner(db), membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, noCustomOrganizationReferences{}, noHierarchyDependencies{})
	tree, err := service.Tree(ctx, 7)
	if err != nil || len(tree) != 1 || tree[0].ID != root.ID || tree[0].Manageable || len(tree[0].Children) != 1 || tree[0].Children[0].ID != visible.ID || !tree[0].Children[0].Manageable {
		t.Fatalf("scoped tree = %#v, %v", tree, err)
	}
	if _, err := service.Users(ctx, 7, root.ID); err == nil {
		t.Fatal("positioning ancestor exposed members")
	}
	parent := uint(0)
	if _, err := service.UpdateUnit(ctx, 7, root.ID, organization.UpdateUnitRequest{ParentID: &parent}); err == nil {
		t.Fatal("positioning ancestor was moved")
	}
	if err := service.SetUsers(ctx, 7, root.ID, []uint{}); err == nil {
		t.Fatal("positioning ancestor membership changed")
	}
	all := organization.NewService(repository, organization.NewHierarchy(repository), membershipVisibility{organization.OrganizationScope{All: true}}, nil, nil, platformdatabase.NewTransactionRunner(db), membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, noCustomOrganizationReferences{}, noHierarchyDependencies{})
	tree, err = all.Tree(ctx, 7)
	if err != nil || len(tree) != 1 || !tree[0].Manageable || len(tree[0].Children) != 2 || !tree[0].Children[0].Manageable || !tree[0].Children[1].Manageable {
		t.Fatalf("all tree = %#v, %v", tree, err)
	}
}

func TestUserMembershipsRejectProtectedSelfAndInvalidOrganizationsAtomically(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(append(organization.Models(), &authgorm.UserAccessVersion{})...); err != nil {
		t.Fatal(err)
	}
	repository := organization.NewGORMRepository(db)
	ctx := context.Background()
	visible := organization.Unit{Name: "Visible", Code: "visible", Status: 1}
	outside := organization.Unit{Name: "Outside", Code: "outside", Status: 1}
	for _, unit := range []*organization.Unit{&visible, &outside} {
		if err := repository.CreateUnit(ctx, unit); err != nil {
			t.Fatal(err)
		}
	}
	joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
	if err := db.Create(&organization.Membership{UserID: 20, OrganizationID: visible.ID, CreatedAt: joined}).Error; err != nil {
		t.Fatal(err)
	}
	versions := authgorm.NewAccessVersions(db)
	if _, err := versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	normal := organization.ManagedUser{Exists: true, Visible: true}
	for _, test := range []struct {
		name     string
		operator uint
		user     organization.ManagedUser
		ids      []uint
		expected int
		code     organization.ErrorCode
	}{
		{"protected", 7, organization.ManagedUser{Exists: true, Visible: true, Protected: true}, []uint{}, 1, organization.CodeConflict},
		{"self", 20, normal, []uint{}, 1, organization.CodeValidationInvalid},
		{"outside user", 7, organization.ManagedUser{Exists: true}, []uint{}, 1, organization.CodePermissionDenied},
		{"missing user", 7, organization.ManagedUser{}, []uint{}, 1, organization.CodeNotFound},
		{"deleted user", 7, organization.ManagedUser{Exists: true, Deleted: true, Visible: true}, []uint{}, 1, organization.CodeNotFound},
		{"outside organization", 7, normal, []uint{outside.ID}, 1, organization.CodePermissionDenied},
		{"missing organization", 7, normal, []uint{999}, 1, organization.CodeValidationInvalid},
		{"zero organization", 7, normal, []uint{0}, 1, organization.CodeValidationInvalid},
		{"missing version", 7, normal, []uint{}, 0, organization.CodeValidationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := organization.NewUserMembershipService(repository, membershipVisibility{organization.OrganizationScope{OrganizationIDs: []uint{visible.ID}}}, membershipTargetPolicy{test.user}, versions, platformdatabase.NewTransactionRunner(db))
			_, err := service.Set(ctx, test.operator, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: test.ids, ExpectedAccessVersion: test.expected})
			if code, _ := organization.CodeOf(err); code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			retained, err := repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || len(retained) != 1 || retained[0].OrganizationID != visible.ID || !retained[0].JoinedAt.Equal(joined) {
				t.Fatalf("rejected memberships = %#v, %v", retained, err)
			}
			version, err := versions.Current(ctx, 20)
			if err != nil || version != 1 {
				t.Fatalf("rejected version = %d, %v", version, err)
			}
		})
	}
}

func TestUserMembershipNoopComparesOnlyManageableLinksAndKeepsCurrentVersion(t *testing.T) {
	for _, test := range []struct {
		name        string
		hasManaged  bool
		emptyScope  bool
		initialized bool
	}{
		{"retained and hidden", true, false, true},
		{"hidden only", false, false, true},
		{"empty manageable scope", false, true, true},
		{"missing version with retained link", true, false, false},
		{"missing version with hidden only", false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
			rows := []organization.Membership{
				{UserID: 20, OrganizationID: fixture.outside.ID, CreatedAt: joined},
				{UserID: 30, OrganizationID: fixture.child.ID, CreatedAt: joined},
			}
			selected := []uint{}
			if test.hasManaged {
				rows = append(rows, organization.Membership{UserID: 20, OrganizationID: fixture.child.ID, CreatedAt: joined})
				selected = []uint{fixture.child.ID, fixture.child.ID}
			}
			if err := fixture.db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			expected := 1
			if test.initialized {
				expected = 7
				if err := fixture.db.Create(&authgorm.UserAccessVersion{UserID: 20, Version: expected}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := fixture.db.Create(&authgorm.UserAccessVersion{UserID: 30, Version: 13}).Error; err != nil {
				t.Fatal(err)
			}
			scope := organization.OrganizationScope{OrganizationIDs: []uint{fixture.child.ID, fixture.leaf.ID}}
			if test.emptyScope {
				scope.OrganizationIDs = nil
			}
			service := organization.NewUserMembershipService(fixture.repository, membershipVisibility{scope}, membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db))
			before, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				result, err := service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: selected, ExpectedAccessVersion: expected})
				if err != nil || result != expected {
					t.Fatalf("equal Set = %d, %v; want %d", result, err, expected)
				}
			}
			after, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("equal memberships = %#v, %v; want %#v", after, err, before)
			}
			for id, version := range map[uint]int{20: expected, 30: 13} {
				current, err := fixture.versions.Current(ctx, id)
				if err != nil || current != version {
					t.Fatalf("stored version for %d = %d, %v; want %d", id, current, err, version)
				}
			}
			if test.emptyScope {
				return
			}
			result, err := service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{fixture.leaf.ID}, ExpectedAccessVersion: expected})
			if err != nil || result != expected+1 {
				t.Fatalf("change after no-op = %d, %v", result, err)
			}
			changed, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || len(changed) != 2 {
				t.Fatalf("changed memberships = %#v, %v", changed, err)
			}
			for _, fact := range changed {
				switch fact.OrganizationID {
				case fixture.outside.ID:
					if !fact.JoinedAt.Equal(joined) {
						t.Fatalf("hidden time changed = %#v", fact)
					}
				case fixture.leaf.ID:
					if !fact.JoinedAt.After(joined) {
						t.Fatalf("new time missing = %#v", fact)
					}
				default:
					t.Fatalf("unexpected retained relationship = %#v", fact)
				}
			}
			_, err = service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{fixture.leaf.ID, fixture.leaf.ID}, ExpectedAccessVersion: expected})
			if code, _ := organization.CodeOf(err); code != organization.CodeConflict {
				t.Fatalf("stale equal request = %v", err)
			}
			after, err = fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || !reflect.DeepEqual(after, changed) {
				t.Fatalf("stale equal memberships = %#v, %v", after, err)
			}
			current, err := fixture.versions.Current(ctx, 20)
			if err != nil || current != expected+1 {
				t.Fatalf("stored change version = %d, %v", current, err)
			}
			other, err := fixture.repository.MemberOrganizationMemberships(ctx, 30)
			if err != nil || len(other) != 1 || other[0].OrganizationID != fixture.child.ID || !other[0].JoinedAt.Equal(joined) {
				t.Fatalf("other user's relationships = %#v, %v", other, err)
			}
		})
	}
}

func TestRejectedUserMembershipNoopRollsBackMissingVersionInitialization(t *testing.T) {
	normal := organization.ManagedUser{Exists: true, Visible: true}
	for _, test := range []struct {
		name     string
		user     organization.ManagedUser
		target   string
		expected int
		code     organization.ErrorCode
	}{
		{"protected unchanged", organization.ManagedUser{Exists: true, Visible: true, Protected: true}, "same", 1, organization.CodeConflict},
		{"deleted unchanged", organization.ManagedUser{Exists: true, Deleted: true, Visible: true}, "same", 1, organization.CodeNotFound},
		{"missing user unchanged", organization.ManagedUser{}, "same", 1, organization.CodeNotFound},
		{"outside user unchanged", organization.ManagedUser{Exists: true}, "same", 1, organization.CodePermissionDenied},
		{"stale unchanged", normal, "same", 2, organization.CodeConflict},
		{"raw zero alongside retained", normal, "zero", 1, organization.CodeValidationInvalid},
		{"missing organization alongside retained", normal, "missing", 1, organization.CodeValidationInvalid},
		{"hidden selected", normal, "hidden", 1, organization.CodePermissionDenied},
		{"positioning ancestor selected", normal, "ancestor", 1, organization.CodePermissionDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
			if err := fixture.db.Create(&[]organization.Membership{
				{UserID: 20, OrganizationID: fixture.child.ID, CreatedAt: joined},
				{UserID: 20, OrganizationID: fixture.outside.ID, CreatedAt: joined},
			}).Error; err != nil {
				t.Fatal(err)
			}
			selected := []uint{fixture.child.ID, fixture.child.ID}
			switch test.target {
			case "zero":
				selected = append(selected, 0)
			case "missing":
				selected = append(selected, 999)
			case "hidden":
				selected = append(selected, fixture.outside.ID)
			case "ancestor":
				selected = append(selected, fixture.root.ID)
			}
			before, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil {
				t.Fatal(err)
			}
			service := organization.NewUserMembershipService(fixture.repository, membershipVisibility{organization.OrganizationScope{OrganizationIDs: []uint{fixture.child.ID, fixture.leaf.ID}}}, membershipTargetPolicy{test.user}, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db))
			_, err = service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: selected, ExpectedAccessVersion: test.expected})
			if code, _ := organization.CodeOf(err); code != test.code {
				t.Fatalf("error = %v; want %s", err, test.code)
			}
			after, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected relationships = %#v, %v", after, err)
			}
			var count int64
			if err := fixture.db.Model(&authgorm.UserAccessVersion{}).Where("user_id = ?", 20).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("rejected initialization persisted %d rows, %v", count, err)
			}
		})
	}
}

func TestUserMembershipVersionFailureRollsBackDeltasAndInitialization(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		name := "missing version"
		if initialized {
			name = "existing version"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			joined := time.Date(2026, 8, 1, 2, 3, 4, 0, time.UTC)
			if err := fixture.db.Create(&[]organization.Membership{
				{UserID: 20, OrganizationID: fixture.child.ID, CreatedAt: joined},
				{UserID: 20, OrganizationID: fixture.outside.ID, CreatedAt: joined},
			}).Error; err != nil {
				t.Fatal(err)
			}
			expected := 1
			if initialized {
				expected = 7
				if err := fixture.db.Create(&authgorm.UserAccessVersion{UserID: 20, Version: expected}).Error; err != nil {
					t.Fatal(err)
				}
			}
			before, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.db.Exec("CREATE TRIGGER reject_user_membership_version BEFORE UPDATE ON user_access_versions WHEN OLD.user_id = 20 BEGIN SELECT RAISE(ABORT, 'membership version failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			service := organization.NewUserMembershipService(fixture.repository, membershipVisibility{organization.OrganizationScope{OrganizationIDs: []uint{fixture.child.ID, fixture.leaf.ID}}}, membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db))
			_, err = service.Set(ctx, 7, 20, organization.UpdateUserOrganizationsRequest{OrganizationIDs: []uint{fixture.leaf.ID}, ExpectedAccessVersion: expected})
			if code, _ := organization.CodeOf(err); code != organization.CodeInternalError {
				t.Fatalf("version failure = %v", err)
			}
			after, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("failed relationships = %#v, %v", after, err)
			}
			if initialized {
				current, err := fixture.versions.Current(ctx, 20)
				if err != nil || current != expected {
					t.Fatalf("failed version = %d, %v", current, err)
				}
			} else {
				var count int64
				if err := fixture.db.Model(&authgorm.UserAccessVersion{}).Where("user_id = ?", 20).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("failed initialization persisted %d rows, %v", count, err)
				}
			}
		})
	}
}
