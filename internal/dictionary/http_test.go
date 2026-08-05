package dictionary_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/app"
	"admin/internal/dictionary"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type apiEnvelope[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

type pageData[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

type dictionaryHTTPFixture struct {
	t       *testing.T
	handler http.Handler
	routes  []routecatalog.Descriptor
}

func TestRoutesExposeDictionaryHTTPContract(t *testing.T) {
	fixture := newDictionaryHTTPFixture(t)
	routes := fixture.routes

	if len(routes) != 9 {
		t.Fatalf("Dictionary routes = %d, want 9", len(routes))
	}
	assertDictionaryRoute(t, routes, http.MethodGet, "/api/dicts/:type_code/items", routecatalog.Public, "")
	assertDictionaryRoute(t, routes, http.MethodGet, "/api/admin/dict-types", routecatalog.PermissionControlled, "admin.dict-types.get")
	assertDictionaryRoute(t, routes, http.MethodPost, "/api/admin/dict-types", routecatalog.PermissionControlled, "admin.dict-types.post")
	assertDictionaryRoute(t, routes, http.MethodPut, "/api/admin/dict-types/:id", routecatalog.PermissionControlled, "admin.dict-types.put")
	assertDictionaryRoute(t, routes, http.MethodDelete, "/api/admin/dict-types/:id", routecatalog.PermissionControlled, "admin.dict-types.delete")
	assertDictionaryRoute(t, routes, http.MethodGet, "/api/admin/dict-items", routecatalog.PermissionControlled, "admin.dict-items.get")
	assertDictionaryRoute(t, routes, http.MethodPost, "/api/admin/dict-items", routecatalog.PermissionControlled, "admin.dict-items.post")
	assertDictionaryRoute(t, routes, http.MethodPut, "/api/admin/dict-items/:id", routecatalog.PermissionControlled, "admin.dict-items.put")
	assertDictionaryRoute(t, routes, http.MethodDelete, "/api/admin/dict-items/:id", routecatalog.PermissionControlled, "admin.dict-items.delete")

	response := fixture.request(http.MethodGet, "/api/dicts/unknown/items", nil, http.StatusOK)
	payload := decodeJSON[apiEnvelope[[]dictionary.ItemInfo]](t, response)
	if payload.Code != http.StatusOK || len(payload.Data) != 0 {
		t.Fatalf("unknown public Dictionary response = %#v, want code 200 and empty data", payload)
	}
}

func TestDictionaryHTTPRejectsInvalidRequests(t *testing.T) {
	fixture := newDictionaryHTTPFixture(t)
	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "type create requires code", method: http.MethodPost, path: "/api/admin/dict-types", body: map[string]any{"name": "Priority"}},
		{name: "type update requires numeric id", method: http.MethodPut, path: "/api/admin/dict-types/not-an-id", body: map[string]any{"name": "Priority"}},
		{name: "item create requires value", method: http.MethodPost, path: "/api/admin/dict-items", body: map[string]any{"type_code": "priority", "label": "High"}},
		{name: "item update validates status", method: http.MethodPut, path: "/api/admin/dict-items/1", body: map[string]any{"status": 2}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := fixture.request(test.method, test.path, test.body, http.StatusBadRequest)
			payload := decodeJSON[apiEnvelope[struct{}]](t, response)
			if payload.Code != http.StatusBadRequest || !strings.HasPrefix(payload.Msg, "参数错误") {
				t.Fatalf("validation response = %#v, want code 400 and parameter error", payload)
			}
		})
	}
}

func TestDictionaryHTTPTypeLifecycleRenamesAndCascadesItems(t *testing.T) {
	fixture := newDictionaryHTTPFixture(t)

	response := fixture.request(http.MethodPost, "/api/admin/dict-types", map[string]any{
		"name": "Priority", "code": "priority", "remark": "Ticket priority", "sort": 20,
	}, http.StatusOK)
	created := decodeJSON[apiEnvelope[dictionary.TypeInfo]](t, response)
	if created.Code != http.StatusOK || created.Msg != "创建成功" || created.Data.ID == 0 ||
		created.Data.Name != "Priority" || created.Data.Code != "priority" || created.Data.Remark != "Ticket priority" ||
		created.Data.Sort != 20 || created.Data.Status != 1 {
		t.Fatalf("create Dictionary Type response = %#v", created)
	}

	response = fixture.request(http.MethodPost, "/api/admin/dict-types", map[string]any{
		"name": "Duplicate", "code": "priority",
	}, http.StatusBadRequest)
	duplicate := decodeJSON[apiEnvelope[struct{}]](t, response)
	if duplicate.Code != http.StatusBadRequest || duplicate.Msg != "字典编码已存在" {
		t.Fatalf("duplicate Dictionary Type response = %#v", duplicate)
	}

	createType(t, fixture, map[string]any{"name": "Severity", "code": "severity", "sort": 10})
	response = fixture.request(http.MethodGet, "/api/admin/dict-types?page=2&size=1", nil, http.StatusOK)
	listed := decodeJSON[apiEnvelope[pageData[dictionary.TypeInfo]]](t, response)
	if listed.Code != http.StatusOK || listed.Data.Total != 2 || listed.Data.Page != 2 || listed.Data.Size != 1 ||
		len(listed.Data.List) != 1 || listed.Data.List[0].ID != created.Data.ID || listed.Data.List[0].Code != "priority" {
		t.Fatalf("paginated Dictionary Types response = %#v", listed)
	}

	createItem(t, fixture, map[string]any{
		"type_code": "priority", "label": "High", "value": "high", "sort": 10,
	})
	response = fixture.request(http.MethodPut, "/api/admin/dict-types/"+strconv.FormatUint(uint64(created.Data.ID), 10), map[string]any{
		"name": "Urgency", "code": "urgency", "sort": 5,
	}, http.StatusOK)
	updated := decodeJSON[apiEnvelope[dictionary.TypeInfo]](t, response)
	if updated.Code != http.StatusOK || updated.Msg != "修改成功" || updated.Data.ID != created.Data.ID ||
		updated.Data.Name != "Urgency" || updated.Data.Code != "urgency" || updated.Data.Sort != 5 {
		t.Fatalf("update Dictionary Type response = %#v", updated)
	}

	response = fixture.request(http.MethodGet, "/api/admin/dict-types?keyword=urgency", nil, http.StatusOK)
	renamedTypes := decodeJSON[apiEnvelope[pageData[dictionary.TypeInfo]]](t, response)
	if renamedTypes.Data.Total != 1 || len(renamedTypes.Data.List) != 1 || renamedTypes.Data.List[0].Code != "urgency" {
		t.Fatalf("Dictionary Types after rename = %#v", renamedTypes)
	}
	response = fixture.request(http.MethodGet, "/api/dicts/priority/items", nil, http.StatusOK)
	oldCodeItems := decodeJSON[apiEnvelope[[]dictionary.ItemInfo]](t, response)
	if len(oldCodeItems.Data) != 0 {
		t.Fatalf("old code public items after rename = %#v, want empty", oldCodeItems.Data)
	}
	response = fixture.request(http.MethodGet, "/api/dicts/urgency/items", nil, http.StatusOK)
	renamedItems := decodeJSON[apiEnvelope[[]dictionary.ItemInfo]](t, response)
	if len(renamedItems.Data) != 1 || renamedItems.Data[0].TypeCode != "urgency" || renamedItems.Data[0].Value != "high" {
		t.Fatalf("renamed code public items = %#v", renamedItems.Data)
	}

	response = fixture.request(http.MethodDelete, "/api/admin/dict-types/"+strconv.FormatUint(uint64(created.Data.ID), 10), nil, http.StatusOK)
	deleted := decodeJSON[apiEnvelope[struct{}]](t, response)
	if deleted.Code != http.StatusOK || deleted.Msg != "删除成功" {
		t.Fatalf("delete Dictionary Type response = %#v", deleted)
	}
	response = fixture.request(http.MethodGet, "/api/admin/dict-types?keyword=urgency", nil, http.StatusOK)
	typesAfterDelete := decodeJSON[apiEnvelope[pageData[dictionary.TypeInfo]]](t, response)
	if typesAfterDelete.Data.Total != 0 || len(typesAfterDelete.Data.List) != 0 {
		t.Fatalf("Dictionary Types after deletion = %#v", typesAfterDelete.Data)
	}
	response = fixture.request(http.MethodGet, "/api/admin/dict-items?type_code=urgency", nil, http.StatusOK)
	itemsAfterDelete := decodeJSON[apiEnvelope[pageData[dictionary.ItemInfo]]](t, response)
	if itemsAfterDelete.Data.Total != 0 || len(itemsAfterDelete.Data.List) != 0 {
		t.Fatalf("Dictionary Items after type deletion = %#v, want cascade deletion", itemsAfterDelete.Data)
	}
}

func TestDictionaryHTTPItemLifecycleSupportsPaginationAndDuplicateProtection(t *testing.T) {
	fixture := newDictionaryHTTPFixture(t)
	createType(t, fixture, map[string]any{"name": "Numbers", "code": "numbers"})

	first := createItem(t, fixture, map[string]any{
		"type_code": "numbers", "label": "One", "value": "1", "remark": "First", "sort": 20,
	})
	if first.ID == 0 || first.TypeCode != "numbers" || first.Label != "One" || first.Value != "1" ||
		first.Remark != "First" || first.Sort != 20 || first.Status != 1 {
		t.Fatalf("create Dictionary Item response data = %#v", first)
	}
	second := createItem(t, fixture, map[string]any{
		"type_code": "numbers", "label": "Two", "value": "2", "sort": 10,
	})
	response := fixture.request(http.MethodPost, "/api/admin/dict-items", map[string]any{
		"type_code": "numbers", "label": "Duplicate one", "value": "1",
	}, http.StatusBadRequest)
	duplicate := decodeJSON[apiEnvelope[struct{}]](t, response)
	if duplicate.Code != http.StatusBadRequest || duplicate.Msg != "同一字典类型下字典值已存在" {
		t.Fatalf("duplicate Dictionary Item response = %#v", duplicate)
	}

	response = fixture.request(http.MethodGet, "/api/admin/dict-items?type_code=numbers&page=1&size=1", nil, http.StatusOK)
	listed := decodeJSON[apiEnvelope[pageData[dictionary.ItemInfo]]](t, response)
	if listed.Code != http.StatusOK || listed.Data.Total != 2 || listed.Data.Page != 1 || listed.Data.Size != 1 ||
		len(listed.Data.List) != 1 || listed.Data.List[0].ID != second.ID {
		t.Fatalf("paginated Dictionary Items response = %#v", listed)
	}

	response = fixture.request(http.MethodPut, "/api/admin/dict-items/"+strconv.FormatUint(uint64(first.ID), 10), map[string]any{
		"label": "Primary", "value": "01", "remark": "Updated", "sort": 5, "status": 0,
	}, http.StatusOK)
	updated := decodeJSON[apiEnvelope[dictionary.ItemInfo]](t, response)
	if updated.Code != http.StatusOK || updated.Msg != "修改成功" || updated.Data.ID != first.ID ||
		updated.Data.TypeCode != "numbers" || updated.Data.Label != "Primary" || updated.Data.Value != "01" ||
		updated.Data.Remark != "Updated" || updated.Data.Sort != 5 || updated.Data.Status != 0 {
		t.Fatalf("update Dictionary Item response = %#v", updated)
	}
	response = fixture.request(http.MethodGet, "/api/admin/dict-items?type_code=numbers&keyword=Primary&status=0", nil, http.StatusOK)
	filtered := decodeJSON[apiEnvelope[pageData[dictionary.ItemInfo]]](t, response)
	if filtered.Data.Total != 1 || len(filtered.Data.List) != 1 || filtered.Data.List[0].Value != "01" {
		t.Fatalf("Dictionary Items after update = %#v", filtered.Data)
	}

	response = fixture.request(http.MethodDelete, "/api/admin/dict-items/"+strconv.FormatUint(uint64(second.ID), 10), nil, http.StatusOK)
	deleted := decodeJSON[apiEnvelope[struct{}]](t, response)
	if deleted.Code != http.StatusOK || deleted.Msg != "删除成功" {
		t.Fatalf("delete Dictionary Item response = %#v", deleted)
	}
	response = fixture.request(http.MethodGet, "/api/admin/dict-items?type_code=numbers", nil, http.StatusOK)
	afterDelete := decodeJSON[apiEnvelope[pageData[dictionary.ItemInfo]]](t, response)
	if afterDelete.Data.Total != 1 || len(afterDelete.Data.List) != 1 || afterDelete.Data.List[0].ID != first.ID {
		t.Fatalf("Dictionary Items after deletion = %#v", afterDelete.Data)
	}
}

func TestDictionaryPublicHTTPListsOnlyEnabledItemsInSortOrder(t *testing.T) {
	fixture := newDictionaryHTTPFixture(t)
	createType(t, fixture, map[string]any{"name": "Priority", "code": "priority"})
	createType(t, fixture, map[string]any{"name": "Hidden", "code": "hidden", "status": 0})
	createItem(t, fixture, map[string]any{"type_code": "priority", "label": "Disabled", "value": "disabled", "sort": 1, "status": 0})
	createItem(t, fixture, map[string]any{"type_code": "priority", "label": "Low", "value": "low", "sort": 20})
	createItem(t, fixture, map[string]any{"type_code": "priority", "label": "High", "value": "high", "sort": 10})
	createItem(t, fixture, map[string]any{"type_code": "hidden", "label": "Visible item", "value": "visible-item", "sort": 1})

	response := fixture.request(http.MethodGet, "/api/dicts/priority/items", nil, http.StatusOK)
	payload := decodeJSON[apiEnvelope[[]dictionary.ItemInfo]](t, response)
	if payload.Code != http.StatusOK || len(payload.Data) != 2 ||
		payload.Data[0].TypeCode != "priority" || payload.Data[0].Label != "High" || payload.Data[0].Value != "high" || payload.Data[0].Sort != 10 || payload.Data[0].Status != 1 ||
		payload.Data[1].TypeCode != "priority" || payload.Data[1].Label != "Low" || payload.Data[1].Value != "low" || payload.Data[1].Sort != 20 || payload.Data[1].Status != 1 {
		t.Fatalf("public enabled Dictionary Items = %#v, want high then low", payload)
	}

	response = fixture.request(http.MethodGet, "/api/dicts/hidden/items", nil, http.StatusOK)
	hidden := decodeJSON[apiEnvelope[[]dictionary.ItemInfo]](t, response)
	if hidden.Code != http.StatusOK || len(hidden.Data) != 0 {
		t.Fatalf("disabled Dictionary Type public response = %#v, want empty data", hidden)
	}
}

func newDictionaryHTTPFixture(t *testing.T) *dictionaryHTTPFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(dictionary.Models()...); err != nil {
		t.Fatalf("migrate Dictionary models: %v", err)
	}
	service := dictionary.NewService(dictionary.NewGORMRepository(db), platformdatabase.NewTransactionRunner(db))
	routes := dictionary.Routes(service)
	catalog, err := routecatalog.New(routes)
	if err != nil {
		t.Fatalf("build Dictionary Route Catalog: %v", err)
	}
	engine := gin.New()
	app.RegisterHTTP(engine, catalog, app.HTTPMiddleware{})
	return &dictionaryHTTPFixture{t: t, handler: engine, routes: routes}
}

func (fixture *dictionaryHTTPFixture) request(method, path string, body any, wantStatus int) *httptest.ResponseRecorder {
	fixture.t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			fixture.t.Fatalf("encode %s %s request: %v", method, path, err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		fixture.t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, response.Code, wantStatus, response.Body.String())
	}
	return response
}

func decodeJSON[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var payload T
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode HTTP response %q: %v", response.Body.String(), err)
	}
	return payload
}

func createType(t *testing.T, fixture *dictionaryHTTPFixture, body map[string]any) dictionary.TypeInfo {
	t.Helper()
	response := fixture.request(http.MethodPost, "/api/admin/dict-types", body, http.StatusOK)
	payload := decodeJSON[apiEnvelope[dictionary.TypeInfo]](t, response)
	if payload.Code != http.StatusOK || payload.Msg != "创建成功" || payload.Data.ID == 0 {
		t.Fatalf("create Dictionary Type response = %#v", payload)
	}
	return payload.Data
}

func createItem(t *testing.T, fixture *dictionaryHTTPFixture, body map[string]any) dictionary.ItemInfo {
	t.Helper()
	response := fixture.request(http.MethodPost, "/api/admin/dict-items", body, http.StatusOK)
	payload := decodeJSON[apiEnvelope[dictionary.ItemInfo]](t, response)
	if payload.Code != http.StatusOK || payload.Msg != "创建成功" || payload.Data.ID == 0 {
		t.Fatalf("create Dictionary Item response = %#v", payload)
	}
	return payload.Data
}

func assertDictionaryRoute(t *testing.T, routes []routecatalog.Descriptor, method, path string, access routecatalog.AccessLevel, permissionCode string) {
	t.Helper()
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			if route.Access != access || route.DefaultPermissionCode != permissionCode {
				t.Fatalf("route %s %s access/code = %d/%q, want %d/%q", method, path, route.Access, route.DefaultPermissionCode, access, permissionCode)
			}
			return
		}
	}
	t.Fatalf("route %s %s missing", method, path)
}
