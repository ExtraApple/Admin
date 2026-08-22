package apidoc_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"admin/internal/apidoc"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type metadataFake struct{ entries []apidoc.Metadata }

func (fake metadataFake) Snapshot(context.Context) ([]apidoc.Metadata, error) {
	return append([]apidoc.Metadata(nil), fake.entries...), nil
}

type createWidgetRequest struct {
	Name string `json:"name" binding:"required"`
}
type widgetResponse struct {
	ID uint `json:"id"`
}

func TestDocumentUsesRouteCatalogAndMetadataSnapshot(t *testing.T) {
	catalog, err := routecatalog.New([]routecatalog.Descriptor{
		{
			Method: http.MethodGet, Path: "/api/admin/widgets/:id", Access: routecatalog.PermissionControlled,
			Handler: func(*gin.Context) {}, Name: "List Widgets", Group: "widgets", DefaultPermissionCode: "widgets.list", DefaultAuditCategory: "widgets",
			OpenAPI: routecatalog.Operation{Summary: "List Widgets", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody}, Responses: map[int]routecatalog.Response{http.StatusOK: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(widgetResponse{})}}},
		},
		{
			Method: http.MethodPost, Path: "/api/public/widgets", Access: routecatalog.Public,
			Handler: func(*gin.Context) {}, Name: "Create Widget", Group: "widgets", DefaultAuditCategory: "widgets",
			OpenAPI: routecatalog.Operation{Summary: "Create Widget", Request: routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(createWidgetRequest{}), Required: true}, Responses: map[int]routecatalog.Response{http.StatusOK: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(widgetResponse{})}}},
		},
	})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	service := apidoc.New(catalog, metadataFake{entries: []apidoc.Metadata{{Method: "GET", Path: "/api/admin/widgets/:id", Name: "Managed Widgets", Group: "managed", Remark: "runtime remark", Status: 0, NeedAuth: 0}}}, apidoc.Config{Title: "Admin", Version: "2.0.0"})
	document, err := service.Document(context.Background(), "https://admin.test")
	if err != nil {
		t.Fatalf("build OpenAPI document: %v", err)
	}
	if document["openapi"] != "3.0.3" {
		t.Fatalf("OpenAPI version = %#v", document["openapi"])
	}
	paths := document["paths"].(map[string]any)
	protected := paths["/api/admin/widgets/{id}"].(map[string]any)["get"].(map[string]any)
	if protected["summary"] != "Managed Widgets" || protected["description"] != "runtime remark" || protected["deprecated"] != true {
		t.Fatalf("metadata overlay = %#v", protected)
	}
	if _, ok := protected["security"]; !ok {
		t.Fatal("PermissionControlled route lost security requirement")
	}
	public := paths["/api/public/widgets"].(map[string]any)["post"].(map[string]any)
	if _, ok := public["security"]; ok {
		t.Fatal("Public route should not require security")
	}
	requestBody := public["requestBody"].(map[string]any)
	if requestBody["required"] != true {
		t.Fatal("JSON request body should be required")
	}
	if document["servers"].([]map[string]string)[0]["url"] != "https://admin.test" {
		t.Fatal("server URL was not applied")
	}
}

func TestDocumentDescribesMultipartUploadAndBinaryResponse(t *testing.T) {
	catalog, err := routecatalog.New([]routecatalog.Descriptor{
		{
			Method: http.MethodPost, Path: "/api/admin/files", Access: routecatalog.PermissionControlled,
			Handler: func(*gin.Context) {}, Name: "Upload", Group: "file", DefaultPermissionCode: "files.post", DefaultAuditCategory: "file",
			OpenAPI: routecatalog.Operation{Summary: "Upload", Request: routecatalog.RequestBody{Kind: routecatalog.MultipartBody, Required: true, FileField: "file", FileDescription: "Validated managed file"}, Responses: map[int]routecatalog.Response{http.StatusOK: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(widgetResponse{})}}},
		},
		{
			Method: http.MethodGet, Path: "/api/admin/files/:id/download", Access: routecatalog.PermissionControlled,
			Handler: func(*gin.Context) {}, Name: "Download", Group: "file", DefaultPermissionCode: "files.id.download.get", DefaultAuditCategory: "file",
			OpenAPI: routecatalog.Operation{Summary: "Download", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody}, Responses: map[int]routecatalog.Response{http.StatusOK: {Description: "file", Kind: routecatalog.BinaryBody, ContentTypes: []string{"application/octet-stream"}}}},
		},
	})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	document, err := apidoc.New(catalog, nil, apidoc.Config{}).Document(context.Background(), "")
	if err != nil {
		t.Fatalf("build document: %v", err)
	}
	paths := document["paths"].(map[string]any)
	upload := paths["/api/admin/files"].(map[string]any)["post"].(map[string]any)
	request := upload["requestBody"].(map[string]any)
	fileSchema := request["content"].(map[string]any)["multipart/form-data"].(map[string]any)["schema"].(map[string]any)
	file := fileSchema["properties"].(map[string]any)["file"].(map[string]any)
	if file["format"] != "binary" || file["description"] != "Validated managed file" {
		t.Fatalf("file schema = %#v", file)
	}
	download := paths["/api/admin/files/{id}/download"].(map[string]any)["get"].(map[string]any)
	responses := download["responses"].(map[string]any)
	if _, ok := responses["200"].(map[string]any)["content"].(map[string]any)["application/octet-stream"]; !ok {
		t.Fatal("binary response content type missing")
	}
}

func TestDocumentDescribesEnvelopeAndErrorCodeEnums(t *testing.T) {
	type responseData struct {
		ID uint `json:"id"`
	}
	descriptor := routecatalog.Descriptor{
		Method: http.MethodGet, Path: "/api/widgets", Access: routecatalog.Public,
		Handler: func(*gin.Context) {}, Name: "Widgets", Group: "widgets", DefaultAuditCategory: "widgets",
		OpenAPI: routecatalog.Operation{
			Summary: "Widgets", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				http.StatusOK:           {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[responseData]{})},
				http.StatusUnauthorized: {Description: "authentication failed", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), Errors: []httpresponse.ErrorDefinition{{Owner: "identity", Code: "AUTHN_INVALID", Status: http.StatusUnauthorized, Message: "authentication failed"}}},
			},
		},
	}
	catalog, err := routecatalog.New([]routecatalog.Descriptor{descriptor})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	document, err := apidoc.New(catalog, nil, apidoc.Config{}).Document(context.Background(), "")
	if err != nil {
		t.Fatalf("build document: %v", err)
	}
	operation := document["paths"].(map[string]any)["/api/widgets"].(map[string]any)["get"].(map[string]any)
	responses := operation["responses"].(map[string]any)
	successSchema := responses["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	if successSchema["properties"].(map[string]any)["error_code"].(map[string]any)["enum"].([]string)[0] != "" {
		t.Fatalf("success error_code schema = %#v", successSchema)
	}
	errorSchema := responses["401"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	if got := errorSchema["properties"].(map[string]any)["error_code"].(map[string]any)["enum"].([]string); len(got) != 1 || got[0] != "AUTHN_INVALID" {
		t.Fatalf("error_code enum = %#v", got)
	}
}
