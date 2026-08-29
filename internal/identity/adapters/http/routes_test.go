package httpadapter_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "admin/internal/identity/adapters/http"
	"admin/internal/identity/application"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

func TestRoutesDeclareIdentityAuthenticationSurface(t *testing.T) {
	routes := httpadapter.Routes(nil, nil, nil, nil, nil, 5)
	want := map[string]routecatalog.AccessLevel{
		"GET /api/captcha":                           routecatalog.Public,
		"POST /api/register":                         routecatalog.Public,
		"POST /api/login":                            routecatalog.Public,
		"POST /api/refresh":                          routecatalog.Public,
		"GET /api/avatars/default":                   routecatalog.Public,
		"GET /api/avatars/:user_id":                  routecatalog.Public,
		"GET /api/user/context":                      routecatalog.Authenticated,
		"GET /api/user/info":                         routecatalog.Authenticated,
		"PUT /api/user/info":                         routecatalog.Authenticated,
		"PUT /api/user/password":                     routecatalog.Authenticated,
		"POST /api/user/avatar":                      routecatalog.Authenticated,
		"DELETE /api/user/avatar":                    routecatalog.Authenticated,
		"POST /api/user/logout":                      routecatalog.Authenticated,
		"GET /api/admin/users":                       routecatalog.PermissionControlled,
		"PUT /api/admin/users/:id":                   routecatalog.PermissionControlled,
		"DELETE /api/admin/users/:id":                routecatalog.PermissionControlled,
		"PUT /api/admin/users/:id/status":            routecatalog.PermissionControlled,
		"PUT /api/admin/users/:id/kick":              routecatalog.PermissionControlled,
		"POST /api/user/email-verifications":         routecatalog.Authenticated,
		"POST /api/user/email-verifications/confirm": routecatalog.Authenticated,
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

func TestEmailVerificationRoutesDeclareSafeSchemasAndStableErrors(t *testing.T) {
	routes := httpadapter.Routes(nil, nil, nil, nil, nil, 5)
	byPath := make(map[string]routecatalog.Descriptor, len(routes))
	for _, route := range routes {
		byPath[route.Method+" "+route.Path] = route
	}
	confirm, ok := byPath["POST /api/user/email-verifications/confirm"]
	if !ok || confirm.Access != routecatalog.Authenticated {
		t.Fatal("confirm email verification route is not authenticated")
	}
	if confirm.OpenAPI.Request.Schema == nil || confirm.OpenAPI.Responses[422].Description == "" || confirm.OpenAPI.Responses[503].Description == "" {
		t.Fatalf("confirm route OpenAPI = %#v", confirm.OpenAPI)
	}
	resend, ok := byPath["POST /api/user/email-verifications"]
	if !ok || resend.OpenAPI.Responses[429].Description == "" {
		t.Fatal("resend route does not declare throttling response")
	}
	for _, response := range []routecatalog.Response{resend.OpenAPI.Responses[429], resend.OpenAPI.Responses[503]} {
		for _, definition := range response.Errors {
			if definition.Code == "IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED" && definition.DataSchema != nil {
				t.Fatal("delivery failure exposes response data")
			}
		}
	}
}

func TestEmailVerificationConfirmErrorUsesSafeEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verification := application.NewEmailVerificationService(nil, nil)
	routes := httpadapter.RoutesWithEmailVerification(nil, nil, nil, nil, nil, 5, 0, verification)
	var confirm routecatalog.Descriptor
	for _, route := range routes {
		if route.Path == "/api/user/email-verifications/confirm" {
			confirm = route
			break
		}
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/email-verifications/confirm", strings.NewReader(`{"token":"raw-token-secret"}`))
	context.Set("userID", uint(7))
	confirm.Handler(context)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("confirm status = %d, want 422", recorder.Code)
	}
	body := recorder.Body.String()
	for _, secret := range []string{"raw-token-secret", "token_hash", "smtp"} {
		if strings.Contains(body, secret) {
			t.Fatalf("confirm response leaked %q: %s", secret, body)
		}
	}
	for _, field := range []string{`"code"`, `"error_code"`, `"msg"`, `"data"`} {
		if !strings.Contains(body, field) {
			t.Fatalf("confirm response missing %s: %s", field, body)
		}
	}
}
