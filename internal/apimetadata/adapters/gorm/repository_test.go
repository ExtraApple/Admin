package gormadapter_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	gormadapter "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
)

func TestRepositoryPersistsFiltersAndReadsAPIPolicy(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	ctx := context.Background()
	first := domain.API{Name: "List Users", Method: "GET", Path: "/api/admin/users", Group: "user", PermissionCode: "admin.users.get", Status: 1, NeedAuth: 1, NeedAudit: 1}
	second := domain.API{Name: "Public Dictionaries", Method: "GET", Path: "/api/dicts/types", Group: "dict", Status: 1, NeedAuth: 0, NeedAudit: 1}
	if err := repository.Create(ctx, &first); err != nil {
		t.Fatalf("create first API: %v", err)
	}
	if err := repository.Create(ctx, &second); err != nil {
		t.Fatalf("create second API: %v", err)
	}

	needAuth := 1
	list, total, err := repository.List(ctx, 0, 1, application.Filter{Keyword: "Users", Group: "user", Method: "GET", NeedAuth: &needAuth})
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("filtered APIs = %#v, total=%d, err=%v", list, total, err)
	}
	policy, err := repository.FindPolicyByRoute(ctx, "GET", "/api/admin/users")
	if err != nil || policy.Status != 1 || policy.NeedAuth != 1 || policy.PermissionCode != "admin.users.get" {
		t.Fatalf("API policy = %#v, %v", policy, err)
	}
	groups, err := repository.Groups(ctx)
	if err != nil || len(groups) != 2 || groups[0].Group != "dict" || groups[1].Group != "user" {
		t.Fatalf("API groups = %#v, %v", groups, err)
	}
	selected, err := repository.ListByIDs(ctx, []uint{second.ID, first.ID})
	if err != nil || len(selected) != 2 || selected[0].ID != first.ID || selected[1].ID != second.ID {
		t.Fatalf("selected APIs = %#v, %v", selected, err)
	}
}
func TestRepositoryUpdatesPermissionCodesForNavigationTransactions(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	ctx := context.Background()
	first := domain.API{Name: "First", Method: "GET", Path: "/api/first", Status: 1, NeedAuth: 1}
	second := domain.API{Name: "Second", Method: "POST", Path: "/api/second", Status: 1, NeedAuth: 1}
	if err := repository.Create(ctx, &first); err != nil {
		t.Fatalf("create first API: %v", err)
	}
	if err := repository.Create(ctx, &second); err != nil {
		t.Fatalf("create second API: %v", err)
	}
	if err := repository.SetPermissionCode(ctx, []uint{first.ID, second.ID}, "shared.code"); err != nil {
		t.Fatalf("set permission code: %v", err)
	}
	count, err := repository.CountPermissionCode(ctx, "shared.code")
	if err != nil || count != 2 {
		t.Fatalf("permission code count = %d, %v; want 2", count, err)
	}
	locked, err := repository.ListByIDsForUpdate(ctx, []uint{second.ID, first.ID})
	if err != nil || len(locked) != 2 || locked[0].ID != first.ID || locked[1].ID != second.ID {
		t.Fatalf("locked API records = %#v, %v", locked, err)
	}
}

func TestModelsKeepAPITableName(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	if !db.Migrator().HasTable("apis") {
		t.Fatal("missing API Metadata-owned apis table")
	}
}
