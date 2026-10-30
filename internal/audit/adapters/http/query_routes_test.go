package httpadapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"admin/internal/audit"
	httpadapter "admin/internal/audit/adapters/http"

	"github.com/gin-gonic/gin"
)

type queryRepositoryStub struct {
	called  bool
	request audit.AuditLogListRequest
}

func (stub *queryRepositoryStub) List(_ context.Context, request audit.AuditLogListRequest, _ []string) ([]audit.AuditLog, int64, error) {
	stub.called = true
	stub.request = request
	return nil, 0, nil
}

func auditQueryRouter(repository audit.QueryRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	for _, descriptor := range httpadapter.Routes(audit.NewQueryService(repository)) {
		router.GET(descriptor.Path, descriptor.Handler)
	}
	return router
}

func TestAuditQueryCapsSizeAndForwardsUsername(t *testing.T) {
	stub := &queryRepositoryStub{}
	router := auditQueryRouter(stub)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?size=1000&username=ali", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var success struct {
		Data audit.AuditLogListResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &success); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if success.Data.Size != audit.MaxAuditPageSize || stub.request.Size != audit.MaxAuditPageSize {
		t.Fatalf("response size = %d request size = %d, want %d", success.Data.Size, stub.request.Size, audit.MaxAuditPageSize)
	}
	if stub.request.Username != "ali" {
		t.Fatalf("username = %q, want ali", stub.request.Username)
	}
}

func TestAuditQueryRejectsInvalidAndReversedTime(t *testing.T) {
	stub := &queryRepositoryStub{}
	router := auditQueryRouter(stub)
	queries := []string{
		"start_time=not-a-time",
		"start_time=2026-10-30+12%3A00%3A00&end_time=2026-10-30+11%3A00%3A00",
	}
	for _, query := range queries {
		stub.called = false
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?"+query, nil))
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("query %q status = %d body = %s", query, recorder.Code, recorder.Body.String())
		}
		var failure struct {
			Code      int    `json:"code"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &failure); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if failure.Code != http.StatusUnprocessableEntity || failure.ErrorCode != "AUDIT_VALIDATION_INVALID" {
			t.Fatalf("query %q failure = %#v", query, failure)
		}
		if stub.called {
			t.Fatalf("query %q reached repository", query)
		}
	}
}

func TestAuditQueryForwardsLocalTimeBounds(t *testing.T) {
	stub := &queryRepositoryStub{}
	router := auditQueryRouter(stub)
	params := url.Values{"start_time": {"2026-10-30 00:00:00"}, "end_time": {"2026-10-30 23:59:59"}}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?"+params.Encode(), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if stub.request.StartAt.Format("2006-01-02 15:04:05") != "2026-10-30 00:00:00" || stub.request.EndAt.Format("2006-01-02 15:04:05") != "2026-10-30 23:59:59" {
		t.Fatalf("bounds = %v / %v", stub.request.StartAt, stub.request.EndAt)
	}
}
