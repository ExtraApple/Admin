package organization_test

import (
	"context"
	"slices"
	"testing"

	"admin/internal/organization"
	"admin/testsupport/testutil"
)

type hierarchyReader interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
}

func TestHierarchyReturnsMemberAndDescendantOrganizationIDs(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(organization.Models()...); err != nil {
		t.Fatalf("migrate Organization models: %v", err)
	}

	root := organization.Unit{Name: "Root", Code: "root", Status: 1}
	other := organization.Unit{Name: "Other", Code: "other", Status: 1}
	if err := db.Create(&[]*organization.Unit{&root, &other}).Error; err != nil {
		t.Fatalf("create root Organization Units: %v", err)
	}
	child := organization.Unit{ParentID: root.ID, Name: "Child", Code: "child", Status: 1}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child Organization Unit: %v", err)
	}
	grandchild := organization.Unit{ParentID: child.ID, Name: "Grandchild", Code: "grandchild", Status: 1}
	if err := db.Create(&grandchild).Error; err != nil {
		t.Fatalf("create grandchild Organization Unit: %v", err)
	}
	if err := db.Create(&[]organization.Membership{
		{UserID: 42, OrganizationID: root.ID},
		{UserID: 42, OrganizationID: other.ID},
	}).Error; err != nil {
		t.Fatalf("create Organization Memberships: %v", err)
	}

	var hierarchy hierarchyReader = organization.NewHierarchy(organization.NewGORMRepository(db))
	memberIDs, err := hierarchy.MemberOrganizationIDs(context.Background(), 42)
	if err != nil {
		t.Fatalf("read member Organization IDs: %v", err)
	}
	if !slices.Equal(memberIDs, []uint{root.ID, other.ID}) {
		t.Fatalf("member Organization IDs = %v, want %v", memberIDs, []uint{root.ID, other.ID})
	}

	descendantIDs, err := hierarchy.DescendantOrganizationIDs(context.Background(), []uint{root.ID, 0, root.ID})
	if err != nil {
		t.Fatalf("read descendant Organization IDs: %v", err)
	}
	if !slices.Equal(descendantIDs, []uint{root.ID, child.ID, grandchild.ID}) {
		t.Fatalf("descendant Organization IDs = %v, want root/child/grandchild", descendantIDs)
	}
}
