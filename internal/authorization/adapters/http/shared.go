package httpadapter

import (
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/authorization/application"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

// Handler holds the capabilities shared by Authorization HTTP routes.
type Handler struct {
	service *application.Service
	routes  application.RoutePermissionSource
}

func authRoute(method, path, name, permission string, handler gin.HandlerFunc, requestSchema, responseSchema any) routecatalog.Descriptor {
	request := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if requestSchema != nil {
		request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(requestSchema), Required: true}
	}
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: handler,
		Name: name, Group: "authorization", DefaultPermissionCode: permission, DefaultAuditCategory: "authorization",
		OpenAPI: routecatalog.Operation{
			Summary: name, Request: request,
			Responses: map[int]routecatalog.Response{
				http.StatusOK:                  routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(responseSchema)),
				http.StatusBadRequest:          routecatalog.ErrorResponse("bad request", httpresponse.RequestInvalidDefinition()),
				http.StatusNotFound:            routecatalog.ErrorResponse("authorization resource was not found", authzNotFound()),
				http.StatusConflict:            routecatalog.ErrorResponse("authorization resource conflicts", authzConflict()),
				http.StatusUnprocessableEntity: routecatalog.ErrorResponse("authorization validation failed", authzValidation(), authzInvalidUser(), authzPermissionCodeAbsent()),
				http.StatusInternalServerError: routecatalog.ErrorResponse("authorization operation failed", authzInternal()),
			},
		},
	}
}

func queryPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	return page, size
}
func pathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
		return 0, false
	}
	return uint(value), true
}

func bindJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
		return false
	}
	return true
}

func badRequest(c *gin.Context, err error) {
	definition := authzInternal()
	if code, ok := application.CodeOf(err); ok {
		switch code {
		case application.CodeNotFound:
			definition = authzNotFound()
		case application.CodeConflict:
			definition = authzConflict()
		case application.CodeInvalidUser:
			definition = authzInvalidUser()
		case application.CodePermissionCodeAbsent:
			definition = authzPermissionCodeAbsent()
		case application.CodeValidationInvalid:
			definition = authzValidation()
		case application.CodeInternalError:
			definition = authzInternal()
		}
	}
	httpresponse.WriteError(c, definition, err, nil)
}

func success(c *gin.Context, data any) {
	httpresponse.WriteSuccess(c, http.StatusOK, data)
}
