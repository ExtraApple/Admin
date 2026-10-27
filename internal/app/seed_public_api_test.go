package app

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	authgorm "admin/internal/authorization/adapters/gorm"
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
	if authenticated.NeedAuth != 1 || authenticated.PermissionCode != "" {
		t.Fatalf("authenticated metadata gained a business permission or lost authentication: %#v", authenticated)
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

func TestSeedCookieControlDefaultsAndHistoricalConfiguration(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new Cookie controls"
		if existing {
			name = "historical Cookie controls"
		}
		t.Run(name, func(t *testing.T) {
			db := testutil.OpenIsolatedSQLite(t)
			if err := db.AutoMigrate(apigorm.Models()...); err != nil {
				t.Fatalf("migrate API metadata: %v", err)
			}
			for _, path := range []string{"/api/refresh", "/api/user/logout"} {
				if existing {
					record := apigorm.API{
						Name: "Operator Name", Method: http.MethodPost, Path: path, Group: "operator",
						PermissionCode: "operator.preserved", Remark: "operator remark", Sort: 23,
						Status: 1, NeedAuth: 1, NeedAudit: 1,
					}
					if err := db.Create(&record).Error; err != nil {
						t.Fatalf("create historical metadata: %v", err)
					}
					if err := db.Model(&record).Updates(map[string]any{"status": 0, "need_audit": 0}).Error; err != nil {
						t.Fatalf("configure historical metadata: %v", err)
					}
				}
			}
			catalog, err := routecatalog.New([]routecatalog.Descriptor{
				a3SeedDescriptor(http.MethodPost, "/api/refresh", routecatalog.Public, ""),
				a3SeedDescriptor(http.MethodPost, "/api/user/logout", routecatalog.Public, ""),
				a3SeedDescriptor(http.MethodGet, "/api/user/context", routecatalog.Authenticated, ""),
				a3SeedDescriptor(http.MethodGet, "/api/admin/widgets", routecatalog.PermissionControlled, "widgets.get"),
			})
			if err != nil {
				t.Fatalf("create seed catalog: %v", err)
			}
			for range 2 {
				if err := syncCatalogAPIs(context.Background(), db, catalog.Snapshot()); err != nil {
					t.Fatalf("seed Cookie catalog: %v", err)
				}
			}
			var records []apigorm.API
			if err := db.Find(&records).Error; err != nil {
				t.Fatalf("read seeded metadata: %v", err)
			}
			if len(records) != 4 {
				t.Fatalf("seeded API count = %d, want 4", len(records))
			}
			for _, record := range records {
				switch record.Path {
				case "/api/refresh", "/api/user/logout":
					if existing {
						if record.Name != "Operator Name" || record.Group != "operator" || record.PermissionCode != "operator.preserved" || record.Remark != "operator remark" || record.Sort != 23 || record.Status != 0 || record.NeedAuth != 1 || record.NeedAudit != 0 {
							t.Fatalf("seed overwrote historical Public configuration: %#v", record)
						}
					} else if record.NeedAuth != 0 || record.PermissionCode != "" {
						t.Fatalf("new Cookie control gained authentication/permission defaults: %#v", record)
					}
				case "/api/user/context":
					if record.NeedAuth != 1 {
						t.Fatalf("current context lost its identity requirement: %#v", record)
					}
				case "/api/admin/widgets":
					if record.NeedAuth != 1 || record.PermissionCode != "widgets.get" {
						t.Fatalf("administrator policy changed: %#v", record)
					}
				}
			}
		})
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

func TestCatalogSeedAccessLevelsPreserveHistoricalPermissionsAndGrants(t *testing.T) {
	fixture := newAtomicSyncGraphFixture(t, false, true)
	descriptors := []routecatalog.Descriptor{
		a3SeedDescriptor(http.MethodGet, "/api/atomic/first", routecatalog.Authenticated, ""),
		a3SeedDescriptor(http.MethodGet, "/api/profile-policy", routecatalog.Authenticated, ""),
		a3SeedDescriptor(http.MethodGet, "/api/public-policy", routecatalog.Public, ""),
		a3SeedDescriptor(http.MethodGet, "/api/protected-policy", routecatalog.PermissionControlled, ""),
	}
	before := atomicSyncFacts(t, fixture.db)
	for range 2 {
		if err := syncCatalogAPIs(context.Background(), fixture.db, descriptors); err != nil {
			t.Fatal(err)
		}
		if err := syncCatalogPermissions(context.Background(), fixture.db, descriptors); err != nil {
			t.Fatal(err)
		}
	}
	after := atomicSyncFacts(t, fixture.db)
	if !reflect.DeepEqual(before.RolePermissions, after.RolePermissions) || !reflect.DeepEqual(before.Versions, after.Versions) {
		t.Fatalf("seed changed historical grants or versions: before=%#v after=%#v", before, after)
	}
	var permissions []authgorm.Permission
	if err := fixture.db.Order("id").Find(&permissions).Error; err != nil {
		t.Fatal(err)
	}
	if len(permissions) != len(before.Permissions)+1 {
		t.Fatalf("seed added permissions outside the controlled route: %#v", permissions)
	}
	for _, permission := range permissions[:len(before.Permissions)] {
		found := false
		for _, old := range before.Permissions {
			if old.ID == permission.ID && old.Code == permission.Code && old.Name == permission.Name && old.Group == permission.Group && old.Sort == permission.Sort {
				found = true
			}
		}
		if !found {
			t.Fatalf("historical permission changed: %#v", permission)
		}
	}
	for _, old := range before.APIs {
		found := false
		for _, current := range after.APIs {
			if current == old {
				found = true
			}
		}
		if !found {
			t.Fatalf("historical API changed: %#v", old)
		}
	}
	var authenticated apigorm.API
	if err := fixture.db.Where("path = ?", "/api/profile-policy").First(&authenticated).Error; err != nil {
		t.Fatal(err)
	}
	if authenticated.NeedAuth != 1 || authenticated.PermissionCode != "" {
		t.Fatalf("authenticated policy metadata = %#v", authenticated)
	}
}
