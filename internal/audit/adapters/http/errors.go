package httpadapter

import (
	"net/http"

	"admin/internal/audit"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

func auditValidation() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "audit", Code: "AUDIT_VALIDATION_INVALID", Status: http.StatusUnprocessableEntity, Message: "audit query validation failed"}
}
func auditInternal() httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "audit", Code: "AUDIT_INTERNAL_ERROR", Status: http.StatusInternalServerError, Message: "audit operation failed"}
}
func classifyAuditError(err error) httpresponse.ErrorDefinition {
	if code, ok := audit.CodeOf(err); ok {
		switch code {
		case audit.CodeValidationInvalid:
			return auditValidation()
		case audit.CodeInternalError:
			return auditInternal()
		}
	}
	return auditInternal()
}
func auditSuccess(c *gin.Context, data any) { httpresponse.WriteSuccess(c, http.StatusOK, data) }
func auditError(c *gin.Context, err error) {
	httpresponse.WriteError(c, classifyAuditError(err), err, nil)
}

var _ audit.Clock
