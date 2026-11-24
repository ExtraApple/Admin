package httpadapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	httpadapter "admin/internal/identity/adapters/http"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	"admin/internal/routecatalog"
)

type createUserTestRepo struct {
	profileRepository
	conflict bool
}

func (repository *createUserTestRepo) ExistsIdentityConflict(context.Context, string, string) (bool, error) {
	return repository.conflict, nil
}

type createUserTestAccess struct {
	profileAccess
	all bool
}

func (access *createUserTestAccess) UserScope(context.Context, uint) (domain.UserScope, error) {
	return domain.UserScope{All: access.all}, nil
}
func (access *createUserTestAccess) ManagementUserScope(context.Context, uint) (domain.UserScope, error) {
	return domain.UserScope{All: access.all}, nil
}

type createUserTestHasher struct{}

func (createUserTestHasher) Compare(string, string) error  { return nil }
func (createUserTestHasher) Hash(string) (string, error)   { return "hashed-secret", nil }

func TestAdminCreateUserRouteDescriptorContract(t *testing.T) {
	routes := httpadapter.Routes(nil, nil, nil, nil, nil, 5, httpadapter.BrowserSessionConfig{Origin: "https://admin.example.test", Secure: true})
	var route routecatalog.Descriptor
	found := false
	for _, item := range routes {
		if item.Method == http.MethodPost && item.Path == "/api/admin/users" {
			route = item
			found = true
			break
		}
	}
	if !found {
		t.Fatal("POST /api/admin/users route missing")
	}
	if route.Access != routecatalog.PermissionControlled || route.DefaultPermissionCode != "admin.users.post" || route.Group != "identity" || route.DefaultAuditCategory != "identity" {
		t.Fatalf("route metadata = %#v", route)
	}
	if route.OpenAPI.Request.Schema == nil || route.OpenAPI.Responses[http.StatusOK].Schema == nil {
		t.Fatalf("route OpenAPI schemas missing: %#v", route.OpenAPI)
	}
}

func TestAdminCreateUserHTTPResponses(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &createUserTestRepo{}
		users := application.NewUserService(repo, createUserTestHasher{}, profileTransactions{}, &createUserTestAccess{all: true})
		recorder := serveProfileRequest(t, users, http.MethodPost, "/api/admin/users", `{"username":"new_admin_user","email":"new.user@example.test","password":"Password1!","nickname":"新员工","status":1}`)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "hashed-secret") || strings.Contains(recorder.Body.String(), "Password1!") {
			t.Fatalf("response leaked password: %s", recorder.Body.String())
		}
		var envelope struct {
			Code      int                   `json:"code"`
			ErrorCode string                `json:"error_code"`
			Msg       string                `json:"msg"`
			Data      application.AdminUser `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Code != 200 || envelope.ErrorCode != "" || envelope.Msg != "success" || envelope.Data.ID != 42 || envelope.Data.Username != "new_admin_user" || envelope.Data.AccessVersion != 1 || envelope.Data.Avatar != "/api/avatars/default" {
			t.Fatalf("envelope = %#v", envelope)
		}
	})

	t.Run("structural validation returns 400", func(t *testing.T) {
		users := application.NewUserService(&createUserTestRepo{}, createUserTestHasher{}, profileTransactions{}, &createUserTestAccess{all: true})
		for _, body := range []string{
			`{"username":"ab","email":"valid@example.test","password":"Password1!"}`,
			`{"username":"valid","email":"not-an-email","password":"Password1!"}`,
			`{"username":"valid","email":"valid@example.test","password":"Aa1!"}`,
			`{"username":"valid","email":"valid@example.test","password":"Password1!","status":2}`,
		} {
			recorder := serveProfileRequest(t, users, http.MethodPost, "/api/admin/users", body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("body %s status = %d, want 400, body = %s", body, recorder.Code, recorder.Body.String())
			}
		}
	})

	t.Run("oversized password returns 422 field error", func(t *testing.T) {
		users := application.NewUserService(&createUserTestRepo{}, createUserTestHasher{}, profileTransactions{}, &createUserTestAccess{all: true})
		oversized := "Aa1!" + strings.Repeat("密", 23)
		recorder := serveProfileRequest(t, users, http.MethodPost, "/api/admin/users", `{"username":"valid_user","email":"valid@example.test","password":"`+oversized+`"}`)
		if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "IDENTITY_PASSWORD_INVALID") {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("non-all scope returns 403", func(t *testing.T) {
		users := application.NewUserService(&createUserTestRepo{conflict: true}, createUserTestHasher{}, profileTransactions{}, &createUserTestAccess{all: false})
		recorder := serveProfileRequest(t, users, http.MethodPost, "/api/admin/users", `{"username":"valid_user","email":"valid@example.test","password":"Password1!"}`)
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "IDENTITY_PERMISSION_DENIED") {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("conflict returns 409", func(t *testing.T) {
		users := application.NewUserService(&createUserTestRepo{conflict: true}, createUserTestHasher{}, profileTransactions{}, &createUserTestAccess{all: true})
		recorder := serveProfileRequest(t, users, http.MethodPost, "/api/admin/users", `{"username":"valid_user","email":"valid@example.test","password":"Password1!"}`)
		if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "IDENTITY_CONFLICT") {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})
}
