package app_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"admin/internal/app"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

func TestRegisterHTTPMountsMiddlewareByAccessLevel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	sequence := make([]string, 0, 4)
	middleware := app.HTTPMiddleware{
		API: func(c *gin.Context) {
			sequence = append(sequence, "api")
			c.Next()
		},
		Authenticated: func(c *gin.Context) {
			sequence = append(sequence, "authenticated")
			c.Next()
		},
		PermissionControlled: func(c *gin.Context) {
			sequence = append(sequence, "permission")
			c.Next()
		},
	}
	descriptors := []routecatalog.Descriptor{
		httpTestDescriptor("GET", "/api/public", routecatalog.Public, func(c *gin.Context) {
			sequence = append(sequence, "handler")
			c.JSON(http.StatusOK, gin.H{"access": "public"})
		}),
		httpTestDescriptor("GET", "/api/user/profile", routecatalog.Authenticated, func(c *gin.Context) {
			sequence = append(sequence, "handler")
			c.JSON(http.StatusOK, gin.H{"access": "authenticated"})
		}),
		httpTestDescriptor("GET", "/api/admin/users", routecatalog.PermissionControlled, func(c *gin.Context) {
			sequence = append(sequence, "handler")
			c.JSON(http.StatusOK, gin.H{"access": "permission"})
		}),
	}
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}

	app.RegisterHTTP(engine, catalog, middleware)

	tests := []struct {
		path string
		want []string
	}{
		{path: "/api/public", want: []string{"api", "handler"}},
		{path: "/api/user/profile", want: []string{"api", "authenticated", "handler"}},
		{path: "/api/admin/users", want: []string{"api", "authenticated", "permission", "handler"}},
	}
	for _, test := range tests {
		sequence = sequence[:0]
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", test.path, response.Code)
		}
		if !slices.Equal(sequence, test.want) {
			t.Fatalf("%s middleware sequence = %v, want %v", test.path, sequence, test.want)
		}
	}
}

func TestRegisterHTTPMatchesCatalogAndTechnicalRoutePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business := []routecatalog.Descriptor{
		httpTestDescriptor(http.MethodGet, "/api/items/:id", routecatalog.Public, func(c *gin.Context) { c.Status(http.StatusNoContent) }),
		httpTestDescriptor(http.MethodGet, "/api/assets/*path", routecatalog.Public, func(c *gin.Context) { c.Status(http.StatusNoContent) }),
	}
	catalog, err := routecatalog.New(business)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}

	engine := gin.New()
	app.RegisterTechnicalHTTP(engine, app.TechnicalHTTP{APIDocsEnabled: false})
	app.RegisterHTTP(engine, catalog, app.HTTPMiddleware{})
	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	if len(routes) != 3 {
		t.Fatalf("registered routes = %v, want exactly Catalog business routes plus /ping", routes)
	}
	for _, key := range []string{"GET /ping", "GET /api/items/:id", "GET /api/assets/*path"} {
		if _, ok := routes[key]; !ok {
			t.Fatalf("registered routes %v missing %s", routes, key)
		}
	}
	if _, ok := routes["GET /docs"]; ok {
		t.Fatalf("docs route registered while disabled: %v", routes)
	}
	for _, path := range []string{"/api/items/42", "/api/assets/css/app.css"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("GET %s status = %d, want 204", path, response.Code)
		}
	}

	docsEngine := gin.New()
	app.RegisterTechnicalHTTP(docsEngine, app.TechnicalHTTP{
		APIDocsEnabled: true,
		Docs:           func(c *gin.Context) { c.Status(http.StatusOK) },
		OpenAPI:        func(c *gin.Context) { c.Status(http.StatusOK) },
	})
	docsRoutes := make(map[string]struct{})
	for _, route := range docsEngine.Routes() {
		docsRoutes[route.Method+" "+route.Path] = struct{}{}
	}
	if len(docsRoutes) != 3 {
		t.Fatalf("technical routes = %v, want exactly ping and enabled docs routes", docsRoutes)
	}
	for _, key := range []string{"GET /ping", "GET /docs", "GET /docs/openapi.json"} {
		if _, ok := docsRoutes[key]; !ok {
			t.Fatalf("technical routes %v missing %s", docsRoutes, key)
		}
	}
}

func httpTestDescriptor(method, path string, access routecatalog.AccessLevel, handler gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := routecatalog.Descriptor{
		Method:               method,
		Path:                 path,
		Access:               access,
		Handler:              handler,
		Name:                 path,
		Group:                "test",
		DefaultAuditCategory: "test",
		OpenAPI: routecatalog.Operation{
			Summary: path,
			Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				200: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]string{})},
			},
		},
	}
	if access == routecatalog.PermissionControlled {
		descriptor.DefaultPermissionCode = "test.read"
	}
	return descriptor
}

func TestRegisterTechnicalHTTPUsesSuccessEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	app.RegisterTechnicalHTTP(engine, app.TechnicalHTTP{})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Body.String(); got != `{"code":200,"error_code":"","msg":"success","data":{"msg":"pong"}}` {
		t.Fatalf("body = %s", got)
	}
}
