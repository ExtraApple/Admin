package jwtadapter_test

import (
	"context"
	"testing"

	jwtadapter "admin/internal/identity/adapters/jwt"
	"admin/internal/identity/application"
)

func TestJWTServicePreservesAccessAndRefreshPurpose(t *testing.T) {
	service := jwtadapter.NewService(jwtadapter.Config{Secret: "identity-secret", AccessExpireMins: 15, RefreshExpireMins: 60})
	pair, err := service.Issue(context.Background(), application.TokenIssue{UserID: 7, TokenVersion: 3, Roles: []string{"admin"}, Permissions: []string{"users.read"}})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	refresh, err := service.Parse(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("Parse(refresh) error = %v", err)
	}
	if refresh.UserID != 7 || refresh.TokenVersion != 3 || refresh.Purpose != application.TokenPurposeRefresh {
		t.Fatalf("refresh claims = %#v", refresh)
	}
	access, err := service.Parse(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatalf("Parse(access) error = %v", err)
	}
	if access.Purpose != application.TokenPurposeAccess {
		t.Fatalf("access purpose = %q", access.Purpose)
	}
}
