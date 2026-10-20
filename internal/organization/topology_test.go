package organization_test

import (
	"context"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

type topologyFixture struct {
	db         *gorm.DB
	repository organization.Repository
	versions   *authgorm.AccessVersions
	root       organization.Unit
	child      organization.Unit
	leaf       organization.Unit
	outside    organization.Unit
}

func newTopologyFixture(t *testing.T) topologyFixture {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(append(organization.Models(), &authgorm.UserAccessVersion{})...); err != nil {
		t.Fatal(err)
	}
	fixture := topologyFixture{db: db, repository: organization.NewGORMRepository(db), versions: authgorm.NewAccessVersions(db)}
	fixture.root = organization.Unit{Name: "Root", Code: "root", Status: 1}
	fixture.outside = organization.Unit{Name: "Outside", Code: "outside", Status: 1}
	for _, unit := range []*organization.Unit{&fixture.root, &fixture.outside} {
		if err := fixture.repository.CreateUnit(context.Background(), unit); err != nil {
			t.Fatal(err)
		}
	}
	fixture.child = organization.Unit{ParentID: fixture.root.ID, Name: "Child", Code: "child", Status: 1}
	if err := fixture.repository.CreateUnit(context.Background(), &fixture.child); err != nil {
		t.Fatal(err)
	}
	fixture.leaf = organization.Unit{ParentID: fixture.child.ID, Name: "Leaf", Code: "leaf", Status: 1}
	if err := fixture.repository.CreateUnit(context.Background(), &fixture.leaf); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture topologyFixture) service(scope organization.OrganizationScope) *organization.Service {
	return organization.NewService(fixture.repository, organization.NewHierarchy(fixture.repository), membershipVisibility{scope}, nil, fixture.versions, platformdatabase.NewTransactionRunner(fixture.db), membershipTargetPolicy{organization.ManagedUser{Exists: true, Visible: true}}, noCustomOrganizationReferences{}, noHierarchyDependencies{})
}

func TestTopologyRejectsInvalidWritesWithoutChangingStoredUnits(t *testing.T) {
	fixture := newTopologyFixture(t)
	ctx := context.Background()
	all := organization.OrganizationScope{All: true}
	scoped := organization.OrganizationScope{OrganizationIDs: []uint{fixture.child.ID, fixture.leaf.ID}}
	before, err := fixture.repository.ListAllUnits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		operation string
		target    uint
		parent    uint
		scope     organization.OrganizationScope
		code      organization.ErrorCode
	}{
		{"self parent", "update", fixture.child.ID, fixture.child.ID, all, organization.CodeValidationInvalid},
		{"descendant parent", "update", fixture.root.ID, fixture.leaf.ID, all, organization.CodeValidationInvalid},
		{"missing parent", "update", fixture.child.ID, 999, all, organization.CodeNotFound},
		{"missing target", "update", 999, 0, all, organization.CodeNotFound},
		{"positioning ancestor target", "update", fixture.root.ID, 0, scoped, organization.CodeValidationInvalid},
		{"positioning ancestor parent", "update", fixture.leaf.ID, fixture.root.ID, scoped, organization.CodeValidationInvalid},
		{"nonempty delete", "delete", fixture.child.ID, 0, all, organization.CodeValidationInvalid},
		{"missing delete", "delete", 999, 0, all, organization.CodeNotFound},
		{"outside delete", "delete", fixture.outside.ID, 0, scoped, organization.CodeValidationInvalid},
		{"missing create parent", "create", 0, 999, all, organization.CodeNotFound},
		{"positioning ancestor create parent", "create", 0, fixture.root.ID, scoped, organization.CodeValidationInvalid},
		{"scoped root create", "create", 0, 0, scoped, organization.CodeValidationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := fixture.service(test.scope)
			var err error
			switch test.operation {
			case "create":
				_, err = service.CreateUnit(ctx, 7, organization.CreateUnitRequest{ParentID: test.parent, Name: "Rejected", Code: "rejected"})
			case "update":
				_, err = service.UpdateUnit(ctx, 7, test.target, organization.UpdateUnitRequest{ParentID: &test.parent, Name: "Rejected"})
			case "delete":
				err = service.DeleteUnit(ctx, 7, test.target)
			}
			if code, _ := organization.CodeOf(err); code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			after, err := fixture.repository.ListAllUnits(ctx)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("stored units changed after rejection: %#v, %v", after, err)
			}
		})
	}
}

func TestTopologyAllowsRootMovesAndPreservesOrdinaryUpdatesAndMemberCleanup(t *testing.T) {
	fixture := newTopologyFixture(t)
	ctx := context.Background()
	all := fixture.service(organization.OrganizationScope{All: true})
	root, err := all.CreateUnit(ctx, 7, organization.CreateUnitRequest{Name: "New root", Code: "new-root"})
	if err != nil || root.ParentID != 0 {
		t.Fatalf("create root = %#v, %v", root, err)
	}
	created, err := all.CreateUnit(ctx, 7, organization.CreateUnitRequest{ParentID: root.ID, Name: "New child", Code: "new-child"})
	if err != nil || created.ParentID != root.ID {
		t.Fatalf("create child = %#v, %v", created, err)
	}
	scoped := fixture.service(organization.OrganizationScope{OrganizationIDs: []uint{created.ID}})
	parent := uint(0)
	moved, err := scoped.UpdateUnit(ctx, 7, created.ID, organization.UpdateUnitRequest{ParentID: &parent})
	if err != nil || moved.ParentID != 0 {
		t.Fatalf("scoped move to root = %#v, %v", moved, err)
	}
	if _, err := all.UpdateUnit(ctx, 7, created.ID, organization.UpdateUnitRequest{ParentID: &root.ID}); err != nil {
		t.Fatal(err)
	}
	status, sort := 0, 9
	updated, err := all.UpdateUnit(ctx, 7, created.ID, organization.UpdateUnitRequest{Name: "Renamed", Code: "renamed", Remark: "Updated", Sort: &sort, Status: &status})
	if err != nil || updated.ParentID != root.ID || updated.Name != "Renamed" || updated.Code != "renamed" || updated.Remark != "Updated" || updated.Sort != sort || updated.Status != status {
		t.Fatalf("ordinary update = %#v, %v", updated, err)
	}
	if err := fixture.repository.CreateMemberships(ctx, []organization.Membership{{UserID: 20, OrganizationID: created.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if err := all.DeleteUnit(ctx, 7, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.FindUnitByID(ctx, created.ID); err != organization.ErrNotFound {
		t.Fatalf("deleted unit lookup = %v", err)
	}
	memberships, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
	if err != nil || len(memberships) != 0 {
		t.Fatalf("deleted unit memberships = %#v, %v", memberships, err)
	}
	version, err := fixture.versions.Current(ctx, 20)
	if err != nil || version != 2 {
		t.Fatalf("member access version = %d, %v", version, err)
	}
}

func TestTopologyStorageFailuresRollBackUnitsMembershipsAndVersions(t *testing.T) {
	for _, operation := range []string{"create", "move", "delete"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newTopologyFixture(t)
			ctx := context.Background()
			if err := fixture.repository.CreateMemberships(ctx, []organization.Membership{{UserID: 20, OrganizationID: fixture.leaf.ID}}); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
				t.Fatal(err)
			}
			before, err := fixture.repository.ListAllUnits(ctx)
			if err != nil {
				t.Fatal(err)
			}
			membershipsBefore, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil {
				t.Fatal(err)
			}
			trigger := "AFTER INSERT ON organizations"
			if operation == "move" {
				trigger = "AFTER UPDATE ON organizations"
			} else if operation == "delete" {
				// Fail after the unit and its memberships have both been deleted.
				trigger = "BEFORE UPDATE ON user_access_versions"
			}
			if err := fixture.db.Exec("CREATE TRIGGER reject_topology_write " + trigger + " BEGIN SELECT RAISE(ABORT, 'topology storage failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			service := fixture.service(organization.OrganizationScope{All: true})
			switch operation {
			case "create":
				_, err = service.CreateUnit(ctx, 7, organization.CreateUnitRequest{ParentID: fixture.root.ID, Name: "Failed", Code: "failed"})
			case "move":
				_, err = service.UpdateUnit(ctx, 7, fixture.leaf.ID, organization.UpdateUnitRequest{ParentID: &fixture.outside.ID, Name: "Failed"})
			case "delete":
				err = service.DeleteUnit(ctx, 7, fixture.leaf.ID)
			}
			if code, _ := organization.CodeOf(err); code != organization.CodeInternalError {
				t.Fatalf("storage error = %v", err)
			}
			after, err := fixture.repository.ListAllUnits(ctx)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("units changed after transaction failure: %#v, %v", after, err)
			}
			membershipsAfter, err := fixture.repository.MemberOrganizationMemberships(ctx, 20)
			if err != nil || !reflect.DeepEqual(membershipsAfter, membershipsBefore) {
				t.Fatalf("memberships changed after transaction failure: %#v, %v", membershipsAfter, err)
			}
			version, err := fixture.versions.Current(ctx, 20)
			if err != nil || version != 1 {
				t.Fatalf("version changed after transaction failure: %d, %v", version, err)
			}
		})
	}
}
