package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditLogInfoSerializesMetadataAsJSONObject(t *testing.T) {
	info := AuditLogInfo{
		Metadata: json.RawMessage(`{"purpose":"avatar","validation_result":"blocked"}`),
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal audit log info: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"metadata":{"purpose":"avatar","validation_result":"blocked"}`) {
		t.Fatalf("metadata should be a JSON object, got %s", got)
	}
}
