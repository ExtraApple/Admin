package dto

import (
	"encoding/json"
	"testing"
)

func TestFileInfoJSONExposesValidationMetadataWithoutStorageDetails(t *testing.T) {
	info := FileInfo{
		ID:                      7,
		Name:                    "report.pdf",
		ContentType:             "application/pdf",
		DetectedContentType:     "application/pdf",
		ContentSHA256:           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Size:                    13,
		UploaderID:              42,
		ValidationStatus:        "validated",
		ValidationPolicyVersion: "file-upload-v1",
		ValidationErrorCode:     "",
		ValidatedAt:             "2026-07-14 09:10:11",
		CreatedAt:               "2026-07-14 09:10:12",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	expected := map[string]any{
		"detected_content_type":     "application/pdf",
		"content_sha256":            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"validation_status":         "validated",
		"validation_policy_version": "file-upload-v1",
		"validation_error_code":     "",
		"validated_at":              "2026-07-14 09:10:11",
	}
	for key, want := range expected {
		if got, exists := payload[key]; !exists || got != want {
			t.Fatalf("JSON field %q = %#v, exists=%v, want %#v", key, got, exists, want)
		}
	}

	for _, forbidden := range []string{"bucket", "object_name", "download_url", "preview_url"} {
		if value, exists := payload[forbidden]; exists {
			t.Fatalf("JSON unexpectedly exposes %q = %#v", forbidden, value)
		}
	}
}
