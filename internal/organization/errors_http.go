package organization

import (
	"net/http"

	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

func orgNotFound() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "organization", Code: "ORG_NOT_FOUND", Status: http.StatusNotFound, Message: "organization resource was not found"}
}
func orgConflict() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "organization", Code: "ORG_CONFLICT", Status: http.StatusConflict, Message: "organization resource conflicts with an existing resource"}
}
func orgValidation() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "organization", Code: "ORG_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "organization validation failed"}
}
func orgInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "organization", Code: "ORG_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "organization operation failed"}
}
func classifyOrganizationError(err error) httpresponse.ErrorDefinition {
	if code, ok := CodeOf(err); ok {
		switch code {
		case CodeNotFound:
			return orgNotFound()
		case CodeConflict:
			return orgConflict()
		case CodeValidationInvalid:
			return orgValidation()
		case CodeInternalError:
			return orgInternal()
		}
	}
	return orgInternal()
}
func organizationSuccess(c *gin.Context, data any) { httpresponse.WriteSuccess(c, http.StatusOK, data) }
func organizationError(c *gin.Context, err error) {
	httpresponse.WriteError(c, classifyOrganizationError(err), err, nil)
}
