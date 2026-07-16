package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"admin/global"
	"admin/model"
	"admin/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAuditLogPersistsUploadMetadataAfterHandlerAndOmitsMultipartBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAuditMiddlewareTestDB(t)

	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/admin/files", func(c *gin.Context) {
		c.Set(service.UploadAuditMetadataContextKey, service.UploadAuditMetadata{
			Purpose:          "managed_file",
			FileName:         "report.pdf",
			FileSize:         128,
			DeclaredMIME:     "application/pdf",
			DetectedMIME:     "application/pdf",
			ValidationResult: service.UploadValidationRejected,
			ReasonCode:       "FILE_CONTENT_INVALID",
			PolicyVersion:    "file-upload-v1",
		})
		c.Status(http.StatusUnprocessableEntity)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/files",
		strings.NewReader("raw multipart bytes TOP-SECRET-FILE-CONTENT"),
	)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	router.ServeHTTP(recorder, request)

	log := waitForAuditLog(t, db, "/api/admin/files")
	if log.Body != "[multipart omitted]" {
		t.Fatalf("audit body = %q, want fixed multipart omission marker", log.Body)
	}

	var metadata map[string]any
	if err := json.Unmarshal(log.Metadata, &metadata); err != nil {
		t.Fatalf("unmarshal audit metadata %q: %v", log.Metadata, err)
	}
	if metadata["purpose"] != "managed_file" ||
		metadata["file_name"] != "report.pdf" ||
		metadata["validation_result"] != service.UploadValidationRejected ||
		metadata["reason_code"] != "FILE_CONTENT_INVALID" {
		t.Fatalf("audit metadata = %#v, want handler-provided controlled fields", metadata)
	}
	if strings.Contains(string(log.Metadata), "TOP-SECRET-FILE-CONTENT") {
		t.Fatalf("audit metadata leaked multipart content: %s", log.Metadata)
	}
	for _, forbidden := range []string{
		`C:\fakepath`,
		"avatars/42/private.png",
		"signature=",
		"minio-access-key",
		"jwt-secret",
		"hmac-secret",
		"zip parser internal detail",
	} {
		if strings.Contains(strings.ToLower(string(log.Metadata)), strings.ToLower(forbidden)) {
			t.Fatalf("audit metadata leaked %q: %s", forbidden, log.Metadata)
		}
	}
}

func TestAuditLogIgnoresUncontrolledUploadMetadataTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAuditMiddlewareTestDB(t)

	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/user/avatar", func(c *gin.Context) {
		c.Set(service.UploadAuditMetadataContextKey, map[string]any{
			"purpose":        "avatar",
			"raw_path":       `C:\fakepath\portrait.png`,
			"object_key":     "avatars/42/private.png",
			"signature_url":  "https://example.invalid?signature=secret",
			"minio_password": "minio-access-key",
			"internal_error": "zip parser internal detail",
		})
		c.Status(http.StatusUnprocessableEntity)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/user/avatar",
		strings.NewReader("private image bytes"),
	)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	router.ServeHTTP(recorder, request)

	log := waitForAuditLog(t, db, "/api/user/avatar")
	if len(log.Metadata) != 0 && string(log.Metadata) != "null" {
		t.Fatalf("uncontrolled metadata was persisted: %s", log.Metadata)
	}
}

func TestAuditLogPersistsRequestFieldsAndRecursivelyMasksJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAuditMiddlewareTestDB(t)

	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/profile", func(c *gin.Context) {
		c.Set("userID", uint(42))
		c.Set("username", "alice")
		c.Status(http.StatusCreated)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/profile?view=full",
		strings.NewReader(`{
			"password":"plain-password",
			"profile":{"access_token":"secret-token","nickname":"Alice"},
			"items":[{"captcha_code":"123456"},{"refresh_token":"refresh-secret"}]
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "audit-test-agent")
	request.RemoteAddr = "203.0.113.8:4321"
	router.ServeHTTP(recorder, request)

	log := waitForAuditLog(t, db, "/api/profile")
	if log.UserID != 42 ||
		log.Username != "alice" ||
		log.Method != http.MethodPost ||
		log.Path != "/api/profile" ||
		log.Query != "view=full" ||
		log.Status != http.StatusCreated ||
		log.Duration < 0 ||
		log.ClientIP != "203.0.113.8" ||
		log.UserAgent != "audit-test-agent" ||
		log.Category != service.AuditCategoryOperation ||
		log.CreatedAt.IsZero() {
		t.Fatalf("persisted audit log fields = %#v, want complete request context", log)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(log.Body), &body); err != nil {
		t.Fatalf("unmarshal sanitized audit body %q: %v", log.Body, err)
	}
	profile, ok := body["profile"].(map[string]any)
	if !ok {
		t.Fatalf("sanitized profile = %#v, want object", body["profile"])
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("sanitized items = %#v, want two objects", body["items"])
	}
	firstItem, firstOK := items[0].(map[string]any)
	secondItem, secondOK := items[1].(map[string]any)
	if body["password"] != "***" ||
		profile["access_token"] != "***" ||
		profile["nickname"] != "Alice" ||
		!firstOK ||
		firstItem["captcha_code"] != "***" ||
		!secondOK ||
		secondItem["refresh_token"] != "***" {
		t.Fatalf("sanitized audit body = %#v, want recursive sensitive-field masking", body)
	}
	for _, secret := range []string{
		"plain-password",
		"secret-token",
		"123456",
		"refresh-secret",
	} {
		if strings.Contains(log.Body, secret) {
			t.Fatalf("sanitized audit body leaked %q: %s", secret, log.Body)
		}
	}
}

func TestAuditLogClassifiesLoginReadsOperationsAndPermissionMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAuditMiddlewareTestDB(t)

	router := gin.New()
	router.Use(AuditLog())
	router.Any("/api/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name         string
		method       string
		path         string
		wantCategory string
	}{
		{"login", http.MethodPost, "/api/login", service.AuditCategoryLogin},
		{"role query is data access", http.MethodGet, "/api/admin/roles", service.AuditCategoryDataAccess},
		{"permission query is data access", http.MethodGet, "/api/admin/roles/7/permissions", service.AuditCategoryDataAccess},
		{"ordinary write is operation", http.MethodPost, "/api/admin/files", service.AuditCategoryOperation},
		{"role binding write is permission", http.MethodPut, "/api/admin/roles/8/permissions", service.AuditCategoryPermission},
		{"menu write is permission", http.MethodDelete, "/api/admin/menus/7", service.AuditCategoryPermission},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusNoContent)
			}

			log := waitForAuditLog(t, db, tt.path)
			if log.Category != tt.wantCategory {
				t.Fatalf("audit category = %q, want %q for %s %s",
					log.Category, tt.wantCategory, tt.method, tt.path)
			}
		})
	}
}

func openAuditMiddlewareTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "audit-middleware.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func waitForAuditLog(t *testing.T, db *gorm.DB, path string) model.AuditLog {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var log model.AuditLog
		err := db.Where("path = ?", path).Order("id desc").First(&log).Error
		if err == nil {
			return log
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			t.Fatalf("query audit log: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for audit log %q", path)
	return model.AuditLog{}
}
