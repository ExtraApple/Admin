package audit

import (
	"strings"
	"testing"
)

func TestMessageAuditMetadataContainsOnlyActionAndResourceReference(t *testing.T) {
	metadata := MessageMetadata("POST", "/api/admin/announcements/41/revoke")
	text := string(metadata)
	if !strings.Contains(text, `"action":"message.revoke"`) || !strings.Contains(text, `"resource_id":41`) {
		t.Fatalf("message audit metadata = %s", text)
	}
	for _, forbidden := range []string{"markdown", "body_html", "url", "token"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("message audit metadata leaked %q: %s", forbidden, text)
		}
	}
}
