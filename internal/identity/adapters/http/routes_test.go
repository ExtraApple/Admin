package httpadapter_test

import (
	"testing"

	httpadapter "admin/internal/identity/adapters/http"
	"admin/internal/routecatalog"
)

func TestRoutesDeclareIdentityAuthenticationSurface(t *testing.T) {
	routes := httpadapter.Routes(nil, nil, nil, nil, nil, 5)
	want := map[string]routecatalog.AccessLevel{
		"GET /api/captcha":                routecatalog.Public,
		"POST /api/register":              routecatalog.Public,
		"POST /api/login":                 routecatalog.Public,
		"POST /api/refresh":               routecatalog.Public,
		"GET /api/avatars/default":        routecatalog.Public,
		"GET /api/avatars/:user_id":       routecatalog.Public,
		"GET /api/user/context":           routecatalog.Authenticated,
		"GET /api/user/info":              routecatalog.Authenticated,
		"PUT /api/user/info":              routecatalog.Authenticated,
		"PUT /api/user/password":          routecatalog.Authenticated,
		"POST /api/user/avatar":           routecatalog.Authenticated,
		"DELETE /api/user/avatar":         routecatalog.Authenticated,
		"POST /api/user/logout":           routecatalog.Authenticated,
		"GET /api/admin/users":            routecatalog.PermissionControlled,
		"PUT /api/admin/users/:id":        routecatalog.PermissionControlled,
		"DELETE /api/admin/users/:id":     routecatalog.PermissionControlled,
		"PUT /api/admin/users/:id/status": routecatalog.PermissionControlled,
		"PUT /api/admin/users/:id/kick":   routecatalog.PermissionControlled,
	}
	got := make(map[string]routecatalog.AccessLevel, len(routes))
	for _, route := range routes {
		got[route.Method+" "+route.Path] = route.Access
	}
	for key, access := range want {
		if got[key] != access {
			t.Fatalf("route %s access = %d, want %d", key, got[key], access)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("Identity routes = %d, want %d", len(got), len(want))
	}
}
