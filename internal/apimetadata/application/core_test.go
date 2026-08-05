package application_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	gormadapter "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
)

func TestCoreCreatesNormalizedAPIMetadataAndQueriesPolicy(t *testing.T) {
	core := newAPIMetadataCore(t)
	ctx := context.Background()
	api, err := core.Create(ctx, application.CreateInput{
		Name: " List Users ", Method: " get ", Path: "api/admin/users/",
		PermissionCode: " admin.users.get ", Remark: " list ",
	})
	if err != nil {
		t.Fatalf("create API metadata: %v", err)
	}
	if api.Name != "List Users" || api.Method != "GET" || api.Path != "/api/admin/users" || api.Group != "user" {
		t.Fatalf("normalized API = %#v", api)
	}
	if api.Status != 1 || api.NeedAuth != 1 || api.NeedAudit != 1 || api.PermissionCode != "admin.users.get" {
		t.Fatalf("default API policy = %#v", api)
	}
	policy, err := core.Policy(ctx, "get", "api/admin/users/")
	if err != nil || policy.PermissionCode != "admin.users.get" || policy.Status != 1 {
		t.Fatalf("normalized policy = %#v, %v", policy, err)
	}
	if _, err := core.Create(ctx, application.CreateInput{Name: "Duplicate", Method: "GET", Path: "/api/admin/users"}); err == nil {
		t.Fatal("duplicate method and path were accepted")
	}
}
func TestCoreListsUpdatesAndDeletesAPIMetadata(t *testing.T) {
	core := newAPIMetadataCore(t)
	ctx := context.Background()
	first, err := core.Create(ctx, application.CreateInput{Name: "First", Method: "GET", Path: "/api/admin/first", Group: "test"})
	if err != nil {
		t.Fatalf("create first API: %v", err)
	}
	second, err := core.Create(ctx, application.CreateInput{Name: "Second", Method: "POST", Path: "/api/admin/second", Group: "test"})
	if err != nil {
		t.Fatalf("create second API: %v", err)
	}
	list, total, err := core.List(ctx, 1, 1, application.Filter{Method: "post"})
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("filtered list = %#v, total=%d, err=%v", list, total, err)
	}
	name, method, path := "Updated", "patch", "api/admin/updated/"
	updated, err := core.Update(ctx, first.ID, application.UpdateInput{Name: &name, Method: &method, Path: &path})
	if err != nil || updated.Name != name || updated.Method != "PATCH" || updated.Path != "/api/admin/updated" {
		t.Fatalf("updated API = %#v, %v", updated, err)
	}
	duplicateMethod, duplicatePath := "POST", "/api/admin/second"
	if _, err := core.Update(ctx, first.ID, application.UpdateInput{Method: &duplicateMethod, Path: &duplicatePath}); err == nil {
		t.Fatal("duplicate API update was accepted")
	}
	if _, err := core.Update(ctx, first.ID, application.UpdateInput{}); err == nil {
		t.Fatal("empty API update was accepted")
	}
	groups, err := core.Groups(ctx)
	if err != nil || len(groups) != 1 || groups[0].Group != "test" || groups[0].Count != 2 {
		t.Fatalf("groups = %#v, %v", groups, err)
	}
	methods := core.Methods()
	if len(methods) != 7 || methods[0].Value != "GET" || methods[1].Value != "POST" {
		t.Fatalf("method options = %#v", methods)
	}
	if err := core.Delete(ctx, first.ID); err != nil {
		t.Fatalf("delete API: %v", err)
	}
	if _, err := core.Get(ctx, first.ID); err == nil {
		t.Fatal("hard-deleted API remained queryable")
	}
}

func newAPIMetadataCore(t *testing.T) *application.Core {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	return application.NewCore(gormadapter.NewRepository(db))
}
