package application

import "errors"

type ErrorCode string

const (
	CodeNotFound             ErrorCode = "AUTHZ_NOT_FOUND"
	CodeInvalidUser          ErrorCode = "AUTHZ_INVALID_USER"
	CodePermissionCodeAbsent ErrorCode = "AUTHZ_PERMISSION_CODE_NOT_FOUND"
	CodeValidationInvalid    ErrorCode = "AUTHZ_VALIDATION_INVALID"
	CodeConflict             ErrorCode = "AUTHZ_CONFLICT"
	CodeInternalError        ErrorCode = "AUTHZ_INTERNAL_ERROR"
)

type Error struct {
	Code  ErrorCode
	cause error
}

func NewError(code ErrorCode, cause error) *Error { return &Error{Code: code, cause: cause} }
func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	return string(err.Code)
}
func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}
func CodeOf(err error) (ErrorCode, bool) {
	var classified *Error
	if !errors.As(err, &classified) || classified == nil {
		return "", false
	}
	return classified.Code, true
}
func wrapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return NewError(CodeNotFound, err)
	}
	return NewError(CodeInternalError, err)
}
