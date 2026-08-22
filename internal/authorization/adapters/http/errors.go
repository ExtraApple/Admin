package httpadapter

import (
	"net/http"

	"admin/internal/platform/httpresponse"
)

func authzNotFound() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "authorization", Code: "AUTHZ_NOT_FOUND", Status: http.StatusNotFound, Message: "authorization resource was not found"}
}

func authzConflict() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "authorization", Code: "AUTHZ_CONFLICT", Status: http.StatusConflict, Message: "authorization resource conflicts with an existing resource"}
}

func authzValidation() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "authorization", Code: "AUTHZ_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "authorization validation failed"}
}

func authzInvalidUser() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "authorization", Code: "AUTHZ_INVALID_USER", Status: http.StatusUnprocessableEntity, Message: "authorization user is invalid"}
}

func authzPermissionCodeAbsent() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "authorization", Code: "AUTHZ_PERMISSION_CODE_NOT_FOUND", Status: http.StatusUnprocessableEntity, Message: "authorization permission code is missing"}
}

func authzInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "authorization", Code: "AUTHZ_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "authorization operation failed"}
}
