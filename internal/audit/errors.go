package audit

import "errors"

type ErrorCode string

const (
	CodeValidationInvalid ErrorCode = "AUDIT_VALIDATION_INVALID"
	CodeInternalError     ErrorCode = "AUDIT_INTERNAL_ERROR"
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
