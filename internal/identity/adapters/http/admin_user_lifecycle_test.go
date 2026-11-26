package httpadapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitymodule "admin/internal/identity"
	identitygorm "admin/internal/identity/adapters/gorm"
	httpadapter "admin/internal/identity/adapters/http"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type httpLifecycleAccess struct {
	db       *gorm.DB
	allScope bool
}

func (access *httpLifecycleAccess) UserScope(context.Context, uint) (domain.UserScope, error) {
	return domain.UserScope{All: access.allScope}, nil
}
func (access *httpLifecycleAccess) ManagementUserScope(ctx context.Context, userID uint) (domain.UserScope, error) {
	return access.UserScope(ctx, userID)
}
func (access *httpLifecycleAccess) IncrementVersions(ctx context.Context, ids []uint) error {
	return authgorm.NewAccessVersions(access.db).Increment(ctx, ids)
}
func (access *httpLifecycleAccess) EnsureVersions(ctx context.Context, ids []uint) (map[uint]int, error) {
	return authgorm.NewAccessVersions(access.db).EnsureMany(ctx, ids)
}
func (access *httpLifecycleAccess) IsProtectedUser(ctx context.Context, _, targetID uint) (bool, error) {
	var count int64
	err := platformdatabase.FromContext(ctx, access.db).Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", targetID, "admin").Count(&count).Error
	return count != 0, err
}
func (access *httpLifecycleAccess) UserRoles(ctx context.Context, ids []uint) ([]application.UserRoleFact, error) {
	var rows []application.UserRoleFact
	if len(ids) == 0 {
		return []application.UserRoleFact{}, nil
	}
	err := platformdatabase.FromContext(ctx, access.db).Table("roles").
		Select("user_roles.user_id AS user_id, roles.id AS id, roles.code AS code, roles.name AS name, roles.status AS status").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id IN ?", ids).Scan(&rows).Error
	return rows, err
}
func (access *httpLifecycleAccess) ClearUserRoles(ctx context.Context, userID uint) error {
	return platformdatabase.FromContext(ctx, access.db).Where("user_id = ?", userID).Delete(&authgorm.UserRole{}).Error
}

type httpLifecycleOrgs struct{ db *gorm.DB }

func (orgs httpLifecycleOrgs) UserOrganizations(context.Context, uint, []uint) ([]application.UserOrganizationFact, error) {
	return []application.UserOrganizationFact{}, nil
}
func (orgs httpLifecycleOrgs) ClearUserOrganizations(ctx context.Context, userID uint) error {
	return platformdatabase.FromContext(ctx, orgs.db).Where("user_id = ?", userID).Delete(&organization.Membership{}).Error
}

func TestUserRecoveryAndAnonymizationRouteContracts(t *testing.T) {
	routes := append(
		httpadapter.Routes(nil, nil, nil, nil, nil, 5, httpadapter.BrowserSessionConfig{Origin: "https://admin.example.test", Secure: true}),
		httpadapter.AdminUserRoutes(nil)...,
	)
	byKey := make(map[string]routecatalog.Descriptor, len(routes))
	for _, route := range routes {
		byKey[route.Method+" "+route.Path] = route
	}
	for key, wantPerm := range map[string]string{
		"GET /api/admin/deleted-users":       "admin.deleted-users.get",
		"POST /api/admin/users/:id/restore":  "admin.users.id.restore.post",
		"POST /api/admin/users/:id/anonymize": "admin.users.id.anonymize.post",
	} {
		route, ok := byKey[key]
		if !ok {
			t.Fatalf("missing route %s", key)
		}
		if route.Access != routecatalog.PermissionControlled || route.DefaultPermissionCode != wantPerm || route.Group != "identity" || route.DefaultAuditCategory != "identity" {
			t.Fatalf("route %s metadata = %#v", key, route)
		}
		if _, has404 := route.OpenAPI.Responses[http.StatusNotFound]; !has404 {
			t.Fatalf("route %s missing 404 OpenAPI response", key)
		}
	}
	anonymize := byKey["POST /api/admin/users/:id/anonymize"]
	if anonymize.OpenAPI.Request.Schema == nil {
		t.Fatal("anonymize route missing request schema")
	}
	foundMismatch := false
	for _, def := range anonymize.OpenAPI.Responses[http.StatusUnprocessableEntity].Errors {
		for _, field := range def.Fields {
			if field.Field == "confirm_username" && field.Code == "IDENTITY_CONFIRM_USERNAME_MISMATCH" {
				foundMismatch = true
			}
		}
	}
	if !foundMismatch {
		t.Fatal("anonymize route OpenAPI missing confirm_username / IDENTITY_CONFIRM_USERNAME_MISMATCH field error")
	}
}

func TestUserRecoveryAndAnonymizationHTTPHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &identitymodule.EmailVerificationCredential{}, &authgorm.Role{}, &authgorm.UserRole{}, &authgorm.UserAccessVersion{}, &organization.Unit{}, &organization.Membership{}); err != nil {
		t.Fatal(err)
	}
	repo := identitygorm.NewRepository(db)
	tx := platformdatabase.NewTransactionRunner(db)
	access := &httpLifecycleAccess{db: db, allScope: true}
	orgs := httpLifecycleOrgs{db: db}
	adminUsers := application.NewAdminUserService(repo, access, orgs, tx)
	users := application.NewUserService(repo, createUserTestHasher{}, tx, access)
	users.ConfigureLifecycle(access, orgs, nil, adminUsers, nil)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("userID", uint(1000))
		c.Next()
	})
	allRoutes := append(
		httpadapter.Routes(nil, users, nil, nil, nil, 5, httpadapter.BrowserSessionConfig{Origin: "https://admin.example.test", Secure: true}),
		httpadapter.AdminUserRoutes(adminUsers)...,
	)
	for _, route := range allRoutes {
		engine.Handle(route.Method, route.Path, route.Handler)
	}

	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		engine.ServeHTTP(rec, req)
		return rec
	}

	target := domain.User{Username: "alice_del", Email: "alice_del@example.test", Nickname: "Alice", Password: "hash", Role: "user", Status: 1}
	if err := repo.Create(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if err := users.Delete(context.Background(), 1000, target.ID); err != nil {
		t.Fatal(err)
	}

	// Non-all scope rejected with 403.
	access.allScope = false
	if rec := call(http.MethodGet, "/api/admin/deleted-users", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("non-all listDeleted status = %d body=%s", rec.Code, rec.Body.String())
	}
	access.allScope = true

	// List deleted returns target.
	rec := call(http.MethodGet, "/api/admin/deleted-users?keyword=alice_del", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"alice_del"`) {
		t.Fatalf("listDeleted status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Missing user returns 404 IDENTITY_USER_NOT_FOUND.
	rec = call(http.MethodPost, "/api/admin/users/99999/restore", "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"IDENTITY_USER_NOT_FOUND"`) {
		t.Fatalf("restore missing status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Restore succeeds and returns AdminUser.
	rec = call(http.MethodPost, "/api/admin/users/1/restore", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"alice_del"`) {
		t.Fatalf("restore status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Anonymize active user returns 409 IDENTITY_CONFLICT.
	rec = call(http.MethodPost, "/api/admin/users/1/anonymize", `{"confirm_username":"alice_del"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"IDENTITY_CONFLICT"`) {
		t.Fatalf("anonymize active status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Soft-delete again, test anonymize validation & success.
	if err := users.Delete(context.Background(), 1000, target.ID); err != nil {
		t.Fatal(err)
	}
	rec = call(http.MethodPost, "/api/admin/users/1/anonymize", `{}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"HTTP_REQUEST_INVALID"`) {
		t.Fatalf("anonymize empty confirm_username status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/admin/users/1/anonymize", `{"confirm_username":"wrong"}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"IDENTITY_CONFIRM_USERNAME_MISMATCH"`) {
		t.Fatalf("anonymize mismatch status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/admin/users/1/anonymize", `{"confirm_username":"alice_del"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymize success status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Deleted list is now empty (anonymized tombstone excluded).
	rec = call(http.MethodGet, "/api/admin/deleted-users", "")
	var payload struct {
		Data application.DeletedAdminUserPage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Total != 0 || len(payload.Data.List) != 0 {
		t.Fatalf("deleted list after anonymize = %#v, want empty", payload.Data)
	}
}
