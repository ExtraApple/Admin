package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	authdomain "admin/internal/authorization/domain"
	"admin/internal/identity"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNewAssemblesOrganizationHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate App database: %v", err)
	}
	admin := identity.User{Username: "organization-admin", Password: "unused", Email: "organization-admin@test.local", Status: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create organization admin: %v", err)
	}
	role := authgorm.Role{Name: "Organization Admin", Code: "admin", Status: 1, DataScope: string(authdomain.DataScopeAll)}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create organization admin role: %v", err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: admin.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("assign organization admin role: %v", err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{UserID: admin.ID, Version: 1}).Error; err != nil {
		t.Fatalf("create organization admin access version: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	authenticate := func(c *gin.Context) {
		c.Set("userID", admin.ID)
		c.Next()
	}
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop(),
	}, app.Options{
		Middleware: app.HTTPMiddleware{Authenticated: authenticate, PermissionControlled: func(c *gin.Context) { c.Next() }},
		Seed:       func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	root := postOrganization(t, application.Handler(), "/api/admin/organizations", map[string]any{
		"name": "Root", "code": "root", "sort": 20,
	})
	rootID := uint(root["id"].(float64))
	child := postOrganization(t, application.Handler(), "/api/admin/organizations", map[string]any{
		"parent_id": rootID, "name": "Child", "code": "child", "sort": 10,
	})
	childID := uint(child["id"].(float64))

	duplicate := requestOrganization(t, application.Handler(), http.MethodPost, "/api/admin/organizations", map[string]any{
		"name": "Duplicate", "code": "root",
	})
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("duplicate Organization status = %d, want 400", duplicate.Code)
	}

	treeResponse := requestOrganization(t, application.Handler(), http.MethodGet, "/api/admin/organizations/tree", nil)
	if treeResponse.Code != http.StatusOK {
		t.Fatalf("Organization tree status = %d body=%s", treeResponse.Code, treeResponse.Body.String())
	}
	var treePayload struct {
		Data []struct {
			ID       uint `json:"id"`
			Children []struct {
				ID uint `json:"id"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(treeResponse.Body.Bytes(), &treePayload); err != nil {
		t.Fatalf("decode Organization tree: %v", err)
	}
	if len(treePayload.Data) != 1 || treePayload.Data[0].ID != rootID || len(treePayload.Data[0].Children) != 1 || treePayload.Data[0].Children[0].ID != childID {
		t.Fatalf("Organization tree = %#v, want root with child", treePayload.Data)
	}

	cycle := requestOrganization(t, application.Handler(), http.MethodPut, "/api/admin/organizations/"+itoa(childID), map[string]any{"parent_id": childID})
	if cycle.Code != http.StatusBadRequest {
		t.Fatalf("self-parent Organization status = %d, want 400", cycle.Code)
	}
	var cycleError struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(cycle.Body.Bytes(), &cycleError); err != nil {
		t.Fatalf("decode self-parent Organization error: %v", err)
	}
	if cycleError.Msg != "不能将组织挂载到自身下面" {
		t.Fatalf("self-parent Organization error = %q", cycleError.Msg)
	}
	protectedDelete := requestOrganization(t, application.Handler(), http.MethodDelete, "/api/admin/organizations/"+itoa(rootID), nil)
	if protectedDelete.Code != http.StatusBadRequest {
		t.Fatalf("parent Organization delete status = %d, want 400", protectedDelete.Code)
	}
	requestOrganization(t, application.Handler(), http.MethodDelete, "/api/admin/organizations/"+itoa(childID), nil)
	requestOrganization(t, application.Handler(), http.MethodDelete, "/api/admin/organizations/"+itoa(rootID), nil)
}

func postOrganization(t *testing.T, handler http.Handler, path string, body map[string]any) map[string]any {
	t.Helper()
	response := requestOrganization(t, handler, http.MethodPost, path, body)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d body=%s", path, response.Code, response.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode POST %s: %v", path, err)
	}
	return payload.Data
}

func requestOrganization(t *testing.T, handler http.Handler, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode %s %s: %v", method, path, err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func itoa(value uint) string {
	return fmt.Sprintf("%d", value)
}
