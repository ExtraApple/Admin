package domain

import "time"

type File struct {
	ID                      uint
	CreatedAt               time.Time
	UpdatedAt               time.Time
	Name                    string
	Bucket                  string
	ObjectName              string
	ContentType             string
	DetectedContentType     string
	ContentSHA256           string
	Size                    int64
	UploaderID              uint
	Purpose                 string
	LogicalMessageID        string
	BindingExpiresAt        *time.Time
	ValidationStatus        string
	ValidationPolicyVersion string
	ValidationErrorCode     string
	ValidatedAt             *time.Time
}

const (
	ValidationStatusLegacyUnverified = "legacy_unverified"
	ValidationStatusValidated        = "validated"
	ValidationStatusBlocked          = "blocked"
	ValidationStatusValidationError  = "validation_error"
)
