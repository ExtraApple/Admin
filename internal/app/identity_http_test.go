package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/internal/app"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

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
