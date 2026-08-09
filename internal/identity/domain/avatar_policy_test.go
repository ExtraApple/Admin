package domain_test

import (
	"testing"

	"admin/internal/identity/domain"
)

func TestTrustedAvatarObjectNameAcceptsOnlyValidatedOwnedObjects(t *testing.T) {
	const objectName = "avatars/7/00000000-0000-4000-8000-000000000007.png"
	tests := []struct {
		name string
		user domain.User
		id   uint
		want bool
	}{
		{name: "trusted", user: domain.User{AvatarObjectName: objectName, AvatarContentType: "image/png", AvatarValidationStatus: domain.AvatarValidationStatusValidated}, id: 7, want: true},
		{name: "unvalidated", user: domain.User{AvatarObjectName: objectName, AvatarContentType: "image/png"}, id: 7},
		{name: "wrong owner", user: domain.User{AvatarObjectName: objectName, AvatarContentType: "image/png", AvatarValidationStatus: domain.AvatarValidationStatusValidated}, id: 8},
		{name: "wrong mime", user: domain.User{AvatarObjectName: objectName, AvatarContentType: "image/jpeg", AvatarValidationStatus: domain.AvatarValidationStatusValidated}, id: 7},
		{name: "nested object", user: domain.User{AvatarObjectName: "avatars/7/nested/00000000-0000-4000-8000-000000000007.png", AvatarContentType: "image/png", AvatarValidationStatus: domain.AvatarValidationStatusValidated}, id: 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotName, gotTrusted := domain.TrustedAvatarObjectName(test.user, test.id)
			if gotTrusted != test.want {
				t.Fatalf("TrustedAvatarObjectName() trusted = %v, want %v", gotTrusted, test.want)
			}
			if test.want && gotName != objectName {
				t.Fatalf("TrustedAvatarObjectName() name = %q, want %q", gotName, objectName)
			}
			if !test.want && gotName != "" {
				t.Fatalf("TrustedAvatarObjectName() rejected name = %q, want empty", gotName)
			}
		})
	}
}
