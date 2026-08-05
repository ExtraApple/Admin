package app_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"admin/testsupport/testutil"
	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestSeedCatalogUsesValidatedSnapshotAndRemainsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate seed database: %v", err)
	}
	catalog, err := routecatalog.New([]routecatalog.Descriptor{{
		Method: http.MethodGet, Path: "/api/admin/users", Access: routecatalog.PermissionControlled,
		Handler: func(*gin.Context) {}, Name: "List Users", Group: "user",
		DefaultPermissionCode: "admin.users.get", DefaultAuditCategory: "user",
		OpenAPI: routecatalog.Operation{
			Summary: "List Users", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				http.StatusOK: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]any{})},
			},
		},
	}})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	conf := platformconfig.Config{}
	conf.Admin.Username = "seed_admin"
	conf.Admin.Password = "SeedAdmin@123456"
	conf.Admin.Email = "seed_admin@test.local"
	conf.Admin.Nickname = "Seed Admin"

	for run := 1; run <= 2; run++ {
		if err := app.SeedCatalog(context.Background(), conf, db, zap.NewNop(), catalog); err != nil {
			t.Fatalf("Seed Catalog run %d: %v", run, err)
		}
	}
	assertSeedCount(t, db.Model(&apigorm.API{}).Where("method = ? AND path = ?", http.MethodGet, "/api/admin/users"), 1, "Catalog API")
	assertSeedCount(t, db.Model(&authgorm.Permission{}).Where("code = ?", "admin.users.get"), 1, "Catalog permission")
	assertSeedCount(t, db.Model(&identity.User{}).Where("username = ?", conf.Admin.Username), 1, "admin user")
}

func assertSeedCount(t *testing.T, query *gorm.DB, want int64, label string) {
	t.Helper()
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", label, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", label, count, want)
	}
}

func TestSeedCatalogDoesNotCreatePermissionForUndeclaredAPI(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate seed database: %v", err)
	}
	ghost := apigorm.API{
		Name: "Ghost API", Method: http.MethodGet, Path: "/api/ghost",
		Status: 1, NeedAuth: 1, PermissionCode: "ghost.read",
	}
	if err := db.Create(&ghost).Error; err != nil {
		t.Fatalf("create undeclared API metadata: %v", err)
	}
	catalog, err := routecatalog.New([]routecatalog.Descriptor{{
		Method: http.MethodGet, Path: "/ping", Access: routecatalog.Public,
		Handler: func(*gin.Context) {}, Name: "Ping", Group: "system",
		DefaultAuditCategory: "system",
		OpenAPI: routecatalog.Operation{
			Summary: "Ping", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{http.StatusNoContent: {Description: "ok", Kind: routecatalog.NoBody}},
		},
	}})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	conf := platformconfig.Config{}
	conf.Admin.Username = "seed_admin"
	conf.Admin.Password = "SeedAdmin@123456"
	conf.Admin.Email = "seed_admin@test.local"
	if err := app.SeedCatalog(context.Background(), conf, db, zap.NewNop(), catalog); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	assertSeedCount(t, db.Model(&authgorm.Permission{}).Where("code = ?", "ghost.read"), 0, "undeclared API permission")
}

func TestSeedCatalogPreservesDescriptorMetadataForNewAPI(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate seed database: %v", err)
	}
	catalog, err := routecatalog.New([]routecatalog.Descriptor{{
		Method: http.MethodPost, Path: "/api/admin/catalog-metadata", Access: routecatalog.PermissionControlled,
		Handler: func(*gin.Context) {}, Name: "Catalog Metadata", Group: "catalog",
		DefaultPermissionCode: "catalog.metadata.post", DefaultAuditCategory: "permission",
		OpenAPI: routecatalog.Operation{
			Summary: "Catalog Metadata", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{http.StatusNoContent: {Description: "ok", Kind: routecatalog.NoBody}},
		},
	}})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	conf := platformconfig.Config{}
	conf.Admin.Username = "seed_admin"
	conf.Admin.Password = "SeedAdmin@123456"
	conf.Admin.Email = "seed_admin@test.local"
	if err := app.SeedCatalog(context.Background(), conf, db, zap.NewNop(), catalog); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	var metadata apigorm.API
	if err := db.Where("method = ? AND path = ?", http.MethodPost, "/api/admin/catalog-metadata").First(&metadata).Error; err != nil {
		t.Fatalf("read seeded API metadata: %v", err)
	}
	if metadata.Name != "Catalog Metadata" || metadata.Group != "catalog" || metadata.PermissionCode != "catalog.metadata.post" {
		t.Fatalf("seeded metadata = name %q group %q permission %q", metadata.Name, metadata.Group, metadata.PermissionCode)
	}
}
