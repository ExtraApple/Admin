package service

const (
	UploadAuditMetadataContextKey = "upload_audit_metadata"
	UploadValidationAccepted      = "accepted"
	UploadValidationRejected      = "rejected"
)

// UploadAuditMetadata is the complete allowlist of upload information that may
// be persisted in an audit log. It intentionally excludes file bytes, raw
// client paths, object names, access URLs, credentials and internal errors.
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
