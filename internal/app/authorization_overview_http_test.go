package app_test

import (
	"encoding/json"
	"net/http"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	authdomain "admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
)

func TestAuthorizationOverviewHTTPReturnsScopedReadOnlyData(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	fixture.permissions = append(fixture.permissions, "admin.authorization.overview.get", "admin.authorization.risks.get")
	operator := identitydomain.User{Username: "overview-operator", Email: "overview-operator@example.test", Password: "private", Status: 1}
	unassigned := identitydomain.User{Username: "overview-unassigned", Email: "overview-unassigned@example.test", Password: "private", Status: 1}
	if err := fixture.db.Create(&[]*identitydomain.User{&operator, &unassigned}).Error; err != nil {
		t.Fatalf("create overview users: %v", err)
	}
	role := authdomain.Role{Name: "Overview Admin", Code: "overview-admin", Status: 1, DataScope: authdomain.DataScopeAll}
	if err := fixture.db.Create(&role).Error; err != nil {
		t.Fatalf("create overview admin role: %v", err)
	}
	if err := fixture.db.Create(&authgorm.UserRole{UserID: operator.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("assign overview admin role: %v", err)
	}
	fixture.operatorID = operator.ID
	beforeRoles := int64(0)
	if err := fixture.db.Model(&authgorm.Role{}).Count(&beforeRoles).Error; err != nil {
		t.Fatalf("count roles before overview: %v", err)
	}
	response := fixture.request(http.MethodGet, "/api/admin/authorization-overview", "", true)
	data := workbenchEnvelope(t, response, http.StatusOK, "")
	var overview struct {
		Scope struct {
			DataScope         string `json:"data_scope"`
			OrganizationCount int    `json:"organization_count"`
		} `json:"scope"`
		Summary struct {
			Users struct {
				Total       int `json:"total"`
				WithoutRole int `json:"without_role"`
			} `json:"users"`
		} `json:"summary"`
		Risks struct {
			Items []json.RawMessage `json:"items"`
			Total int64             `json:"total"`
		} `json:"risks"`
	}
	if err := json.Unmarshal(data, &overview); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if overview.Scope.DataScope != "all" || overview.Summary.Users.Total < 2 || overview.Summary.Users.WithoutRole < 1 || overview.Risks.Total < 1 || overview.Risks.Items == nil {
		t.Fatalf("overview = %#v", overview)
	}
	var afterRoles int64
	if err := fixture.db.Model(&authgorm.Role{}).Count(&afterRoles).Error; err != nil {
		t.Fatalf("count roles after overview: %v", err)
	}
	if beforeRoles != afterRoles {
		t.Fatalf("overview changed role count from %d to %d", beforeRoles, afterRoles)
	}
	response = fixture.request(http.MethodGet, "/api/admin/authorization-risks?page=1&size=10", "", true)
	data = workbenchEnvelope(t, response, http.StatusOK, "")
	var risks struct {
		List []json.RawMessage `json:"list"`
		Page int               `json:"page"`
		Size int               `json:"size"`
	}
	if err := json.Unmarshal(data, &risks); err != nil {
		t.Fatalf("decode risk page: %v", err)
	}
	if risks.List == nil || risks.Page != 1 || risks.Size != 10 {
		t.Fatalf("risk page = %#v", risks)
	}
}

func TestAuthorizationOverviewHTTPEnforcesAuthenticationPermissionAndQueryValidation(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/authorization-overview", "", false), http.StatusUnauthorized, "AUTHN_HEADER_MISSING")
	fixture.permissions = nil
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/authorization-overview", "", true), http.StatusForbidden, "API_META_PERMISSION_DENIED")
	fixture.permissions = []string{"admin.authorization.risks.get"}
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/authorization-risks?page=0&size=10", "", true), http.StatusUnprocessableEntity, "AUTHZ_VALIDATION_INVALID")
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/authorization-risks?page=1&size=101", "", true), http.StatusUnprocessableEntity, "AUTHZ_VALIDATION_INVALID")
	fixture.permissions = []string{"admin.authorization.overview.get"}
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/authorization-risks?page=1&size=10", "", true), http.StatusForbidden, "API_META_PERMISSION_DENIED")
}

func TestAuthorizationOverviewRoutesPublishCatalogPermissionsAndSchemas(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	want := map[string]string{
		"/api/admin/authorization-overview": "admin.authorization.overview.get",
		"/api/admin/authorization-risks":    "admin.authorization.risks.get",
	}
	for path, permission := range want {
		found := false
		for _, descriptor := range fixture.application.Catalog().Snapshot() {
			if descriptor.Method != http.MethodGet || descriptor.Path != path {
				continue
			}
			found = true
			if descriptor.DefaultPermissionCode != permission || descriptor.OpenAPI.Summary == "" || descriptor.OpenAPI.Responses[http.StatusOK].DataSchema == nil || descriptor.OpenAPI.Responses[http.StatusUnauthorized].Schema == nil || descriptor.OpenAPI.Responses[http.StatusForbidden].Schema == nil {
				t.Fatalf("authorization descriptor %s = %#v", path, descriptor)
			}
			if path == "/api/admin/authorization-risks" && descriptor.OpenAPI.QuerySchema == nil {
				t.Fatalf("risk descriptor has no query schema")
			}
		}
		if !found {
			t.Fatalf("authorization descriptor %s was not registered", path)
		}
	}
}
