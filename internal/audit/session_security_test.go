package audit

import (
	"encoding/json"
	"testing"
)

func TestSessionCredentialsAndIssuanceMaterialAreRedacted(t *testing.T) {
	body := []byte(`{"operation":"refresh","Cookie":"credential-one","Set-Cookie":"credential-two","nested":[{"RefreshID":"credential-three","refresh_id":"credential-four","jti":"credential-five","AccessToken":"credential-six","Material":{"RefreshID":"credential-seven"}}]}`)
	var actual map[string]any
	if err := json.Unmarshal([]byte(SanitizeBody(body)), &actual); err != nil {
		t.Fatal(err)
	}
	if actual["operation"] != "refresh" || actual["Cookie"] != "***" || actual["Set-Cookie"] != "***" {
		t.Fatal("safe operation or credential redaction did not satisfy the audit contract")
	}
	nested := actual["nested"].([]any)[0].(map[string]any)
	for _, field := range []string{"RefreshID", "refresh_id", "jti", "AccessToken", "Material"} {
		if nested[field] != "***" {
			t.Errorf("credential field %s was retained", field)
		}
	}
}
