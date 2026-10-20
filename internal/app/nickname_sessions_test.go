package app_test

import (
	"encoding/json"
	"net/http"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
)

// This fixture signs real JWTs and checks their persisted version at the HTTP
// boundary. Original-main verification separately covers login/refresh and the
// production authentication/permission middleware.
func TestAdminNicknameHTTPPreservesOldAccessUntilRealIdentityChange(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	user := fixture.createUser("nickname-http-target")
	if err := fixture.db.Model(&user).Updates(map[string]any{"nickname": "Before", "role": "user", "status": 1}).Error; err != nil {
		t.Fatal(err)
	}
	oldAccess := fixture.token(user)
	path := "/api/admin/users/" + itoa(user.ID)
	for _, body := range []map[string]any{
		{"nickname": "After"},
		{"nickname": "After", "role": "user", "status": 1},
	} {
		response := fixture.request(http.MethodPut, path, fixture.adminToken, body)
		if response.Code != http.StatusOK {
			t.Fatalf("profile update=%d %s", response.Code, response.Body.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data["nickname"] != "After" {
			t.Fatalf("profile response=%s", response.Body.String())
		}
		for _, private := range []string{"password", "avatar_object_name", "email_verification_token"} {
			if _, exposed := envelope.Data[private]; exposed {
				t.Fatalf("profile exposed %s", private)
			}
		}
		var version authgorm.UserAccessVersion
		if err := fixture.db.First(&version, "user_id = ?", user.ID).Error; err != nil || version.Version != 1 {
			t.Fatalf("profile version=%d error=%v", version.Version, err)
		}
		access := fixture.request(http.MethodGet, "/api/user/info", oldAccess, nil)
		if access.Code != http.StatusOK {
			t.Fatalf("pure nickname rejected existing access=%d %s", access.Code, access.Body.String())
		}
	}
	response := fixture.request(http.MethodPut, path, fixture.adminToken, map[string]any{"nickname": "Mixed", "role": "legacy-editor", "status": 0})
	if response.Code != http.StatusOK {
		t.Fatalf("mixed update=%d %s", response.Code, response.Body.String())
	}
	var version authgorm.UserAccessVersion
	if err := fixture.db.First(&version, "user_id = ?", user.ID).Error; err != nil || version.Version != 2 {
		t.Fatalf("mixed update version=%d error=%v want=2", version.Version, err)
	}
	var stored identity.User
	if err := fixture.db.First(&stored, user.ID).Error; err != nil || stored.Nickname != "Mixed" || stored.Role != "legacy-editor" || stored.Status != 0 {
		t.Fatalf("mixed identity=%#v error=%v", stored, err)
	}
	access := fixture.request(http.MethodGet, "/api/user/info", oldAccess, nil)
	if access.Code != http.StatusUnauthorized {
		t.Fatalf("real identity change retained old access=%d %s", access.Code, access.Body.String())
	}
}

func TestAdminNicknameHTTPDoesNotInitializeNeverLoggedVersion(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	user := identity.User{Username: "nickname-never-logged", Password: "unused", Email: "never@test.local", Nickname: "Before", Role: "user", Status: 1}
	if err := fixture.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/users/" + itoa(user.ID)
	for _, body := range []map[string]any{
		{"nickname": "After"},
		{"nickname": "After", "role": "user", "status": 1},
	} {
		response := fixture.request(http.MethodPut, path, fixture.adminToken, body)
		if response.Code != http.StatusOK {
			t.Fatalf("never-logged update=%d %s", response.Code, response.Body.String())
		}
		var count int64
		if err := fixture.db.Model(&authgorm.UserAccessVersion{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("profile initialized version count=%d error=%v", count, err)
		}
		var stored identity.User
		if err := fixture.db.First(&stored, user.ID).Error; err != nil || stored.Nickname != "After" || stored.Role != "user" || stored.Status != 1 {
			t.Fatalf("never-logged profile=%#v error=%v", stored, err)
		}
	}
}
