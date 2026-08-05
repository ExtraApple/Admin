package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin/testsupport/testutil"
	apigorm "admin/internal/apimetadata/adapters/gorm"
	apihttp "admin/internal/apimetadata/adapters/http"
	apiapplication "admin/internal/apimetadata/application"
	apidomain "admin/internal/apimetadata/domain"
	"admin/internal/app"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestRegisterHTTPEnforcesStaticAccessBeforeDynamicPermissionPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&apigorm.API{}); err != nil {
		t.Fatalf("migrate API metadata: %v", err)
	}
	createAccessPolicyMetadata(t, db, "/api/admin/open", 0, "")
	createAccessPolicyMetadata(t, db, "/api/admin/locked", 1, "test.read")
	createAccessPolicyMetadata(t, db, "/api/admin/invalid-policy", 2, "test.read")

	descriptors := []routecatalog.Descriptor{
		accessPolicyDescriptor("/api/public", routecatalog.Public),
		accessPolicyDescriptor("/api/user/profile", routecatalog.Authenticated),
		accessPolicyDescriptor("/api/admin/open", routecatalog.PermissionControlled),
		accessPolicyDescriptor("/api/admin/locked", routecatalog.PermissionControlled),
		accessPolicyDescriptor("/api/admin/invalid-policy", routecatalog.PermissionControlled),
		accessPolicyDescriptor("/api/admin/apis/sync", routecatalog.PermissionControlled),
		accessPolicyDescriptor("/api/admin/apis/sync-permissions", routecatalog.PermissionControlled),
		accessPolicyDescriptor("/api/admin/permissions/sync", routecatalog.PermissionControlled),
	}
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	engine := gin.New()
	app.RegisterHTTP(engine, catalog, app.HTTPMiddleware{
		Authenticated: func(c *gin.Context) {
			identity := c.GetHeader("X-Test-Identity")
			if identity == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing identity"})
				return
			}
			c.Set("userID", uint(1))
			c.Set("roles", strings.Fields(c.GetHeader("X-Test-Roles")))
			c.Set("permissions", strings.Fields(c.GetHeader("X-Test-Permissions")))
			c.Next()
		},
		PermissionControlled: apihttp.PermissionMiddleware(apiapplication.NewCore(apigorm.NewRepository(db))),
	})

	tests := []struct {
		name        string
		path        string
		identity    string
		roles       string
		permissions string
		wantStatus  int
	}{
		{name: "Public allows anonymous", path: "/api/public", wantStatus: http.StatusNoContent},
		{name: "Authenticated rejects anonymous", path: "/api/user/profile", wantStatus: http.StatusUnauthorized},
		{name: "Authenticated accepts identity", path: "/api/user/profile", identity: "user", wantStatus: http.StatusNoContent},
		{name: "need_auth zero still rejects anonymous", path: "/api/admin/open", wantStatus: http.StatusUnauthorized},
		{name: "need_auth zero skips only permission code", path: "/api/admin/open", identity: "user", wantStatus: http.StatusNoContent},
		{name: "protected metadata requires permission", path: "/api/admin/locked", identity: "user", wantStatus: http.StatusForbidden},
		{name: "matching permission enters handler", path: "/api/admin/locked", identity: "user", permissions: "test.read", wantStatus: http.StatusNoContent},
		{name: "invalid need_auth fails closed", path: "/api/admin/invalid-policy", identity: "user", wantStatus: http.StatusForbidden},
		{name: "anonymous cannot use API sync bootstrap", path: "/api/admin/apis/sync", roles: "admin", wantStatus: http.StatusUnauthorized},
		{name: "admin can bootstrap API sync", path: "/api/admin/apis/sync", identity: "admin", roles: "admin", wantStatus: http.StatusNoContent},
		{name: "admin can bootstrap API permission sync", path: "/api/admin/apis/sync-permissions", identity: "admin", roles: "admin", wantStatus: http.StatusNoContent},
		{name: "ordinary user cannot bootstrap API sync", path: "/api/admin/apis/sync", identity: "user", roles: "user", wantStatus: http.StatusForbidden},
		{name: "RBAC sync has no bootstrap exception", path: "/api/admin/permissions/sync", identity: "admin", roles: "admin", wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request.Header.Set("X-Test-Identity", test.identity)
			request.Header.Set("X-Test-Roles", test.roles)
			request.Header.Set("X-Test-Permissions", test.permissions)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestPermissionMiddlewareRejectsDisabledBootstrapMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/admin/apis/sync",
		func(c *gin.Context) {
			c.Set("roles", []string{"admin"})
			c.Next()
		},
		apihttp.PermissionMiddleware(disabledBootstrapPolicyReader{}),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/apis/sync", nil)
	request.Header.Set("X-Test-Identity", "admin")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("disabled bootstrap status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
}

type disabledBootstrapPolicyReader struct{}

func (disabledBootstrapPolicyReader) Policy(context.Context, string, string) (apidomain.Policy, error) {
	return apidomain.Policy{Status: 0, NeedAuth: 1, PermissionCode: "admin.apis.sync"}, nil
}

func createAccessPolicyMetadata(t *testing.T, db *gorm.DB, path string, needAuth int, permissionCode string) {
	t.Helper()
	metadata := apigorm.API{
		Name: "access policy", Method: http.MethodPost, Path: path,
		Status: 1, NeedAuth: needAuth, PermissionCode: permissionCode,
	}
	if err := db.Create(&metadata).Error; err != nil {
		t.Fatalf("create API metadata for %s: %v", path, err)
	}
	if needAuth == 0 {
		if err := db.Model(&metadata).UpdateColumn("need_auth", 0).Error; err != nil {
			t.Fatalf("set public API metadata for %s: %v", path, err)
		}
	}
}

func accessPolicyDescriptor(path string, access routecatalog.AccessLevel) routecatalog.Descriptor {
	descriptor := routecatalog.Descriptor{
		Method: http.MethodPost, Path: path, Access: access,
		Handler: func(c *gin.Context) { c.Status(http.StatusNoContent) },
		Name:    path, Group: "test", DefaultAuditCategory: "test",
		OpenAPI: routecatalog.Operation{
			Summary: path,
			Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				http.StatusNoContent: {Description: "success", Kind: routecatalog.NoBody},
			},
		},
	}
	if access == routecatalog.PermissionControlled {
		descriptor.DefaultPermissionCode = "test.read"
	}
	return descriptor
}
