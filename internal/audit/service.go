package audit

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
)

const (
	AuditCategoryAPI        = "api"
	AuditCategoryLogin      = "login"
	AuditCategoryOperation  = "operation"
	AuditCategoryPermission = "permission"
	AuditCategoryDataAccess = "data_access"

	UploadAuditMetadataContextKey = "upload_audit_metadata"
	UploadValidationAccepted      = "accepted"
	UploadValidationRejected      = "rejected"
	MaxAuditBodySize              = 2000
)

// UploadAuditMetadata is the allowlist persisted for upload operations. It
// excludes bytes, local paths, object names, URLs, credentials and errors.
type UploadAuditMetadata struct {
	Purpose          string `json:"purpose"`
	FileName         string `json:"file_name,omitempty"`
	FileSize         int64  `json:"file_size,omitempty"`
	DeclaredMIME     string `json:"declared_mime,omitempty"`
	DetectedMIME     string `json:"detected_mime,omitempty"`
	ValidationResult string `json:"validation_result"`
	ReasonCode       string `json:"reason_code,omitempty"`
	PolicyVersion    string `json:"policy_version,omitempty"`
}

func UploadMetadata(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	kind := reflect.TypeOf(value).Kind()
	if kind == reflect.Ptr {
		if reflect.ValueOf(value).IsNil() {
			return nil
		}
		kind = reflect.TypeOf(value).Elem().Kind()
	}
	// Metadata must come from a typed caller-owned struct. Maps are rejected
	// so arbitrary Gin context values cannot smuggle fields into the log.
	if kind != reflect.Struct {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var metadata UploadAuditMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil
	}
	if metadata.Purpose == "" && metadata.ValidationResult == "" {
		return nil
	}
	data, err = json.Marshal(metadata)
	if err != nil {
		return nil
	}
	return json.RawMessage(data)
}

var sensitiveAuditFields = map[string]struct{}{
	"password": {}, "old_password": {}, "new_password": {}, "confirm_password": {},
	"captcha_code": {}, "access_token": {}, "refresh_token": {}, "token": {},
}

func SanitizeBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if string(body) == "[multipart omitted]" {
		return string(body)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return TruncateBody(string(body))
	}
	MaskSensitiveFields(data)
	sanitized, err := json.Marshal(data)
	if err != nil {
		return TruncateBody(string(body))
	}
	return TruncateBody(string(sanitized))
}

func TruncateBody(body string) string {
	if len(body) <= MaxAuditBodySize {
		return body
	}
	return body[:MaxAuditBodySize] + "..."
}

func MaskSensitiveFields(data map[string]any) {
	for key, value := range data {
		if _, ok := sensitiveAuditFields[strings.ToLower(key)]; ok {
			data[key] = "***"
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			MaskSensitiveFields(nested)
		case []any:
			for _, item := range nested {
				if object, ok := item.(map[string]any); ok {
					MaskSensitiveFields(object)
				}
			}
		}
	}
}

func Classify(method, path string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	normalized := strings.ToLower(strings.TrimSpace(path))
	if normalized != "/" {
		normalized = strings.TrimRight(normalized, "/")
	}
	if normalized == "/api/login" {
		return AuditCategoryLogin
	}
	if method == "GET" {
		return AuditCategoryDataAccess
	}
	if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
		if strings.HasPrefix(normalized, "/api/admin/permissions") || strings.HasPrefix(normalized, "/api/admin/permission-groups") || strings.HasPrefix(normalized, "/api/admin/menus") || strings.HasPrefix(normalized, "/api/admin/apis") {
			return AuditCategoryPermission
		}
		if strings.HasPrefix(normalized, "/api/admin/roles/") && (strings.HasSuffix(normalized, "/permissions") || strings.HasSuffix(normalized, "/menus") || strings.HasSuffix(normalized, "/users")) {
			return AuditCategoryPermission
		}
		return AuditCategoryOperation
	}
	return AuditCategoryAPI
}

// Worker is the queue consumer seam used by process composition.
type Worker interface {
	Record(context.Context, AuditLog)
}

var _ Worker = (*RecorderWorker)(nil)

type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) *QueryService {
	return &QueryService{repository: repository}
}

func (service *QueryService) List(ctx context.Context, request AuditLogListRequest) ([]AuditLogInfo, int64, error) {
	return service.list(ctx, request, nil)
}
func (service *QueryService) ListByCategories(ctx context.Context, request AuditLogListRequest, categories ...string) ([]AuditLogInfo, int64, error) {
	return service.list(ctx, request, categories)
}
func (service *QueryService) list(ctx context.Context, request AuditLogListRequest, categories []string) ([]AuditLogInfo, int64, error) {
	request = NormalizePage(request)
	logs, total, err := service.repository.List(ctx, request, categories)
	if err != nil {
		return nil, 0, err
	}
	result := make([]AuditLogInfo, len(logs))
	for index, log := range logs {
		result[index] = ToInfo(log)
	}
	return result, total, nil
}

func NormalizePage(request AuditLogListRequest) AuditLogListRequest {
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.Size <= 0 {
		request.Size = 10
	}
	return request
}

func ToInfo(log AuditLog) AuditLogInfo {
	return AuditLogInfo{ID: log.ID, UserID: log.UserID, Username: log.Username, Method: log.Method, Path: log.Path,
		Query: log.Query, Body: log.Body, Metadata: log.Metadata, Status: log.Status, Duration: log.Duration,
		ClientIP: log.ClientIP, UserAgent: log.UserAgent, Category: log.Category, CreatedAt: log.CreatedAt.Format("2006-01-02 15:04:05")}
}

type RecorderWorker struct {
	repository RecorderRepository
	username   UsernameReader
	logger     Logger
	clock      Clock
}

func NewRecorderWorker(repository RecorderRepository, logger Logger, clock Clock, username ...UsernameReader) *RecorderWorker {
	if clock == nil {
		clock = systemClock{}
	}
	var reader UsernameReader
	if len(username) > 0 {
		reader = username[0]
	}
	return &RecorderWorker{repository: repository, username: reader, logger: logger, clock: clock}
}
func (worker *RecorderWorker) Record(ctx context.Context, log AuditLog) {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = worker.clock.Now()
	}
	if worker.username != nil && log.UserID > 0 && log.Username == "" {
		if username, err := worker.username.Username(ctx, log.UserID); err == nil {
			log.Username = username
		}
	}
	if err := worker.repository.Create(ctx, &log); err != nil && worker.logger != nil {
		worker.logger.Error("create audit log failed", err)
	}
}

// AsyncRecorder delegates submission to a caller-owned queue and never blocks
// the request path on persistence.
type AsyncRecorder struct{ queue RecorderQueue }

func NewAsyncRecorder(queue RecorderQueue) *AsyncRecorder { return &AsyncRecorder{queue: queue} }
func (recorder *AsyncRecorder) Submit(log AuditLog) {
	if recorder != nil && recorder.queue != nil {
		recorder.queue.Submit(log)
	}
}

// ArchiveAuditLogs is the descriptive application-service spelling used by
// background job composition.
func (service *ArchiveService) ArchiveAuditLogs(ctx context.Context, options ArchiveOptions) error {
	return service.Archive(ctx, options)
}
