package application

import "errors"

type ErrorCode string

const (
	CodeCaptchaInvalid      ErrorCode = "AUTHN_CAPTCHA_INVALID"
	CodeCredentialsInvalid  ErrorCode = "AUTHN_CREDENTIALS_INVALID"
	CodeLoginLocked         ErrorCode = "AUTHN_LOGIN_LOCKED"
	CodeRefreshTokenInvalid ErrorCode = "AUTHN_REFRESH_TOKEN_INVALID"
	CodeTokenInvalid        ErrorCode = "AUTHN_TOKEN_INVALID"
	CodeAccountDisabled     ErrorCode = "IDENTITY_ACCOUNT_DISABLED"
	CodeUserNotFound        ErrorCode = "IDENTITY_USER_NOT_FOUND"
	CodePermissionDenied    ErrorCode = "IDENTITY_PERMISSION_DENIED"
	CodeValidationInvalid   ErrorCode = "IDENTITY_VALIDATION_INVALID"
	CodeConflict            ErrorCode = "IDENTITY_CONFLICT"
	CodeInternalError       ErrorCode = "IDENTITY_INTERNAL_ERROR"
)

type FieldError struct {
	Field     string
	ErrorCode string
	Message   string
}

type ErrorDetails struct {
	Fields            []FieldError
	RemainingAttempts int
	RetryAfterSeconds int
}

type Error struct {
	Code    ErrorCode
	Details ErrorDetails
	cause   error
}

func NewError(code ErrorCode, cause error) *Error { return &Error{Code: code, cause: cause} }

func NewValidationError(fields []FieldError, cause error) *Error {
	return &Error{Code: CodeValidationInvalid, Details: ErrorDetails{Fields: append([]FieldError(nil), fields...)}, cause: cause}
}

func NewCredentialsError(remainingAttempts int, cause error) *Error {
	return &Error{Code: CodeCredentialsInvalid, Details: ErrorDetails{RemainingAttempts: remainingAttempts}, cause: cause}
}

func NewLoginLockedError(retryAfterSeconds int, cause error) *Error {
	return &Error{Code: CodeLoginLocked, Details: ErrorDetails{RetryAfterSeconds: retryAfterSeconds}, cause: cause}
}

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

func DetailsOf(err error) (ErrorDetails, bool) {
	var classified *Error
	if !errors.As(err, &classified) || classified == nil {
		return ErrorDetails{}, false
	}
	return classified.Details, true
}
