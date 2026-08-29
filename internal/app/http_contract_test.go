package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLoggingExcludesSensitiveFieldsAndRecoveryUsesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	engine := gin.New()
	engine.Use(requestLoggingMiddleware(logger), recoveryMiddleware(logger))
	engine.POST("/api/panic", func(c *gin.Context) { panic("password=secret token=private") })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/panic", strings.NewReader(`{"password":"secret","captcha":"1234"}`))
	request.Header.Set("Authorization", "Bearer private")
	request.Header.Set("User-Agent", "test-agent")
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "private") {
		t.Fatalf("response leaked sensitive data: %s", recorder.Body.String())
	}
	entries := logs.All()
	if len(entries) < 2 {
		t.Fatalf("log entries = %d, want recovery and request entries", len(entries))
	}
	for _, entry := range entries {
		for _, forbidden := range []string{"secret", "private", "captcha", "authorization"} {
			if strings.Contains(entry.Message, forbidden) {
				t.Fatalf("log message leaked %q: %s", forbidden, entry.Message)
			}
		}
	}
	requestEntry := entries[len(entries)-1]
	if fields := requestEntry.ContextMap(); fields["method"] != http.MethodPost || fields["path"] != "/api/panic" || fields["status"] != int64(500) {
		t.Fatalf("request fields = %#v", fields)
	}
}

func TestRequestLoggingOmitsRawCauseAndRecoveryValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	engine := gin.New()
	engine.Use(requestLoggingMiddleware(logger), recoveryMiddleware(logger))
	engine.GET("/api/failure", func(c *gin.Context) {
		httpresponse.WriteError(c, httpresponse.InternalErrorDefinition(), errors.New("token=private url=https://secret.example/path"), nil)
	})
	engine.GET("/api/panic", func(c *gin.Context) { panic("password=secret token=private") })
	for _, path := range []string{"/api/failure", "/api/panic"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+"?ticket=secret", nil))
	}
	for _, entry := range logs.All() {
		serialized := fmt.Sprintf("%v", entry.ContextMap())
		for _, forbidden := range []string{"secret", "private", "https://secret.example/path", "password"} {
			if strings.Contains(serialized, forbidden) {
				t.Fatalf("log context leaked %q: %#v", forbidden, entry.ContextMap())
			}
		}
	}
}

func TestRecoveryDoesNotAppendAfterCommittedNativeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(recoveryMiddleware(zap.NewNop()))
	engine.GET("/native", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte("partial"))
		panic("after commit")
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/native", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "partial" {
		t.Fatalf("committed response = %d %q", recorder.Code, recorder.Body.String())
	}
}
