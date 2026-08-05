package httpadapter

import (
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type routeHandler struct{ service *application.Service }

type apiCreateRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Method         string `json:"method" binding:"required,min=2,max=10"`
	Path           string `json:"path" binding:"required,min=1,max=255"`
	Group          string `json:"group" binding:"max=50"`
	PermissionCode string `json:"permission_code" binding:"max=100"`
	Remark         string `json:"remark" binding:"max=255"`
	Sort           int    `json:"sort"`
	Status         *int   `json:"status" binding:"omitempty,oneof=0 1"`
	NeedAuth       *int   `json:"need_auth" binding:"omitempty,oneof=0 1"`
	NeedAudit      *int   `json:"need_audit" binding:"omitempty,oneof=0 1"`
}
type apiUpdateRequest struct {
	Name           *string `json:"name" binding:"omitempty,min=1,max=100"`
	Method         *string `json:"method" binding:"omitempty,min=2,max=10"`
	Path           *string `json:"path" binding:"omitempty,min=1,max=255"`
	Group          *string `json:"group" binding:"omitempty,max=50"`
	PermissionCode *string `json:"permission_code" binding:"omitempty,max=100"`
	Remark         *string `json:"remark" binding:"omitempty,max=255"`
	Sort           *int    `json:"sort"`
	Status         *int    `json:"status" binding:"omitempty,oneof=0 1"`
	NeedAuth       *int    `json:"need_auth" binding:"omitempty,oneof=0 1"`
	NeedAudit      *int    `json:"need_audit" binding:"omitempty,oneof=0 1"`
}
type buttonRequest struct {
	ParentID uint   `json:"parent_id" binding:"required"`
	Name     string `json:"name" binding:"required,min=1,max=100"`
	Sort     int    `json:"sort"`
}
type apiInfo struct {
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
type apiSyncResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Created []apiInfo `json:"created"`
		Count   int       `json:"count"`
	} `json:"data"`
}
type permissionSyncResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Created      []string `json:"created"`
		CreatedCount int      `json:"created_count"`
		UpdatedAPI   int      `json:"updated_api"`
	} `json:"data"`
}
type apiResponse struct {
	Code int     `json:"code"`
	Msg  string  `json:"msg,omitempty"`
	Data apiInfo `json:"data"`
}
type apiListResponse struct {
	Code int `json:"code"`
	Data struct {
		List  []apiInfo `json:"list"`
		Total int64     `json:"total"`
		Page  int       `json:"page"`
		Size  int       `json:"size"`
	} `json:"data"`
}
type methodResponse struct {
	Code int                        `json:"code"`
	Data []application.MethodOption `json:"data"`
}
type groupResponse struct {
	Code int                  `json:"code"`
	Data []domain.GroupOption `json:"data"`
}
type buttonResponse struct {
	Code int                `json:"code"`
	Msg  string             `json:"msg"`
	Data application.Button `json:"data"`
}
type apiErrorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}
type apiSuccessResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// Routes returns API Metadata handlers. Route discovery is supplied through the service's RouteSource option.
func Routes(service *application.Service) []routecatalog.Descriptor {
	handler := &routeHandler{service: service}
	return []routecatalog.Descriptor{
		apiRoute(http.MethodGet, "/api/admin/api-groups", "List API Groups", "admin.api-groups.get", handler.groups, nil, groupResponse{}),
		apiRoute(http.MethodGet, "/api/admin/api-methods", "List API Methods", "admin.api-methods.get", handler.methods, nil, methodResponse{}),
		apiRoute(http.MethodGet, "/api/admin/apis", "List APIs", "admin.apis.get", handler.list, nil, apiListResponse{}),
		apiRoute(http.MethodPost, "/api/admin/apis", "Create API", "admin.apis.post", handler.create, apiCreateRequest{}, apiResponse{}),
		apiRoute(http.MethodPut, "/api/admin/apis/:id", "Update API", "admin.apis.id.put", handler.update, apiUpdateRequest{}, apiResponse{}),
		apiRoute(http.MethodDelete, "/api/admin/apis/:id", "Delete API", "admin.apis.id.delete", handler.delete, nil, apiSuccessResponse{}),
		apiRoute(http.MethodPost, "/api/admin/apis/:id/menu-button", "Generate API Menu Button", "admin.apis.id.menu-button.post", handler.menuButton, buttonRequest{}, buttonResponse{}),
		apiRoute(http.MethodPost, "/api/admin/apis/sync", "Sync APIs", "admin.apis.sync.post", handler.sync, nil, apiSyncResponse{}),
		apiRoute(http.MethodPost, "/api/admin/apis/sync-permissions", "Sync API Permissions", "admin.apis.sync-permissions.post", handler.syncPermissions, nil, permissionSyncResponse{}),
		apiRoute(http.MethodGet, "/api/admin/apis/:id", "Get API", "admin.apis.id.get", handler.get, nil, apiResponse{}),
	}
}

func apiRoute(method, path, name, permission string, handler gin.HandlerFunc, requestSchema, responseSchema any) routecatalog.Descriptor {
	request := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if requestSchema != nil {
		request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(requestSchema), Required: true}
	}
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: handler,
		Name: name, Group: "api", DefaultPermissionCode: permission, DefaultAuditCategory: "api",
		OpenAPI: routecatalog.Operation{Summary: name, Request: request, Responses: map[int]routecatalog.Response{
			http.StatusOK:         {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(responseSchema)},
			http.StatusBadRequest: {Description: "bad request", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(apiErrorResponse{})},
		}},
	}
}

func (handler *routeHandler) list(c *gin.Context) {
	page, size := queryInt(c, "page", 1), queryInt(c, "size", 10)
	apis, total, err := handler.service.List(c.Request.Context(), page, size, application.Filter{
		Keyword: c.Query("keyword"), Group: c.Query("group"), Method: c.Query("method"),
		Status: optionalQueryInt(c.Query("status")), NeedAuth: optionalQueryInt(c.Query("need_auth")), NeedAudit: optionalQueryInt(c.Query("need_audit")),
	})
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	response := apiListResponse{Code: 200}
	response.Data.List = make([]apiInfo, len(apis))
	for index := range apis {
		response.Data.List[index] = toAPIInfo(apis[index])
	}
	response.Data.Total, response.Data.Page, response.Data.Size = total, page, size
	c.JSON(http.StatusOK, response)
}

func (handler *routeHandler) get(c *gin.Context) {
	id, ok := apiPathID(c)
	if !ok {
		return
	}
	api, err := handler.service.Get(c.Request.Context(), id)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, apiResponse{Code: 200, Data: toAPIInfo(api)})
}
func (handler *routeHandler) create(c *gin.Context) {
	var request apiCreateRequest
	if !apiBindJSON(c, &request) {
		return
	}
	api, err := handler.service.Create(c.Request.Context(), application.CreateInput{Name: request.Name, Method: request.Method, Path: request.Path, Group: request.Group, PermissionCode: request.PermissionCode, Remark: request.Remark, Sort: request.Sort, Status: request.Status, NeedAuth: request.NeedAuth, NeedAudit: request.NeedAudit})
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, apiResponse{Code: 200, Msg: "创建成功", Data: toAPIInfo(api)})
}
func (handler *routeHandler) update(c *gin.Context) {
	id, ok := apiPathID(c)
	if !ok {
		return
	}
	var request apiUpdateRequest
	if !apiBindJSON(c, &request) {
		return
	}
	api, err := handler.service.Update(c.Request.Context(), id, application.UpdateInput{Name: request.Name, Method: request.Method, Path: request.Path, Group: request.Group, PermissionCode: request.PermissionCode, Remark: request.Remark, Sort: request.Sort, Status: request.Status, NeedAuth: request.NeedAuth, NeedAudit: request.NeedAudit})
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, apiResponse{Code: 200, Msg: "修改成功", Data: toAPIInfo(api)})
}
func (handler *routeHandler) delete(c *gin.Context) {
	id, ok := apiPathID(c)
	if !ok {
		return
	}
	if err := handler.service.Delete(c.Request.Context(), id); err != nil {
		apiBadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, apiSuccessResponse{Code: 200, Msg: "删除成功"})
}
func (handler *routeHandler) groups(c *gin.Context) {
	groups, err := handler.service.Groups(c.Request.Context())
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, groupResponse{Code: 200, Data: groups})
}
func (handler *routeHandler) methods(c *gin.Context) {
	c.JSON(http.StatusOK, methodResponse{Code: 200, Data: handler.service.Methods()})
}
func (handler *routeHandler) menuButton(c *gin.Context) {
	id, ok := apiPathID(c)
	if !ok {
		return
	}
	var request buttonRequest
	if !apiBindJSON(c, &request) {
		return
	}
	button, err := handler.service.GenerateMenuButton(c.Request.Context(), id, request.ParentID, request.Name, request.Sort)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, buttonResponse{Code: 200, Msg: "生成成功", Data: button})
}

func (handler *routeHandler) sync(c *gin.Context) {
	apis, err := handler.service.SyncRoutes(c.Request.Context())
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	response := apiSyncResponse{Code: 200, Msg: "同步成功"}
	response.Data.Created = make([]apiInfo, len(apis))
	for index := range apis {
		response.Data.Created[index] = toAPIInfo(apis[index])
	}
	response.Data.Count = len(apis)
	c.JSON(http.StatusOK, response)
}

func (handler *routeHandler) syncPermissions(c *gin.Context) {
	created, updated, err := handler.service.SyncPermissions(c.Request.Context())
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	response := permissionSyncResponse{Code: 200, Msg: "同步成功"}
	response.Data.Created, response.Data.CreatedCount, response.Data.UpdatedAPI = created, len(created), updated
	c.JSON(http.StatusOK, response)
}

func apiPathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: 400, Msg: "参数错误"})
		return 0, false
	}
	return uint(value), true
}

func queryInt(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.Query(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func optionalQueryInt(value string) *int {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func apiBindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: 400, Msg: "参数错误: " + err.Error()})
		return false
	}
	return true
}

func apiBadRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, apiErrorResponse{Code: 400, Msg: err.Error()})
}

func toAPIInfo(api domain.API) apiInfo {
	return apiInfo{ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, PermissionCode: api.PermissionCode, Remark: api.Remark, Sort: api.Sort, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit}
}
