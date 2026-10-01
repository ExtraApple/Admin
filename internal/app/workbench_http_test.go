package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	authdomain "admin/internal/authorization/domain"
	identityhttp "admin/internal/identity/adapters/http"
	identityapplication "admin/internal/identity/application"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/navigation"
	"admin/internal/organization"
	"admin/testsupport/testutil"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type workbenchHTTPFixture struct {
	application *app.Application
	db          *gorm.DB
	operatorID  uint
	permissions []string
}

func newWorkbenchHTTPFixture(t *testing.T) *workbenchHTTPFixture {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t).Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	fixture := &workbenchHTTPFixture{db: db, operatorID: 1, permissions: []string{"admin.users.get", "admin.users.id.get", "admin.roles.id.get", "admin.users.id.roles.put", "admin.users.id.organizations.put", "admin.organizations.tree.get", "admin.roles.id.menus.get"}}
	conf := testAppConfig()
	conf.Admin.Username = "workbench-admin"
	conf.Admin.Password = "workbench-fixture-password"
	conf.Admin.Email = "workbench-admin@example.test"
	conf.APIDocs.Enabled = true
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	authenticate := func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-session" {
			identityhttp.AuthMiddleware(nil)(c)
			return
		}
		c.Set("userID", fixture.operatorID)
		c.Set("roles", []string{})
		c.Set("permissions", fixture.permissions)
		c.Next()
	}
	application, err := app.New(context.Background(), conf, app.Resources{DB: db, Redis: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Middleware: app.HTTPMiddleware{Authenticated: authenticate}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.application = application
	t.Cleanup(func() { _ = application.Close() })
	return fixture
}
func (fixture *workbenchHTTPFixture) request(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer fixture-session")
	}
	response := httptest.NewRecorder()
	fixture.application.Handler().ServeHTTP(response, request)
	return response
}
func workbenchEnvelope(t *testing.T, response *httptest.ResponseRecorder, status int, errorCode string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %d %s: %v", response.Code, response.Body.String(), err)
	}
	var code int
	var actualError string
	_ = json.Unmarshal(fields["code"], &code)
	_ = json.Unmarshal(fields["error_code"], &actualError)
	if response.Code != status || code != status || actualError != errorCode || len(fields) != 4 || fields["msg"] == nil || fields["data"] == nil {
		t.Fatalf("envelope = %d %s; want %d / %q", response.Code, response.Body.String(), status, errorCode)
	}
	return fields["data"]
}
func TestWorkbenchHTTPReadsUseScopedSafeMultiRelationProjection(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	db := fixture.db
	manager := identitydomain.User{Username: "workbench-manager", Email: "manager@example.test", Password: "private", Status: 1}
	target := identitydomain.User{Username: "target", Nickname: "运营", Email: "target@example.test", Password: "private", Role: "admin", Status: 1}
	outside := identitydomain.User{Username: "outside", Nickname: "运营外", Email: "outside@example.test", Password: "private", Status: 1}
	if err := db.Create(&[]*identitydomain.User{&manager, &target, &outside}).Error; err != nil {
		t.Fatal(err)
	}
	team := organization.Unit{Name: "可管理团队", Code: "team", Status: 1}
	hidden := organization.Unit{Name: "机密组织", Code: "hidden", Status: 1}
	if err := db.Create(&[]*organization.Unit{&team, &hidden}).Error; err != nil {
		t.Fatal(err)
	}
	managerRole := authdomain.Role{Name: "Manager", Code: "manager", Status: 1, DataScope: authdomain.DataScopeOrg}
	reader := authdomain.Role{Name: "Reader", Code: "reader", Sort: 1, Status: 1, DataScope: authdomain.DataScopeSelf}
	disabled := authdomain.Role{Name: "Disabled", Code: "disabled", Sort: 2, Status: 1, DataScope: authdomain.DataScopeSelf}
	if err := db.Create(&[]*authdomain.Role{&managerRole, &reader, &disabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&disabled).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]authgorm.UserRole{{UserID: manager.ID, RoleID: managerRole.ID}, {UserID: target.ID, RoleID: reader.ID}, {UserID: target.ID, RoleID: disabled.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]organization.Membership{{UserID: manager.ID, OrganizationID: team.ID}, {UserID: target.ID, OrganizationID: team.ID}, {UserID: target.ID, OrganizationID: hidden.ID}, {UserID: outside.ID, OrganizationID: hidden.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.operatorID = manager.ID
	response := fixture.request(http.MethodGet, "/api/admin/users?keyword="+"%E8%BF%90%E8%90%A5"+"&status=1&page=1&size=1", "", true)
	data := workbenchEnvelope(t, response, 200, "")
	var page identityapplication.AdminUserPage
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.List) != 1 || page.List[0].ID != target.ID || len(page.List[0].Roles) != 2 || page.List[0].Roles[1].Status != 0 || len(page.List[0].Organizations) != 1 || page.List[0].Organizations[0].ID != team.ID || !page.List[0].HasUnmanagedOrganizations || page.List[0].AccessVersion != 1 {
		t.Fatalf("admin page = %#v", page)
	}
	var rawPage struct {
		List []map[string]json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(data, &rawPage); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "role", "pending_email", "avatar_object_name", "deleted_at"} {
		if _, exists := rawPage.List[0][forbidden]; exists {
			t.Fatalf("admin projection exposed %s", forbidden)
		}
	}
	if strings.Contains(string(data), hidden.Name) {
		t.Fatal("hidden organization name was exposed")
	}
	response = fixture.request(http.MethodGet, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10), "", true)
	data = workbenchEnvelope(t, response, 200, "")
	var detail identityapplication.AdminUser
	if err := json.Unmarshal(data, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != target.ID || detail.AccessVersion != 1 || len(detail.Roles) != 2 || !detail.HasUnmanagedOrganizations {
		t.Fatalf("admin detail = %#v", detail)
	}
	workbenchEnvelope(t, fixture.request(http.MethodGet, fmt.Sprintf("/api/admin/users/%d", outside.ID), "", true), 403, "IDENTITY_PERMISSION_DENIED")
	fixture.operatorID = 1
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/users/999999", "", true), 404, "IDENTITY_USER_NOT_FOUND")
	data = workbenchEnvelope(t, fixture.request(http.MethodGet, fmt.Sprintf("/api/admin/roles/%d", reader.ID), "", true), 200, "")
	var role authdomain.Role
	if err := json.Unmarshal(data, &role); err != nil {
		t.Fatal(err)
	}
	if role.ID != reader.ID || role.Code != "reader" {
		t.Fatalf("role detail = %#v", role)
	}
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/roles/999999", "", true), 404, "AUTHZ_NOT_FOUND")
	workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/users?status=2", "", true), 422, "IDENTITY_VALIDATION_INVALID")
}

func TestWorkbenchHTTPIndividualWritesRequireExplicitArraysAndShareAccessVersion(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	db := fixture.db
	manager := identitydomain.User{Username: "scoped-manager", Email: "scoped-manager@example.test", Status: 1}
	target := identitydomain.User{Username: "write-target", Email: "write-target@example.test", Role: "admin", Status: 1}
	other := identitydomain.User{Username: "write-other", Email: "write-other@example.test", Status: 1}
	if err := db.Create(&[]*identitydomain.User{&manager, &target, &other}).Error; err != nil {
		t.Fatal(err)
	}
	a := organization.Unit{Name: "A", Code: "a", Status: 1}
	b := organization.Unit{Name: "B", Code: "b", Status: 1}
	hidden := organization.Unit{Name: "Hidden", Code: "hidden", Status: 1}
	if err := db.Create(&[]*organization.Unit{&a, &b, &hidden}).Error; err != nil {
		t.Fatal(err)
	}
	managerRole := authdomain.Role{Name: "Scoped Manager", Code: "scoped-manager", Status: 1, DataScope: authdomain.DataScopeOrg}
	reader := authdomain.Role{Name: "Reader", Code: "reader", Status: 1, DataScope: authdomain.DataScopeSelf}
	if err := db.Create(&[]*authdomain.Role{&managerRole, &reader}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: manager.ID, RoleID: managerRole.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]organization.Membership{{UserID: manager.ID, OrganizationID: a.ID}, {UserID: manager.ID, OrganizationID: b.ID}, {UserID: target.ID, OrganizationID: a.ID}, {UserID: target.ID, OrganizationID: hidden.ID}, {UserID: other.ID, OrganizationID: b.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.operatorID = manager.ID
	rolesPath := fmt.Sprintf("/api/admin/users/%d/roles", target.ID)
	orgsPath := fmt.Sprintf("/api/admin/users/%d/organizations", target.ID)
	for _, test := range []struct {
		path, body, errorCode string
		status                int
	}{
		{rolesPath, `{"expected_access_version":1}`, "AUTHZ_VALIDATION_INVALID", 422},
		{rolesPath, `{"role_ids":null,"expected_access_version":1}`, "AUTHZ_VALIDATION_INVALID", 422},
		{rolesPath, `{"role_ids":[],"expected_access_version":0}`, "AUTHZ_VALIDATION_INVALID", 422},
		{rolesPath, `{"role_ids":[0],"expected_access_version":1}`, "AUTHZ_VALIDATION_INVALID", 422},
		{rolesPath, `{"role_ids":[999999],"expected_access_version":1}`, "AUTHZ_VALIDATION_INVALID", 422},
		{rolesPath, `{"role_ids":[`, "HTTP_REQUEST_INVALID", 400},
		{orgsPath, `{"expected_access_version":1}`, "ORG_VALIDATION_INVALID", 422},
		{orgsPath, `{"organization_ids":null,"expected_access_version":1}`, "ORG_VALIDATION_INVALID", 422},
		{orgsPath, `{"organization_ids":[],"expected_access_version":0}`, "ORG_VALIDATION_INVALID", 422},
		{orgsPath, `{"organization_ids":[999999],"expected_access_version":1}`, "ORG_VALIDATION_INVALID", 422},
	} {
		workbenchEnvelope(t, fixture.request(http.MethodPut, test.path, test.body, true), test.status, test.errorCode)
	}
	data := workbenchEnvelope(t, fixture.request(http.MethodPut, rolesPath, fmt.Sprintf(`{"role_ids":[%d],"expected_access_version":1}`, reader.ID), true), 200, "")
	var version struct {
		AccessVersion int `json:"access_version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		t.Fatal(err)
	}
	if version.AccessVersion != 2 {
		t.Fatalf("role version = %d", version.AccessVersion)
	}
	workbenchEnvelope(t, fixture.request(http.MethodPut, orgsPath, `{"organization_ids":[],"expected_access_version":1}`, true), 409, "ORG_CONFLICT")
	data = workbenchEnvelope(t, fixture.request(http.MethodPut, orgsPath, fmt.Sprintf(`{"organization_ids":[%d],"expected_access_version":2}`, b.ID), true), 200, "")
	if err := json.Unmarshal(data, &version); err != nil {
		t.Fatal(err)
	}
	if version.AccessVersion != 3 {
		t.Fatalf("organization version = %d", version.AccessVersion)
	}
	workbenchEnvelope(t, fixture.request(http.MethodPut, rolesPath, `{"role_ids":[],"expected_access_version":2}`, true), 409, "AUTHZ_CONFLICT")
	workbenchEnvelope(t, fixture.request(http.MethodPut, rolesPath, `{"role_ids":[],"expected_access_version":3}`, true), 200, "")
	workbenchEnvelope(t, fixture.request(http.MethodPut, orgsPath, `{"organization_ids":[],"expected_access_version":4}`, true), 200, "")
	ids, err := organization.NewGORMRepository(db).MemberOrganizationIDs(context.Background(), target.ID)
	if err != nil || len(ids) != 1 || ids[0] != hidden.ID {
		t.Fatalf("hidden memberships = %v, %v", ids, err)
	}
	otherIDs, err := organization.NewGORMRepository(db).MemberOrganizationIDs(context.Background(), other.ID)
	if err != nil || len(otherIDs) != 1 || otherIDs[0] != b.ID {
		t.Fatalf("other memberships = %v, %v", otherIDs, err)
	}
	roleIDs, err := authgorm.NewRepository(db).RoleIDsByUser(context.Background(), target.ID)
	if err != nil || len(roleIDs) != 0 {
		t.Fatalf("cleared roles = %v, %v", roleIDs, err)
	}
	current, err := authgorm.NewAccessVersions(db).Current(context.Background(), target.ID)
	if err != nil || current != 5 {
		t.Fatalf("shared version = %d, %v", current, err)
	}
}

type workbenchDocSchema struct {
	Type       string                        `json:"type"`
	Minimum    *float64                      `json:"minimum"`
	Properties map[string]workbenchDocSchema `json:"properties"`
	Items      *workbenchDocSchema           `json:"items"`
	Required   []string                      `json:"required"`
	Enum       []json.RawMessage             `json:"enum"`
}
type workbenchDocOperation struct {
	Parameters []struct {
		Name   string             `json:"name"`
		In     string             `json:"in"`
		Schema workbenchDocSchema `json:"schema"`
	} `json:"parameters"`
	RequestBody struct {
		Content map[string]struct {
			Schema workbenchDocSchema `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]struct {
			Schema workbenchDocSchema `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
	Security []map[string][]string `json:"security"`
}

func TestWorkbenchOpenAPIDescribesActualQueryReadAndVersionedWriteContracts(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	response := fixture.request(http.MethodGet, "/docs/openapi.json", "", false)
	if response.Code != 200 {
		t.Fatalf("raw OpenAPI status = %d %s", response.Code, response.Body.String())
	}
	var document struct {
		OpenAPI string                                      `json:"openapi"`
		Paths   map[string]map[string]workbenchDocOperation `json:"paths"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.OpenAPI != "3.0.3" {
		t.Fatalf("raw OpenAPI version = %q", document.OpenAPI)
	}
	users := document.Paths["/api/admin/users"]["get"]
	query := map[string]workbenchDocSchema{}
	for _, parameter := range users.Parameters {
		if parameter.In == "query" {
			query[parameter.Name] = parameter.Schema
		}
	}
	if len(query) != 4 || query["page"].Type != "integer" || query["size"].Type != "integer" || query["keyword"].Type != "string" || query["status"].Type != "integer" || len(query["status"].Enum) != 2 {
		t.Fatalf("query schema = %#v", query)
	}
	detail := document.Paths["/api/admin/users/{id}"]["get"]
	user := detail.Responses["200"].Content["application/json"].Schema.Properties["data"]
	if user.Properties["username"].Type != "string" || user.Properties["roles"].Type != "array" || user.Properties["organizations"].Type != "array" || user.Properties["has_unmanaged_organizations"].Type != "boolean" || user.Properties["access_version"].Type != "integer" {
		t.Fatalf("user schema = %#v", user)
	}
	if user.Properties["roles"].Items == nil || user.Properties["roles"].Items.Properties["code"].Type != "string" || user.Properties["roles"].Items.Properties["status"].Type != "integer" {
		t.Fatalf("role summary schema = %#v", user.Properties["roles"])
	}
	for _, dimension := range []string{"roles", "organizations"} {
		operation := document.Paths["/api/admin/users/{id}/"+dimension]["put"]
		request := operation.RequestBody.Content["application/json"].Schema
		field := "role_ids"
		if dimension == "organizations" {
			field = "organization_ids"
		}
		ids := request.Properties[field]
		version := request.Properties["expected_access_version"]
		if len(request.Required) != 2 || ids.Type != "array" || ids.Items == nil || ids.Items.Type != "integer" || ids.Items.Minimum == nil || *ids.Items.Minimum != 1 || version.Type != "integer" || version.Minimum == nil || *version.Minimum != 1 {
			t.Fatalf("%s request schema = %#v", dimension, request)
		}
		success := operation.Responses["200"].Content["application/json"].Schema.Properties["data"]
		if success.Properties["access_version"].Type != "integer" || len(operation.Security) != 1 || operation.Responses["409"].Content["application/json"].Schema.Type != "object" {
			t.Fatalf("%s write schema = %#v", dimension, operation)
		}
	}
	tree := document.Paths["/api/admin/organizations/tree"]["get"].Responses["200"].Content["application/json"].Schema.Properties["data"]
	if tree.Type != "array" || tree.Items == nil || tree.Items.Properties["manageable"].Type != "boolean" {
		t.Fatalf("tree schema = %#v", tree)
	}
	menus := document.Paths["/api/admin/roles/{id}/menus"]["get"].Responses["200"].Content["application/json"].Schema.Properties["data"]
	if menus.Type != "array" || menus.Items == nil || menus.Items.Properties["parent_id"].Type != "integer" || menus.Items.Properties["status"].Type != "integer" {
		t.Fatalf("assigned menu schema = %#v", menus)
	}
}

func TestWorkbenchHTTPNewRoutesRejectMissingAuthenticationAndPermission(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	routes := []struct{ method, path string }{{http.MethodGet, "/api/admin/roles/1"}, {http.MethodGet, "/api/admin/users/1"}, {http.MethodPut, "/api/admin/users/2/roles"}, {http.MethodPut, "/api/admin/users/2/organizations"}}
	for _, route := range routes {
		workbenchEnvelope(t, fixture.request(route.method, route.path, "", false), 401, "AUTHN_HEADER_MISSING")
	}
	fixture.permissions = nil
	for _, route := range routes {
		workbenchEnvelope(t, fixture.request(route.method, route.path, "", true), 403, "API_META_PERMISSION_DENIED")
	}
}

func TestWorkbenchHTTPShowsPositioningAncestorsAndCompleteAssignedMenus(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	db := fixture.db
	manager := identitydomain.User{Username: "scope-manager", Email: "scope@example.test", Password: "private", Status: 1}
	if err := db.Create(&manager).Error; err != nil {
		t.Fatal(err)
	}
	parent := organization.Unit{Name: "Positioning only", Code: "parent", Status: 1}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := organization.Unit{Name: "Managed", Code: "managed", ParentID: parent.ID, Status: 1}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	role := authdomain.Role{Name: "Scoped", Code: "scoped", Status: 1, DataScope: authdomain.DataScopeOrg}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: manager.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&organization.Membership{UserID: manager.ID, OrganizationID: child.ID}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.operatorID = manager.ID
	var tree []organization.TreeNode
	data := workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/admin/organizations/tree", "", true), 200, "")
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].ID != parent.ID || tree[0].Manageable || len(tree[0].Children) != 1 || !tree[0].Children[0].Manageable {
		t.Fatalf("tree = %#v", tree)
	}
	fixture.permissions = append(fixture.permissions, "admin.organizations.id.users.get")
	workbenchEnvelope(t, fixture.request(http.MethodGet, fmt.Sprintf("/api/admin/organizations/%d/users", parent.ID), "", true), 422, "ORG_VALIDATION_INVALID")
	menuParent := navigation.MenuModel{Name: "Unassigned parent", Type: 1, Status: 1}
	if err := db.Create(&menuParent).Error; err != nil {
		t.Fatal(err)
	}
	assignedChild := navigation.MenuModel{Name: "Assigned child", ParentID: menuParent.ID, PermissionCode: "not-granted", Sort: 1, Type: 2, Status: 1}
	disabled := navigation.MenuModel{Name: "Disabled assignment", ParentID: menuParent.ID, Sort: 2, Type: 2, Status: 1}
	if err := db.Create(&[]*navigation.MenuModel{&assignedChild, &disabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&disabled).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]navigation.RoleMenuModel{{RoleID: role.ID, MenuID: assignedChild.ID}, {RoleID: role.ID, MenuID: disabled.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	data = workbenchEnvelope(t, fixture.request(http.MethodGet, fmt.Sprintf("/api/admin/roles/%d/menus", role.ID), "", true), 200, "")
	var menus []navigation.MenuDetail
	if err := json.Unmarshal(data, &menus); err != nil {
		t.Fatal(err)
	}
	if len(menus) != 2 || menus[0].ID != assignedChild.ID || menus[0].ParentID != menuParent.ID || menus[1].ID != disabled.ID || menus[1].Status != 0 {
		t.Fatalf("assigned menus = %#v", menus)
	}
}

func TestWorkbenchHTTPUnassignedUserHasEmptyContextArrays(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	user := identitydomain.User{Username: "unassigned", Email: "unassigned@example.test", Status: 1}
	if err := fixture.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := authgorm.NewAccessVersions(fixture.db).EnsureMany(context.Background(), []uint{user.ID}); err != nil {
		t.Fatal(err)
	}
	fixture.operatorID = user.ID
	data := workbenchEnvelope(t, fixture.request(http.MethodGet, "/api/user/context", "", true), 200, "")
	var context map[string]json.RawMessage
	if err := json.Unmarshal(data, &context); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"roles", "permissions", "menus"} {
		if string(context[field]) != "[]" {
			t.Fatalf("unassigned user's %s = %s, want []", field, context[field])
		}
	}
}

func TestWorkbenchOpenAPIDocumentsActualAuthenticationFailures(t *testing.T) {
	fixture := newWorkbenchHTTPFixture(t)
	var document map[string]any
	if err := json.Unmarshal(fixture.request(http.MethodGet, "/docs/openapi.json", "", false).Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/user/info", "/api/admin/users"} {
		for _, header := range []string{"", "Basic invalid"} {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			if header != "" {
				request.Header.Set("Authorization", header)
			}
			response := httptest.NewRecorder()
			fixture.application.Handler().ServeHTTP(response, request)
			var failure struct {
				ErrorCode string `json:"error_code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusUnauthorized || failure.ErrorCode == "" {
				t.Fatalf("%s with %q: %d %s", path, header, response.Code, response.Body.String())
			}
			operation := document["paths"].(map[string]any)[path].(map[string]any)["get"].(map[string]any)
			contract := operation["responses"].(map[string]any)["401"].(map[string]any)
			schema := contract["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
			codes := schema["properties"].(map[string]any)["error_code"].(map[string]any)["enum"].([]any)
			documented := false
			for _, code := range codes {
				documented = documented || code == failure.ErrorCode
			}
			if !documented {
				t.Errorf("%s returns %s, absent from OpenAPI 401 enum %v", path, failure.ErrorCode, codes)
			}
		}
	}
}
