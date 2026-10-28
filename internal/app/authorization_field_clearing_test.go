package app_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
	"admin/internal/navigation"

	"gorm.io/gorm"
)

type clearingFixture struct {
	handler http.Handler
	db      *gorm.DB
}

func newClearingFixture(t *testing.T) clearingFixture {
	t.Helper()
	application, db := newAuthorizationApp(t)
	taken, clearingPath := "/taken", "/clearing"
	for _, records := range []any{
		&[]identity.User{
			{Model: gorm.Model{ID: 51}, Username: "clearing-first", Email: "first@test.local", Password: "unused", Status: 1},
			{Model: gorm.Model{ID: 52}, Username: "clearing-second", Email: "second@test.local", Password: "unused", Status: 1},
		},
		&authgorm.Role{Model: gorm.Model{ID: 41}, Name: "Clearing role", Code: "clearing-role", Description: "keep description", Status: 1, DataScope: "self"},
		&authgorm.Permission{Model: gorm.Model{ID: 71}, Name: "Clearing permission", Code: "clearing.permission", Group: "keep group"},
		&[]navigation.MenuModel{
			{Model: gorm.Model{ID: 31}, Name: "Clearing menu", Icon: "keep-icon", PermissionCode: "clearing.permission", Type: 3, Status: 1},
			{Model: gorm.Model{ID: 32}, Name: "Taken path", Path: &taken, Type: 2, Status: 1},
			{Model: gorm.Model{ID: 33}, Name: "Clearing page", Path: &clearingPath, Type: 2, Status: 1},
		},
		&apigorm.API{Model: gorm.Model{ID: 11}, Name: "Linked API", Method: "GET", Path: "/api/clearing", PermissionCode: "clearing.permission", NeedAuth: 1, Status: 1},
		&[]authgorm.UserRole{{UserID: 51, RoleID: 41}, {UserID: 52, RoleID: 41}},
		&authgorm.RolePermission{RoleID: 41, PermissionID: 71},
		&navigation.RoleMenuModel{RoleID: 41, MenuID: 31},
		&navigation.MenuAPIModel{MenuID: 31, APIID: 11},
		&[]authgorm.UserAccessVersion{{UserID: 51, Version: 10}, {UserID: 52, Version: 20}},
	} {
		if err := db.Create(records).Error; err != nil {
			t.Fatalf("create clearing fixture %T: %v", records, err)
		}
	}
	return clearingFixture{handler: application.Handler(), db: db}
}

func newClearingPageFixture(t *testing.T) clearingFixture {
	t.Helper()
	fixture := newClearingFixture(t)
	if err := fixture.db.Unscoped().Delete(&navigation.MenuModel{}, 33).Error; err != nil {
		t.Fatalf("remove duplicate clearing page fixture: %v", err)
	}
	path, component := "/clearing", "KeepComponent"
	if err := fixture.db.Model(&navigation.MenuModel{}).Where("id = ?", 31).Updates(map[string]any{"path": path, "component": component, "type": 2}).Error; err != nil {
		t.Fatalf("configure clearing page fixture: %v", err)
	}
	if err := fixture.db.Where("menu_id = ?", 31).Delete(&navigation.MenuAPIModel{}).Error; err != nil {
		t.Fatalf("detach clearing page APIs: %v", err)
	}
	return fixture
}

func clearingRequest(t *testing.T, fixture clearingFixture, path string, body map[string]any, status int) map[string]any {
	t.Helper()
	if strings.HasPrefix(path, "/api/admin/menus/") {
		candidate := map[string]any{"menu_id": uint(31)}
		operation := "update"
		if status == http.StatusOK || status >= http.StatusInternalServerError {
			if strings.HasSuffix(path, "/apis") {
				operation = "bind-apis"
				candidate["api_ids"] = body["api_ids"]
				candidate["permission_code"] = body["permission_code"]
			} else {
				for key, value := range body {
					candidate[key] = value
				}
			}
		} else if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			candidate = nil
		}
		if candidate != nil {
			body["expected_fingerprint"] = menuImpactFingerprint(t, fixture.handler, operation, candidate)
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("PUT %s status=%d want=%d body=%s", path, response.Code, status, response.Body.String())
	}
	var reply struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK && reply.Data != nil {
		t.Fatalf("rejected update returned success data: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "clearing_version_failure") {
		t.Fatalf("response exposed storage failure: %s", response.Body.String())
	}
	return reply.Data
}

func clearingErrorCodeRequest(t *testing.T, fixture clearingFixture, path string, body map[string]any, status int) string {
	t.Helper()
	body["expected_fingerprint"] = menuImpactFingerprint(t, fixture.handler, "update", map[string]any{"menu_id": uint(31)})
	encoded, err := json.Marshal(body)
	if err != nil {
	}
	request := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("PUT %s status=%d want=%d body=%s", path, response.Code, status, response.Body.String())
	}
	var reply struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	return reply.ErrorCode
}

type clearingState struct {
	Role            authgorm.Role
	Permission      authgorm.Permission
	Menus           []navigation.MenuModel
	APIs            []apigorm.API
	Versions        []authgorm.UserAccessVersion
	UserRoles       []authgorm.UserRole
	RolePermissions []authgorm.RolePermission
	RoleMenus       []navigation.RoleMenuModel
	MenuAPIs        []navigation.MenuAPIModel
}

func clearingFacts(t *testing.T, db *gorm.DB) clearingState {
	t.Helper()
	var facts clearingState
	for _, query := range []struct {
		name string
		run  func() error
	}{
		{"role", func() error { return db.First(&facts.Role, 41).Error }},
		{"permission", func() error { return db.First(&facts.Permission, 71).Error }},
		{"menus", func() error { return db.Order("id").Find(&facts.Menus).Error }},
		{"APIs", func() error { return db.Order("id").Find(&facts.APIs).Error }},
		{"versions", func() error { return db.Order("user_id").Find(&facts.Versions).Error }},
		{"user roles", func() error { return db.Order("user_id, role_id").Find(&facts.UserRoles).Error }},
		{"role permissions", func() error { return db.Order("role_id, permission_id").Find(&facts.RolePermissions).Error }},
		{"role menus", func() error { return db.Order("role_id, menu_id").Find(&facts.RoleMenus).Error }},
		{"menu APIs", func() error { return db.Order("menu_id, api_id").Find(&facts.MenuAPIs).Error }},
	} {
		if err := query.run(); err != nil {
			t.Fatalf("read %s: %v", query.name, err)
		}
	}
	return facts
}

func TestAuthorizationOptionalFieldsSeparateOmissionEmptyAndReplacement(t *testing.T) {
	for _, field := range []struct {
		name, route, table, column, initial, replacement string
		id                                               uint
		invalidates                                      bool
	}{
		{"role description", "roles", "roles", "description", "keep description", "new description", 41, true},
		{"permission group", "permissions", "permissions", "group", "keep group", "new group", 71, false},
		{"menu icon", "menus", "menus", "icon", "keep-icon", "new-icon", 31, true},
		{"menu component", "menus", "menus", "component", "KeepComponent", "NewComponent", 31, true},
	} {
		t.Run(field.name, func(t *testing.T) {
			fixture := newClearingFixture(t)
			if field.route == "menus" {
				fixture = newClearingPageFixture(t)
			}
			before := clearingFacts(t, fixture.db)
			path := fmt.Sprintf("/api/admin/%s/%d", field.route, field.id)
			for index, step := range []struct {
				name string
				body map[string]any
				want string
			}{
				{"omitted", map[string]any{"sort": 1}, field.initial},
				{"explicit empty alone", map[string]any{field.column: ""}, ""},
				{"replacement", map[string]any{field.column: field.replacement}, field.replacement},
				{"null retains existing parsing", map[string]any{field.column: nil, "sort": 2}, field.replacement},
			} {
				t.Run(step.name, func(t *testing.T) {
					data := clearingRequest(t, fixture, path, step.body, http.StatusOK)
					var stored struct{ Value sql.NullString }
					if err := fixture.db.Raw("SELECT `"+field.column+"` AS value FROM `"+field.table+"` WHERE id = ?", field.id).Scan(&stored).Error; err != nil {
						t.Fatal(err)
					}
					if stored.Value.String != step.want || data[field.column] != step.want {
						t.Fatalf("field value storage=%#v response=%#v want=%q", stored, data[field.column], step.want)
					}
					if field.column == "path" && step.want == "" && stored.Value.Valid {
						t.Fatalf("cleared path must use existing NULL normalization: %#v", stored)
					}
					after := clearingFacts(t, fixture.db)
					if after.Permission.ID != before.Permission.ID || after.Permission.Code != before.Permission.Code || after.Role.Code != before.Role.Code || after.Role.Name != before.Role.Name || after.Permission.Name != before.Permission.Name {
						t.Fatalf("optional update changed resource identity: %#v", after)
					}
					if !reflect.DeepEqual(before.APIs, after.APIs) || !reflect.DeepEqual(before.UserRoles, after.UserRoles) || !reflect.DeepEqual(before.RolePermissions, after.RolePermissions) || !reflect.DeepEqual(before.RoleMenus, after.RoleMenus) || !reflect.DeepEqual(before.MenuAPIs, after.MenuAPIs) {
						t.Fatalf("optional update changed unrelated API or relation facts: before=%#v after=%#v", before, after)
					}
					for position, version := range after.Versions {
						want := before.Versions[position].Version
						if field.invalidates {
							want += index + 1
						}
						if version.Version != want {
							t.Fatalf("user %d version=%d want=%d", version.UserID, version.Version, want)
						}
					}
				})
			}
		})
	}
}

func TestAuthorizationInvalidClearingRejectsEverySubmittedField(t *testing.T) {
	for _, test := range []struct {
		name, path string
		body       map[string]any
		status     int
	}{
		{"empty role name", "/api/admin/roles/41", map[string]any{"name": "", "description": ""}, 400},
		{"empty role code", "/api/admin/roles/41", map[string]any{"code": "", "description": ""}, 400},
		{"blank role name", "/api/admin/roles/41", map[string]any{"name": " \t", "description": ""}, 422},
		{"blank role code", "/api/admin/roles/41", map[string]any{"code": " \t", "description": ""}, 422},
		{"empty permission name", "/api/admin/permissions/71", map[string]any{"name": "", "group": ""}, 400},
		{"blank permission name", "/api/admin/permissions/71", map[string]any{"name": " \t", "group": ""}, 422},
		{"empty menu name", "/api/admin/menus/31", map[string]any{"name": "", "icon": ""}, 400},
		{"blank menu name", "/api/admin/menus/31", map[string]any{"name": " \t", "icon": ""}, 422},
		{"path on button menu", "/api/admin/menus/31", map[string]any{"path": "/button-path", "icon": ""}, 422},
		{"no role fields", "/api/admin/roles/41", map[string]any{}, 422},
		{"no permission fields", "/api/admin/permissions/71", map[string]any{}, 422},
		{"no menu fields", "/api/admin/menus/31", map[string]any{}, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newClearingFixture(t)
			before := clearingFacts(t, fixture.db)
			if test.name == "path on button menu" {
				if code := clearingErrorCodeRequest(t, fixture, test.path, test.body, test.status); code != "NAV_VALIDATION_INVALID" {
					t.Fatalf("error code=%q, want NAV_VALIDATION_INVALID", code)
				}
			} else {
				clearingRequest(t, fixture, test.path, test.body, test.status)
			}
			if after := clearingFacts(t, fixture.db); !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected update changed facts: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestAuthorizationClearingRollsBackEarlierVersionIncrements(t *testing.T) {
	for _, test := range []struct {
		name, path string
		body       map[string]any
	}{
		{"role description", "/api/admin/roles/41", map[string]any{"description": ""}},
		{"menu display and path", "/api/admin/menus/31", map[string]any{"icon": "", "component": "", "path": ""}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newClearingFixture(t)
			before := clearingFacts(t, fixture.db)
			if err := fixture.db.Exec("CREATE TRIGGER clearing_version_failure BEFORE UPDATE OF version ON user_access_versions WHEN NEW.user_id = 52 BEGIN SELECT RAISE(ABORT, 'clearing_version_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			clearingRequest(t, fixture, test.path, test.body, http.StatusInternalServerError)
			if after := clearingFacts(t, fixture.db); !reflect.DeepEqual(before, after) {
				t.Fatalf("later version failure retained field/version changes: before=%#v after=%#v", before, after)
			}
		})
	}
}
