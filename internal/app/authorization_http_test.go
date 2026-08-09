package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
	identitydomain "admin/internal/identity/domain"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

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

func TestAuthorizationRoleUsersUseIdentityDirectoryAvatars(t *testing.T) {
	application, db := newAuthorizationApp(t)
	trusted := identity.User{Username: "role-trusted", Password: "unused", Email: "role-trusted@test.local", Avatar: "https://legacy.example/trusted.png", Status: 1}
	untrusted := identity.User{Username: "role-untrusted", Password: "unused", Email: "role-untrusted@test.local", Avatar: "https://legacy.example/untrusted.png", Status: 1}
	invalidMetadata := identity.User{Username: "role-invalid-metadata", Password: "unused", Email: "role-invalid-metadata@test.local", Avatar: "https://legacy.example/invalid.png", Status: 1}
	if err := db.Create(&[]*identity.User{&trusted, &untrusted, &invalidMetadata}).Error; err != nil {
		t.Fatalf("create role users: %v", err)
	}
	trustedObject := "avatars/" + strconv.FormatUint(uint64(trusted.ID), 10) + "/00000000-0000-4000-8000-000000000007.png"
	if err := db.Model(&trusted).Updates(map[string]any{
		"avatar_object_name": trustedObject, "avatar_content_type": "image/png", "avatar_validation_status": identitydomain.AvatarValidationStatusValidated,
	}).Error; err != nil {
		t.Fatalf("trust role user avatar: %v", err)
	}
	invalidObject := "avatars/" + strconv.FormatUint(uint64(invalidMetadata.ID), 10) + "/00000000-0000-4000-8000-000000000010.png"
	if err := db.Model(&invalidMetadata).Updates(map[string]any{
		"avatar_object_name": invalidObject, "avatar_content_type": "image/jpeg", "avatar_validation_status": identitydomain.AvatarValidationStatusValidated,
	}).Error; err != nil {
		t.Fatalf("set invalid role user avatar metadata: %v", err)
	}
	role := authgorm.Role{Name: "Directory Role", Code: "directory-role", Status: 1}
	emptyRole := authgorm.Role{Name: "Empty Directory Role", Code: "empty-directory-role", Status: 1}
	if err := db.Create(&[]*authgorm.Role{&role, &emptyRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := db.Create(&[]authgorm.UserRole{{UserID: trusted.ID, RoleID: role.ID}, {UserID: untrusted.ID, RoleID: role.ID}, {UserID: invalidMetadata.ID, RoleID: role.ID}}).Error; err != nil {
		t.Fatalf("assign role users: %v", err)
	}

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/roles/"+strconv.FormatUint(uint64(role.ID), 10)+"/users", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list role users status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []struct {
			ID     uint   `json:"id"`
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode role users: %v", err)
	}
	if len(payload.Data) != 3 || payload.Data[0].ID != trusted.ID || payload.Data[0].Avatar != "/api/avatars/"+strconv.FormatUint(uint64(trusted.ID), 10) || payload.Data[1].ID != untrusted.ID || payload.Data[1].Avatar != "/api/avatars/default" || payload.Data[2].ID != invalidMetadata.ID || payload.Data[2].Avatar != "/api/avatars/default" {
		t.Fatalf("role users = %#v", payload.Data)
	}

	emptyResponse := httptest.NewRecorder()
	application.Handler().ServeHTTP(emptyResponse, httptest.NewRequest(http.MethodGet, "/api/admin/roles/"+strconv.FormatUint(uint64(emptyRole.ID), 10)+"/users", nil))
	var emptyPayload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(emptyResponse.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatalf("decode empty role users: %v", err)
	}
	if emptyResponse.Code != http.StatusOK || emptyPayload.Data == nil || len(emptyPayload.Data) != 0 {
		t.Fatalf("empty role users status=%d data=%#v body=%s", emptyResponse.Code, emptyPayload.Data, emptyResponse.Body.String())
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
