package apidoc_test

import (
	"context"
	"testing"

	"admin/internal/apidoc"
	identityhttp "admin/internal/identity/adapters/http"
	"admin/internal/routecatalog"
)

func TestProfileSelfServiceDocumentProjectsIdentityCatalog(t *testing.T) {
	catalog, err := routecatalog.New(identityhttp.Routes(nil, nil, nil, nil, nil, 7, identityhttp.BrowserSessionConfig{Origin: "https://admin.example.test", Secure: true}))
	if err != nil {
		t.Fatal(err)
	}
	document, err := apidoc.New(catalog, metadataFake{}, apidoc.Config{}).Document(context.Background(), "https://admin.example.test")
	if err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	policy := paths["/api/user/avatar-upload-policy"].(map[string]any)["get"].(map[string]any)
	if _, exists := policy["security"]; !exists {
		t.Fatal("policy document lost authentication")
	}
	responses := policy["responses"].(map[string]any)
	success := responses["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	properties := success["properties"].(map[string]any)
	data := properties["data"].(map[string]any)["properties"].(map[string]any)
	maxBytes := data["max_bytes"].(map[string]any)
	if len(properties) != 4 || len(data) != 1 || maxBytes["type"] != "integer" || maxBytes["minimum"] != float64(1) {
		t.Fatalf("policy schema = %#v", success)
	}
	for _, endpoint := range []struct {
		path, method string
		fields       []string
	}{
		{"/api/register", "post", []string{"password"}},
		{"/api/user/password", "put", []string{"new_password", "confirm_password"}},
	} {
		operation := paths[endpoint.path].(map[string]any)[endpoint.method].(map[string]any)
		request := operation["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		fields := request["properties"].(map[string]any)
		for _, name := range endpoint.fields {
			field := fields[name].(map[string]any)
			if field["minLength"] != 6 {
				t.Fatalf("%s %s lost structural minimum: %#v", endpoint.path, name, field)
			}
			if _, hasCharacterMaximum := field["maxLength"]; hasCharacterMaximum {
				t.Fatalf("%s %s misrepresents a UTF-8 byte policy as a character maximum", endpoint.path, name)
			}
		}
		for _, status := range []string{"400", "422", "500"} {
			if _, exists := operation["responses"].(map[string]any)[status]; !exists {
				t.Fatalf("%s lacks %s error projection", endpoint.path, status)
			}
		}
	}
}
