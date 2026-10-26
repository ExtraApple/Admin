package jwtadapter_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

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

func TestJWTReconstructsExactCommittedSigningResult(t *testing.T) {
	service := jwtadapter.NewService(jwtadapter.Config{Secret: "identity-secret", AccessExpireMins: 15, RefreshExpireMins: 60})
	ctx := context.Background()
	material, err := service.Prepare(ctx, application.TokenIssue{UserID: 7, TokenVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Issue(ctx, material)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Issue(ctx, material)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccessToken != second.AccessToken || first.RefreshToken != second.RefreshToken || first.SessionID != second.SessionID {
		t.Fatal("fixed material produced a different signed result")
	}
	access, err := service.Parse(ctx, first.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := service.Parse(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if access.SessionID != material.SessionID || refresh.SessionID != material.SessionID || refresh.RefreshID != material.RefreshID || refresh.Generation != 1 || !access.ExpiresAt.Equal(material.AccessExpiresAt) || !refresh.ExpiresAt.Equal(material.RefreshExpiresAt) {
		t.Fatal("signed result lost a fixed session binding or deadline")
	}
}

func TestJWTRejectsLegacyAndIncompleteBindings(t *testing.T) {
	service := jwtadapter.NewService(jwtadapter.Config{Secret: "identity-secret", AccessExpireMins: 15, RefreshExpireMins: 60})
	now := time.Now().UTC().Truncate(time.Second)
	for _, field := range []string{"user_id", "token_version", "token_type", "sid", "iat", "exp", "refresh_generation", "jti"} {
		t.Run(field, func(t *testing.T) {
			claims := jwt.MapClaims{"user_id": 7, "token_version": 3, "token_type": "refresh", "sid": "fixture-session-000001", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "refresh_generation": 1, "jti": "fixture-refresh-000001"}
			delete(claims, field)
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("identity-secret"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Parse(context.Background(), token); err == nil {
				t.Fatal("incomplete token accepted")
			}
			if _, err := service.ParseRefresh(context.Background(), token); err == nil {
				t.Fatal("recovery accepted incomplete token")
			}
		})
	}
}

func TestJWTExpiredRefreshIsOnlyExposedForCommittedRecovery(t *testing.T) {
	service := jwtadapter.NewService(jwtadapter.Config{Secret: "identity-secret", AccessExpireMins: 15, RefreshExpireMins: 60})
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	pair, err := service.Issue(ctx, application.TokenIssue{UserID: 7, TokenVersion: 3, IssuedAt: now.Add(-2 * time.Hour), AccessExpiresAt: now.Add(-time.Hour), RefreshExpiresAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Parse(ctx, pair.RefreshToken); err == nil {
		t.Fatal("expired refresh accepted for ordinary parsing")
	}
	if _, err := service.ParseRefresh(ctx, pair.AccessToken); err == nil {
		t.Fatal("expired access accepted for recovery")
	}
	claims, err := service.ParseRefresh(ctx, pair.RefreshToken)
	if err != nil || !claims.Expired || claims.Purpose != application.TokenPurposeRefresh {
		t.Fatal("expired signed predecessor not exposed to the atomic recovery check")
	}
}

func TestRefreshCookieFitsBrowserBudgetWithLargeAuthorizationSnapshot(t *testing.T) {
	const secret = "identity-cookie-budget"
	permissions := make([]string, 160)
	for index := range permissions {
		permissions[index] = fmt.Sprintf("resource.%03d.manage", index)
	}
	service := jwtadapter.NewService(jwtadapter.Config{Secret: secret, AccessExpireMins: 15, RefreshExpireMins: 60})
	pair, err := service.Issue(context.Background(), application.TokenIssue{UserID: 7, TokenVersion: 3, Roles: []string{"admin"}, Permissions: permissions})
	if err != nil {
		t.Fatal(err)
	}
	cookie := http.Cookie{Name: "admin_refresh", Value: pair.RefreshToken, Path: "/api", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
	if size := len(cookie.String()); size > 4096 {
		t.Fatalf("refresh cookie exceeds browser capacity: %d bytes", size)
	}
	refresh, err := service.ParseRefresh(context.Background(), pair.RefreshToken)
	if err != nil || refresh.UserID != 7 || refresh.TokenVersion != 3 || refresh.SessionID != pair.SessionID || refresh.Generation != 1 {
		t.Fatal("bounded refresh credential lost a required session binding")
	}
	access, err := jwt.Parse(pair.AccessToken, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		t.Fatal(err)
	}
	values, ok := access.Claims.(jwt.MapClaims)["permissions"].([]any)
	if !ok || len(values) != len(permissions) {
		t.Fatal("access token lost the authorization snapshot")
	}
	for index, permission := range permissions {
		if values[index] != permission {
			t.Fatal("access token changed an authorization fact")
		}
	}
}
