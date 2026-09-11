package httpadapter_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"admin/internal/audit"
	httpadapter "admin/internal/audit/adapters/http"
	"admin/internal/platform/httpresponse"

	"github.com/gin-gonic/gin"
)

type recordingRecorder struct{ logs []audit.AuditLog }

func (recorder *recordingRecorder) Submit(log audit.AuditLog) {
	recorder.logs = append(recorder.logs, log)
}

type sequenceClock struct {
	values []time.Time
	index  int
}

func (clock *sequenceClock) Now() time.Time {
	value := clock.values[clock.index]
	clock.index++
	return value
}

type guardedBody struct {
	io.Reader
	reads int
}

func (body *guardedBody) Read(buffer []byte) (int, error) {
	body.reads++
	return body.Reader.Read(buffer)
}
func (*guardedBody) Close() error { return nil }

type uploadMetadata struct {
	Purpose          string `json:"purpose"`
	FileName         string `json:"file_name"`
	ValidationResult string `json:"validation_result"`
	ObjectName       string `json:"object_name"`
}

func TestMiddlewareCapturesRedactedRequestAndAllowlistedUploadMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	start := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	clock := &sequenceClock{values: []time.Time{start, start.Add(25 * time.Millisecond)}}
	recorder := &recordingRecorder{}
	router := gin.New()
	router.Use(httpadapter.NewMiddleware(recorder, httpadapter.MiddlewareOptions{Clock: clock}))
	router.POST("/api/admin/files", func(c *gin.Context) {
		c.Set("userID", uint(7))
		c.Set("username", "alice")
		c.Set(httpresponse.UploadAuditMetadataKey, uploadMetadata{Purpose: "managed_file", FileName: "report.pdf", ValidationResult: audit.UploadValidationAccepted, ObjectName: "private/object"})
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	request := httptest.NewRequest(http.MethodPost, "/api/admin/files?source=test", bytes.NewBufferString(`{"password":"secret","nested":{"token":"private"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "audit-test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if len(recorder.logs) != 1 {
		t.Fatalf("recorded logs = %d", len(recorder.logs))
	}
	log := recorder.logs[0]
	if log.UserID != 7 || log.Username != "alice" || log.Method != http.MethodPost || log.Path != "/api/admin/files" || log.Query != "source=test" || log.Status != http.StatusCreated || log.Duration != 25 || log.UserAgent != "audit-test" || log.Category != audit.AuditCategoryOperation || !log.CreatedAt.Equal(start) {
		t.Fatalf("audit log = %#v", log)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(log.Body), &body); err != nil {
		t.Fatalf("decode audit body: %v", err)
	}
	if body["password"] != "***" || body["nested"].(map[string]any)["token"] != "***" {
		t.Fatalf("sensitive audit body = %#v", body)
	}
	if string(log.Metadata) != `{"purpose":"managed_file","file_name":"report.pdf","validation_result":"accepted"}` {
		t.Fatalf("upload metadata = %s", log.Metadata)
	}
}

func TestMiddlewareNeverReadsMultipartFileContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &recordingRecorder{}
	body := &guardedBody{Reader: bytes.NewBufferString("private file bytes")}
	router := gin.New()
	router.Use(httpadapter.NewMiddleware(recorder))
	router.POST("/api/admin/files", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodPost, "/api/admin/files", nil)
	request.Body = body
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if body.reads != 0 {
		t.Fatalf("multipart audit read file body %d times", body.reads)
	}
	if len(recorder.logs) != 1 || recorder.logs[0].Body != "[multipart omitted]" {
		t.Fatalf("multipart audit log = %#v", recorder.logs)
	}
}
