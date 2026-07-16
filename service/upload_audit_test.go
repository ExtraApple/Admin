package service

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestUploadAuditMetadataJSONUsesOnlyControlledFields(t *testing.T) {
	metadata := UploadAuditMetadata{
		Purpose:          "managed_file",
		FileName:         "report.pdf",
		FileSize:         1024,
		DeclaredMIME:     "application/pdf",
		DetectedMIME:     "application/pdf",
		ValidationResult: UploadValidationAccepted,
		ReasonCode:       "FILE_CONTENT_INVALID",
		PolicyVersion:    "file-upload-v1",
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal upload audit metadata: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal upload audit metadata: %v", err)
	}

	got := make([]string, 0, len(fields))
	for field := range fields {
		got = append(got, field)
	}
	sort.Strings(got)

	want := []string{
		"declared_mime",
		"detected_mime",
		"file_name",
		"file_size",
		"policy_version",
		"purpose",
		"reason_code",
		"validation_result",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON fields = %v, want controlled allowlist %v", got, want)
	}
}

func TestUploadAuditValidationResultsAreStable(t *testing.T) {
	if UploadValidationAccepted != "accepted" {
		t.Fatalf("accepted result = %q", UploadValidationAccepted)
	}
	if UploadValidationRejected != "rejected" {
		t.Fatalf("rejected result = %q", UploadValidationRejected)
	}
}
