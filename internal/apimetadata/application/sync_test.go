package application_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	gormadapter "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
	platformdatabase "admin/internal/platform/database"
)

type routeSourceFake struct {
	facts []application.RouteFact
	calls int
}

func (fake *routeSourceFake) Routes(context.Context) ([]application.RouteFact, error) {
	fake.calls++
	return append([]application.RouteFact(nil), fake.facts...), nil
}

func TestServiceSyncRoutesPreservesExistingMetadataAndReturnsOnlyCreated(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	core := application.NewCore(repository)
	ctx := context.Background()
	needAudit := 0
	existing, err := core.Create(ctx, application.CreateInput{Name: "Operator Name", Method: "GET", Path: "/api/admin/users", Group: "operator", PermissionCode: "operator.custom", NeedAudit: &needAudit})
	if err != nil {
		t.Fatalf("create existing API: %v", err)
	}
	deleted, err := core.Create(ctx, application.CreateInput{Name: "Deleted", Method: "DELETE", Path: "/api/admin/users/:id"})
	if err != nil {
		t.Fatalf("create deleted API: %v", err)
	}
	if err := db.Delete(&gormadapter.API{}, deleted.ID).Error; err != nil {
		t.Fatalf("soft delete API: %v", err)
	}

	source := &routeSourceFake{facts: []application.RouteFact{
		{Method: "GET", Path: "/api/admin/users", Name: "Catalog Name", Group: "user", PermissionControlled: true, DefaultPermissionCode: "admin.users.get", NeedAudit: true},
		{Method: "DELETE", Path: "/api/admin/users/:id", Name: "Delete User", Group: "user", PermissionControlled: true, DefaultPermissionCode: "admin.users.id.delete", NeedAudit: true},
		{Method: "GET", Path: "/api/public", Name: "Public", Group: "public"},
	}}
	service := application.NewService(core, nil, application.WithRouteSource(platformdatabase.NewTransactionRunner(db), source))
	created, err := service.SyncRoutes(ctx)
	if err != nil {
		t.Fatalf("sync routes: %v", err)
	}
	if len(created) != 1 || created[0].Path != "/api/public" || created[0].NeedAuth != 0 || created[0].PermissionCode != "" {
		t.Fatalf("created APIs = %#v", created)
	}
	preserved, err := core.Get(ctx, existing.ID)
	if err != nil {
		t.Fatalf("get preserved API: %v", err)
	}
	if preserved.Name != "Operator Name" || preserved.Group != "operator" || preserved.PermissionCode != "operator.custom" || preserved.NeedAudit != 0 {
		t.Fatalf("existing metadata was overwritten: %#v", preserved)
	}
	restored, err := core.Get(ctx, deleted.ID)
	if err != nil || restored.Status != 1 {
		t.Fatalf("restored API = %#v, %v", restored, err)
	}
}

func TestServiceSyncRoutesFiltersNonAPIPathsAndRepairsRefresh(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	core := application.NewCore(repository)
	ctx := context.Background()
	wrongAuth := 1
	refresh, err := core.Create(ctx, application.CreateInput{Name: "Refresh", Method: "POST", Path: "/api/refresh", NeedAuth: &wrongAuth, PermissionCode: "admin.refresh"})
	if err != nil {
		t.Fatalf("create refresh API: %v", err)
	}
	source := &routeSourceFake{facts: []application.RouteFact{
		{Method: "GET", Path: "/ping", Name: "Ping"},
		{Method: "GET", Path: "/docs", Name: "Docs"},
		{Method: "POST", Path: "/api/refresh", Name: "Catalog Refresh", Authenticated: true, PermissionControlled: true, DefaultPermissionCode: "admin.refresh"},
	}}
	service := application.NewService(core, nil, application.WithRouteSource(platformdatabase.NewTransactionRunner(db), source))
	created, err := service.SyncRoutes(ctx)
	if err != nil {
		t.Fatalf("sync routes: %v", err)
	}
	if len(created) != 0 || source.calls != 1 {
		t.Fatalf("sync result=%#v source calls=%d", created, source.calls)
	}
	apis, _, err := core.List(ctx, 1, 20, application.Filter{})
	if err != nil {
		t.Fatalf("list APIs: %v", err)
	}
	if len(apis) != 1 || apis[0].Path != "/api/refresh" {
		t.Fatalf("non-API routes were synchronized: %#v", apis)
	}
	refreshed, err := core.Get(ctx, refresh.ID)
	if err != nil {
		t.Fatalf("get refresh API: %v", err)
	}
	if refreshed.NeedAuth != 0 || refreshed.PermissionCode != "" {
		t.Fatalf("refresh compatibility = %#v", refreshed)
	}
}

var _ application.RouteSource = (*routeSourceFake)(nil)
