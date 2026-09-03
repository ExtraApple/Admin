package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"admin/internal/app"
	"admin/internal/audit"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type appAuditMetadata struct {
	Purpose          string `json:"purpose"`
	ValidationResult string `json:"validation_result"`
	ObjectName       string `json:"object_name"`
}

func TestAppRegistersFilesAuditAndPersistsAPIRequestsAsynchronously(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	descriptor := routecatalog.Descriptor{
		Method: http.MethodPost,
		Path:   "/api/audit-probe",
		Access: routecatalog.Public,
		Handler: func(c *gin.Context) {
			c.Set("userID", uint(7))
			c.Set("username", "alice")
			c.Set(audit.UploadAuditMetadataContextKey, appAuditMetadata{Purpose: "managed_file", ValidationResult: audit.UploadValidationAccepted, ObjectName: "private/object"})
			c.JSON(http.StatusAccepted, gin.H{"accepted": true})
		},
		Name: "Audit Probe", Group: "test", DefaultAuditCategory: "operation",
		OpenAPI: routecatalog.Operation{
			Summary:   "Audit Probe",
			Request:   routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]string{}), Required: true},
			Responses: map[int]routecatalog.Response{http.StatusAccepted: {Description: "accepted", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]bool{})}},
		},
	}
	application, err := app.New(context.Background(), testAppConfig(), app.Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}, app.Options{
		Descriptors: []routecatalog.Descriptor{descriptor},
		Seed:        func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
	})
	if err != nil {
		t.Fatalf("assemble Files/Audit App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	catalogRoutes := make(map[string]struct{})
	for _, route := range application.Catalog().Snapshot() {
		catalogRoutes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{"POST /api/admin/files", "GET /api/admin/files-browse", "GET /api/admin/audit-logs", "GET /api/admin/data-access-logs"} {
		if _, ok := catalogRoutes[route]; !ok {
			t.Fatalf("App Route Catalog missing %s", route)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/api/audit-probe?source=test", bytes.NewBufferString(`{"password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("audit probe status = %d body=%s", response.Code, response.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	var log audit.AuditLog
	for {
		err = db.Where("path = ?", "/api/audit-probe").First(&log).Error
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for asynchronous audit log: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if log.UserID != 7 || log.Username != "alice" || log.Status != http.StatusAccepted || log.Query != "source=test" || log.Body != `{"password":"***"}` || log.Category != audit.AuditCategoryOperation {
		t.Fatalf("persisted audit log = %#v", log)
	}
	var metadata map[string]any
	if err := json.Unmarshal(log.Metadata, &metadata); err != nil {
		t.Fatalf("decode upload audit metadata: %v", err)
	}
	if metadata["purpose"] != "managed_file" || metadata["validation_result"] != "accepted" {
		t.Fatalf("persisted upload metadata = %#v", metadata)
	}
	if _, exists := metadata["object_name"]; exists {
		t.Fatalf("private object name persisted: %#v", metadata)
	}
}
