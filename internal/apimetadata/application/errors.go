package application

import "errors"

type ErrorCode string

const (
	CodeNotFound                ErrorCode = "API_META_NOT_FOUND"
	CodeConflict                ErrorCode = "API_META_CONFLICT"
	CodeValidationInvalid       ErrorCode = "API_META_VALIDATION_INVALID"
	CodePermissionNotConfigured ErrorCode = "API_META_PERMISSION_NOT_CONFIGURED"
	CodeDisabled                ErrorCode = "API_META_DISABLED"
	CodePermissionMissing       ErrorCode = "API_META_PERMISSION_MISSING"
	CodePermissionDenied        ErrorCode = "API_META_PERMISSION_DENIED"
	CodeInternalError           ErrorCode = "API_META_INTERNAL_ERROR"
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
