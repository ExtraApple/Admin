package uploadsecurity_test

import (
	"errors"
	"fmt"
	"testing"

	"admin/internal/uploadsecurity"
)

func TestUploadSecurityErrorCodesAreStable(t *testing.T) {
	tests := map[string]uploadsecurity.Code{
		"REQUEST_INVALID":           uploadsecurity.CodeRequestInvalid,
		"AVATAR_FIELD_NOT_WRITABLE": uploadsecurity.CodeAvatarFieldNotWritable,
		"UPLOAD_BODY_INVALID":       uploadsecurity.CodeUploadBodyInvalid,
		"UPLOAD_BODY_TOO_LARGE":     uploadsecurity.CodeUploadBodyTooLarge,
		"UPLOAD_FILE_MISSING":       uploadsecurity.CodeUploadFileMissing,
		"UPLOAD_MULTIPLE_FILES":     uploadsecurity.CodeUploadMultipleFiles,
		"FILE_EMPTY":                uploadsecurity.CodeFileEmpty,
		"FILE_TOO_LARGE":            uploadsecurity.CodeFileTooLarge,
		"FILE_NAME_INVALID":         uploadsecurity.CodeFileNameInvalid,
		"FILE_TYPE_NOT_ALLOWED":     uploadsecurity.CodeFileTypeNotAllowed,
		"FILE_TYPE_MISMATCH":        uploadsecurity.CodeFileTypeMismatch,
		"FILE_ENCODING_INVALID":     uploadsecurity.CodeFileEncodingInvalid,
		"FILE_CONTENT_INVALID":      uploadsecurity.CodeFileContentInvalid,
		"IMAGE_DIMENSION_LIMIT":     uploadsecurity.CodeImageDimensionLimit,
		"IMAGE_DECODE_INVALID":      uploadsecurity.CodeImageDecodeInvalid,
		"FILE_NOT_FOUND":            uploadsecurity.CodeFileNotFound,
		"FILE_ACCESS_INVALID":       uploadsecurity.CodeFileAccessInvalid,
		"FILE_STATE_BLOCKED":        uploadsecurity.CodeFileStateBlocked,
		"FILE_STATE_CONFLICT":       uploadsecurity.CodeFileStateConflict,
		"STORAGE_OBJECT_NOT_FOUND":  uploadsecurity.CodeStorageObjectNotFound,
		"STORAGE_UNAVAILABLE":       uploadsecurity.CodeStorageUnavailable,
		"PERSISTENCE_FAILED":        uploadsecurity.CodePersistenceFailed,
		"INTERNAL_ERROR":            uploadsecurity.CodeInternalError,
	}

	for want, got := range tests {
		if string(got) != want {
			t.Errorf("code value: got %q, want %q", got, want)
		}
	}
}

func TestCodeOfFindsUploadSecurityErrorThroughWrapping(t *testing.T) {
	internal := errors.New("image decoder details")
	securityErr := uploadsecurity.NewError(uploadsecurity.CodeImageDecodeInvalid, internal)
	wrapped := fmt.Errorf("validate upload: %w", securityErr)

	code, ok := uploadsecurity.CodeOf(wrapped)
	if !ok {
		t.Fatal("expected upload security error code")
	}
	if code != uploadsecurity.CodeImageDecodeInvalid {
		t.Fatalf("code: got %q", code)
	}
	if !errors.Is(wrapped, internal) {
		t.Fatal("upload security error should retain its internal cause for server-side handling")
	}
}

func TestCodeOfRejectsUnclassifiedErrors(t *testing.T) {
	if code, ok := uploadsecurity.CodeOf(errors.New("plain error")); ok || code != "" {
		t.Fatalf("unexpected classified code %q", code)
	}
}
