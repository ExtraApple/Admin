package httpadapter

import (
	"net/http"
	"strings"
	"testing"

	"admin/internal/routecatalog"
)

func messageResponseHasCode(response routecatalog.Response, code string) bool {
	for _, definition := range response.Errors {
		if definition.Code == code {
			return true
		}
	}
	return false
}

func TestMessagingRouteDescriptorsCoverAllConfiguredHTTPAndWebSocketEntrypoints(t *testing.T) {
	routes := append([]routecatalog.Descriptor{}, Routes(nil)...)
	routes = append(routes, WebSocketRoutes(nil, nil, "https://admin.example.test")...)
	routes = append(routes, AdminRoutes(nil)...)
	catalog, err := routecatalog.New(routes)
	if err != nil {
		t.Fatalf("build messaging route catalog: %v", err)
	}
	want := map[string]bool{
		"POST /api/user/messages/private":                 true,
		"GET /api/user/messages":                          true,
		"GET /api/user/messages/unread-count":             true,
		"GET /api/user/messages/:id":                      true,
		"PUT /api/user/messages/:id/read":                 true,
		"DELETE /api/user/messages/:id/inbox":             true,
		"POST /api/user/messages/:id/revoke":              true,
		"POST /api/user/messages/images":                  true,
		"GET /api/user/messages/:id/images/:image_id":     true,
		"POST /api/user/messages/ws-ticket":               true,
		"GET /api/user/messages/ws":                       true,
		"GET /api/admin/messages":                         true,
		"POST /api/admin/messages/broadcast":              true,
		"POST /api/admin/messages/:id/revoke":             true,
		"GET /api/admin/announcements":                    true,
		"POST /api/admin/announcements":                   true,
		"GET /api/admin/announcements/:id":                true,
		"PUT /api/admin/announcements/:id":                true,
		"POST /api/admin/announcements/:id/publish":       true,
		"POST /api/admin/announcements/:id/revoke":        true,
		"GET /api/admin/message-categories":               true,
		"POST /api/admin/message-categories":              true,
		"PUT /api/admin/message-categories/:id":           true,
		"DELETE /api/admin/message-categories/:id":        true,
		"POST /api/admin/messages/images":                 true,
		"GET /api/admin/message-outboxes":                 true,
		"POST /api/admin/message-outboxes/:id/replay":     true,
		"GET /api/admin/message-dead-letters":             true,
		"POST /api/admin/message-dead-letters/:id/replay": true,
		"DELETE /api/admin/message-dead-letters/:id":      true,
	}
	if len(catalog.Snapshot()) != len(want) {
		t.Fatalf("descriptor count=%d want=%d", len(catalog.Snapshot()), len(want))
	}
	for _, descriptor := range catalog.Snapshot() {
		key := descriptor.Method + " " + descriptor.Path
		if !want[key] {
			t.Fatalf("unexpected messaging route %s", key)
		}
		if descriptor.DefaultAuditCategory != "message" || descriptor.OpenAPI.Summary == "" {
			t.Fatalf("incomplete metadata for %s", key)
		}
	}
	ws := findMessageRoute(t, catalog.Snapshot(), http.MethodGet, "/api/user/messages/ws")
	if ws.Access != routecatalog.Public || ws.OpenAPI.Protocol != "websocket" || ws.OpenAPI.QuerySchema == nil || ws.OpenAPI.Responses[http.StatusSwitchingProtocols].Kind != routecatalog.NoBody || !strings.Contains(ws.OpenAPI.Description, "ticket") {
		t.Fatalf("WebSocket descriptor = %#v", ws)
	}
	if !messageResponseHasCode(ws.OpenAPI.Responses[http.StatusUnauthorized], CodeTicketInvalid) || !messageResponseHasCode(ws.OpenAPI.Responses[http.StatusServiceUnavailable], CodeWebSocketUnavailable) {
		t.Fatalf("WebSocket error responses = %#v", ws.OpenAPI.Responses)
	}
	ticket := findMessageRoute(t, catalog.Snapshot(), http.MethodPost, "/api/user/messages/ws-ticket")
	if ticket.Access != routecatalog.Authenticated {
		t.Fatalf("ticket route access = %v, want Authenticated", ticket.Access)
	}
	for _, descriptor := range catalog.Snapshot() {
		if descriptor.Access < routecatalog.Public || descriptor.Access > routecatalog.PermissionControlled {
			t.Fatalf("route %s %s introduced access level %v", descriptor.Method, descriptor.Path, descriptor.Access)
		}
	}
}
