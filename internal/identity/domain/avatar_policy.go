package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const AvatarValidationStatusValidated = "validated"

// TrustedAvatarObjectName accepts only server-issued avatar objects owned by
// the requested user and carrying the MIME type that matches the file suffix.
func TrustedAvatarObjectName(user User, userID uint) (string, bool) {
	if userID == 0 || user.AvatarValidationStatus != AvatarValidationStatusValidated {
		return "", false
	}
	prefix := fmt.Sprintf("avatars/%d/", userID)
	if !strings.HasPrefix(user.AvatarObjectName, prefix) {
		return "", false
	}
	fileName := strings.TrimPrefix(user.AvatarObjectName, prefix)
	if fileName == "" || strings.ContainsAny(fileName, `/\\`) {
		return "", false
	}
	extension := ""
	expectedMIME := ""
	switch {
	case strings.HasSuffix(fileName, ".jpg"):
		extension, expectedMIME = ".jpg", "image/jpeg"
	case strings.HasSuffix(fileName, ".png"):
		extension, expectedMIME = ".png", "image/png"
	default:
		return "", false
	}
	if user.AvatarContentType != expectedMIME {
		return "", false
	}
	idText := strings.TrimSuffix(fileName, extension)
	id, err := uuid.Parse(idText)
	return user.AvatarObjectName, err == nil && id.String() == idText
}

// DirectoryUser is the Identity-owned, persistence-free user value shared by
// member and administrator read models. Avatar contains only a public route.
type DirectoryUser struct {
	ID       uint
	Username string
	Nickname string
	Avatar   string
	Email    string
	Role     string
	Status   int
}
