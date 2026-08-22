package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/app"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNewInstallsAPIMetadataPermissionPolicyByDefault(t *testing.T) {
	conf := testAppConfig()
	conf.Admin.Username = "seed_admin"
	conf.Admin.Password = "seed_password"
	conf.Admin.Email = "seed@example.com"
	conf.Admin.Nickname = "Seed Admin"
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App models: %v", err)
	}
	if err := db.Create(&apigorm.API{Name: "Policy Probe", Method: http.MethodPost, Path: "/api/admin/policy-probe", Status: 1, NeedAuth: 1, NeedAudit: 1, PermissionCode: "policy.probe"}).Error; err != nil {
		t.Fatalf("create API policy: %v", err)
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
		roles := []string{}
		if role := c.GetHeader("X-Role"); role != "" {
			roles = append(roles, role)
		}
		permissions := []string{}
		if permission := c.GetHeader("X-Permission"); permission != "" {
			permissions = append(permissions, permission)
		}
		c.Set("roles", roles)
		c.Set("permissions", permissions)
		c.Next()
	}
	descriptor := routecatalog.Descriptor{
		Method: http.MethodPost, Path: "/api/admin/policy-probe", Access: routecatalog.PermissionControlled,
		Handler: func(c *gin.Context) { c.Status(http.StatusNoContent) },
		Name:    "Policy Probe", Group: "test", DefaultPermissionCode: "policy.probe", DefaultAuditCategory: "permission",
		OpenAPI: routecatalog.Operation{Summary: "Policy Probe", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody}, Responses: map[int]routecatalog.Response{http.StatusNoContent: {Description: "accepted", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]any{})}}},
	}
	application, err := app.New(context.Background(), conf, app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{
		Descriptors: []routecatalog.Descriptor{descriptor},
		Middleware:  app.HTTPMiddleware{Authenticated: authenticate},
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	request := httptest.NewRequest(http.MethodPost, descriptor.Path, nil)
	request.Header.Set("X-Identity", "user")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Body.String() != `{"code":403,"error_code":"API_META_PERMISSION_DENIED","msg":"permission is denied","data":null}` {
		t.Fatalf("missing permission response = %d %s", response.Code, response.Body.String())
	}
}
