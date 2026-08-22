package dictionary

import (
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type httpHandler struct {
	service *Service
}

type typeResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data TypeInfo `json:"data"`
}

type typeListEnvelope struct {
	Code int              `json:"code"`
	Data TypeListResponse `json:"data"`
}

type itemResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data ItemInfo `json:"data"`
}

type itemListEnvelope struct {
	Code int              `json:"code"`
	Data ItemListResponse `json:"data"`
}

type enabledItemsEnvelope struct {
	Code int        `json:"code"`
	Data []ItemInfo `json:"data"`
}

func Routes(service *Service) []routecatalog.Descriptor {
	handler := &httpHandler{service: service}
	return []routecatalog.Descriptor{
		dictionaryRoute(http.MethodGet, "/api/dicts/:type_code/items", routecatalog.Public, "List enabled Dictionary Items", "", handler.listEnabledItems, nil, enabledItemsEnvelope{}),
		dictionaryRoute(http.MethodGet, "/api/admin/dict-types", routecatalog.PermissionControlled, "List Dictionary Types", "admin.dict-types.get", handler.listTypes, nil, typeListEnvelope{}),
		dictionaryRoute(http.MethodPost, "/api/admin/dict-types", routecatalog.PermissionControlled, "Create Dictionary Type", "admin.dict-types.post", handler.createType, CreateTypeRequest{}, typeResponse{}),
		dictionaryRoute(http.MethodPut, "/api/admin/dict-types/:id", routecatalog.PermissionControlled, "Update Dictionary Type", "admin.dict-types.put", handler.updateType, UpdateTypeRequest{}, typeResponse{}),
		dictionaryRoute(http.MethodDelete, "/api/admin/dict-types/:id", routecatalog.PermissionControlled, "Delete Dictionary Type", "admin.dict-types.delete", handler.deleteType, nil, nil),
		dictionaryRoute(http.MethodGet, "/api/admin/dict-items", routecatalog.PermissionControlled, "List Dictionary Items", "admin.dict-items.get", handler.listItems, nil, itemListEnvelope{}),
		dictionaryRoute(http.MethodPost, "/api/admin/dict-items", routecatalog.PermissionControlled, "Create Dictionary Item", "admin.dict-items.post", handler.createItem, CreateItemRequest{}, itemResponse{}),
		dictionaryRoute(http.MethodPut, "/api/admin/dict-items/:id", routecatalog.PermissionControlled, "Update Dictionary Item", "admin.dict-items.put", handler.updateItem, UpdateItemRequest{}, itemResponse{}),
		dictionaryRoute(http.MethodDelete, "/api/admin/dict-items/:id", routecatalog.PermissionControlled, "Delete Dictionary Item", "admin.dict-items.delete", handler.deleteItem, nil, nil),
	}
}

func dictionaryRoute(method, path string, access routecatalog.AccessLevel, name, permissionCode string, handler gin.HandlerFunc, requestSchema, responseSchema any) routecatalog.Descriptor {
	request := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if requestSchema != nil {
		request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(requestSchema), Required: true}
	}
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: access, Handler: handler,
		Name: name, Group: "dict", DefaultPermissionCode: permissionCode, DefaultAuditCategory: "dict",
		OpenAPI: routecatalog.Operation{Summary: name, Request: request, Responses: map[int]routecatalog.Response{
			http.StatusOK:                  routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(responseSchema)),
			http.StatusBadRequest:          routecatalog.ErrorResponse("request is invalid", httpresponse.RequestInvalidDefinition()),
			http.StatusNotFound:            routecatalog.ErrorResponse("dictionary resource was not found", dictNotFound()),
			http.StatusConflict:            routecatalog.ErrorResponse("dictionary resource conflicts with an existing resource", dictConflict()),
			http.StatusUnprocessableEntity: routecatalog.ErrorResponse("dictionary validation failed", dictValidation()),
			http.StatusInternalServerError: routecatalog.ErrorResponse("dictionary operation failed", dictInternal()),
		}},
	}
}

func (handler *httpHandler) listTypes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	list, total, err := handler.service.ListTypes(c.Request.Context(), page, size, c.Query("keyword"), parseOptionalInt(c.Query("status")))
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, TypeListResponse{List: list, Total: total, Page: page, Size: size})
}

func (handler *httpHandler) createType(c *gin.Context) {
	var request CreateTypeRequest
	if !bindJSON(c, &request) {
		return
	}
	dictionaryType, err := handler.service.CreateType(c.Request.Context(), request)
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, dictionaryType)
}

func (handler *httpHandler) updateType(c *gin.Context) {
	typeID, ok := pathID(c)
	if !ok {
		return
	}
	var request UpdateTypeRequest
	if !bindJSON(c, &request) {
		return
	}
	dictionaryType, err := handler.service.UpdateType(c.Request.Context(), typeID, request)
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, dictionaryType)
}

func (handler *httpHandler) deleteType(c *gin.Context) {
	typeID, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeleteType(c.Request.Context(), typeID); err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, nil)
}

func (handler *httpHandler) listItems(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	list, total, err := handler.service.ListItems(c.Request.Context(), page, size, c.Query("type_code"), c.Query("keyword"), parseOptionalInt(c.Query("status")))
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, ItemListResponse{List: list, Total: total, Page: page, Size: size})
}

func (handler *httpHandler) createItem(c *gin.Context) {
	var request CreateItemRequest
	if !bindJSON(c, &request) {
		return
	}
	item, err := handler.service.CreateItem(c.Request.Context(), request)
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, item)
}

func (handler *httpHandler) updateItem(c *gin.Context) {
	itemID, ok := pathID(c)
	if !ok {
		return
	}
	var request UpdateItemRequest
	if !bindJSON(c, &request) {
		return
	}
	item, err := handler.service.UpdateItem(c.Request.Context(), itemID, request)
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, item)
}

func (handler *httpHandler) deleteItem(c *gin.Context) {
	itemID, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeleteItem(c.Request.Context(), itemID); err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, nil)
}

func (handler *httpHandler) listEnabledItems(c *gin.Context) {
	items, err := handler.service.ListEnabledItems(c.Request.Context(), c.Param("type_code"))
	if err != nil {
		badRequest(c, err)
		return
	}
	dictionarySuccess(c, items)
}

func parseOptionalInt(value string) *int {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func pathID(c *gin.Context) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || parsed == 0 {
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
		return 0, false
	}
	return uint(parsed), true
}

func bindJSON(c *gin.Context, request any) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
		return false
	}
	return true
}

func badRequest(c *gin.Context, err error) {
	dictionaryError(c, err)
}
