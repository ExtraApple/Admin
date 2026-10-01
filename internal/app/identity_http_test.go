package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/internal/app"
	identitygorm "admin/internal/identity/adapters/gorm"
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
}
