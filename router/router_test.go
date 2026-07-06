package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"admin/service"
)

// TestInitRouterRegistersAPIRoutes 验证路由初始化会注册 API 管理和文档路由。
func TestInitRouterRegistersAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := InitRouter(service.JWTConfig{Secret: "test"}, Options{
		APIDocs: APIDocsOptions{Enabled: true},
	})
	if r == nil {
		t.Fatal("router should not be nil")
	}

	expected := map[string]struct{}{
		"GET /docs":                            {},
		"GET /docs/openapi.json":               {},
		"GET /api/admin/apis/:id":              {},
		"GET /api/admin/api-groups":            {},
		"GET /api/admin/api-methods":           {},
		"POST /api/admin/apis/sync":            {},
		"POST /api/admin/apis/:id/menu-button": {},
	}
	registered := map[string]struct{}{}
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	for route := range expected {
		if _, ok := registered[route]; !ok {
			t.Fatalf("route %s should be registered", route)
		}
	}
}

// TestOpenAPIDocumentEndpoint 验证 OpenAPI JSON 文档接口可正常返回核心结构。
func TestOpenAPIDocumentEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := InitRouter(service.JWTConfig{Secret: "test"}, Options{
		APIDocs: APIDocsOptions{Enabled: true},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi document should be valid json: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %v", doc["openapi"])
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("openapi document should contain paths")
	}
	pathItem, ok := paths["/api/admin/apis/{id}"]
	if !ok {
		t.Fatal("openapi document should contain api detail route")
	}
	menuButtonPathItem, ok := paths["/api/admin/apis/{id}/menu-button"]
	if !ok {
		t.Fatal("openapi document should contain api menu-button route")
	}
	_ = pathItem
	operation := getOpenAPIOperation(t, menuButtonPathItem, "post")
	properties := getRequestBodyProperties(t, operation)
	for _, name := range []string{"parent_id", "name", "sort"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("menu-button request schema should contain %s", name)
		}
	}
	if _, ok := paths["/docs"]; ok {
		t.Fatal("openapi document should not expose docs route")
	}
}

// TestAPIDocsCanBeDisabled 验证 API 文档路由默认关闭。
func TestAPIDocsCanBeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := InitRouter(service.JWTConfig{Secret: "test"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected docs route disabled by default, got %d", w.Code)
	}
}

// getOpenAPIOperation 从 OpenAPI path item 中取出指定方法的 operation。
func getOpenAPIOperation(t *testing.T, pathItem any, method string) map[string]any {
	t.Helper()

	item, ok := pathItem.(map[string]any)
	if !ok {
		t.Fatal("path item should be object")
	}
	operation, ok := item[method].(map[string]any)
	if !ok {
		t.Fatalf("%s operation should exist", method)
	}
	return operation
}

// getRequestBodyProperties 从 OpenAPI operation 中取出请求体属性定义。
func getRequestBodyProperties(t *testing.T, operation map[string]any) map[string]any {
	t.Helper()

	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatal("operation should contain requestBody")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatal("requestBody should contain content")
	}
	jsonContent, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatal("requestBody should contain application/json")
	}
	schema, ok := jsonContent["schema"].(map[string]any)
	if !ok {
		t.Fatal("application/json should contain schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema should contain properties")
	}
	return properties
}
