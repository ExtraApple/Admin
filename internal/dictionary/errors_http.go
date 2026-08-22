package dictionary

import (
	"net/http"

	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

func dictNotFound() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "dictionary", Code: "DICT_NOT_FOUND", Status: http.StatusNotFound, Message: "dictionary resource was not found"}
}
func dictConflict() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "dictionary", Code: "DICT_CONFLICT", Status: http.StatusConflict, Message: "dictionary resource conflicts with an existing resource"}
}
func dictValidation() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "dictionary", Code: "DICT_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "dictionary validation failed"}
}
func dictInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "dictionary", Code: "DICT_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "dictionary operation failed"}
}
func classifyDictionaryError(err error) httpresponse.ErrorDefinition {
	if code, ok := CodeOf(err); ok {
		switch code {
		case CodeNotFound:
			return dictNotFound()
		case CodeConflict:
			return dictConflict()
		case CodeValidationInvalid:
			return dictValidation()
		case CodeInternalError:
			return dictInternal()
		}
	}
	return dictInternal()
}
func dictionarySuccess(c *gin.Context, data any) { httpresponse.WriteSuccess(c, http.StatusOK, data) }
func dictionaryError(c *gin.Context, err error) {
	httpresponse.WriteError(c, classifyDictionaryError(err), err, nil)
}
