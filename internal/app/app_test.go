package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"admin/testsupport/testutil"
	"admin/internal/app"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNewAssemblesPlatformRoutesSeedAndBackgroundJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	logger := zap.NewNop()
	seededRoutes := make(chan []routecatalog.Descriptor, 1)
	jobStarted := make(chan struct{})
	jobStopped := make(chan struct{})
	descriptor := routecatalog.Descriptor{
		Method: http.MethodGet, Path: "/api/health", Access: routecatalog.Public,
		Handler: func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) },
		Name:    "Health", Group: "system", DefaultAuditCategory: "health",
		OpenAPI: routecatalog.Operation{
			Summary: "Health",
			Request: routecatalog.RequestBody{Kind: routecatalog.NoBody},
			Responses: map[int]routecatalog.Response{
				http.StatusOK: {Description: "healthy", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]string{})},
			},
		},
	}

	application, err := app.New(context.Background(), testAppConfig(), app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: logger,
	}, app.Options{
		Descriptors: []routecatalog.Descriptor{descriptor},
		Seed: func(_ context.Context, _ platformconfig.Config, snapshot []routecatalog.Descriptor) error {
			seededRoutes <- snapshot
			return nil
		},
		BackgroundJobs: []app.BackgroundJob{{Name: "test", Run: func(ctx context.Context) {
			close(jobStarted)
			<-ctx.Done()
			close(jobStopped)
		}}},
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	select {
	case snapshot := <-seededRoutes:
		if len(snapshot) != 88 || snapshot[0].Path != "/api/dicts/:type_code/items" || snapshot[9].Path != "/api/admin/organizations" || snapshot[16].Path != "/api/admin/menus" || snapshot[25].Path != "/api/admin/api-groups" || snapshot[27].Path != "/api/admin/apis" || snapshot[35].Path != "/api/health" || snapshot[36].Path != "/api/admin/roles" || snapshot[56].Path != "/api/captcha" || snapshot[73].Path != "/api/admin/users/:id/kick" || snapshot[74].Path != "/api/admin/files" || snapshot[82].Path != "/api/admin/files-browse" || snapshot[83].Path != "/api/admin/audit-logs" || snapshot[87].Path != "/api/admin/data-access-logs" {
			t.Fatalf("Seed Route Catalog Snapshot has %d routes; want Dictionary, Organization, Navigation, API Metadata, health, Authorization, Identity, Files, then Audit", len(snapshot))
		}
	default:
		t.Fatal("App did not run Seed with Route Catalog Snapshot")
	}

	application.StartBackgroundJobs(context.Background())
	select {
	case <-jobStarted:
	case <-time.After(time.Second):
		t.Fatal("background job did not start")
	}

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("App HTTP status = %d body %s", response.Code, response.Body.String())
	}
	apiResponse := httptest.NewRecorder()
	application.Handler().ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/admin/apis", nil))
	if apiResponse.Code == http.StatusNotFound {
		t.Fatalf("API Metadata route was not registered: %s", apiResponse.Body.String())
	}
	if err := application.Close(); err != nil {
		t.Fatalf("close App: %v", err)
	}
	select {
	case <-jobStopped:
	case <-time.After(time.Second):
		t.Fatal("background job did not stop")
	}
}
