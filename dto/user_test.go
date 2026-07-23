package dto

import (
	"encoding/json"
	"testing"
)

func TestUserInfoJSONDoesNotExposeAvatarContentDigest(t *testing.T) {
	info := UserInfo{
		ID:       7,
		Username: "alice",
		Nickname: "Alice",
		Avatar:   "/api/avatars/7",
		Email:    "alice@example.com",
		Role:     "user",
		Status:   1,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, exists := payload["avatar_content_sha256"]; exists {
		t.Fatalf("JSON unexpectedly exposes avatar_content_sha256: %s", data)
	}
}
