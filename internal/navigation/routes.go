package navigation

import (
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type menuHTTPHandler struct{ service *Service }

type menuCreateRequest struct {
	ParentID       uint   `json:"parent_id"`
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Path           string `json:"path" binding:"max=255"`
	Component      string `json:"component" binding:"max=255"`
	Icon           string `json:"icon" binding:"max=100"`
	PermissionCode string `json:"permission_code" binding:"max=100"`
	Sort           int    `json:"sort"`
	Type           int    `json:"type"`
	Status         int    `json:"status"`
}
type menuUpdateRequest struct {
	ParentID       *uint   `json:"parent_id"`
	Name           string  `json:"name" binding:"max=100"`
	Path           string  `json:"path" binding:"max=255"`
	Component      string  `json:"component" binding:"max=255"`
	Icon           string  `json:"icon" binding:"max=100"`
	PermissionCode *string `json:"permission_code" binding:"omitempty,max=100"`
	Sort           *int    `json:"sort"`
	Type           *int    `json:"type"`
	Status         *int    `json:"status"`
}
type assignMenuRequest struct {
	MenuIDs []uint `json:"menu_ids" binding:"required"`
}
type assignAPIsRequest struct {
	APIIDs         []uint `json:"api_ids" binding:"required"`
	PermissionCode string `json:"permission_code" binding:"max=100"`
}
type syncMenuItemRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Path           string `json:"path" binding:"required,max=255"`
	Component      string `json:"component" binding:"max=255"`
	Icon           string `json:"icon" binding:"max=100"`
	PermissionCode string `json:"permission_code" binding:"max=100"`
	ParentPath     string `json:"parent_path" binding:"max=255"`
	Sort           int    `json:"sort"`
	Type           int    `json:"type"`
}
type syncMenusRequest struct {
	Routes []syncMenuItemRequest `json:"routes" binding:"required"`
}
type menuResponse struct {
	Code int        `json:"code"`
	Msg  string     `json:"msg"`
	Data MenuDetail `json:"data"`
}
type menuListResponse struct {
	Code int          `json:"code"`
	Data []MenuDetail `json:"data"`
}
type navigationAPIInfo struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	Group          string `json:"group"`
	PermissionCode string `json:"permission_code"`
	Remark         string `json:"remark"`
	Sort           int    `json:"sort"`
	Status         int    `json:"status"`
	NeedAuth       int    `json:"need_auth"`
	NeedAudit      int    `json:"need_audit"`
}
type menuAPIResponse struct {
	Code int                 `json:"code"`
	Data []navigationAPIInfo `json:"data"`
}
type syncMenusResponse struct {
	Data struct {
		Created int `json:"created"`
	} `json:"data"`
}

func Routes(service *Service) []routecatalog.Descriptor {
	handler := &menuHTTPHandler{service: service}
	return []routecatalog.Descriptor{
		navigationRoute(http.MethodGet, "/api/admin/menus", "List Menus", "admin.menus.get", handler.listMenus, nil, menuListResponse{}),
		navigationRoute(http.MethodPost, "/api/admin/menus", "Create Menu", "admin.menus.post", handler.createMenu, menuCreateRequest{}, menuResponse{}),
		navigationRoute(http.MethodPut, "/api/admin/menus/:id", "Update Menu", "admin.menus.id.put", handler.updateMenu, menuUpdateRequest{}, menuResponse{}),
		navigationRoute(http.MethodDelete, "/api/admin/menus/:id", "Delete Menu", "admin.menus.id.delete", handler.deleteMenu, nil, nil),
		navigationRoute(http.MethodPost, "/api/admin/menus/sync", "Sync Menus", "admin.menus.sync.post", handler.syncMenus, syncMenusRequest{}, syncMenusResponse{}),
		navigationRoute(http.MethodPost, "/api/admin/menus/:id/apis", "Assign Menu APIs", "admin.menus.id.apis.post", handler.assignAPIs, assignAPIsRequest{}, nil),
		navigationRoute(http.MethodGet, "/api/admin/menus/:id/apis", "List Menu APIs", "admin.menus.id.apis.get", handler.listAPIs, nil, menuAPIResponse{}),
		navigationRoute(http.MethodPost, "/api/admin/roles/:id/menus", "Assign Role Menus", "admin.roles.id.menus.post", handler.assignRoleMenus, assignMenuRequest{}, nil),
		navigationRoute(http.MethodGet, "/api/admin/roles/:id/menus", "List Role Menus", "admin.roles.id.menus.get", handler.listRoleMenus, nil, menuListResponse{}),
	}
}

func navigationRoute(method, path, name, permission string, handler gin.HandlerFunc, requestSchema, responseSchema any) routecatalog.Descriptor {
	request := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if requestSchema != nil {
		request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(requestSchema), Required: true}
	}
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: handler,
		Name: name, Group: "menu", DefaultPermissionCode: permission, DefaultAuditCategory: "menu",
		OpenAPI: routecatalog.Operation{Summary: name, Request: request, Responses: map[int]routecatalog.Response{
			http.StatusOK:                  routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(responseSchema)),
			http.StatusBadRequest:          routecatalog.ErrorResponse("request is invalid", httpresponse.RequestInvalidDefinition()),
			http.StatusNotFound:            routecatalog.ErrorResponse("navigation resource was not found", navNotFound()),
			http.StatusConflict:            routecatalog.ErrorResponse("navigation resource conflicts with an existing resource", navConflict()),
			http.StatusUnprocessableEntity: routecatalog.ErrorResponse("navigation validation failed", navValidation()),
			http.StatusInternalServerError: routecatalog.ErrorResponse("navigation operation failed", navInternal()),
		}},
	}
}

func (handler *menuHTTPHandler) listMenus(c *gin.Context) {
	menus, err := handler.service.MenuTree(c.Request.Context())
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, menus)
}
func (handler *menuHTTPHandler) createMenu(c *gin.Context) {
	var request menuCreateRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	menu, err := handler.service.CreateMenu(c.Request.Context(), CreateInput{ParentID: request.ParentID, Name: request.Name, Path: request.Path, Component: request.Component, Icon: request.Icon, PermissionCode: request.PermissionCode, Sort: request.Sort, Type: request.Type, Status: request.Status})
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, menu)
}
func (handler *menuHTTPHandler) updateMenu(c *gin.Context) {
	id, ok := navigationPathID(c)
	if !ok {
		return
	}
	var request menuUpdateRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	menu, err := handler.service.UpdateMenu(c.Request.Context(), id, UpdateInput{ParentID: request.ParentID, Name: request.Name, Path: request.Path, Component: request.Component, Icon: request.Icon, PermissionCode: request.PermissionCode, Sort: request.Sort, Type: request.Type, Status: request.Status})
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, menu)
}
func (handler *menuHTTPHandler) deleteMenu(c *gin.Context) {
	id, ok := navigationPathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeleteMenu(c.Request.Context(), id); err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, nil)
}
func (handler *menuHTTPHandler) syncMenus(c *gin.Context) {
	var request syncMenusRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	routes := make([]SyncItem, len(request.Routes))
	for index, item := range request.Routes {
		routes[index] = SyncItem{Name: item.Name, Path: item.Path, Component: item.Component, Icon: item.Icon, PermissionCode: item.PermissionCode, ParentPath: item.ParentPath, Sort: item.Sort, Type: item.Type}
	}
	created, err := handler.service.SyncMenus(c.Request.Context(), routes)
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	response := syncMenusResponse{}
	response.Data.Created = created
	navigationSuccess(c, response.Data)
}

func (handler *menuHTTPHandler) assignAPIs(c *gin.Context) {
	id, ok := navigationPathID(c)
	if !ok {
		return
	}
	var request assignAPIsRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	if err := handler.service.AssignAPIs(c.Request.Context(), id, request.APIIDs, request.PermissionCode); err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, nil)
}
func (handler *menuHTTPHandler) listAPIs(c *gin.Context) {
	id, ok := navigationPathID(c)
	if !ok {
		return
	}
	apis, err := handler.service.MenuAPIs(c.Request.Context(), id)
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	result := make([]navigationAPIInfo, len(apis))
	for index, api := range apis {
		result[index] = navigationAPIInfo{ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, PermissionCode: api.PermissionCode, Remark: api.Remark, Sort: api.Sort, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit}
	}
	navigationSuccess(c, result)
}
func (handler *menuHTTPHandler) assignRoleMenus(c *gin.Context) {
	id, ok := navigationPathID(c)
	if !ok {
		return
	}
	var request assignMenuRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	if err := handler.service.AssignRoleMenus(c.Request.Context(), id, request.MenuIDs); err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, nil)
}
func (handler *menuHTTPHandler) listRoleMenus(c *gin.Context) {
	id, ok := navigationPathID(c)
	if !ok {
		return
	}
	menus, err := handler.service.RoleMenus(c.Request.Context(), id)
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, menus)
}

func navigationPathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		navigationError(c, err)
		return 0, false
	}
	return uint(value), true
}
func navigationBindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
		return false
	}
	return true
}
func navigationBadRequest(c *gin.Context, err error) {
	navigationError(c, err)
}
