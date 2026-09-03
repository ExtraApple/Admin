package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/internal/app"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNewAssemblesDictionaryHTTPFromResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{
		Creds: credentials.NewStaticV4("test", "test", ""), Secure: false,
	})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}

	application, err := app.New(context.Background(), testAppConfig(), app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop(),
	}, app.Options{
		Seed:       func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
		Middleware: app.HTTPMiddleware{Authenticated: func(c *gin.Context) { c.Next() }, PermissionControlled: func(c *gin.Context) { c.Next() }},
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	handler := application.Handler()
	postDictionaryJSON(t, handler, "/api/admin/dict-types", map[string]any{
		"name": "Priority",
		"code": "priority",
	})
	postDictionaryJSON(t, handler, "/api/admin/dict-items", map[string]any{
		"type_code": "priority",
		"label":     "High",
		"value":     "high",
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dicts/priority/items", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET enabled Dictionary items status = %d body %s", response.Code, response.Body.String())
	}

	var payload struct {
		Code int `json:"code"`
		Data []struct {
			TypeCode string `json:"type_code"`
			Label    string `json:"label"`
			Value    string `json:"value"`
			Status   int    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode enabled Dictionary items response: %v", err)
	}
	if payload.Code != http.StatusOK {
		t.Fatalf("GET enabled Dictionary items code = %d, want %d", payload.Code, http.StatusOK)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("GET enabled Dictionary items data = %#v, want one item", payload.Data)
	}
	item := payload.Data[0]
	if item.TypeCode != "priority" || item.Label != "High" || item.Value != "high" || item.Status != 1 {
		t.Fatalf("GET enabled Dictionary items item = %#v, want priority/High/high/enabled", item)
	}
}

func postDictionaryJSON(t *testing.T, handler http.Handler, path string, body map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode %s request: %v", path, err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d body %s", path, response.Code, response.Body.String())
	}
}
