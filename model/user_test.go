package model

import (
	"testing"
	"time"
)

func TestUserKeepsLegacyAvatarAndExposesValidatedAvatarMetadata(t *testing.T) {
	validatedAt := time.Date(2026, time.July, 14, 13, 0, 0, 0, time.UTC)
	user := User{
		Avatar:                 "https://legacy.example/avatar.png",
		AvatarObjectName:       "avatars/7/avatar.jpg",
		AvatarContentType:      "image/jpeg",
		AvatarContentSHA256:    "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		AvatarValidationStatus: FileValidationStatusValidated,
		AvatarValidatedAt:      &validatedAt,
	}

	if user.Avatar != "https://legacy.example/avatar.png" {
		t.Fatalf("legacy avatar: got %q", user.Avatar)
	}
	if user.AvatarObjectName != "avatars/7/avatar.jpg" {
		t.Fatalf("avatar object name: got %q", user.AvatarObjectName)
	}
	if user.AvatarContentType != "image/jpeg" {
		t.Fatalf("avatar content type: got %q", user.AvatarContentType)
	}
	if !isLowercaseSHA256Hex(user.AvatarContentSHA256) {
		t.Fatalf("avatar content sha256 = %q, want 64 lowercase hex characters", user.AvatarContentSHA256)
	}
	if user.AvatarValidationStatus != "validated" {
		t.Fatalf("avatar validation status: got %q", user.AvatarValidationStatus)
	}
	if user.AvatarValidatedAt == nil || !user.AvatarValidatedAt.Equal(validatedAt) {
		t.Fatalf("avatar validated at: got %v, want %v", user.AvatarValidatedAt, validatedAt)
	}
}
