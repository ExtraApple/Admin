package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestApplicationDoesNotCreateRouteFromDatabaseMetadata(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App models: %v", err)
	}
	metadata := apigorm.API{Name: "database only", Method: http.MethodGet, Path: "/api/admin/database-only", Status: 1, NeedAuth: 0}
	if err := db.Create(&metadata).Error; err != nil {
		t.Fatalf("create database-only API metadata: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, metadata.Path, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("database-only API status = %d, want 404; body=%s", response.Code, response.Body.String())
	}
}

func TestApplicationServesOpenAPIDocumentFromRouteCatalog(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App models: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	conf := testAppConfig()
	conf.APIDocs.Enabled = true
	application, err := app.New(context.Background(), conf, app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("OpenAPI status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"openapi":"3.0.3"`) {
		t.Fatalf("OpenAPI response missing version: %s", response.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths are missing")
	}
	for _, descriptor := range application.Catalog().Snapshot() {
		if !strings.HasPrefix(descriptor.Path, "/api/") {
			continue
		}
		path := documentedPath(descriptor.Path)
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI missing Catalog path %s", path)
		}
		if _, ok := pathItem[strings.ToLower(descriptor.Method)]; !ok {
			t.Fatalf("OpenAPI missing %s %s", descriptor.Method, path)
		}
	}
	menuButton := paths["/api/admin/apis/{id}/menu-button"].(map[string]any)["post"].(map[string]any)
	requestBody := menuButton["requestBody"].(map[string]any)
	content := requestBody["content"].(map[string]any)["application/json"].(map[string]any)
	properties := content["schema"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"parent_id", "name", "sort"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("menu-button schema missing %s", field)
		}
	}
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "swagger-ui") {
		t.Fatalf("Swagger UI response = %d %s", response.Code, response.Body.String())
	}
}

func documentedPath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[index] = "{" + strings.TrimLeft(part, ":*") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func TestApplicationResynchronizesRouteAfterMetadataPathDiverges(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App models: %v", err)
	}
	if err := db.Create(&apigorm.API{Name: "wrong path", Method: http.MethodGet, Path: "/api/admin/wrong-path", Status: 1, NeedAuth: 1, PermissionCode: "admin.apis.get"}).Error; err != nil {
		t.Fatalf("create divergent API metadata: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	authenticate := func(c *gin.Context) { c.Set("roles", []string{"admin"}); c.Set("permissions", []string{}); c.Next() }
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Middleware: app.HTTPMiddleware{Authenticated: authenticate}, Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/apis", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("diverged route access = %d, want 403", response.Code)
	}
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/admin/apis/sync", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("API resync status = %d body=%s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&apigorm.API{}).Where("method = ? AND path = ?", http.MethodGet, "/api/admin/apis").Count(&count).Error; err != nil {
		t.Fatalf("count resynchronized API: %v", err)
	}
	if count != 1 {
		t.Fatalf("resynchronized API count = %d", count)
	}
}

func TestApplicationSynchronizersShareRouteCatalogSet(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App models: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	authenticate := func(c *gin.Context) { c.Set("roles", []string{"admin"}); c.Set("permissions", []string{}); c.Next() }
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Middleware: app.HTTPMiddleware{Authenticated: authenticate}, Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/admin/apis/sync", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("API sync status = %d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/admin/permissions/sync", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("RBAC sync status = %d body=%s", response.Code, response.Body.String())
	}
	var apiRecords []apigorm.API
	if err := db.Find(&apiRecords).Error; err != nil {
		t.Fatalf("read synchronized APIs: %v", err)
	}
	apiSet := make(map[string]struct{}, len(apiRecords))
	for _, api := range apiRecords {
		apiSet[api.Method+" "+api.Path] = struct{}{}
	}
	permissionSet := make(map[string]struct{})
	var permissions []authgorm.Permission
	if err := db.Find(&permissions).Error; err != nil {
		t.Fatalf("read synchronized permissions: %v", err)
	}
	for _, permission := range permissions {
		permissionSet[permission.Code] = struct{}{}
	}
	for _, descriptor := range application.Catalog().Snapshot() {
		if !strings.HasPrefix(descriptor.Path, "/api/") {
			continue
		}
		key := descriptor.Method + " " + descriptor.Path
		if _, ok := apiSet[key]; !ok {
			t.Fatalf("API Metadata missing Catalog route %s", key)
		}
		if descriptor.Access == routecatalog.PermissionControlled {
			if _, ok := permissionSet[descriptor.DefaultPermissionCode]; !ok {
				t.Fatalf("RBAC Permission missing Catalog code %s", descriptor.DefaultPermissionCode)
			}
		}
	}
}

func TestApplicationStaticAccessPrecedesMetadataBootstrap(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App models: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	authenticate := func(c *gin.Context) {
		if c.GetHeader("X-Identity") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("roles", []string{c.GetHeader("X-Role")})
		c.Set("permissions", []string{})
		c.Next()
	}
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Middleware: app.HTTPMiddleware{Authenticated: authenticate}, Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	request := httptest.NewRequest(http.MethodPost, "/api/admin/apis/sync", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous bootstrap status = %d, want 401", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/admin/apis/sync", nil)
	request.Header.Set("X-Identity", "admin")
	request.Header.Set("X-Role", "admin")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("admin bootstrap status = %d body=%s", response.Code, response.Body.String())
	}
	if err := db.Model(&apigorm.API{}).Where("method = ? AND path = ?", http.MethodGet, "/api/admin/apis").Updates(map[string]any{"need_auth": 0, "permission_code": ""}).Error; err != nil {
		t.Fatalf("set runtime metadata: %v", err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/admin/apis", nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("metadata need_auth=0 downgraded static route: %d", response.Code)
	}
}
