package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin/internal/app"
	"admin/internal/identity"
	identitygorm "admin/internal/identity/adapters/gorm"
	identitypassword "admin/internal/identity/adapters/password"
	identitydomain "admin/internal/identity/domain"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestApplicationRegistersIdentityAuthenticationRoutes(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{DB: db, Redis: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/captcha", nil))
	if response.Code == http.StatusNotFound {
		t.Fatalf("Identity captcha route was not registered")
	}
	for _, test := range []struct {
		method, path, body, origin string
		status                     int
	}{
		{http.MethodPost, "/api/user/logout", `{}`, "https://admin.example.test", http.StatusOK},
		{http.MethodPost, "/api/user/logout", `{}`, "https://evil.example.test", http.StatusForbidden},
		{http.MethodGet, "/api/user/info", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/user/context", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/user/avatar-upload-policy", "", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/admin/users", "", "", http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Origin", test.origin)
		request.Header.Set("X-Admin-Request", "1")
		request.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		application.Handler().ServeHTTP(result, request)
		if result.Code != test.status {
			t.Fatalf("%s %s status=%d, want %d", test.method, test.path, result.Code, test.status)
		}
	}
}

func TestApplicationDisabledSMTPKeepsUserProfileAvailable(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	conf := testAppConfig()
	conf.SMTP.Host = "smtp.example.test"
	conf.SMTP.TLSMode = "unused-while-disabled"
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	var userID uint
	application, err := app.New(context.Background(), conf, app.Resources{
		DB: db, Redis: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), MinIO: minioClient, Logger: zap.NewNop(),
	}, app.Options{
		Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
		Middleware: app.HTTPMiddleware{Authenticated: func(c *gin.Context) {
			c.Set("userID", userID)
			c.Next()
		}},
	})
	if err != nil {
		t.Fatalf("disabled SMTP blocked application assembly: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	user := identitydomain.User{Username: "mail-disabled-user", Password: "hash", Email: "unverified@example.test", Status: 1}
	hashed, err := identitypassword.Bcrypt{}.Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	user.Password = hashed
	if err := identitygorm.NewRepository(db).Create(context.Background(), &user); err != nil {
		t.Fatal(err)
	}
	userID = user.ID
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/user/info", nil))
	var result struct {
		Data struct {
			Username      string `json:"username"`
			EmailVerified bool   `json:"email_verified"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Data.Username != user.Username || result.Data.EmailVerified {
		t.Fatalf("profile with disabled SMTP = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/user/email-verification-policy", nil))
	var policy struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || policy.Data.Enabled {
		t.Fatalf("disabled SMTP policy = %d %s", response.Code, response.Body.String())
	}

	request := httptest.NewRequest(http.MethodPut, "/api/user/info", strings.NewReader(`{"email":"replacement@example.test","current_password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	var failure struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || failure.ErrorCode != "IDENTITY_EMAIL_VERIFICATION_DISABLED" {
		t.Fatalf("disabled SMTP email change = %d %s", response.Code, response.Body.String())
	}
}

func TestApplicationAdminUserUpdatesPreserveEmailOwnership(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	user := fixture.createUser("admin-update-target")
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := fixture.db.Model(&user).Updates(map[string]any{
		"pending_email": "pending@example.test", "email_verified_at": verifiedAt,
	}).Error; err != nil {
		t.Fatalf("prepare verified and pending email: %v", err)
	}
	path := "/api/admin/users/" + itoa(user.ID)
	readProfile := func() identity.UserInfo {
		t.Helper()
		response := fixture.request(http.MethodGet, "/api/user/info", fixture.token(user), nil)
		var payload struct {
			Data identity.UserInfo `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("read profile: %d %s", response.Code, response.Body.String())
		}
		return payload.Data
	}
	readVersion := func() int {
		t.Helper()
		response := fixture.request(http.MethodGet, path, fixture.adminToken, nil)
		var payload struct {
			Data struct {
				AccessVersion int `json:"access_version"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("read managed user: %d %s", response.Code, response.Body.String())
		}
		return payload.Data.AccessVersion
	}

	for _, test := range []struct {
		name   string
		body   map[string]any
		status int
	}{
		{name: "omitted email", body: map[string]any{"nickname": "without-email"}, status: 1},
		{name: "empty email", body: map[string]any{"nickname": "with-empty-email", "email": "", "status": 0}, status: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := fixture.request(http.MethodPut, path, fixture.adminToken, test.body)
			if response.Code != http.StatusOK {
				t.Fatalf("update managed user: %d %s", response.Code, response.Body.String())
			}
			profile := readProfile()
			if profile.Nickname != test.body["nickname"] || profile.Status != test.status {
				t.Fatalf("updated profile = %#v", profile)
			}
			if profile.Email != user.Email || profile.PendingEmail != "pending@example.test" || !profile.EmailVerified {
				t.Fatalf("administrator changed email ownership: %#v", profile)
			}
		})
	}

	before, version := readProfile(), readVersion()
	for _, email := range []string{"replacement@example.test", "not-an-email"} {
		t.Run(email, func(t *testing.T) {
			response := fixture.request(http.MethodPut, path, fixture.adminToken, map[string]any{
				"email": email, "nickname": "must-not-persist", "status": 1,
			})
			var payload struct {
				ErrorCode string `json:"error_code"`
				Data      struct {
					Fields []struct {
						Field     string `json:"field"`
						ErrorCode string `json:"error_code"`
					} `json:"fields"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusUnprocessableEntity || payload.ErrorCode != "IDENTITY_VALIDATION_INVALID" {
				t.Fatalf("nonempty email must reach business refusal: %d %s", response.Code, response.Body.String())
			}
			if len(payload.Data.Fields) != 1 || payload.Data.Fields[0].Field != "email" || payload.Data.Fields[0].ErrorCode != "IDENTITY_ADMIN_EMAIL_NOT_WRITABLE" {
				t.Fatalf("email ownership error = %s", response.Body.String())
			}
			if got := readProfile(); got != before {
				t.Fatalf("rejected update partially applied: before=%#v after=%#v", before, got)
			}
			if got := readVersion(); got != version {
				t.Fatalf("rejected update changed access version: %d -> %d", version, got)
			}
		})
	}
}

func TestApplicationRejectsInvalidBrowserSessionBeforeModuleConstruction(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	for _, condition := range []string{"missing origin", "non-loopback http", "nonpositive duration"} {
		conf := testAppConfig()
		switch condition {
		case "missing origin":
			conf.BrowserSession.Origin = ""
		case "non-loopback http":
			conf.BrowserSession.Origin = "http://192.168.1.10:5173"
			conf.BrowserSession.AllowLoopbackHTTP = true
		case "nonpositive duration":
			conf.Jwt.Expire = 0
		}
		seeded := false
		application, err := app.New(context.Background(), conf, app.Resources{DB: db, Redis: client, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error {
			seeded = true
			return nil
		}})
		if err == nil || application != nil || seeded {
			t.Fatalf("invalid browser configuration was not rejected before seeding: %s", condition)
		}
	}
}

func TestApplicationAvatarUploadPolicyRequiresAccessButNoPermission(t *testing.T) {
	fixture := newOrganizationJWTFixture(t)
	user := fixture.createUser("profile-policy-user")
	path := "/api/user/avatar-upload-policy"
	response := fixture.request(http.MethodGet, path, fixture.token(user), nil)
	var envelope struct {
		Data identity.AvatarUploadPolicy `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || envelope.Data.MaxBytes != int64(testAppConfig().FileUpload.AvatarMaxSizeMB)*1024*1024 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("ordinary user's policy = %d %s", response.Code, response.Body)
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: "admin_refresh", Value: "cookie-alone"})
	recorder := httptest.NewRecorder()
	fixture.application.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("cookie-alone policy = %d %s", recorder.Code, recorder.Body)
	}
}
