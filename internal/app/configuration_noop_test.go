package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"

	"gorm.io/gorm"
)

func configurationNoopRequest(t *testing.T, fixture clearingFixture, method, path string, body map[string]any, status int) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, status, response.Body.String())
	}
	var reply map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func configurationNoopFacts(t *testing.T, db *gorm.DB) map[string][]map[string]any {
	t.Helper()
	facts := make(map[string][]map[string]any)
	for _, table := range []struct{ name, order string }{
		{"users", "id"}, {"roles", "id"}, {"permissions", "id"}, {"menus", "id"}, {"apis", "id"},
		{"user_roles", "user_id,role_id"}, {"role_permissions", "role_id,permission_id"},
		{"role_menus", "role_id,menu_id"}, {"menu_apis", "menu_id,api_id"},
		{"role_data_scopes", "role_id,organization_id"}, {"user_access_versions", "user_id"},
		{"organizations", "id"}, {"user_organizations", "user_id,organization_id"},
	} {
		var rows []map[string]any
		query := db.Table(table.name).Order(table.order)
		if table.name == "users" {
			query = query.Select("id,nickname,role,status,deleted_at")
		}
		if err := query.Find(&rows).Error; err != nil {
			t.Fatalf("read %s facts: %v", table.name, err)
		}
		facts[table.name] = rows
	}
	return facts
}

func TestConfigurationNoopsPreservePersistedFactsAndMissingVersions(t *testing.T) {
	fixture := newClearingFixture(t)
	if err := fixture.db.Create(&identity.User{Model: gorm.Model{ID: 53}, Username: "configuration-never-logged", Email: "never@test.local", Password: "unused", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&authgorm.UserRole{UserID: 53, RoleID: 41}).Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, method, path string
		body               map[string]any
	}{
		{"role fields", http.MethodPut, "/api/admin/roles/41", map[string]any{"name": "Clearing role", "code": "clearing-role", "description": "keep description", "sort": 0, "status": 1, "data_scope": "self"}},
		{"permission set", http.MethodPost, "/api/admin/roles/41/permissions", map[string]any{"permission_ids": []uint{71, 71}}},
		{"data scope", http.MethodPost, "/api/admin/roles/41/data-scope", map[string]any{"data_scope": "self", "organization_ids": []uint{}}},
		{"menu set", http.MethodPost, "/api/admin/roles/41/menus", map[string]any{"menu_ids": []uint{31, 31}}},
		{"menu fields", http.MethodPut, "/api/admin/menus/31", map[string]any{"name": " Clearing menu ", "path": " /clearing ", "component": "KeepComponent", "icon": "keep-icon", "permission_code": "clearing.permission", "parent_id": 0, "sort": 0, "type": 3, "status": 1}},
		{"API binding", http.MethodPost, "/api/admin/menus/31/apis", map[string]any{"api_ids": []uint{11, 11}, "permission_code": "clearing.permission"}},
		{"all-skip sync", http.MethodPost, "/api/admin/menus/sync", map[string]any{"routes": []map[string]any{{"name": "Already configured", "path": "/clearing", "permission_code": "clearing.permission", "type": 3}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := configurationNoopFacts(t, fixture.db)
			reply := configurationNoopRequest(t, fixture, test.method, test.path, test.body, http.StatusOK)
			if after := configurationNoopFacts(t, fixture.db); !reflect.DeepEqual(before, after) {
				t.Fatalf("equivalent configuration changed persisted facts: before=%#v after=%#v", before, after)
			}
			if test.name == "all-skip sync" && reply["data"].(map[string]any)["created"] != float64(0) {
				t.Fatalf("all-skip sync response=%#v", reply)
			}
		})
	}
}

func TestMenuBindingSameInputsStillRepairRequiredOwnerFacts(t *testing.T) {
	for _, repair := range []string{"restore permission", "repair direct API code"} {
		t.Run(repair, func(t *testing.T) {
			fixture := newClearingFixture(t)
			method, path := http.MethodPost, "/api/admin/menus/31/apis"
			body := map[string]any{"api_ids": []uint{11}, "permission_code": "clearing.permission"}
			if repair == "restore permission" {
				if err := fixture.db.Exec("UPDATE permissions SET deleted_at = CURRENT_TIMESTAMP WHERE id = 71").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := fixture.db.Exec("UPDATE apis SET permission_code = 'stale.direct.code' WHERE id = 11").Error; err != nil {
					t.Fatal(err)
				}
				method, path = http.MethodPut, "/api/admin/menus/31"
				body = map[string]any{"permission_code": "clearing.permission"}
			}
			configurationNoopRequest(t, fixture, method, path, body, http.StatusOK)
			after := clearingFacts(t, fixture.db)
			if after.Permission.ID != 71 || after.Permission.DeletedAt.Valid || after.APIs[0].PermissionCode != "clearing.permission" || after.Menus[0].PermissionCode != "clearing.permission" {
				t.Fatalf("required binding repair was skipped: %#v", after)
			}
			for index, version := range after.Versions {
				if want := []int{11, 21}[index]; version.Version != want {
					t.Fatalf("repair user %d version=%d want=%d", version.UserID, version.Version, want)
				}
			}
			beforeRepeat := configurationNoopFacts(t, fixture.db)
			configurationNoopRequest(t, fixture, method, path, body, http.StatusOK)
			if afterRepeat := configurationNoopFacts(t, fixture.db); !reflect.DeepEqual(beforeRepeat, afterRepeat) {
				t.Fatalf("repaired binding did not become a no-op: before=%#v after=%#v", beforeRepeat, afterRepeat)
			}
		})
	}
}

func TestRealBindingGrantMigrationRollsBackOnSecondUserVersionFailure(t *testing.T) {
	fixture := newClearingFixture(t)
	for _, record := range []any{
		&authgorm.Role{Model: gorm.Model{ID: 42}, Name: "Existing target role", Code: "existing-target", Status: 1, DataScope: "self"},
		&authgorm.Permission{Model: gorm.Model{ID: 72}, Name: "Existing target permission", Code: "replacement.permission"},
		&authgorm.RolePermission{RoleID: 42, PermissionID: 72},
		&authgorm.UserRole{UserID: 52, RoleID: 42},
	} {
		if err := fixture.db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := configurationNoopFacts(t, fixture.db)
	if err := fixture.db.Exec("CREATE TRIGGER configuration_version_failure BEFORE UPDATE OF version ON user_access_versions WHEN NEW.user_id = 52 BEGIN SELECT RAISE(ABORT, 'configuration_version_failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"api_ids": []uint{11}, "permission_code": "replacement.permission"}
	configurationNoopRequest(t, fixture, http.MethodPost, "/api/admin/menus/31/apis", body, http.StatusInternalServerError)
	if after := configurationNoopFacts(t, fixture.db); !reflect.DeepEqual(before, after) {
		t.Fatalf("late failure retained binding/grant/version writes: before=%#v after=%#v", before, after)
	}
	if err := fixture.db.Exec("DROP TRIGGER configuration_version_failure").Error; err != nil {
		t.Fatal(err)
	}
	configurationNoopRequest(t, fixture, http.MethodPost, "/api/admin/menus/31/apis", body, http.StatusOK)
	var grants []authgorm.RolePermission
	if err := fixture.db.Order("role_id,permission_id").Find(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if want := []authgorm.RolePermission{{RoleID: 41, PermissionID: 72}, {RoleID: 42, PermissionID: 72}}; !reflect.DeepEqual(grants, want) {
		t.Fatalf("real code change did not preserve and merge grants: got=%#v want=%#v", grants, want)
	}
	after := configurationNoopFacts(t, fixture.db)
	if after["menus"][0]["permission_code"] != "replacement.permission" || after["apis"][0]["permission_code"] != "replacement.permission" {
		t.Fatalf("real code change did not reach both owners: %#v", after)
	}
}
