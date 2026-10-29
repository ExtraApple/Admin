package application_test

import (
	"context"
	"net/http"
	"testing"

	gormadapter "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

func TestServiceListFiltersProjectedRouteStatesBeforePagination(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	ctx := context.Background()
	if _, err := core.Create(ctx, application.CreateInput{Name: "Matched", Method: http.MethodGet, Path: "/api/matched", PermissionCode: "matched"}); err != nil {
		t.Fatalf("create matched API: %v", err)
	}
	if _, err := core.Create(ctx, application.CreateInput{Name: "Mismatch", Method: http.MethodGet, Path: "/api/mismatch", PermissionCode: "wrong"}); err != nil {
		t.Fatalf("create mismatch API: %v", err)
	}
	if _, err := core.Create(ctx, application.CreateInput{Name: "Metadata only", Method: http.MethodGet, Path: "/api/metadata-only", PermissionCode: "metadata"}); err != nil {
		t.Fatalf("create metadata-only API: %v", err)
	}

	service := application.NewService(core, nil, application.WithRouteSource(&routeSourceFake{facts: []application.RouteFact{
		{Method: http.MethodGet, Path: "/api/matched", PermissionControlled: true, DefaultPermissionCode: "matched"},
		{Method: http.MethodGet, Path: "/api/mismatch", PermissionControlled: true, DefaultPermissionCode: "expected"},
	}}))
	apis, total, err := service.List(ctx, 1, 1, application.Filter{RouteState: application.RouteStateAccessMismatch})
	if err != nil {
		t.Fatalf("filter route state: %v", err)
	}
	if total != 1 || len(apis) != 1 || apis[0].Path != "/api/mismatch" || apis[0].RouteState != application.RouteStateAccessMismatch {
		t.Fatalf("filtered APIs = %#v total=%d", apis, total)
	}

	apis, total, err = service.List(ctx, 1, 20, application.Filter{RouteState: application.RouteStateMetadataOnly})
	if err != nil {
		t.Fatalf("filter metadata-only state: %v", err)
	}
	if total != 1 || len(apis) != 1 || apis[0].Path != "/api/metadata-only" {
		t.Fatalf("metadata-only APIs = %#v total=%d", apis, total)
	}
}

func TestServiceListRejectsUnknownRouteStateFilter(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	_, _, err := application.NewService(core, nil).List(context.Background(), 1, 20, application.Filter{RouteState: "unknown"})
	if code, ok := application.CodeOf(err); !ok || code != application.CodeValidationInvalid {
		t.Fatalf("unknown route state error = %v, code=%s, classified=%v", err, code, ok)
	}
}
