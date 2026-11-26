package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	identitygorm "admin/internal/identity/adapters/gorm"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/organization"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestApplicationUserRecoveryAndAnonymizationEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	conf := testAppConfig()
	conf.Admin.Username = "superadmin"
	conf.Admin.Password = "{{SECRET_00000001}}"
	conf.Admin.Email = "superadmin@example.test"
	conf.APIDocs.Enabled = true

	// Point MinIO at an unreachable local port so post-commit avatar cleanup deterministically fails
	// and exercises degraded structured logging without leaking object keys or raw errors.
	minioClient, err := minio.New("127.0.0.1:1", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zapcore.InfoLevel)
	observedLogger := zap.New(core)

	var actorID uint
	var actorRoles []string
	var actorPermissions []string
	application, err := app.New(context.Background(), conf, app.Resources{
		DB:     db,
		Redis:  redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
		MinIO:  minioClient,
		Logger: observedLogger,
	}, app.Options{
		Middleware: app.HTTPMiddleware{Authenticated: func(c *gin.Context) {
			c.Set("userID", actorID)
			c.Set("roles", actorRoles)
			c.Set("permissions", actorPermissions)
			c.Next()
		}},
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	var admin identitygorm.UserModel
	if err := db.Where("username = ?", "superadmin").First(&admin).Error; err != nil {
		t.Fatalf("load seeded admin: %v", err)
	}
	var adminRole authgorm.Role
	if err := db.Where("code = ?", "admin").First(&adminRole).Error; err != nil {
		t.Fatalf("load seeded admin role: %v", err)
	}

	// Verify Seed synced all 3 new routes to apis, permissions, and admin role_permissions.
	seededPerms := []string{}
	if err := db.Table("permissions").
		Select("permissions.code").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", adminRole.ID).
		Pluck("permissions.code", &seededPerms).Error; err != nil {
		t.Fatalf("load seeded admin permissions: %v", err)
	}
	permSet := make(map[string]bool, len(seededPerms))
	for _, code := range seededPerms {
		permSet[code] = true
	}
	for _, route := range []struct {
		method string
		path   string
		code   string
	}{
		{http.MethodGet, "/api/admin/deleted-users", "admin.deleted-users.get"},
		{http.MethodPost, "/api/admin/users/:id/restore", "admin.users.id.restore.post"},
		{http.MethodPost, "/api/admin/users/:id/anonymize", "admin.users.id.anonymize.post"},
	} {
		if !permSet[route.code] {
			t.Fatalf("seeded admin role missing permission %q", route.code)
		}
		var apiCount int64
		if err := db.Table("apis").Where("method = ? AND path = ? AND permission_code = ?", route.method, route.path, route.code).Count(&apiCount).Error; err != nil || apiCount != 1 {
			t.Fatalf("seeded api %s %s (%s) count = %d, err = %v", route.method, route.path, route.code, apiCount, err)
		}
	}

	req := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		application.Handler().ServeHTTP(rec, request)
		return rec
	}

	// Verify OpenAPI exposes all 3 lifecycle routes and confirm_username mismatch error.
	specRec := req(http.MethodGet, "/docs/openapi.json", "")
	if specRec.Code != http.StatusOK {
		t.Fatalf("openapi status=%d body=%s", specRec.Code, specRec.Body.String())
	}
	specBody := specRec.Body.String()
	for _, needle := range []string{
		`"/api/admin/deleted-users"`,
		`"/api/admin/users/{id}/restore"`,
		`"/api/admin/users/{id}/anonymize"`,
		`"confirm_username"`,
		`"IDENTITY_CONFIRM_USERNAME_MISMATCH"`,
	} {
		if !strings.Contains(specBody, needle) {
			t.Fatalf("openapi spec missing %s", needle)
		}
	}

	// Missing permission is rejected by real DynamicPermissionMiddleware with 403 API_META_PERMISSION_DENIED.
	actorID = admin.ID
	actorRoles = []string{}
	actorPermissions = []string{}
	if rec := req(http.MethodGet, "/api/admin/deleted-users", ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"API_META_PERMISSION_DENIED"`) {
		t.Fatalf("unpermitted deleted-users status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Grant seeded permissions loaded from DB (without admin role bypass) to verify permission codes end-to-end.
	actorRoles = []string{"operator"}
	actorPermissions = seededPerms

	// 1. Admin creates user.
	createRec := req(http.MethodPost, "/api/admin/users", `{"username":"lifecycle_u1","password":"Password1!","email":"lifecycle_u1@example.test","nickname":"Lifecycle"}`)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create user status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	uid := created.Data.ID

	// Attach a role, organization membership, and a trusted custom avatar object.
	var userRole authgorm.Role
	if err := db.Where("code = ?", "user").First(&userRole).Error; err != nil {
		t.Fatal(err)
	}
	org := organization.Unit{Name: "Engineering", Code: "eng", Status: 1}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: uid, RoleID: userRole.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&organization.Membership{UserID: uid, OrganizationID: org.ID}).Error; err != nil {
		t.Fatal(err)
	}
	trustedObject := fmt.Sprintf("avatars/%d/00000000-0000-4000-8000-000000000009.png", uid)
	now := time.Now().UTC()
	if err := db.Model(&identitygorm.UserModel{}).Where("id = ?", uid).Updates(map[string]any{
		"avatar_object_name":       trustedObject,
		"avatar_content_type":      "image/png",
		"avatar_content_sha256":    strings.Repeat("d", 64),
		"avatar_validation_status": identitydomain.AvatarValidationStatusValidated,
		"avatar_validated_at":      &now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// 2. Soft-delete user -> appears in /api/admin/deleted-users; creating same username/email still returns 409.
	if rec := req(http.MethodDelete, fmt.Sprintf("/api/admin/users/%d", uid), ""); rec.Code != http.StatusOK {
		t.Fatalf("delete user status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := req(http.MethodPost, "/api/admin/users", `{"username":"lifecycle_u1","password":"Password1!","email":"other@example.test"}`); rec.Code != http.StatusConflict {
		t.Fatalf("create colliding soft-deleted username status=%d want 409", rec.Code)
	}
	delRec := req(http.MethodGet, "/api/admin/deleted-users?keyword=lifecycle_u1", "")
	if delRec.Code != http.StatusOK || !strings.Contains(delRec.Body.String(), `"Engineering"`) || strings.Contains(delRec.Body.String(), "00000000-0000-4000-8000-000000000009") {
		t.Fatalf("deleted-users status=%d body=%s", delRec.Code, delRec.Body.String())
	}

	// 3. Restore user -> returns to active list with retained role and org.
	restRec := req(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/restore", uid), "")
	if restRec.Code != http.StatusOK || !strings.Contains(restRec.Body.String(), `"Engineering"`) {
		t.Fatalf("restore user status=%d body=%s", restRec.Code, restRec.Body.String())
	}

	// 4. Soft-delete again -> anonymize with exact confirm_username -> succeeds despite unreachable MinIO,
	// logs controlled Zap error without leaking object_name or raw MinIO error, wipes memberships, retains version.
	if rec := req(http.MethodDelete, fmt.Sprintf("/api/admin/users/%d", uid), ""); rec.Code != http.StatusOK {
		t.Fatalf("re-delete user status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := req(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/anonymize", uid), `{"confirm_username":"lifecycle_u1"}`); rec.Code != http.StatusOK {
		t.Fatalf("anonymize user status=%d body=%s", rec.Code, rec.Body.String())
	}

	cleanupLogs := logs.FilterMessage("identity anonymized user avatar cleanup failed").All()
	if len(cleanupLogs) != 1 || cleanupLogs[0].Level != zapcore.ErrorLevel {
		t.Fatalf("avatar cleanup error logs = %#v, want 1 Error entry", cleanupLogs)
	}
	logFields := cleanupLogs[0].ContextMap()
	if logFields["error_code"] != "avatar_cleanup_failed" || fmt.Sprint(logFields["user_id"]) != fmt.Sprint(uid) {
		t.Fatalf("avatar cleanup log fields = %#v", logFields)
	}
	serializedLog := fmt.Sprintf("%#v", logFields)
	if strings.Contains(serializedLog, trustedObject) || strings.Contains(serializedLog, "00000000-0000-4000-8000-000000000009") || strings.Contains(serializedLog, "127.0.0.1") {
		t.Fatalf("avatar cleanup log leaked object key or raw storage error: %s", serializedLog)
	}

	var roleLinks, orgLinks int64
	_ = db.Model(&authgorm.UserRole{}).Where("user_id = ?", uid).Count(&roleLinks).Error
	_ = db.Model(&organization.Membership{}).Where("user_id = ?", uid).Count(&orgLinks).Error
	if roleLinks != 0 || orgLinks != 0 {
		t.Fatalf("memberships after anonymize = roles:%d orgs:%d, want 0/0", roleLinks, orgLinks)
	}
	var tombstone identitygorm.UserModel
	if err := db.Unscoped().First(&tombstone, uid).Error; err != nil {
		t.Fatalf("tombstone row missing: %v", err)
	}
	if tombstone.AnonymizedAt == nil || !tombstone.DeletedAt.Valid {
		t.Fatalf("tombstone timestamps invalid: %#v", tombstone)
	}
	var version authgorm.UserAccessVersion
	if err := db.Where("user_id = ?", uid).First(&version).Error; err != nil || version.Version < 4 {
		t.Fatalf("retained access version = %#v, err=%v", version, err)
	}

	// 5. Reusing original username and email now succeeds!
	recreateRec := req(http.MethodPost, "/api/admin/users", `{"username":"lifecycle_u1","password":"Password1!","email":"lifecycle_u1@example.test","nickname":"Reborn"}`)
	if recreateRec.Code != http.StatusOK {
		t.Fatalf("re-create with released username/email status=%d body=%s", recreateRec.Code, recreateRec.Body.String())
	}
}
