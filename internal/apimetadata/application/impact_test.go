package application_test

import (
	"context"
	"testing"

	gormadapter "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
	"admin/internal/platform/configimpact"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

func TestServicePreviewImpactIsReadOnlyAndCoversAPIOperations(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	ctx := context.Background()
	api, err := core.Create(ctx, application.CreateInput{Name: "Preview API", Method: "GET", Path: "/api/preview", PermissionCode: "preview.get"})
	if err != nil {
		t.Fatalf("create API: %v", err)
	}
	source := &routeSourceFake{facts: []application.RouteFact{{Method: "GET", Path: "/api/preview", PermissionControlled: true, DefaultPermissionCode: "preview.get"}, {Method: "POST", Path: "/api/catalog-only", PermissionControlled: true, DefaultPermissionCode: "catalog.post"}}}
	coordinator := &previewCoordinatorFake{}
	service := application.NewService(core, coordinator, application.WithRouteSource(source))
	name := "Renamed"
	updateInput := application.ImpactPreviewInput{Operation: application.ImpactUpdateAPI, APIID: api.ID, Update: application.UpdateInput{Name: &name}}
	first, err := service.PreviewImpact(ctx, updateInput)
	if err != nil {
		t.Fatalf("preview update: %v", err)
	}
	second, err := service.PreviewImpact(ctx, updateInput)
	if err != nil || first.Fingerprint != second.Fingerprint || !configimpact.ValidFingerprint(first.Fingerprint) {
		t.Fatalf("unstable update preview: first=%#v second=%#v err=%v", first, second, err)
	}
	unchanged, err := core.Get(ctx, api.ID)
	if err != nil || unchanged.Name != "Preview API" {
		t.Fatalf("preview changed API metadata: %#v, %v", unchanged, err)
	}

	syncPreview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncAPIs})
	if err != nil || syncPreview.Summary.CreatedCount != 1 {
		t.Fatalf("sync preview = %#v, error = %v", syncPreview, err)
	}
	permissionPreview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncPermissions})
	if err != nil || len(permissionPreview.Summary.AffectedAPIIDs) != 1 {
		t.Fatalf("permission preview = %#v, error = %v", permissionPreview, err)
	}
	buttonPreview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactGenerateMenuButton, APIID: api.ID, ButtonParentID: 7, ButtonName: "Preview button", ButtonSort: 2})
	if err != nil || buttonPreview.Summary.AffectedAPIIDs[0] != api.ID {
		t.Fatalf("button preview = %#v, error = %v", buttonPreview, err)
	}
	deletePreview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactDeleteAPI, APIID: api.ID})
	if err != nil || deletePreview.Summary.AffectedAPIIDs[0] != api.ID {
		t.Fatalf("delete preview = %#v, error = %v", deletePreview, err)
	}
}

type previewCoordinatorFake struct{}

func (*previewCoordinatorFake) WithAPIPermissionSync(context.Context, []domain.API, func(context.Context) error) error {
	return nil
}
func (*previewCoordinatorFake) ExistingPermissionCodes(context.Context, []string, bool) ([]string, error) {
	return nil, nil
}
func (*previewCoordinatorFake) ChangeAPIPermissionCode(context.Context, uint, string, func(context.Context) error) error {
	return nil
}
func (*previewCoordinatorFake) DeleteAPI(context.Context, uint, func(context.Context) error) error {
	return nil
}
func (*previewCoordinatorFake) EnsureAPIPermission(context.Context, domain.API) (bool, error) {
	return false, nil
}
func (*previewCoordinatorFake) GenerateMenuButton(context.Context, uint, uint, string, int) (application.Button, error) {
	return application.Button{}, nil
}
func (*previewCoordinatorFake) ValidateMenuButtonParent(context.Context, uint) error {
	return nil
}

var _ application.PermissionCoordinator = (*previewCoordinatorFake)(nil)

func (*previewCoordinatorFake) GenerateMenuButtonWithGuard(context.Context, uint, uint, string, int, func(context.Context) error) (application.Button, error) {
	return application.Button{}, nil
}
func (*previewCoordinatorFake) ChangeAPIPermissionCodeWithGuard(context.Context, uint, string, func(context.Context) error, func(context.Context) error) error {
	return nil
}
func TestServiceRejectsStaleAPIFingerprintBeforeUpdate(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	ctx := context.Background()
	api, err := core.Create(ctx, application.CreateInput{Name: "Original", Method: "GET", Path: "/api/stale"})
	if err != nil {
		t.Fatalf("create API: %v", err)
	}
	service := application.NewService(core, nil)
	previewName := "Preview Name"
	preview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactUpdateAPI, APIID: api.ID, Update: application.UpdateInput{Name: &previewName}})
	if err != nil {
		t.Fatalf("preview API update: %v", err)
	}
	otherName := "Concurrent Name"
	if _, err := service.Update(ctx, api.ID, application.UpdateInput{Name: &otherName}); err != nil {
		t.Fatalf("concurrent API update: %v", err)
	}
	_, err = service.Update(ctx, api.ID, application.UpdateInput{Name: &previewName, ExpectedFingerprint: preview.Fingerprint})
	if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
		t.Fatalf("stale update error = %v, want %s", err, application.CodeConflict)
	}
	current, err := service.Get(ctx, api.ID)
	if err != nil || current.Name != otherName {
		t.Fatalf("stale update changed current API = %#v, error = %v", current, err)
	}
}
func TestServiceRejectsStaleAPISyncFingerprint(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), &permissionReferenceLockerFake{})
	ctx := context.Background()
	source := &routeSourceFake{facts: []application.RouteFact{{Method: "POST", Path: "/api/catalog-only", PermissionControlled: true, DefaultPermissionCode: "catalog.post"}}}
	service := application.NewService(core, nil, application.WithRouteSource(source))
	preview, err := service.PreviewImpact(ctx, application.ImpactPreviewInput{Operation: application.ImpactSyncAPIs})
	if err != nil {
		t.Fatalf("preview API sync: %v", err)
	}
	if _, err := core.Create(ctx, application.CreateInput{Name: "Concurrent", Method: "GET", Path: "/api/concurrent"}); err != nil {
		t.Fatalf("create concurrent metadata: %v", err)
	}
	_, err = service.SyncRoutes(ctx, preview.Fingerprint)
	if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
		t.Fatalf("stale API sync error = %v, want %s", err, application.CodeConflict)
	}
	apis, _, err := core.List(ctx, 1, 20, application.Filter{})
	if err != nil {
		t.Fatalf("list APIs after stale sync: %v", err)
	}
	for _, api := range apis {
		if api.Path == "/api/catalog-only" {
			t.Fatal("stale API sync created a route")
		}
	}
}
