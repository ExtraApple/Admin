package objectstorage

import (
	"fmt"

	"github.com/google/uuid"

	"admin/service/uploadsecurity"
)

// NewManagedFileObjectName returns a server-generated object name that contains
// no client-supplied display-name component.
func NewManagedFileObjectName(canonicalType uploadsecurity.CanonicalType) (string, error) {
	definition, ok := uploadsecurity.DefinitionForType(canonicalType)
	if !ok {
		return "", fmt.Errorf("unsupported canonical type %q", canonicalType)
	}
	return uuid.NewString() + definition.CanonicalExtension, nil
}

// NewAvatarObjectName returns a server-generated object name for normalized
// avatar output. Avatar normalization only produces JPEG or PNG.
func NewAvatarObjectName(userID uint, canonicalType uploadsecurity.CanonicalType) (string, error) {
	var extension string
	switch canonicalType {
	case uploadsecurity.TypeJPEG:
		extension = ".jpg"
	case uploadsecurity.TypePNG:
		extension = ".png"
	default:
		return "", fmt.Errorf("unsupported normalized avatar type %q", canonicalType)
	}
	return fmt.Sprintf("avatars/%d/%s%s", userID, uuid.NewString(), extension), nil
}
