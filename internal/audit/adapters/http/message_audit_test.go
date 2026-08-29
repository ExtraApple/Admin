package httpadapter_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin/internal/audit"
	httpadapter "admin/internal/audit/adapters/http"
	"github.com/gin-gonic/gin"
)

func TestMiddlewareRecordsControlledMessageMetadataWithoutContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &recordingRecorder{}
	router := gin.New()
	router.Use(httpadapter.NewMiddleware(recorder))
	router.POST("/api/admin/announcements/:id/revoke", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodPost, "/api/admin/announcements/41/revoke?ticket=secret-ticket", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if len(recorder.logs) != 1 || recorder.logs[0].Category != audit.AuditCategoryMessage || recorder.logs[0].Query != "ticket=%2A%2A%2A" {
		t.Fatalf("message audit log = %#v", recorder.logs)
	}
	var metadata map[string]any
	if err := json.Unmarshal(recorder.logs[0].Metadata, &metadata); err != nil || metadata["action"] != "message.revoke" || metadata["resource_id"] != float64(41) {
		t.Fatalf("message metadata = %s err=%v", recorder.logs[0].Metadata, err)
	}
}
