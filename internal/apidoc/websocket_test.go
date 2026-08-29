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

type websocketResponse struct{}

func TestDocumentDescribesNativeWebSocketUpgradeAndPreUpgradeErrors(t *testing.T) {
	descriptor := routecatalog.Descriptor{
		Method: http.MethodGet, Path: "/api/user/messages/ws", Access: routecatalog.Authenticated,
		Handler: func(*gin.Context) {}, Name: "Upgrade Message WebSocket", Group: "messaging", DefaultAuditCategory: "message",
		OpenAPI: routecatalog.Operation{
			Summary: "Upgrade Message WebSocket", Protocol: "websocket",
			Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				http.StatusSwitchingProtocols: {Description: "native RFC 6455 WebSocket protocol", Kind: routecatalog.NoBody},
				http.StatusUnauthorized:       {Description: "ticket invalid", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(httpresponse.Envelope[any]{}), Errors: []httpresponse.ErrorDefinition{{Owner: "messaging", Code: "MSG_WS_TICKET_INVALID", Status: http.StatusUnauthorized, Message: "ticket invalid"}}},
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
	operation := document["paths"].(map[string]any)["/api/user/messages/ws"].(map[string]any)["get"].(map[string]any)
	if operation["x-native-protocol"] != "websocket" || operation["description"] == "" {
		t.Fatalf("WebSocket operation = %#v", operation)
	}
	responses := operation["responses"].(map[string]any)
	if _, ok := responses["101"]; !ok {
		t.Fatalf("switching protocols response missing: %#v", responses)
	}
	if _, ok := responses["401"]; !ok {
		t.Fatalf("ticket error response missing: %#v", responses)
	}
}
