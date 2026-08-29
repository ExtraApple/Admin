package organization_test

import (
	"context"
	"testing"

	"admin/internal/organization"
	"admin/testsupport/testutil"
)

func TestRepositoryAppliesOrganizationScopeSemantics(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(organization.Models()...); err != nil {
		t.Fatalf("migrate organization models: %v", err)
	}
	repository := organization.NewGORMRepository(db)
	units := []organization.Unit{
		{Name: "First", Code: "first", Status: 1},
		{Name: "Second", Code: "second", Status: 1},
	}
	for index := range units {
		if err := repository.CreateUnit(context.Background(), &units[index]); err != nil {
			t.Fatalf("create organization %d: %v", index, err)
		}
	}

	all, total, err := repository.ListUnits(context.Background(), 0, 10, "", nil, organization.OrganizationScope{All: true})
	if err != nil || total != 2 || len(all) != 2 {
		t.Fatalf("all scope = %d, %#v, %v; want two rows", total, all, err)
	}
	empty, total, err := repository.ListUnits(context.Background(), 0, 10, "", nil, organization.OrganizationScope{})
	if err != nil || total != 0 || len(empty) != 0 {
		t.Fatalf("empty scope = %d, %#v, %v; want no rows", total, empty, err)
	}
	selected, total, err := repository.ListUnits(context.Background(), 0, 10, "", nil, organization.OrganizationScope{OrganizationIDs: []uint{units[1].ID}})
	if err != nil || total != 1 || len(selected) != 1 || selected[0].ID != units[1].ID {
		t.Fatalf("selected scope = %d, %#v, %v; want second row", total, selected, err)
	}
}

func TestScopeReaderListsAllOrganizationIDsForSystemWideAudience(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(organization.Models()...); err != nil {
		t.Fatalf("migrate organization models: %v", err)
	}
	repository := organization.NewGORMRepository(db)
	for _, unit := range []organization.Unit{{Name: "First", Code: "first", Status: 1}, {Name: "Second", Code: "second", Status: 1}} {
		if err := repository.CreateUnit(context.Background(), &unit); err != nil {
			t.Fatalf("create organization: %v", err)
		}
	}
	reader := organization.NewScopeReader(repository, organization.NewHierarchy(repository))
	ids, err := reader.AllOrganizationIDs(context.Background())
	if err != nil || len(ids) != 2 || ids[0] == 0 || ids[1] == 0 || ids[0] == ids[1] {
		t.Fatalf("AllOrganizationIDs() = %#v, %v", ids, err)
	}
}
