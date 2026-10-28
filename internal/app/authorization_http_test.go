package app_test

import (
	"bytes"
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

func impactFingerprint(t *testing.T, handler http.Handler, path, operation string, candidate any) string {
	t.Helper()
	return impactFingerprintWithHeaders(t, handler, path, operation, candidate, nil)
}

func impactFingerprintWithHeaders(t *testing.T, handler http.Handler, path, operation string, candidate any, headers map[string]string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"operation": operation, "candidate": candidate})
	if err != nil {
		t.Fatalf("encode %s impact preview: %v", operation, err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("%s impact preview status=%d body=%s", operation, response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Fingerprint string `json:"preview_fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s impact preview: %v", operation, err)
	}
	if len(envelope.Data.Fingerprint) != 64 {
		t.Fatalf("%s impact preview fingerprint=%q", operation, envelope.Data.Fingerprint)
	}
	return envelope.Data.Fingerprint
}

func menuImpactFingerprint(t *testing.T, handler http.Handler, operation string, candidate any) string {
	t.Helper()
	return impactFingerprint(t, handler, "/api/admin/menus/impact-preview", operation, candidate)
}

func apiImpactFingerprint(t *testing.T, handler http.Handler, operation string, candidate any) string {
	t.Helper()
	return impactFingerprint(t, handler, "/api/admin/apis/impact-preview", operation, candidate)
}

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
	if codes["user.avatar-upload-policy.get"] {
		t.Fatal("authenticated-only avatar policy unexpectedly created a permission")
	}
}

func TestAuthorizationHTTPMenuUpdateKeepsBoundAPIsAndRoleGrantsConsistent(t *testing.T) {
	for _, test := range []struct {
		name          string
		failAPIUpdate bool
	}{
		{name: "commits menu API and merged grants"},
		{name: "API failure rolls back ordinary fields and linked facts", failAPIUpdate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// This fixture supplies test authentication and bypasses permission checks;
			// these assertions cover App writes and reads, not production ACL enforcement.
			fixture := newOrganizationJWTFixture(t)
			create := func(path string, body map[string]any) map[string]any {
				t.Helper()
				if path == "/api/admin/menus" {
					body["expected_fingerprint"] = impactFingerprintWithHeaders(t, fixture.application.Handler(), "/api/admin/menus/impact-preview", "create", body, map[string]string{"Authorization": "Bearer " + fixture.adminToken})
				}
				data := fixture.post(path, body)
				fixture.adminToken = fixture.token(fixture.admin)
				return data
			}
			decodeData := func(response *httptest.ResponseRecorder, target any) {
				t.Helper()
				if response.Code != http.StatusOK {
					t.Fatalf("HTTP status = %d body=%s", response.Code, response.Body.String())
				}
				var payload struct {
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode response envelope: %v", err)
				}
				if err := json.Unmarshal(payload.Data, target); err != nil {
					t.Fatalf("decode response data: %v", err)
				}
			}
			read := func(path string, target any) {
				t.Helper()
				decodeData(fixture.request(http.MethodGet, path, fixture.adminToken, nil), target)
			}

			const oldCode, newCode = "menu-linked.before", "menu-linked.after"
			oldPermission := create("/api/admin/permissions", map[string]any{"name": "Before", "code": oldCode})
			newPermission := create("/api/admin/permissions", map[string]any{"name": "After", "code": newCode})
			oldPermissionID := uint(oldPermission["id"].(float64))
			newPermissionID := uint(newPermission["id"].(float64))
			menu := create("/api/admin/menus", map[string]any{
				"name": "Before", "type": 3, "permission_code": oldCode, "sort": 4,
			})
			menuID := uint(menu["id"].(float64))
			menuPath := "/api/admin/menus/" + itoa(menuID)
			var apiIDs []uint
			for _, path := range []string{"/api/menu-linked/first", "/api/menu-linked/second"} {
				api := create("/api/admin/apis", map[string]any{
					"name": "Linked API", "method": http.MethodGet, "path": path,
					"permission_code": oldCode, "status": 1, "need_auth": 1,
				})
				apiIDs = append(apiIDs, uint(api["id"].(float64)))
			}
			bindBody := map[string]any{"api_ids": apiIDs, "permission_code": oldCode}
			bindBody["expected_fingerprint"] = impactFingerprintWithHeaders(t, fixture.application.Handler(), "/api/admin/menus/impact-preview", "bind-apis", map[string]any{"menu_id": menuID, "api_ids": apiIDs, "permission_code": oldCode}, map[string]string{"Authorization": "Bearer " + fixture.adminToken})
			assigned := fixture.request(http.MethodPost, menuPath+"/apis", fixture.adminToken, bindBody)
			if assigned.Code != http.StatusOK {
				t.Fatalf("bind menu APIs status=%d body=%s", assigned.Code, assigned.Body.String())
			}
			fixture.adminToken = fixture.token(fixture.admin)

			roles := []authgorm.Role{
				{Name: "Old grant only", Code: "menu-old-only", Status: 1},
				{Name: "Overlapping grants", Code: "menu-overlap", Status: 1},
				{Name: "Destination grant only", Code: "menu-new-only", Status: 1},
			}
			if err := fixture.db.Create(&roles).Error; err != nil {
				t.Fatalf("create grant roles: %v", err)
			}
			grants := []authgorm.RolePermission{
				{RoleID: roles[0].ID, PermissionID: oldPermissionID},
				{RoleID: roles[1].ID, PermissionID: oldPermissionID},
				{RoleID: roles[1].ID, PermissionID: newPermissionID},
				{RoleID: roles[2].ID, PermissionID: newPermissionID},
			}
			if err := fixture.db.Create(&grants).Error; err != nil {
				t.Fatalf("create preexisting permission grants: %v", err)
			}
			if test.failAPIUpdate {
				if err := fixture.db.Exec(`CREATE TRIGGER reject_menu_linked_code
					BEFORE UPDATE OF permission_code ON apis
					WHEN NEW.permission_code = 'menu-linked.after'
					BEGIN SELECT RAISE(ABORT, 'API permission update rejected'); END`).Error; err != nil {
					t.Fatalf("install API update failure: %v", err)
				}
			}

			updateBody := map[string]any{"name": "After", "sort": 19, "permission_code": newCode}
			updateBody["expected_fingerprint"] = impactFingerprintWithHeaders(t, fixture.application.Handler(), "/api/admin/menus/impact-preview", "update", map[string]any{"menu_id": menuID, "name": "After", "sort": 19, "permission_code": newCode}, map[string]string{"Authorization": "Bearer " + fixture.adminToken})
			response := fixture.request(http.MethodPut, menuPath, fixture.adminToken, updateBody)
			type menuState struct {
				ID             uint   `json:"id"`
				Name           string `json:"name"`
				Sort           int    `json:"sort"`
				PermissionCode string `json:"permission_code"`
			}
			wantMenu := menuState{ID: menuID, Name: "After", Sort: 19, PermissionCode: newCode}
			if test.failAPIUpdate {
				if response.Code != http.StatusInternalServerError {
					t.Fatalf("rejected menu update status=%d body=%s", response.Code, response.Body.String())
				}
				wantMenu = menuState{ID: menuID, Name: "Before", Sort: 4, PermissionCode: oldCode}
			} else {
				var returned menuState
				decodeData(response, &returned)
				if returned != wantMenu {
					t.Fatalf("menu PUT response = %#v, want %#v", returned, wantMenu)
				}
			}
			fixture.adminToken = fixture.token(fixture.admin)

			var menus []menuState
			read("/api/admin/menus", &menus)
			if len(menus) != 1 || menus[0] != wantMenu {
				t.Fatalf("persisted menus = %#v, want [%#v]", menus, wantMenu)
			}
			type apiState struct {
				ID             uint   `json:"id"`
				PermissionCode string `json:"permission_code"`
			}
			var bound []apiState
			read(menuPath+"/apis", &bound)
			if len(bound) != len(apiIDs) {
				t.Fatalf("bound APIs = %#v, want IDs %v", bound, apiIDs)
			}
			seen := make(map[uint]bool, len(bound))
			for _, api := range bound {
				if seen[api.ID] || api.PermissionCode != wantMenu.PermissionCode {
					t.Fatalf("bound APIs = %#v, want unique IDs %v with code %q", bound, apiIDs, wantMenu.PermissionCode)
				}
				seen[api.ID] = true
			}
			for _, id := range apiIDs {
				if !seen[id] {
					t.Fatalf("binding lost API %d: %#v", id, bound)
				}
				var persisted apiState
				read("/api/admin/apis/"+itoa(id), &persisted)
				if persisted.ID != id || persisted.PermissionCode != wantMenu.PermissionCode {
					t.Fatalf("persisted API = %#v, want ID %d code %q", persisted, id, wantMenu.PermissionCode)
				}
			}

			type permissionState struct {
				ID   uint   `json:"id"`
				Code string `json:"code"`
			}
			for index, role := range roles {
				var permissions []permissionState
				read("/api/admin/roles/"+itoa(role.ID)+"/permissions", &permissions)
				want := map[uint]string{newPermissionID: newCode}
				if test.failAPIUpdate {
					switch index {
					case 0:
						want = map[uint]string{oldPermissionID: oldCode}
					case 1:
						want[oldPermissionID] = oldCode
					}
				}
				if len(permissions) != len(want) {
					t.Fatalf("role %q permissions = %#v, want %v", role.Code, permissions, want)
				}
				for _, permission := range permissions {
					if code, ok := want[permission.ID]; !ok || code != permission.Code {
						t.Fatalf("role %q permissions = %#v, want %v", role.Code, permissions, want)
					}
					delete(want, permission.ID)
				}
				if len(want) != 0 {
					t.Fatalf("role %q missing permissions %v", role.Code, want)
				}
			}
			var permissions struct {
				List []permissionState `json:"list"`
			}
			read("/api/admin/permissions?size=100", &permissions)
			wantPermissions := map[uint]string{newPermissionID: newCode}
			if test.failAPIUpdate {
				wantPermissions[oldPermissionID] = oldCode
			}
			if len(permissions.List) != len(wantPermissions) {
				t.Fatalf("persisted permissions = %#v, want %v", permissions.List, wantPermissions)
			}
			for _, permission := range permissions.List {
				if code, ok := wantPermissions[permission.ID]; !ok || code != permission.Code {
					t.Fatalf("persisted permissions = %#v, want %v", permissions.List, wantPermissions)
				}
				delete(wantPermissions, permission.ID)
			}
			if len(wantPermissions) != 0 {
				t.Fatalf("missing persisted permissions %v", wantPermissions)
			}
		})
	}
}
