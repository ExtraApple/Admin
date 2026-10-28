package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gormadapter "admin/internal/apimetadata/adapters/gorm"
	apihttp "admin/internal/apimetadata/adapters/http"
	"admin/internal/apimetadata/application"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
	"github.com/gin-gonic/gin"
)

type impactRouteSource struct {
	facts []application.RouteFact
}

func (source impactRouteSource) Routes(context.Context) ([]application.RouteFact, error) {
	return append([]application.RouteFact(nil), source.facts...), nil
}

type impactPermissionLocker struct{}

func (impactPermissionLocker) LockPermissions(context.Context, []string) error { return nil }

func TestAPIImpactPreviewHTTPIsReadOnlyAndReturnsFingerprint(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(gormadapter.Models()...); err != nil {
		t.Fatalf("migrate API Metadata: %v", err)
	}
	core := application.NewCore(gormadapter.NewRepository(db), platformdatabase.NewTransactionRunner(db), impactPermissionLocker{})
	ctx := context.Background()
	api, err := core.Create(ctx, application.CreateInput{Name: "Preview", Method: "GET", Path: "/api/preview", PermissionCode: "preview.get"})
	if err != nil {
		t.Fatalf("create API: %v", err)
	}
	service := application.NewService(core, nil, application.WithRouteSource(impactRouteSource{facts: []application.RouteFact{{Method: "GET", Path: "/api/preview", PermissionControlled: true, DefaultPermissionCode: "preview.get"}}}))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	for _, descriptor := range apihttp.Routes(service) {
		router.Handle(descriptor.Method, descriptor.Path, descriptor.Handler)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/apis/impact-preview", bytes.NewBufferString(`{"operation":"update","candidate":{"api_id":`+itoa(api.ID)+`,"name":"Changed"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Fingerprint string `json:"preview_fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}
	if len(envelope.Data.Fingerprint) != 64 {
		t.Fatalf("preview fingerprint=%q", envelope.Data.Fingerprint)
	}
	unchanged, err := core.Get(ctx, api.ID)
	if err != nil || unchanged.Name != "Preview" {
		t.Fatalf("preview changed API metadata=%#v error=%v", unchanged, err)
	}
}

func itoa(value uint) string {
	return fmt.Sprintf("%d", value)
}
