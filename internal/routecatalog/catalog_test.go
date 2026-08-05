package routecatalog_test

import (
	"reflect"
	"strings"
	"testing"

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
			Path:                 "api/health/",
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
	if first[0].Method != "GET" || first[0].Path != "/api/health" || first[0].Access != routecatalog.Public || first[0].Handler == nil {
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
	second.Path = "api/health/"
	second.Name = "Duplicate Health"

	_, err := routecatalog.New([]routecatalog.Descriptor{first, second})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") || !strings.Contains(err.Error(), "GET /api/health") {
		t.Fatalf("catalog error = %v, want normalized duplicate route", err)
	}
}

func validDescriptor() routecatalog.Descriptor {
	return routecatalog.Descriptor{
		Method:               "GET",
		Path:                 "/api/health",
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
