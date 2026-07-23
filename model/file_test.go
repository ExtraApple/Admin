package model

import (
	"regexp"
	"testing"
	"time"
)

func TestFileExposesUploadValidationMetadata(t *testing.T) {
	validatedAt := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	file := File{
		DetectedContentType:     "application/pdf",
		ContentSHA256:           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ValidationStatus:        FileValidationStatusValidated,
		ValidationPolicyVersion: FileUploadPolicyVersion,
		ValidationErrorCode:     "FILE_CONTENT_INVALID",
		ValidatedAt:             &validatedAt,
	}

	if file.DetectedContentType != "application/pdf" {
		t.Fatalf("detected content type: got %q", file.DetectedContentType)
	}
	if !isLowercaseSHA256Hex(file.ContentSHA256) {
		t.Fatalf("content sha256 = %q, want 64 lowercase hex characters", file.ContentSHA256)
	}
	if file.ValidationStatus != "validated" {
		t.Fatalf("validation status: got %q, want validated", file.ValidationStatus)
	}
	if file.ValidationPolicyVersion != "file-upload-v1" {
		t.Fatalf("policy version: got %q, want file-upload-v1", file.ValidationPolicyVersion)
	}
	if file.ValidationErrorCode != "FILE_CONTENT_INVALID" {
		t.Fatalf("validation error code: got %q", file.ValidationErrorCode)
	}
	if file.ValidatedAt == nil || !file.ValidatedAt.Equal(validatedAt) {
		t.Fatalf("validated at: got %v, want %v", file.ValidatedAt, validatedAt)
	}
}

func isLowercaseSHA256Hex(value string) bool {
	return regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(value)
}

func TestFileValidationStatusConstantsAreStable(t *testing.T) {
	tests := map[string]string{
		"legacy":           FileValidationStatusLegacyUnverified,
		"validated":        FileValidationStatusValidated,
		"blocked":          FileValidationStatusBlocked,
		"validation error": FileValidationStatusValidationError,
	}
	want := map[string]string{
		"legacy":           "legacy_unverified",
		"validated":        "validated",
		"blocked":          "blocked",
		"validation error": "validation_error",
	}

	for name, got := range tests {
		if got != want[name] {
			t.Fatalf("%s status: got %q, want %q", name, got, want[name])
		}
	}
}
