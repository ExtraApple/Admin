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
