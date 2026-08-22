package httpadapter

import (
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/identity/application"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"admin/internal/uploadsecurity"
	"github.com/gin-gonic/gin"
)

type loginErrorData struct {
	RemainingAttempts int `json:"remaining_attempts,omitempty"`
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`
}

func authnCaptchaInvalid() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_CAPTCHA_INVALID", Status: http.StatusUnprocessableEntity, Message: "captcha is invalid"}
}
func authnCredentialsInvalid() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_CREDENTIALS_INVALID", Status: http.StatusUnauthorized, Message: "username or password is invalid", DataSchema: reflect.TypeOf(loginErrorData{})}
}
func authnLoginLocked() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_LOGIN_LOCKED", Status: http.StatusTooManyRequests, Message: "login is temporarily locked", DataSchema: reflect.TypeOf(loginErrorData{})}
}
func authnRefreshInvalid() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_REFRESH_TOKEN_INVALID", Status: http.StatusUnauthorized, Message: "refresh token is invalid"}
}
func authnTokenInvalid() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_TOKEN_INVALID", Status: http.StatusUnauthorized, Message: "authentication token is invalid"}
}
func identityAccountDisabled() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "IDENTITY_ACCOUNT_DISABLED", Status: http.StatusForbidden, Message: "account is disabled"}
}
func identityValidationInvalid() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{
		Owner: "identity", Code: "IDENTITY_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity,
		Message: "request validation failed", DataSchema: reflect.TypeOf(httpresponse.ValidationErrorData{}),
		Fields: []httpresponse.FieldErrorDefinition{
			{Field: "password", Code: "IDENTITY_PASSWORD_INVALID", Message: "password does not meet the security requirements"},
			{Field: "confirm_password", Code: "IDENTITY_PASSWORD_CONFIRMATION_INVALID", Message: "password confirmation does not match"},
			{Field: "new_password", Code: "IDENTITY_PASSWORD_REUSE", Message: "new password must differ from the old password"},
			{Field: "old_password", Code: "IDENTITY_OLD_PASSWORD_INVALID", Message: "old password is invalid"},
		},
	}
}
func identityConflict() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "IDENTITY_CONFLICT", Status: http.StatusConflict, Message: "identity resource conflicts with an existing resource"}
}
func identityPermissionDenied() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "IDENTITY_PERMISSION_DENIED", Status: http.StatusForbidden, Message: "identity operation is not permitted"}
}
func identityInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "IDENTITY_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "identity operation failed"}
}

func identityErrorDefinitions() []httpresponse.ErrorDefinition {
	return []httpresponse.ErrorDefinition{
		httpresponse.RequestInvalidDefinition(), authnCaptchaInvalid(), authnCredentialsInvalid(), authnLoginLocked(), authnRefreshInvalid(), authnTokenInvalid(), identityAccountDisabled(), identityValidationInvalid(), identityConflict(), identityPermissionDenied(), identityInternal(),
	}
}

func identityErrorResponse(status int) routecatalog.Response {
	var definitions []httpresponse.ErrorDefinition
	for _, definition := range identityErrorDefinitions() {
		if definition.Status == status {
			definitions = append(definitions, definition)
		}
	}
	if len(definitions) == 0 {
		return routecatalog.ErrorResponse("request failed", httpresponse.InternalErrorDefinition())
	}
	return routecatalog.ErrorResponse(definitions[0].Message, definitions...)
}

func writeIdentityError(c *gin.Context, err error, refresh bool) {
	if refresh {
		httpresponse.WriteError(c, authnRefreshInvalid(), err, nil)
		return
	}
	definition, data := classifyIdentityError(err)
	if definition.Code == string(application.CodeLoginLocked) {
		if details, ok := application.DetailsOf(err); ok && details.RetryAfterSeconds > 0 {
			c.Header("Retry-After", strconv.Itoa(details.RetryAfterSeconds))
		}
	}
	httpresponse.WriteError(c, definition, err, data)
}

func classifyIdentityError(err error) (httpresponse.ErrorDefinition, any) {
	data := identityErrorData(err)
	if code, ok := application.CodeOf(err); ok {
		switch code {
		case application.CodeCaptchaInvalid:
			return authnCaptchaInvalid(), nil
		case application.CodeCredentialsInvalid:
			return authnCredentialsInvalid(), data
		case application.CodeLoginLocked:
			return authnLoginLocked(), data
		case application.CodeRefreshTokenInvalid:
			return authnRefreshInvalid(), nil
		case application.CodeTokenInvalid:
			return authnTokenInvalid(), nil
		case application.CodeAccountDisabled:
			return identityAccountDisabled(), nil
		case application.CodeValidationInvalid:
			return identityValidationInvalid(), data
		case application.CodeConflict:
			return identityConflict(), nil
		case application.CodePermissionDenied:
			return identityPermissionDenied(), nil
		case application.CodeUserNotFound:
			return authnTokenInvalid(), nil
		case application.CodeInternalError:
			return identityInternal(), nil
		}
	}
	return identityInternal(), nil
}

func identityErrorData(err error) any {
	details, ok := application.DetailsOf(err)
	if !ok {
		return nil
	}
	if len(details.Fields) > 0 {
		fields := make([]httpresponse.FieldError, len(details.Fields))
		for index, field := range details.Fields {
			fields[index] = httpresponse.FieldError{Field: field.Field, ErrorCode: field.ErrorCode, Message: field.Message}
		}
		return httpresponse.ValidationErrorData{Fields: fields}
	}
	if details.RemainingAttempts > 0 || details.RetryAfterSeconds > 0 {
		return loginErrorData{RemainingAttempts: details.RemainingAttempts, RetryAfterSeconds: details.RetryAfterSeconds}
	}
	return nil
}

func identitySuccess(c *gin.Context, data any) {
	httpresponse.WriteSuccess(c, http.StatusOK, data)
}

func identityRequestError(c *gin.Context, err error) {
	httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
}

func authnHeaderMissing() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_HEADER_MISSING", Status: http.StatusUnauthorized, Message: "authorization header is required"}
}

func authnHeaderInvalid() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "identity", Code: "AUTHN_HEADER_INVALID", Status: http.StatusUnauthorized, Message: "authorization header is invalid"}
}

func avatarDefinition(code string, cause error) httpresponse.ErrorDefinition {
	mapped := uploadsecurity.ToHTTPError(uploadsecurity.NewError(uploadsecurity.Code(code), cause))
	return httpresponse.ErrorDefinition{Owner: "files", Code: code, Status: mapped.Status, Message: avatarEnglishMessage(code)}
}

func avatarEnglishMessage(code string) string {
	switch code {
	case "REQUEST_INVALID":
		return "request is invalid"
	case "AVATAR_FIELD_NOT_WRITABLE":
		return "avatar must be changed through the avatar endpoint"
	case "UPLOAD_BODY_INVALID":
		return "upload body is invalid"
	case "UPLOAD_BODY_TOO_LARGE", "FILE_TOO_LARGE":
		return "upload exceeds the size limit"
	case "UPLOAD_FILE_MISSING":
		return "upload file is required"
	case "UPLOAD_MULTIPLE_FILES":
		return "only one upload file is allowed"
	case "FILE_EMPTY":
		return "file is empty"
	case "FILE_TYPE_NOT_ALLOWED":
		return "file type is not allowed"
	case "FILE_TYPE_MISMATCH":
		return "file type does not match its content"
	case "FILE_ENCODING_INVALID":
		return "file encoding is invalid"
	case "FILE_CONTENT_INVALID":
		return "file content is invalid"
	case "IMAGE_DIMENSION_LIMIT":
		return "image dimensions exceed the limit"
	case "IMAGE_DECODE_INVALID":
		return "image cannot be decoded"
	case "STORAGE_OBJECT_NOT_FOUND":
		return "storage object was not found"
	case "STORAGE_UNAVAILABLE":
		return "file storage is unavailable"
	case "PERSISTENCE_FAILED":
		return "file persistence failed"
	default:
		return "file operation failed"
	}
}

func AuthenticationErrorDefinitions() []httpresponse.ErrorDefinition {
	return []httpresponse.ErrorDefinition{authnHeaderMissing(), authnHeaderInvalid(), authnTokenInvalid(), identityAccountDisabled()}
}
