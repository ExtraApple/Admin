package audit

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSanitizeBodyMasksNestedSensitiveFieldsAndTruncates(t *testing.T) {
	body := []byte(`{"profile":{"password":"secret"},"items":[{"token":"secret"}]}`)
	got := SanitizeBody(body)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("sanitize result: %v", err)
	}
	if decoded["profile"].(map[string]any)["password"] != "***" {
		t.Fatal("nested password was not masked")
	}
	if decoded["items"].([]any)[0].(map[string]any)["token"] != "***" {
		t.Fatal("array token was not masked")
	}
}

func TestUploadMetadataRejectsUntypedMapAndAllowsTypedEquivalent(t *testing.T) {
	if got := UploadMetadata(map[string]any{"purpose": "avatar", "token": "secret"}); got != nil {
		t.Fatalf("untyped metadata accepted: %s", got)
	}
	type callerMetadata struct {
		Purpose          string `json:"purpose"`
		ValidationResult string `json:"validation_result"`
		ObjectName       string `json:"object_name"`
	}
	got := UploadMetadata(callerMetadata{Purpose: "avatar", ValidationResult: UploadValidationAccepted, ObjectName: "private/secret"})
	if string(got) != `{"purpose":"avatar","validation_result":"accepted"}` {
		t.Fatalf("typed metadata = %s", got)
	}
}

func TestToArchiveCopiesMetadataAndTimestamp(t *testing.T) {
	metadata := json.RawMessage(`{"purpose":"managed_file"}`)
	archivedAt := time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)
	archive := ToArchive(AuditLog{Metadata: metadata}, archivedAt)
	if string(archive.Metadata) != string(metadata) || !archive.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("archive copy = %#v", archive)
	}
}
