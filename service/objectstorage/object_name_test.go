package objectstorage

import (
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"

	"admin/service/uploadsecurity"
)

func TestNewManagedFileObjectNameUsesUUIDAndCanonicalExtension(t *testing.T) {
	tests := []struct {
		canonicalType uploadsecurity.CanonicalType
		wantExtension string
	}{
		{uploadsecurity.TypeJPEG, ".jpg"},
		{uploadsecurity.TypePDF, ".pdf"},
		{uploadsecurity.TypeDOCX, ".docx"},
		{uploadsecurity.TypeCSV, ".csv"},
	}

	for _, tt := range tests {
		t.Run(string(tt.canonicalType), func(t *testing.T) {
			name, err := NewManagedFileObjectName(tt.canonicalType)
			if err != nil {
				t.Fatalf("NewManagedFileObjectName() error = %v", err)
			}
			if path.Ext(name) != tt.wantExtension {
				t.Fatalf("object extension = %q, want %q", path.Ext(name), tt.wantExtension)
			}
			if strings.Contains(name, "/") {
				t.Fatalf("managed object name = %q, want UUID at bucket root", name)
			}
			if _, err := uuid.Parse(strings.TrimSuffix(name, tt.wantExtension)); err != nil {
				t.Fatalf("object name %q does not contain a UUID: %v", name, err)
			}
		})
	}
}

func TestNewManagedFileObjectNameRejectsUnknownCanonicalType(t *testing.T) {
	if _, err := NewManagedFileObjectName(uploadsecurity.CanonicalType("zip")); err == nil {
		t.Fatal("NewManagedFileObjectName() error = nil, want unknown type rejection")
	}
}

func TestNewAvatarObjectNameUsesUserDirectoryUUIDAndNormalizedOutputExtension(t *testing.T) {
	tests := []struct {
		canonicalType uploadsecurity.CanonicalType
		wantExtension string
	}{
		{uploadsecurity.TypeJPEG, ".jpg"},
		{uploadsecurity.TypePNG, ".png"},
	}

	for _, tt := range tests {
		t.Run(string(tt.canonicalType), func(t *testing.T) {
			name, err := NewAvatarObjectName(42, tt.canonicalType)
			if err != nil {
				t.Fatalf("NewAvatarObjectName() error = %v", err)
			}
			const prefix = "avatars/42/"
			if !strings.HasPrefix(name, prefix) {
				t.Fatalf("avatar object name = %q, want prefix %q", name, prefix)
			}
			fileName := strings.TrimPrefix(name, prefix)
			if path.Ext(fileName) != tt.wantExtension {
				t.Fatalf("avatar extension = %q, want %q", path.Ext(fileName), tt.wantExtension)
			}
			if _, err := uuid.Parse(strings.TrimSuffix(fileName, tt.wantExtension)); err != nil {
				t.Fatalf("avatar object name %q does not contain a UUID: %v", name, err)
			}
		})
	}
}

func TestNewAvatarObjectNameRejectsNonNormalizedAvatarTypes(t *testing.T) {
	for _, canonicalType := range []uploadsecurity.CanonicalType{
		uploadsecurity.TypeWebP,
		uploadsecurity.TypePDF,
		uploadsecurity.CanonicalType("zip"),
	} {
		if _, err := NewAvatarObjectName(42, canonicalType); err == nil {
			t.Errorf("NewAvatarObjectName(%q) error = nil, want type rejection", canonicalType)
		}
	}
}
