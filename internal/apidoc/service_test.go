package apidoc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"admin/internal/apidoc"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type metadataFake struct {
	entries []apidoc.Metadata
	err     error
}

func (fake metadataFake) Snapshot(context.Context) ([]apidoc.Metadata, error) {
	if fake.err != nil {
		return nil, fake.err
	}
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

func TestOpenAPIHandlerHidesMetadataGenerationFailure(t *testing.T) {
	const (
		internalCause = "database password=secret generator=private"
		secretMarker  = "metadata-secret-marker"
	)
	catalog, err := routecatalog.New([]routecatalog.Descriptor{{
		Method: http.MethodGet, Path: "/api/widgets", Access: routecatalog.Public,
		Handler: func(*gin.Context) {}, Name: "Widgets", Group: "widgets", DefaultAuditCategory: "widgets",
		OpenAPI: routecatalog.Operation{
			Summary: "Widgets", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{http.StatusOK: {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(widgetResponse{})}},
		},
	}})
	if err != nil {
		t.Fatalf("build Route Catalog: %v", err)
	}
	service := apidoc.New(catalog, metadataFake{err: errors.New(internalCause + " " + secretMarker)}, apidoc.Config{})
	router := gin.New()
	router.GET("/docs/openapi.json", service.OpenAPI)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("OpenAPI failure status = %d, want 500; body=%s", response.Code, response.Body.String())
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode OpenAPI failure envelope: %v; body=%s", err, response.Body.String())
	}
	if len(envelope) != 4 {
		t.Fatalf("OpenAPI failure envelope fields = %#v, want exactly four fields", envelope)
	}
	for _, field := range []string{"code", "error_code", "msg", "data"} {
		if _, ok := envelope[field]; !ok {
			t.Fatalf("OpenAPI failure envelope missing %q: %#v", field, envelope)
		}
	}
	var code int
	if err := json.Unmarshal(envelope["code"], &code); err != nil || code != http.StatusInternalServerError {
		t.Fatalf("OpenAPI failure code = %s, want 500", envelope["code"])
	}
	var errorCode string
	if err := json.Unmarshal(envelope["error_code"], &errorCode); err != nil || errorCode != "HTTP_INTERNAL_ERROR" {
		t.Fatalf("OpenAPI failure error_code = %s, want HTTP_INTERNAL_ERROR", envelope["error_code"])
	}
	if string(envelope["data"]) != "null" {
		t.Fatalf("OpenAPI failure data = %s, want null", envelope["data"])
	}
	if body := response.Body.String(); strings.Contains(body, internalCause) || strings.Contains(body, secretMarker) {
		t.Fatalf("OpenAPI failure leaked internal error: %s", body)
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

func TestDocumentProjectsCookieControlWithoutPathSpecificRules(t *testing.T) {
	type accessData struct {
		AccessToken string `json:"access_token"`
		SessionID   string `json:"session_id"`
	}
	var descriptors []routecatalog.Descriptor
	for _, operation := range []struct {
		path     string
		required bool
		data     reflect.Type
	}{
		{"/api/browser/start", false, reflect.TypeOf(accessData{})},
		{"/api/browser/resume", true, reflect.TypeOf(accessData{})},
		{"/api/browser/end", false, nil},
	} {
		success := routecatalog.JSONResponse("success", operation.data)
		success.Headers = []routecatalog.Header{
			{Name: "Cache-Control", Description: "Do not cache credentials", Required: true, Value: "no-store"},
			{Name: "Set-Cookie", Description: "Set or clear the same scoped session Cookie", Required: true},
		}
		responses := map[int]routecatalog.Response{200: success}
		for _, definition := range []httpresponse.ErrorDefinition{
			httpresponse.RequestInvalidDefinition(),
			{Owner: "identity", Code: "AUTHN_REFRESH_TOKEN_INVALID", Status: 401, Message: "refresh token is invalid"},
			{Owner: "identity", Code: "AUTHN_CSRF_INVALID", Status: 403, Message: "browser request verification failed"},
			{Owner: "identity", Code: "AUTHN_REFRESH_SUPERSEDED", Status: 409, Message: "refresh attempt was superseded"},
			{Owner: "identity", Code: "IDENTITY_INTERNAL_ERROR", Status: 500, Message: "identity operation failed"},
		} {
			response := routecatalog.ErrorResponse(definition.Message, definition)
			response.Headers = []routecatalog.Header{{Name: "Cache-Control", Description: "Do not cache authentication errors", Required: true, Value: "no-store"}}
			responses[definition.Status] = response
		}
		descriptors = append(descriptors, routecatalog.Descriptor{
			Method: http.MethodPost, Path: operation.path, Access: routecatalog.Public,
			Handler: func(*gin.Context) {}, Name: operation.path, Group: "browser", DefaultAuditCategory: "identity",
			OpenAPI: routecatalog.Operation{
				Summary: operation.path,
				Cookie:  &routecatalog.CookieSecurity{Name: "admin_refresh", Description: "Secure on HTTPS; only explicit loopback HTTP may disable Secure", Path: "/api", SameSite: "Strict", Required: operation.required, HTTPOnly: true, Secure: true, HostOnly: true, Session: true},
				Headers: []routecatalog.Header{
					{Name: "Origin", Description: "Exact configured origin", Required: true},
					{Name: "X-Admin-Request", Description: "Browser request verification", Required: true, Value: "1"},
					{Name: "X-Refresh-Attempt-ID", Description: "Unpredictable attempt identifier; reuse only within 30 seconds", Required: true},
				},
				Request:   routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(struct{}{}), Required: true},
				Responses: responses,
			},
		})
	}
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		t.Fatalf("create Cookie catalog: %v", err)
	}
	document, err := apidoc.New(catalog, metadataFake{entries: []apidoc.Metadata{
		{Method: "POST", Path: "/api/browser/end", Status: 1, NeedAuth: 1, PermissionCode: "historical.setting"},
	}}, apidoc.Config{}).Document(context.Background(), "https://admin.test")
	if err != nil {
		t.Fatalf("generate Cookie document: %v", err)
	}
	schemes := document["components"].(map[string]any)["securitySchemes"].(map[string]any)
	cookie := schemes["admin_refresh"].(map[string]any)
	if cookie["type"] != "apiKey" || cookie["in"] != "cookie" || cookie["name"] != "admin_refresh" {
		t.Fatalf("Cookie security scheme = %#v", cookie)
	}
	wantAttributes := map[string]any{"path": "/api", "httpOnly": true, "secure": true, "sameSite": "Strict", "hostOnly": true, "session": true}
	if !reflect.DeepEqual(cookie["x-cookie-attributes"], wantAttributes) {
		t.Fatalf("Cookie attributes = %#v", cookie["x-cookie-attributes"])
	}
	paths := document["paths"].(map[string]any)
	for _, descriptor := range descriptors {
		operation := paths[descriptor.Path].(map[string]any)["post"].(map[string]any)
		security := operation["security"].([]map[string][]string)
		if descriptor.OpenAPI.Cookie.Required {
			if len(security) != 1 || len(security[0]) != 1 {
				t.Fatalf("required Cookie security = %#v", security)
			}
		} else if len(security) != 2 || len(security[0]) != 0 {
			t.Fatalf("optional Cookie security = %#v", security)
		}
		for _, requirement := range security {
			if _, found := requirement["BearerAuth"]; found {
				t.Fatal("Public Cookie operation gained a Bearer prerequisite from Metadata")
			}
		}
		parameters := operation["parameters"].([]map[string]any)
		if len(parameters) != 3 || parameters[0]["in"] != "header" || parameters[0]["required"] != true || parameters[1]["schema"].(map[string]any)["enum"].([]string)[0] != "1" {
			t.Fatalf("required request headers = %#v", parameters)
		}
		requestSchema := operation["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if requestSchema["additionalProperties"] != false || len(requestSchema["properties"].(map[string]any)) != 0 {
			t.Fatalf("empty-object request must reject the old credential body: %#v", requestSchema)
		}
		responses := operation["responses"].(map[string]any)
		for _, status := range []string{"200", "400", "401", "403", "409", "500"} {
			response := responses[status].(map[string]any)
			headers := response["headers"].(map[string]any)
			if headers["Cache-Control"].(map[string]any)["schema"].(map[string]any)["enum"].([]string)[0] != "no-store" {
				t.Fatalf("response %s lost no-store: %#v", status, response)
			}
			if status == "200" {
				if headers["Set-Cookie"].(map[string]any)["required"] != true {
					t.Fatal("success lost required Set-Cookie")
				}
			} else if _, found := headers["Set-Cookie"]; found {
				t.Fatalf("response %s invented a Cookie write", status)
			}
			if status != "200" {
				schema := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
				properties := schema["properties"].(map[string]any)
				want := map[string]string{"400": "HTTP_REQUEST_INVALID", "401": "AUTHN_REFRESH_TOKEN_INVALID", "403": "AUTHN_CSRF_INVALID", "409": "AUTHN_REFRESH_SUPERSEDED", "500": "IDENTITY_INTERNAL_ERROR"}
				if len(properties) != 4 || properties["error_code"].(map[string]any)["enum"].([]string)[0] != want[status] || properties["data"].(map[string]any)["nullable"] != true {
					t.Fatalf("response %s lost the safe four-field error contract: %#v", status, schema)
				}
			}
		}
		success := responses["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		data := success["properties"].(map[string]any)["data"].(map[string]any)
		if descriptor.OpenAPI.Responses[200].DataSchema == nil {
			if data["nullable"] != true || !reflect.DeepEqual(data["enum"], []any{nil}) {
				t.Fatalf("logout data must be null: %#v", data)
			}
		} else {
			properties := data["properties"].(map[string]any)
			if len(properties) != 2 || properties["access_token"] == nil || properties["session_id"] == nil {
				t.Fatalf("Cookie response leaked or omitted credential fields: %#v", data)
			}
		}
	}
}
