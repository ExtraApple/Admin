package organization

import (
	"net/http"
	"reflect"

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
	return httpresponse.ErrorDefinition{
		Owner: "organization", Code: "ORG_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "organization validation failed",
		DataSchema: reflect.TypeOf(httpresponse.ValidationErrorData{}),
		Fields: []httpresponse.FieldErrorDefinition{
			{Field: "organization_ids", Code: "ORG_ORGANIZATION_IDS_INVALID", Message: "organization_ids must be an explicit array of positive IDs"},
			{Field: "expected_access_version", Code: "ORG_ACCESS_VERSION_INVALID", Message: "expected_access_version must be positive"},
		},
	}
}
func orgInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "organization", Code: "ORG_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "organization operation failed"}
}
func orgPermissionDenied() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "organization", Code: "ORG_PERMISSION_DENIED", Status: http.StatusForbidden, Message: "organization operation is not permitted"}
}
func classifyOrganizationError(err error) httpresponse.ErrorDefinition {
	if code, ok := CodeOf(err); ok {
		switch code {
		case CodeNotFound:
			return orgNotFound()
		case CodeConflict:
			return orgConflict()
		case CodePermissionDenied:
			return orgPermissionDenied()
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
