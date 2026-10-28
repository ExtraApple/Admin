package application_test

import (
	"context"
	"testing"

	gormadapter "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
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
	core := application.NewCore(repository, platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
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
	service := application.NewService(core, nil, application.WithRouteSource(source))
	preview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncAPIs})
	if err != nil {
		t.Fatalf("preview sync routes: %v", err)
	}
	created, err := service.SyncRoutes(ctx, preview.Fingerprint)
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

func TestServiceSyncRoutesFiltersNonAPIPathsAndPreservesPublicRefreshConfiguration(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	core := application.NewCore(repository, platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	ctx := context.Background()
	configuredAuth := 1
	refresh, err := core.Create(ctx, application.CreateInput{Name: "Refresh", Method: "POST", Path: "/api/refresh", NeedAuth: &configuredAuth, PermissionCode: "operator.refresh"})
	if err != nil {
		t.Fatalf("create refresh API: %v", err)
	}
	source := &routeSourceFake{facts: []application.RouteFact{
		{Method: "GET", Path: "/ping", Name: "Ping"},
		{Method: "GET", Path: "/docs", Name: "Docs"},
		{Method: "POST", Path: "/api/refresh", Name: "Catalog Refresh"},
	}}
	service := application.NewService(core, nil, application.WithRouteSource(source))
	preview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncAPIs})
	if err != nil {
		t.Fatalf("preview sync routes: %v", err)
	}
	created, err := service.SyncRoutes(ctx, preview.Fingerprint)
	if err != nil {
		t.Fatalf("sync routes: %v", err)
	}
	if len(created) != 0 || source.calls != 2 {
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
	if refreshed.NeedAuth != 1 || refreshed.PermissionCode != "operator.refresh" {
		t.Fatalf("historical Public refresh configuration was overwritten: %#v", refreshed)
	}
}

func TestServiceSyncRoutesCreatesCookieLogoutWithoutPermissionAndKeepsHistoricalConfiguration(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new logout"
		if existing {
			name = "existing logout"
		}
		t.Run(name, func(t *testing.T) {
			db := testutil.OpenIsolatedSQLite(t)
			if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
				t.Fatalf("migrate API Metadata: %v", err)
			}
			core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
			ctx := context.Background()
			if existing {
				disabled, auditOff := 0, 0
				if _, err := core.Create(ctx, application.CreateInput{
					Name: "Operator Logout", Method: "POST", Path: "/api/user/logout", Group: "custom",
					PermissionCode: "historical.logout", Remark: "operator remark", Sort: 17, Status: &disabled, NeedAudit: &auditOff,
				}); err != nil {
					t.Fatalf("create historical logout metadata: %v", err)
				}
			}
			source := &routeSourceFake{facts: []application.RouteFact{
				{Method: "POST", Path: "/api/user/logout", Name: "Cookie Logout", Group: "identity", NeedAudit: true},
				{Method: "GET", Path: "/api/user/context", Name: "Current Context", Authenticated: true, NeedAudit: true},
				{Method: "GET", Path: "/api/admin/widgets", Name: "Widgets", PermissionControlled: true, DefaultPermissionCode: "widgets.get", NeedAudit: true},
			}}
			service := application.NewService(core, nil, application.WithRouteSource(source))
			preview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncAPIs})
			if err != nil {
				t.Fatalf("preview Cookie logout routes: %v", err)
			}
			if _, err := service.SyncRoutes(ctx, preview.Fingerprint); err != nil {
				t.Fatalf("sync Cookie logout routes: %v", err)
			}
			apis, _, err := core.List(ctx, 1, 20, application.Filter{})
			if err != nil {
				t.Fatalf("read synchronized APIs: %v", err)
			}
			if len(apis) != 3 {
				t.Fatalf("synchronized API count = %d, want 3", len(apis))
			}
			for _, api := range apis {
				switch api.Path {
				case "/api/user/logout":
					if existing {
						if api.Name != "Operator Logout" || api.Group != "custom" || api.NeedAuth != 1 || api.PermissionCode != "historical.logout" || api.Status != 0 || api.NeedAudit != 0 || api.Remark != "operator remark" || api.Sort != 17 {
							t.Fatalf("operator configuration was overwritten: %#v", api)
						}
					} else if api.NeedAuth != 0 || api.PermissionCode != "" {
						t.Fatalf("new Cookie logout gained Bearer/permission defaults: %#v", api)
					}
				case "/api/user/context":
					if api.NeedAuth != 1 || api.PermissionCode != "" {
						t.Fatalf("current-user metadata boundary changed: %#v", api)
					}
				case "/api/admin/widgets":
					if api.NeedAuth != 1 || api.PermissionCode != "widgets.get" {
						t.Fatalf("administrator policy changed: %#v", api)
					}
				}
			}
			preview, err = service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncAPIs})
			if err != nil {
				t.Fatalf("preview repeated Cookie logout routes: %v", err)
			}
			createdAgain, err := service.SyncRoutes(ctx, preview.Fingerprint)
			if err != nil || len(createdAgain) != 0 {
				t.Fatalf("repeated sync created duplicate routes: %#v, %v", createdAgain, err)
			}
		})
	}
}
func TestServiceProjectsRouteCatalogStateOnListAndGet(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	ctx := context.Background()
	matched, err := core.Create(ctx, application.CreateInput{Name: "Matched", Method: "GET", Path: "/api/matched", PermissionCode: "matched.get"})
	if err != nil {
		t.Fatalf("create matched API: %v", err)
	}
	metadataOnly, err := core.Create(ctx, application.CreateInput{Name: "Metadata Only", Method: "GET", Path: "/api/metadata-only"})
	if err != nil {
		t.Fatalf("create metadata-only API: %v", err)
	}
	mismatch, err := core.Create(ctx, application.CreateInput{Name: "Mismatch", Method: "GET", Path: "/api/mismatch", PermissionCode: "historical.mismatch"})
	if err != nil {
		t.Fatalf("create mismatch API: %v", err)
	}
	source := &routeSourceFake{facts: []application.RouteFact{
		{Method: "GET", Path: "/api/matched", PermissionControlled: true, DefaultPermissionCode: "matched.get"},
		{Method: "GET", Path: "/api/mismatch", PermissionControlled: true, DefaultPermissionCode: "mismatch.get"},
	}}
	service := application.NewService(core, nil, application.WithRouteSource(source))
	apis, _, err := service.List(ctx, 1, 20, application.Filter{})
	if err != nil {
		t.Fatalf("list projected APIs: %v", err)
	}
	states := map[uint]string{}
	for _, api := range apis {
		states[api.ID] = api.RouteState
	}
	if states[matched.ID] != application.RouteStateMatched || states[metadataOnly.ID] != application.RouteStateMetadataOnly || states[mismatch.ID] != application.RouteStateAccessMismatch {
		t.Fatalf("route states = %#v", states)
	}
	projected, err := service.Get(ctx, matched.ID)
	if err != nil || projected.RouteState != application.RouteStateMatched {
		t.Fatalf("projected API = %#v, error = %v", projected, err)
	}
}

var _ application.RouteSource = (*routeSourceFake)(nil)
