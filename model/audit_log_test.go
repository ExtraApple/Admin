package model

import (
	"encoding/json"
	"testing"
)

func TestAuditModelsExposeStructuredMetadata(t *testing.T) {
	metadata := json.RawMessage(`{"purpose":"managed_file","validation_result":"validated"}`)
	log := AuditLog{Metadata: metadata}
	archive := AuditLogArchive{Metadata: metadata}

	if string(log.Metadata) != string(metadata) {
		t.Fatalf("audit log metadata: got %s", log.Metadata)
	}
	if string(archive.Metadata) != string(metadata) {
		t.Fatalf("audit archive metadata: got %s", archive.Metadata)
	}
}
