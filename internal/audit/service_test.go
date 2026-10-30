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

func TestNormalizeListRequestUsesLocalTimeAndCapsSize(t *testing.T) {
	request, err := NormalizeListRequest(AuditLogListRequest{
		Size:      1000,
		StartTime: "2026-10-30 00:00:00",
		EndTime:   "2026-10-30 23:59:59",
	})
	if err != nil {
		t.Fatalf("normalize request: %v", err)
	}
	if request.Size != MaxAuditPageSize {
		t.Fatalf("size = %d, want %d", request.Size, MaxAuditPageSize)
	}
	if request.StartAt.Location() != time.Local || request.EndAt.Location() != time.Local {
		t.Fatalf("locations = %v / %v, want local", request.StartAt.Location(), request.EndAt.Location())
	}
	if request.StartAt.Format(auditTimeLayout) != "2026-10-30 00:00:00" || request.EndAt.Format(auditTimeLayout) != "2026-10-30 23:59:59" {
		t.Fatalf("bounds = %v / %v", request.StartAt, request.EndAt)
	}
	if got := NormalizePage(AuditLogListRequest{}).Size; got != 10 {
		t.Fatalf("default size = %d, want 10", got)
	}
}

func TestNormalizeListRequestRejectsInvalidRanges(t *testing.T) {
	cases := []AuditLogListRequest{
		{StartTime: "not-a-time"},
		{EndTime: "2026-10-30"},
		{StartTime: "2026-10-30 12:00:00", EndTime: "2026-10-30 11:00:00"},
	}
	for _, request := range cases {
		_, err := NormalizeListRequest(request)
		code, ok := CodeOf(err)
		if !ok || code != CodeValidationInvalid {
			t.Fatalf("request %#v error = %v, want %s", request, err, CodeValidationInvalid)
		}
	}
}

func TestSanitizeQueryKeepsUsernameAndDropsSecrets(t *testing.T) {
	got := SanitizeQuery("username=alice&page=2&token=plain-test-value")
	if got != "page=2&username=alice" {
		t.Fatalf("sanitized query = %q", got)
	}
}
