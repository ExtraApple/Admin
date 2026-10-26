package routecatalog_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type routeResponse struct {
	Message string `json:"message"`
}

func TestNewBuildsStableNormalizedSnapshot(t *testing.T) {
	handler := func(*gin.Context) {}
	catalog, err := routecatalog.New([]routecatalog.Descriptor{
		{
			Method:               " get ",
			Path:                 "api/probe-health/",
			Access:               routecatalog.Public,
			Handler:              handler,
			Name:                 "Health",
			Group:                "system",
			DefaultAuditCategory: "health",
			OpenAPI: routecatalog.Operation{
				Summary: "Check health",
				Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
				Responses: map[int]routecatalog.Response{
					200: {
						Description: "healthy",
						Kind:        routecatalog.JSONBody,
						Schema:      reflect.TypeOf(routeResponse{}),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("build route catalog: %v", err)
	}

	first := catalog.Snapshot()
	if len(first) != 1 {
		t.Fatalf("snapshot length = %d, want 1", len(first))
	}
	if first[0].Method != "GET" || first[0].Path != "/api/probe-health" || first[0].Access != routecatalog.Public || first[0].Handler == nil {
		t.Fatalf("normalized descriptor changed: %#v", first[0])
	}
	if first[0].Name != "Health" || first[0].Group != "system" || first[0].DefaultAuditCategory != "health" || first[0].OpenAPI.Responses[200].Description != "healthy" {
		t.Fatalf("descriptor metadata changed: %#v", first[0])
	}

	first[0].Name = "mutated"
	response := first[0].OpenAPI.Responses[200]
	response.Description = "mutated"
	first[0].OpenAPI.Responses[200] = response
	second := catalog.Snapshot()
	if second[0].Name != "Health" || second[0].OpenAPI.Responses[200].Description != "healthy" {
		t.Fatalf("snapshot exposed mutable catalog state: %#v", second[0])
	}
}

func TestNewRejectsInvalidDescriptorCombinations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*routecatalog.Descriptor)
		field  string
	}{
		{name: "missing method", mutate: func(d *routecatalog.Descriptor) { d.Method = "" }, field: "method"},
		{name: "unsupported method", mutate: func(d *routecatalog.Descriptor) { d.Method = "TRACE" }, field: "method"},
		{name: "missing path", mutate: func(d *routecatalog.Descriptor) { d.Path = "" }, field: "path"},
		{name: "missing handler", mutate: func(d *routecatalog.Descriptor) { d.Handler = nil }, field: "handler"},
		{name: "unknown access level", mutate: func(d *routecatalog.Descriptor) { d.Access = 0 }, field: "access level"},
		{name: "public permission code", mutate: func(d *routecatalog.Descriptor) { d.DefaultPermissionCode = "health.read" }, field: "permission code"},
		{name: "missing API name", mutate: func(d *routecatalog.Descriptor) { d.Name = "" }, field: "name"},
		{name: "missing API group", mutate: func(d *routecatalog.Descriptor) { d.Group = "" }, field: "group"},
		{name: "missing audit category", mutate: func(d *routecatalog.Descriptor) { d.DefaultAuditCategory = "" }, field: "audit category"},
		{name: "missing operation summary", mutate: func(d *routecatalog.Descriptor) { d.OpenAPI.Summary = "" }, field: "summary"},
		{name: "unknown request kind", mutate: func(d *routecatalog.Descriptor) { d.OpenAPI.Request.Kind = 0 }, field: "request kind"},
		{name: "JSON request without schema", mutate: func(d *routecatalog.Descriptor) {
			d.OpenAPI.Request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Required: true}
		}, field: "request schema"},
		{name: "multipart request without file field", mutate: func(d *routecatalog.Descriptor) {
			d.OpenAPI.Request = routecatalog.RequestBody{Kind: routecatalog.MultipartBody, Required: true, FileDescription: "upload"}
		}, field: "file field"},
		{name: "multipart request without file description", mutate: func(d *routecatalog.Descriptor) {
			d.OpenAPI.Request = routecatalog.RequestBody{Kind: routecatalog.MultipartBody, Required: true, FileField: "file"}
		}, field: "file description"},
		{name: "missing responses", mutate: func(d *routecatalog.Descriptor) { d.OpenAPI.Responses = nil }, field: "responses"},
		{name: "JSON response without schema", mutate: func(d *routecatalog.Descriptor) {
			d.OpenAPI.Responses[200] = routecatalog.Response{Description: "ok", Kind: routecatalog.JSONBody}
		}, field: "response schema"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := validDescriptor()
			test.mutate(&descriptor)
			_, err := routecatalog.New([]routecatalog.Descriptor{descriptor})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.field) {
				t.Fatalf("catalog error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestNewRejectsNormalizedDuplicateRoutes(t *testing.T) {
	first := validDescriptor()
	second := validDescriptor()
	second.Method = " get "
	second.Path = "api/probe-health/"
	second.Name = "Duplicate Health"

	_, err := routecatalog.New([]routecatalog.Descriptor{first, second})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") || !strings.Contains(err.Error(), "GET /api/probe-health") {
		t.Fatalf("catalog error = %v, want normalized duplicate route", err)
	}
}

func validDescriptor() routecatalog.Descriptor {
	return routecatalog.Descriptor{
		Method:               "GET",
		Path:                 "/api/probe-health",
		Access:               routecatalog.Public,
		Handler:              func(*gin.Context) {},
		Name:                 "Health",
		Group:                "system",
		DefaultAuditCategory: "health",
		OpenAPI: routecatalog.Operation{
			Summary: "Check health",
			Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				200: {Description: "healthy", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(routeResponse{})},
			},
		},
	}
}

func TestCatalogCarriesAndClonesErrorDefinitions(t *testing.T) {
	definition := httpresponse.ErrorDefinition{
		Owner:   "identity",
		Code:    "AUTHN_INVALID",
		Status:  http.StatusUnauthorized,
		Message: "authentication failed",
		Fields:  []httpresponse.FieldErrorDefinition{{Field: "username", Code: "AUTHN_USERNAME_INVALID", Message: "username is invalid"}},
	}
	descriptor := validDescriptor()
	descriptor.OpenAPI.Responses[http.StatusUnauthorized] = routecatalog.Response{
		Description: "authentication failed",
		Kind:        routecatalog.JSONBody,
		Schema:      reflect.TypeOf(httpresponse.Envelope[any]{}),
		Errors:      []httpresponse.ErrorDefinition{definition},
	}
	catalog, err := routecatalog.New([]routecatalog.Descriptor{descriptor})
	if err != nil {
		t.Fatalf("build route catalog: %v", err)
	}
	definition.Fields[0].Message = "mutated"
	snapshot := catalog.Snapshot()
	got := snapshot[0].OpenAPI.Responses[http.StatusUnauthorized].Errors[0]
	if got.Fields[0].Message != "username is invalid" {
		t.Fatalf("snapshot error definition was mutable: %#v", got)
	}
}

func TestCatalogRejectsErrorDefinitionDriftAndInvalidEnvelope(t *testing.T) {
	definition := httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_INVALID", Status: http.StatusUnauthorized, Message: "authentication failed"}
	tests := []struct {
		name   string
		mutate func(*routecatalog.Descriptor)
		want   string
	}{
		{name: "missing errors", mutate: func(d *routecatalog.Descriptor) {
			d.OpenAPI.Responses[http.StatusUnauthorized] = routecatalog.Response{Description: "error", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{})}
		}, want: "error definitions"},
		{name: "schema is not envelope", mutate: func(d *routecatalog.Descriptor) {
			d.OpenAPI.Responses[http.StatusUnauthorized] = routecatalog.Response{Description: "error", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(routeResponse{}), Errors: []httpresponse.ErrorDefinition{definition}}
		}, want: "envelope"},
		{name: "status drift", mutate: func(d *routecatalog.Descriptor) {
			wrong := definition
			wrong.Status = http.StatusForbidden
			d.OpenAPI.Responses[http.StatusUnauthorized] = routecatalog.Response{Description: "error", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), Errors: []httpresponse.ErrorDefinition{wrong}}
		}, want: "status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := validDescriptor()
			test.mutate(&descriptor)
			_, err := routecatalog.New([]routecatalog.Descriptor{descriptor})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("catalog error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCatalogRejectsPublicErrorCodeOwnershipConflict(t *testing.T) {
	first := validDescriptor()
	first.OpenAPI.Responses[http.StatusUnauthorized] = routecatalog.Response{Description: "error", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), Errors: []httpresponse.ErrorDefinition{{Owner: "identity", Code: "AUTHN_INVALID", Status: http.StatusUnauthorized, Message: "authentication failed"}}}
	second := validDescriptor()
	second.Path = "/api/other"
	second.OpenAPI.Responses[http.StatusUnauthorized] = routecatalog.Response{Description: "error", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), Errors: []httpresponse.ErrorDefinition{{Owner: "authorization", Code: "AUTHN_INVALID", Status: http.StatusUnauthorized, Message: "authorization failed"}}}
	if _, err := routecatalog.New([]routecatalog.Descriptor{first, second}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "ownership") {
		t.Fatalf("catalog error = %v, want ownership conflict", err)
	}
}

func cookieDescriptor() routecatalog.Descriptor {
	descriptor := validDescriptor()
	descriptor.Method = http.MethodPost
	descriptor.Path = "/api/browser-control"
	descriptor.OpenAPI.Request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(struct{}{}), Required: true}
	descriptor.OpenAPI.Cookie = &routecatalog.CookieSecurity{
		Name: "admin_refresh", Description: "HTTPS Secure; explicit loopback HTTP exception only",
		Path: "/api", SameSite: "Strict", Required: true, HTTPOnly: true, Secure: true, HostOnly: true, Session: true,
	}
	descriptor.OpenAPI.Headers = []routecatalog.Header{
		{Name: "Origin", Description: "Exact configured browser origin", Required: true},
		{Name: "X-Admin-Request", Description: "Browser request protection", Required: true, Value: "1"},
		{Name: "X-Refresh-Attempt-ID", Description: "Unpredictable refresh attempt identifier", Required: true},
	}
	success := routecatalog.JSONResponse("success", reflect.TypeOf(struct{}{}))
	success.Headers = []routecatalog.Header{
		{Name: "Cache-Control", Description: "Do not store authentication responses", Required: true, Value: "no-store"},
		{Name: "Set-Cookie", Description: "Replace the HttpOnly refresh session cookie", Required: true},
	}
	descriptor.OpenAPI.Responses[http.StatusOK] = success
	return descriptor
}

func TestCookieContractRejectsIncompleteOrInvalidMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*routecatalog.Descriptor)
	}{
		{"cookie name", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.Name = "invalid cookie" }},
		{"cookie description", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.Description = "" }},
		{"cookie path", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.Path = "/" }},
		{"cookie same site", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.SameSite = "None" }},
		{"cookie http only", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.HTTPOnly = false }},
		{"cookie secure", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.Secure = false }},
		{"cookie host only", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.HostOnly = false }},
		{"cookie lifetime", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie.Session = false }},
		{"missing cookie scheme", func(d *routecatalog.Descriptor) { d.OpenAPI.Cookie = nil }},
		{"missing origin", func(d *routecatalog.Descriptor) { d.OpenAPI.Headers = d.OpenAPI.Headers[1:] }},
		{"optional origin", func(d *routecatalog.Descriptor) { d.OpenAPI.Headers[0].Required = false }},
		{"invalid protection value", func(d *routecatalog.Descriptor) { d.OpenAPI.Headers[1].Value = "0" }},
		{"duplicate header", func(d *routecatalog.Descriptor) {
			d.OpenAPI.Headers = append(d.OpenAPI.Headers, routecatalog.Header{Name: "origin", Description: "duplicate"})
		}},
		{"invalid header name", func(d *routecatalog.Descriptor) { d.OpenAPI.Headers[0].Name = "Origin:" }},
		{"missing header description", func(d *routecatalog.Descriptor) { d.OpenAPI.Headers[0].Description = "" }},
		{"invalid header value", func(d *routecatalog.Descriptor) { d.OpenAPI.Headers[0].Value = "value\r\nInjected: true" }},
		{"raw cookie parameter", func(d *routecatalog.Descriptor) {
			d.OpenAPI.Headers = append(d.OpenAPI.Headers, routecatalog.Header{Name: "Cookie", Description: "raw cookie"})
		}},
		{"missing response headers", func(d *routecatalog.Descriptor) {
			response := d.OpenAPI.Responses[200]
			response.Headers = nil
			d.OpenAPI.Responses[200] = response
		}},
		{"missing set cookie", func(d *routecatalog.Descriptor) {
			response := d.OpenAPI.Responses[200]
			response.Headers = response.Headers[:1]
			d.OpenAPI.Responses[200] = response
		}},
		{"cacheable response", func(d *routecatalog.Descriptor) {
			response := d.OpenAPI.Responses[200]
			response.Headers[0].Value = "public"
			d.OpenAPI.Responses[200] = response
		}},
		{"credential example", func(d *routecatalog.Descriptor) {
			response := d.OpenAPI.Responses[200]
			response.Headers[1].Value = "credential-example"
			d.OpenAPI.Responses[200] = response
		}},
		{"error missing no store", func(d *routecatalog.Descriptor) {
			d.OpenAPI.Responses[400] = routecatalog.ErrorResponse("invalid request", httpresponse.RequestInvalidDefinition())
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := cookieDescriptor()
			test.mutate(&descriptor)
			if _, err := routecatalog.New([]routecatalog.Descriptor{descriptor}); err == nil {
				t.Fatal("invalid Cookie/header contract was accepted")
			}
		})
	}
}
