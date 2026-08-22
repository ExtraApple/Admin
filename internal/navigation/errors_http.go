package navigation

import (
	"net/http"

	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

func navNotFound() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "navigation", Code: "NAV_NOT_FOUND", Status: http.StatusNotFound, Message: "navigation resource was not found"}
}
func navConflict() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "navigation", Code: "NAV_CONFLICT", Status: http.StatusConflict, Message: "navigation resource conflicts with an existing resource"}
}
func navValidation() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "navigation", Code: "NAV_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "navigation validation failed"}
}
func navInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "navigation", Code: "NAV_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "navigation operation failed"}
}
func classifyNavigationError(err error) httpresponse.ErrorDefinition {
	if code, ok := CodeOf(err); ok {
		switch code {
		case CodeNotFound:
			return navNotFound()
		case CodeConflict:
			return navConflict()
		case CodeValidationInvalid:
			return navValidation()
		case CodeInternalError:
			return navInternal()
		}
	}
	return navInternal()
}
func navigationSuccess(c *gin.Context, data any) { httpresponse.WriteSuccess(c, http.StatusOK, data) }
func navigationError(c *gin.Context, err error) {
	httpresponse.WriteError(c, classifyNavigationError(err), err, nil)
}
