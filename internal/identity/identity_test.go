package identity_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"admin/internal/identity"
)

func TestIdentityOwnsUsersTableAndAuthenticationDTOs(t *testing.T) {
	models := identity.Models()
	if len(models) != 1 {
		t.Fatalf("Identity models = %d, want one User model", len(models))
	}
	if reflect.TypeOf(models[0]) != reflect.TypeOf(identity.User{}) {
		t.Fatalf("Identity model = %T, want identity.User", models[0])
	}

	request := identity.RegisterRequest{
		Username:    "alice",
		Password:    "ValidPass123!",
		Email:       "alice@example.com",
		Nickname:    "Alice",
		CaptchaID:   "captcha-id",
		CaptchaCode: "123456",
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal RegisterRequest: %v", err)
	}
	want := `{"username":"alice","password":"ValidPass123!","email":"alice@example.com","nickname":"Alice","captcha_id":"captcha-id","captcha_code":"123456"}`
	if string(encoded) != want {
		t.Fatalf("RegisterRequest JSON = %s, want %s", encoded, want)
	}

	var update identity.UpdateSelfRequest
	if err := json.Unmarshal([]byte(`{"nickname":"after","avatar":null}`), &update); err != nil {
		t.Fatalf("unmarshal UpdateSelfRequest: %v", err)
	}
	if !update.HasAvatarField() {
		t.Fatal("UpdateSelfRequest did not preserve explicit avatar field")
	}
}
