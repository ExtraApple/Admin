package uploadsecurity_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"admin/service/uploadsecurity"
)

func TestToHTTPErrorMapsStableCodesCentrally(t *testing.T) {
	tests := []struct {
		code       uploadsecurity.Code
		wantStatus int
	}{
		{uploadsecurity.CodeRequestInvalid, http.StatusBadRequest},
		{uploadsecurity.CodeAvatarFieldNotWritable, http.StatusBadRequest},
		{uploadsecurity.CodeUploadBodyInvalid, http.StatusBadRequest},
		{uploadsecurity.CodeUploadBodyTooLarge, http.StatusRequestEntityTooLarge},
		{uploadsecurity.CodeUploadFileMissing, http.StatusBadRequest},
		{uploadsecurity.CodeUploadMultipleFiles, http.StatusBadRequest},
		{uploadsecurity.CodeFileEmpty, http.StatusBadRequest},
		{uploadsecurity.CodeFileTooLarge, http.StatusRequestEntityTooLarge},
		{uploadsecurity.CodeFileNameInvalid, http.StatusBadRequest},
		{uploadsecurity.CodeFileTypeNotAllowed, http.StatusUnsupportedMediaType},
		{uploadsecurity.CodeFileTypeMismatch, http.StatusUnsupportedMediaType},
		{uploadsecurity.CodeFileEncodingInvalid, http.StatusUnprocessableEntity},
		{uploadsecurity.CodeFileContentInvalid, http.StatusUnprocessableEntity},
		{uploadsecurity.CodeImageDimensionLimit, http.StatusUnprocessableEntity},
		{uploadsecurity.CodeImageDecodeInvalid, http.StatusUnprocessableEntity},
		{uploadsecurity.CodeFileNotFound, http.StatusNotFound},
		{uploadsecurity.CodeFileAccessInvalid, http.StatusForbidden},
		{uploadsecurity.CodeFileStateBlocked, http.StatusConflict},
		{uploadsecurity.CodeFileStateConflict, http.StatusConflict},
		{uploadsecurity.CodeStorageObjectNotFound, http.StatusNotFound},
		{uploadsecurity.CodeStorageUnavailable, http.StatusServiceUnavailable},
		{uploadsecurity.CodePersistenceFailed, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			got := uploadsecurity.ToHTTPError(uploadsecurity.NewError(tt.code, errors.New("internal detail")))
			if got.Status != tt.wantStatus {
				t.Fatalf("status: got %d, want %d", got.Status, tt.wantStatus)
			}
			if got.Code != tt.code {
				t.Fatalf("code: got %q, want %q", got.Code, tt.code)
			}
			if strings.TrimSpace(got.Message) == "" {
				t.Fatal("client message should not be empty")
			}
		})
	}
}

func TestToHTTPErrorDoesNotExposeInternalCause(t *testing.T) {
	secrets := []string{
		"zip parser offset 0xdeadbeef",
		"mysql password=secret",
		"minio access key AKIA_INTERNAL",
	}

	for _, secret := range secrets {
		got := uploadsecurity.ToHTTPError(
			uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, errors.New(secret)),
		)
		if strings.Contains(got.Message, secret) {
			t.Fatalf("client message leaked internal cause %q", secret)
		}
	}
}

func TestToHTTPErrorUsesSafeInternalFallback(t *testing.T) {
	got := uploadsecurity.ToHTTPError(errors.New("unclassified database details"))
	if got.Status != http.StatusInternalServerError {
		t.Fatalf("status: got %d", got.Status)
	}
	if got.Code != uploadsecurity.CodeInternalError {
		t.Fatalf("code: got %q", got.Code)
	}
	if got.Message != "服务器内部错误" {
		t.Fatalf("message: got %q", got.Message)
	}
}
