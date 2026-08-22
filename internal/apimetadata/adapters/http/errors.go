package httpadapter

import (
	"net/http"

	"admin/internal/apimetadata/application"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

func apiMetaNotFound() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_NOT_FOUND", Status: http.StatusNotFound, Message: "api metadata was not found"}
}
func apiMetaConflict() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_CONFLICT", Status: http.StatusConflict, Message: "api metadata conflicts with an existing resource"}
}
func apiMetaValidation() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "api metadata validation failed"}
}
func apiMetaInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "api metadata operation failed"}
}
func apiMetaPermissionNotConfigured() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_PERMISSION_NOT_CONFIGURED", Status: http.StatusForbidden, Message: "api permission policy is not configured"}
}
func apiMetaDisabled() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_DISABLED", Status: http.StatusForbidden, Message: "api is disabled"}
}
func apiMetaPermissionMissing() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_PERMISSION_MISSING", Status: http.StatusForbidden, Message: "api permission code is missing"}
}
func apiMetaPermissionDenied() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "apimetadata", Code: "API_META_PERMISSION_DENIED", Status: http.StatusForbidden, Message: "permission is denied"}
}

func classifyAPIMetaError(err error) httpresponse.ErrorDefinition {
	if code, ok := application.CodeOf(err); ok {
		switch code {
		case application.CodeNotFound:
			return apiMetaNotFound()
		case application.CodeConflict:
			return apiMetaConflict()
		case application.CodeValidationInvalid:
			return apiMetaValidation()
		case application.CodePermissionNotConfigured:
			return apiMetaPermissionNotConfigured()
		case application.CodeDisabled:
			return apiMetaDisabled()
		case application.CodePermissionMissing:
			return apiMetaPermissionMissing()
		case application.CodePermissionDenied:
			return apiMetaPermissionDenied()
		case application.CodeInternalError:
			return apiMetaInternal()
		}
	}
	return apiMetaInternal()
}

func writeAPIMetaError(c *gin.Context, err error) {
	httpresponse.WriteError(c, classifyAPIMetaError(err), err, nil)
}

func apiMetaSuccess(c *gin.Context, data any) {
	httpresponse.WriteSuccess(c, http.StatusOK, data)
}

func PermissionErrorDefinitions() []httpresponse.ErrorDefinition {
	return []httpresponse.ErrorDefinition{apiMetaPermissionNotConfigured(), apiMetaDisabled(), apiMetaPermissionMissing(), apiMetaPermissionDenied(), apiMetaInternal()}
}
