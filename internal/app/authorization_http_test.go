package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newAuthorizationApp(t *testing.T) (*app.Application, *gorm.DB) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop(),
	}, app.Options{
		Seed:       func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
		Middleware: app.HTTPMiddleware{Authenticated: func(c *gin.Context) { c.Next() }, PermissionControlled: func(c *gin.Context) { c.Next() }},
	})
	if err != nil {
		t.Fatalf("build authorization App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return application, db
}

func TestAuthorizationHTTPRoutesPreserveRBACContracts(t *testing.T) {
	application, _ := newAuthorizationApp(t)
	engine, ok := application.Handler().(*gin.Engine)
	if !ok {
		t.Fatal("App Handler is not a Gin engine")
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/roles", strings.NewReader(`{"name":"Operator","code":"operator","data_scope":"self"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create role status = %d, body = %s", response.Code, response.Body.String())
	}
	var roleResponse struct {
		Code int `json:"code"`
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &roleResponse); err != nil {
		t.Fatalf("decode role response: %v", err)
	}
	if roleResponse.Code != http.StatusOK || roleResponse.Data.ID == 0 {
		t.Fatalf("role response = %#v", roleResponse)
	}

	request = httptest.NewRequest(http.MethodPut, "/api/admin/roles/"+strconv.FormatUint(uint64(roleResponse.Data.ID), 10), strings.NewReader(`{"name":"Changed"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("update role status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAuthorizationPermissionSyncConsumesStaticCatalog(t *testing.T) {
	application, db := newAuthorizationApp(t)
	engine := application.Handler().(*gin.Engine)
	engine.GET("/api/runtime-only", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/admin/permissions/sync", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("permission sync status = %d, body = %s", response.Code, response.Body.String())
	}

	var permissions []authgorm.Permission
	if err := db.Order("code asc").Find(&permissions).Error; err != nil {
		t.Fatalf("read synced permissions: %v", err)
	}
	codes := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		codes[permission.Code] = true
	}
	if !codes["admin.roles.get"] {
		t.Fatal("static Authorization route permission was not synchronized")
	}
	if codes["runtime-only.get"] {
		t.Fatal("Gin-only route was incorrectly synchronized")
	}
}
