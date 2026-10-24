package audit

import (
	"strings"
	"testing"
)

func TestSanitizeMessageAuditBodyOmitsContentAndURLs(t *testing.T) {
	got := SanitizeBody([]byte(`{"title":"Notice","markdown":"<script>alert(1)</script>","body_html":"<p>body</p>","external_url":"https://example.test/private"}`))
	for _, secret := range []string{"<script>", "<p>body</p>", "https://example.test/private"} {
		if strings.Contains(got, secret) {
			t.Fatalf("audit body leaked %q: %s", secret, got)
		}
	}
}

func TestSanitizeAuditQueryRedactsWebSocketTicket(t *testing.T) {
	got := SanitizeQuery("cursor=abc&ticket=secret-ticket")
	if strings.Contains(got, "secret-ticket") {
		t.Fatalf("audit query leaked ticket: %s", got)
	}
}
func TestSanitizeMessageAuditRejectsUnstructuredSensitivePayloads(t *testing.T) {
	for _, body := range []string{
		`[{"markdown":"<p>body</p>"},{"image_url":"https://secret.example/img"}]`,
		`markdown=<p>body</p>&external_url=https://secret.example/path`,
	} {
		got := SanitizeBody([]byte(body))
		for _, secret := range []string{"<p>body</p>", "https://secret.example/img", "https://secret.example/path"} {
			if strings.Contains(got, secret) {
				t.Fatalf("audit body leaked %q: %s", secret, got)
			}
		}
	}
}

func TestSanitizeAuditQueryRedactsAuthorizationAndRedirectURL(t *testing.T) {
	got := SanitizeQuery("authorization=Bearer-secret&redirect_url=https%3A%2F%2Fsecret.example%2Fpath")
	if strings.Contains(got, "Bearer-secret") || strings.Contains(got, "secret.example") {
		t.Fatalf("audit query leaked sensitive values: %s", got)
	}
}

func TestSanitizeAuditBodyRedactsCurrentPassword(t *testing.T) {
	got := SanitizeBody([]byte(`{"email":"new@example.com","current_password":"password-secret"}`))
	if strings.Contains(got, "password-secret") {
		t.Fatalf("audit body leaked current password: %s", got)
	}
}

func TestSanitizeAuditBodyRedactsEmailAddresses(t *testing.T) {
	got := SanitizeBody([]byte(`{"email":"user@example.com","pending_email":"new@example.com"}`))
	for _, secret := range []string{"user@example.com", "new@example.com"} {
		if strings.Contains(got, secret) {
			t.Fatalf("audit body leaked email address %q: %s", secret, got)
		}
	}
}

func TestSanitizeMessageAuditBodyUsesFixedOmissionMarker(t *testing.T) {
	got := SanitizeBodyForPath("POST", "/api/user/messages/private", []byte(`{"title":"Notice","markdown":"body","image_ids":[7],"external_url":"https://secret.example"}`))
	if got != "[message omitted]" {
		t.Fatalf("message audit body = %q, want fixed omission marker", got)
	}
	if strings.Contains(got, "Notice") || strings.Contains(got, "secret.example") {
		t.Fatalf("message audit body leaked content: %q", got)
	}
	if got := SanitizeBodyForPath("POST", "/api/user/messages/images", []byte("[multipart omitted]")); got != "[multipart omitted]" {
		t.Fatalf("multipart audit body = %q", got)
	}
}

func TestSanitizeAuditQueryUsesExplicitAllowlist(t *testing.T) {
	got := SanitizeQuery("page=2&size=20&user_id=7&method=GET&path=%2Fapi%2Fadmin%2Faudit-logs&status=200&category=message&start_time=2026-01-01&end_time=2026-01-31&signature=secret&expires=123&ticket=private&token=private&source=unexpected")
	for _, allowed := range []string{"page=2", "size=20", "user_id=7", "method=GET", "path=%2Fapi%2Fadmin%2Faudit-logs", "status=200", "category=message", "start_time=2026-01-01", "end_time=2026-01-31"} {
		if !strings.Contains(got, allowed) {
			t.Fatalf("allowlisted query value %q missing from %q", allowed, got)
		}
	}
	for _, forbidden := range []string{"signature", "expires", "ticket", "token", "source", "secret", "private", "unexpected"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("unapproved query value %q persisted: %s", forbidden, got)
		}
	}
}
