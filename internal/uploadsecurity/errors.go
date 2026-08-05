package uploadsecurity

import "errors"

// Code is a stable machine-readable upload security error code.
type Code string

const (
	CodeRequestInvalid         Code = "REQUEST_INVALID"
	CodeAvatarFieldNotWritable Code = "AVATAR_FIELD_NOT_WRITABLE"
	CodeUploadBodyInvalid      Code = "UPLOAD_BODY_INVALID"
	CodeUploadBodyTooLarge     Code = "UPLOAD_BODY_TOO_LARGE"
	CodeUploadFileMissing      Code = "UPLOAD_FILE_MISSING"
	CodeUploadMultipleFiles    Code = "UPLOAD_MULTIPLE_FILES"
	CodeFileEmpty              Code = "FILE_EMPTY"
	CodeFileTooLarge           Code = "FILE_TOO_LARGE"
	CodeFileNameInvalid        Code = "FILE_NAME_INVALID"
	CodeFileTypeNotAllowed     Code = "FILE_TYPE_NOT_ALLOWED"
	CodeFileTypeMismatch       Code = "FILE_TYPE_MISMATCH"
	CodeFileEncodingInvalid    Code = "FILE_ENCODING_INVALID"
	CodeFileContentInvalid     Code = "FILE_CONTENT_INVALID"
	CodeImageDimensionLimit    Code = "IMAGE_DIMENSION_LIMIT"
	CodeImageDecodeInvalid     Code = "IMAGE_DECODE_INVALID"
	CodeFileNotFound           Code = "FILE_NOT_FOUND"
	CodeFileAccessInvalid      Code = "FILE_ACCESS_INVALID"
	CodeFileStateBlocked       Code = "FILE_STATE_BLOCKED"
	CodeFileStateConflict      Code = "FILE_STATE_CONFLICT"
	CodeStorageObjectNotFound  Code = "STORAGE_OBJECT_NOT_FOUND"
	CodeStorageUnavailable     Code = "STORAGE_UNAVAILABLE"
	CodePersistenceFailed      Code = "PERSISTENCE_FAILED"
	CodeInternalError          Code = "INTERNAL_ERROR"
)

// Error classifies a failure while retaining its internal cause for controlled
// server-side logging and retry decisions.
type Error struct {
	Code  Code
	cause error
}

// NewError creates a classified upload security error.
func NewError(code Code, cause error) *Error {
	return &Error{Code: code, cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return string(e.Code)
}

// Unwrap exposes the internal cause to errors.Is/errors.As without including
// it in the public error string.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// CodeOf returns the stable upload security code from an error chain.
func CodeOf(err error) (Code, bool) {
	var securityErr *Error
	if !errors.As(err, &securityErr) || securityErr == nil {
		return "", false
	}
	return securityErr.Code, true
}
