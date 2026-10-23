package app

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
)

func TestSyncCatalogAPIPersistsPublicNeedAuthZeroAndPreservesExistingMetadata(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(apigorm.Models()...); err != nil {
		t.Fatalf("migrate API metadata: %v", err)
	}

	existing := apigorm.API{
		Name: "Existing Public", Method: http.MethodGet, Path: "/api/a3-existing",
		Group: "legacy", PermissionCode: "legacy.public", Status: 1, NeedAuth: 1, NeedAudit: 0,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing metadata: %v", err)
	}
	softDeleted := apigorm.API{
		Name: "Soft Deleted Public", Method: http.MethodGet, Path: "/api/a3-soft-deleted",
		Group: "legacy", PermissionCode: "legacy.soft-deleted", Status: 1, NeedAuth: 1, NeedAudit: 0,
	}
	if err := db.Create(&softDeleted).Error; err != nil {
		t.Fatalf("create soft-deleted metadata: %v", err)
	}
	if err := db.Delete(&softDeleted).Error; err != nil {
		t.Fatalf("soft-delete metadata: %v", err)
	}

	descriptors := []routecatalog.Descriptor{
		a3SeedDescriptor(http.MethodGet, "/api/a3-public", routecatalog.Public, ""),
		a3SeedDescriptor(http.MethodGet, "/api/a3-authenticated", routecatalog.Authenticated, ""),
		a3SeedDescriptor(http.MethodPost, "/api/a3-protected", routecatalog.PermissionControlled, "admin.a3.protected"),
		a3SeedDescriptor(http.MethodGet, "/api/a3-existing", routecatalog.Public, ""),
		a3SeedDescriptor(http.MethodGet, "/api/a3-soft-deleted", routecatalog.Public, ""),
	}
	if err := syncCatalogAPIs(context.Background(), db, descriptors); err != nil {
		t.Fatalf("sync catalog APIs: %v", err)
	}

	assertAPI := func(method, path string) apigorm.API {
		t.Helper()
		var api apigorm.API
		if err := db.Unscoped().Where("method = ? AND path = ?", method, path).First(&api).Error; err != nil {
			t.Fatalf("read %s %s: %v", method, path, err)
		}
		return api
	}
	public := assertAPI(http.MethodGet, "/api/a3-public")
	if public.NeedAuth != 0 || public.PermissionCode != "" {
		t.Fatalf("public metadata = need_auth:%d permission_code:%q, want 0 and empty", public.NeedAuth, public.PermissionCode)
	}
	authenticated := assertAPI(http.MethodGet, "/api/a3-authenticated")
	if authenticated.NeedAuth != 1 || authenticated.PermissionCode == "" {
		t.Fatalf("authenticated metadata = need_auth:%d permission_code:%q, want authenticated policy", authenticated.NeedAuth, authenticated.PermissionCode)
	}
	protected := assertAPI(http.MethodPost, "/api/a3-protected")
	if protected.NeedAuth != 1 || protected.PermissionCode != "admin.a3.protected" {
		t.Fatalf("protected metadata = need_auth:%d permission_code:%q", protected.NeedAuth, protected.PermissionCode)
	}
	preserved := assertAPI(http.MethodGet, "/api/a3-existing")
	if preserved.Name != existing.Name || preserved.Group != existing.Group || preserved.PermissionCode != existing.PermissionCode || preserved.NeedAuth != existing.NeedAuth || preserved.NeedAudit != existing.NeedAudit {
		t.Fatalf("existing metadata changed: before=%#v after=%#v", existing, preserved)
	}
	restored := assertAPI(http.MethodGet, "/api/a3-soft-deleted")
	if restored.DeletedAt.Valid || restored.Status != 1 || restored.PermissionCode != softDeleted.PermissionCode || restored.NeedAuth != softDeleted.NeedAuth {
		t.Fatalf("soft-deleted metadata changed unexpectedly: %#v", restored)
	}
}

func TestSyncCatalogAPIPublicWriteRollsBackWhenNeedAuthUpdateFails(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(apigorm.Models()...); err != nil {
		t.Fatalf("migrate API metadata: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER a3_public_need_auth_failure BEFORE UPDATE OF need_auth ON apis WHEN NEW.path = '/api/a3-rollback-public' BEGIN SELECT RAISE(ABORT, 'a3 forced update failure'); END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := syncCatalogAPIs(context.Background(), db, []routecatalog.Descriptor{
		a3SeedDescriptor(http.MethodGet, "/api/a3-rollback-public", routecatalog.Public, ""),
	})
	if err == nil {
		t.Fatal("sync succeeded despite forced need_auth update failure")
	}
	var count int64
	if err := db.Unscoped().Model(&apigorm.API{}).Where("method = ? AND path = ?", http.MethodGet, "/api/a3-rollback-public").Count(&count).Error; err != nil {
		t.Fatalf("count rolled-back metadata: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back metadata count = %d, want 0", count)
	}
}

func a3SeedDescriptor(method, path string, access routecatalog.AccessLevel, permission string) routecatalog.Descriptor {
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: access, Handler: func(*gin.Context) {},
		Name: path, Group: "a3", DefaultPermissionCode: permission, DefaultAuditCategory: "a3",
		OpenAPI: routecatalog.Operation{
			Summary: path, Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				http.StatusOK: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]any{})},
			},
		},
	}
}
