package httpadapter

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
	"admin/internal/platform/configimpact"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type routeHandler struct{ service *application.Service }

type apiCreateRequest struct {
	Name                string `json:"name" binding:"required,min=1,max=100"`
	Method              string `json:"method" binding:"required,min=2,max=10"`
	Path                string `json:"path" binding:"required,min=1,max=255"`
	Group               string `json:"group" binding:"max=50"`
	PermissionCode      string `json:"permission_code" binding:"max=100"`
	Remark              string `json:"remark" binding:"max=255"`
	Sort                int    `json:"sort"`
	Status              *int   `json:"status" binding:"omitempty,oneof=0 1"`
	NeedAuth            *int   `json:"need_auth" binding:"omitempty,oneof=0 1"`
	NeedAudit           *int   `json:"need_audit" binding:"omitempty,oneof=0 1"`
	ExpectedFingerprint string `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type apiUpdateRequest struct {
	Name                *string `json:"name" binding:"omitempty,min=1,max=100"`
	Method              *string `json:"method" binding:"omitempty,min=2,max=10"`
	Path                *string `json:"path" binding:"omitempty,min=1,max=255"`
	Group               *string `json:"group" binding:"omitempty,max=50"`
	PermissionCode      *string `json:"permission_code" binding:"omitempty,max=100"`
	Remark              *string `json:"remark" binding:"omitempty,max=255"`
	Sort                *int    `json:"sort"`
	Status              *int    `json:"status" binding:"omitempty,oneof=0 1"`
	NeedAuth            *int    `json:"need_auth" binding:"omitempty,oneof=0 1"`
	NeedAudit           *int    `json:"need_audit" binding:"omitempty,oneof=0 1"`
	ExpectedFingerprint string  `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type buttonRequest struct {
	ParentID            uint   `json:"parent_id" binding:"required"`
	Name                string `json:"name" binding:"required,min=1,max=100"`
	Sort                int    `json:"sort"`
	ExpectedFingerprint string `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
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
	RouteState     string `json:"route_state,omitempty"`
}
type apiSyncResponse struct {
	Data struct {
		Created []apiInfo `json:"created"`
		Count   int       `json:"count"`
	} `json:"data"`
}
type permissionSyncResponse struct {
	Data struct {
		Created      []string `json:"created"`
		CreatedCount int      `json:"created_count"`
		UpdatedAPI   int      `json:"updated_api"`
	} `json:"data"`
}
type apiSyncRequest struct {
	ExpectedFingerprint string `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type apiPermissionSyncRequest struct {
	ExpectedFingerprint string `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type apiResponse struct {
	Data apiInfo `json:"data"`
}
type apiListResponse struct {
	Data struct {
		List  []apiInfo `json:"list"`
		Total int64     `json:"total"`
		Page  int       `json:"page"`
		Size  int       `json:"size"`
	} `json:"data"`
}
type methodResponse struct {
	Data []application.MethodOption `json:"data"`
}
type groupResponse struct {
	Data []domain.GroupOption `json:"data"`
}
type buttonResponse struct {
	Data application.Button `json:"data"`
}
type impactPreviewRequest struct {
	Operation string          `json:"operation" binding:"required,max=40"`
	Candidate json.RawMessage `json:"candidate" binding:"required"`
}
type impactPreviewResponse struct {
	Data application.ImpactPreview `json:"data"`
}
type apiImpactUpdateCandidate struct {
	APIID          uint    `json:"api_id"`
	Name           *string `json:"name"`
	Method         *string `json:"method"`
	Path           *string `json:"path"`
	Group          *string `json:"group"`
	PermissionCode *string `json:"permission_code"`
	Remark         *string `json:"remark"`
	Sort           *int    `json:"sort"`
	Status         *int    `json:"status"`
	NeedAuth       *int    `json:"need_auth"`
	NeedAudit      *int    `json:"need_audit"`
}
type apiImpactDeleteCandidate struct {
	APIID uint `json:"api_id"`
}
type apiImpactButtonCandidate struct {
	APIID    uint   `json:"api_id"`
	ParentID uint   `json:"parent_id"`
	Name     string `json:"name"`
	Sort     int    `json:"sort"`
}
type apiImpactRouteCandidate struct {
	Method                string `json:"method"`
	Path                  string `json:"path"`
	Name                  string `json:"name"`
	Group                 string `json:"group"`
	Authenticated         bool   `json:"authenticated"`
	PermissionControlled  bool   `json:"permission_controlled"`
	DefaultPermissionCode string `json:"default_permission_code"`
	NeedAudit             bool   `json:"need_audit"`
}
type apiImpactSyncCandidate struct {
	Routes []apiImpactRouteCandidate `json:"routes"`
}

// Routes returns API Metadata handlers. Route discovery is supplied through the service's RouteSource option.
func Routes(service *application.Service) []routecatalog.Descriptor {
	handler := &routeHandler{service: service}
	impactPreview := apiRoute(http.MethodPost, "/api/admin/apis/impact-preview", "Preview API Impact", "admin.apis.get", handler.impactPreview, impactPreviewRequest{}, impactPreviewResponse{})
	impactPreview.OpenAPI.Description = "只读读取 Route Catalog 和 API Metadata 当前事实，返回规范化目标、影响摘要和无状态 fingerprint，不修改任何配置或授权事实。"
	deleteAPI := apiRoute(http.MethodDelete, "/api/admin/apis/:id", "Delete API", "admin.apis.id.delete", handler.delete, nil, nil)
	deleteAPI.OpenAPI.Headers = []routecatalog.Header{{Name: "X-Expected-Fingerprint", Description: "Fingerprint returned by the API impact preview.", Required: true}}
	menuButton := apiRoute(http.MethodPost, "/api/admin/apis/:id/menu-button", "Generate API Menu Button", "admin.apis.id.menu-button.post", handler.menuButton, buttonRequest{}, buttonResponse{})
	syncAPIs := apiRoute(http.MethodPost, "/api/admin/apis/sync", "Sync APIs", "admin.apis.sync.post", handler.sync, apiSyncRequest{}, apiSyncResponse{})
	syncPermissions := apiRoute(http.MethodPost, "/api/admin/apis/sync-permissions", "Sync API Permissions", "admin.apis.sync-permissions.post", handler.syncPermissions, apiPermissionSyncRequest{}, permissionSyncResponse{})
	return []routecatalog.Descriptor{
		impactPreview,
		apiRoute(http.MethodGet, "/api/admin/api-groups", "List API Groups", "admin.api-groups.get", handler.groups, nil, groupResponse{}),
		apiRoute(http.MethodGet, "/api/admin/api-methods", "List API Methods", "admin.api-methods.get", handler.methods, nil, methodResponse{}),
		apiRoute(http.MethodGet, "/api/admin/apis", "List APIs", "admin.apis.get", handler.list, nil, apiListResponse{}),
		apiRoute(http.MethodPost, "/api/admin/apis", "Create API", "admin.apis.post", handler.create, apiCreateRequest{}, apiResponse{}),
		apiRoute(http.MethodPut, "/api/admin/apis/:id", "Update API", "admin.apis.id.put", handler.update, apiUpdateRequest{}, apiResponse{}),
		deleteAPI,
		menuButton,
		syncAPIs,
		syncPermissions,
		apiRoute(http.MethodGet, "/api/admin/apis/:id", "Get API", "admin.apis.id.get", handler.get, nil, apiResponse{}),
	}
}

func apiRoute(method, path, name, permission string, handler gin.HandlerFunc, requestSchema, responseSchema any) routecatalog.Descriptor {
	request := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if requestSchema != nil {
		request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(requestSchema), Required: true}
	}
	responses := map[int]routecatalog.Response{
		http.StatusOK:                  routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(responseSchema)),
		http.StatusBadRequest:          routecatalog.ErrorResponse("request is invalid", httpresponse.RequestInvalidDefinition()),
		http.StatusForbidden:           routecatalog.ErrorResponse("permission is denied", apiMetaPermissionDenied(), apiMetaPermissionNotConfigured(), apiMetaDisabled(), apiMetaPermissionMissing()),
		http.StatusNotFound:            routecatalog.ErrorResponse("api metadata was not found", apiMetaNotFound()),
		http.StatusConflict:            routecatalog.ErrorResponse("api metadata conflicts with an existing resource", apiMetaConflict()),
		http.StatusUnprocessableEntity: routecatalog.ErrorResponse("api metadata validation failed", apiMetaValidation()),
		http.StatusInternalServerError: routecatalog.ErrorResponse("api metadata operation failed", apiMetaInternal()),
	}
	return routecatalog.Descriptor{Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: handler, Name: name, Group: "api", DefaultPermissionCode: permission, DefaultAuditCategory: "api", OpenAPI: routecatalog.Operation{Summary: name, Request: request, Responses: responses}}
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
	response := apiListResponse{}
	response.Data.List = make([]apiInfo, len(apis))
	for index := range apis {
		response.Data.List[index] = toAPIInfo(apis[index])
	}
	response.Data.Total, response.Data.Page, response.Data.Size = total, page, size
	apiMetaSuccess(c, response.Data)
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
	apiMetaSuccess(c, toAPIInfo(api))
}
func (handler *routeHandler) impactPreview(c *gin.Context) {
	var request impactPreviewRequest
	if !apiBindJSON(c, &request) {
		return
	}
	input, err := decodeAPIImpactInput(request)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	preview, err := handler.service.PreviewImpact(c.Request.Context(), input)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	apiMetaSuccess(c, preview)
}

func decodeAPIImpactInput(request impactPreviewRequest) (application.ImpactPreviewInput, error) {
	if strings.TrimSpace(string(request.Candidate)) == "" || strings.TrimSpace(string(request.Candidate)) == "null" {
		return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
	}
	operation := application.ImpactOperation(strings.TrimSpace(request.Operation))
	switch operation {
	case application.ImpactUpdateAPI:
		var candidate apiImpactUpdateCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.APIID == 0 {
			return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
		}
		return application.ImpactPreviewInput{Operation: operation, APIID: candidate.APIID, Update: application.UpdateInput{Name: candidate.Name, Method: candidate.Method, Path: candidate.Path, Group: candidate.Group, PermissionCode: candidate.PermissionCode, Remark: candidate.Remark, Sort: candidate.Sort, Status: candidate.Status, NeedAuth: candidate.NeedAuth, NeedAudit: candidate.NeedAudit}}, nil
	case application.ImpactDeleteAPI:
		var candidate apiImpactDeleteCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.APIID == 0 {
			return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
		}
		return application.ImpactPreviewInput{Operation: operation, APIID: candidate.APIID}, nil
	case application.ImpactSyncAPIs:
		var candidate apiImpactSyncCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil {
			return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
		}
		facts := make([]application.RouteFact, len(candidate.Routes))
		for index, route := range candidate.Routes {
			facts[index] = application.RouteFact{Method: route.Method, Path: route.Path, Name: route.Name, Group: route.Group, Authenticated: route.Authenticated, PermissionControlled: route.PermissionControlled, DefaultPermissionCode: route.DefaultPermissionCode, NeedAudit: route.NeedAudit}
		}
		return application.ImpactPreviewInput{Operation: operation, SyncFacts: facts}, nil
	case application.ImpactSyncPermissions:
		var candidate map[string]json.RawMessage
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil {
			return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
		}
		return application.ImpactPreviewInput{Operation: operation}, nil
	case application.ImpactGenerateMenuButton:
		var candidate apiImpactButtonCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.APIID == 0 || candidate.ParentID == 0 {
			return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
		}
		return application.ImpactPreviewInput{Operation: operation, APIID: candidate.APIID, ButtonParentID: candidate.ParentID, ButtonName: candidate.Name, ButtonSort: candidate.Sort}, nil
	default:
		return application.ImpactPreviewInput{}, application.NewError(application.CodeValidationInvalid, nil)
	}
}
func (handler *routeHandler) create(c *gin.Context) {
	var request apiCreateRequest
	if !apiBindJSON(c, &request) {
		return
	}
	if !validOptionalAPIFingerprint(request.ExpectedFingerprint) {
		apiBadRequest(c, application.NewError(application.CodeValidationInvalid, nil))
		return
	}
	api, err := handler.service.Create(c.Request.Context(), application.CreateInput{Name: request.Name, Method: request.Method, Path: request.Path, Group: request.Group, PermissionCode: request.PermissionCode, Remark: request.Remark, Sort: request.Sort, Status: request.Status, NeedAuth: request.NeedAuth, NeedAudit: request.NeedAudit})
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	apiMetaSuccess(c, toAPIInfo(api))
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
	if !validAPIFingerprint(request.ExpectedFingerprint) {
		apiBadRequest(c, application.NewError(application.CodeValidationInvalid, nil))
		return
	}
	api, err := handler.service.Update(c.Request.Context(), id, application.UpdateInput{Name: request.Name, Method: request.Method, Path: request.Path, Group: request.Group, PermissionCode: request.PermissionCode, Remark: request.Remark, Sort: request.Sort, Status: request.Status, NeedAuth: request.NeedAuth, NeedAudit: request.NeedAudit, ExpectedFingerprint: request.ExpectedFingerprint})
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	apiMetaSuccess(c, toAPIInfo(api))
}
func (handler *routeHandler) delete(c *gin.Context) {
	id, ok := apiPathID(c)
	if !ok {
		return
	}
	expectedFingerprint := requestFingerprint(c)
	if !validAPIFingerprint(expectedFingerprint) {
		apiBadRequest(c, application.NewError(application.CodeValidationInvalid, nil))
		return
	}
	if err := handler.service.Delete(c.Request.Context(), id, expectedFingerprint); err != nil {
		apiBadRequest(c, err)
		return
	}
	apiMetaSuccess(c, nil)
}
func (handler *routeHandler) groups(c *gin.Context) {
	groups, err := handler.service.Groups(c.Request.Context())
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	apiMetaSuccess(c, groups)
}
func (handler *routeHandler) methods(c *gin.Context) {
	apiMetaSuccess(c, handler.service.Methods())
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
	if !validAPIFingerprint(request.ExpectedFingerprint) {
		apiBadRequest(c, application.NewError(application.CodeValidationInvalid, nil))
		return
	}
	button, err := handler.service.GenerateMenuButton(c.Request.Context(), id, request.ParentID, request.Name, request.Sort, request.ExpectedFingerprint)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	apiMetaSuccess(c, button)
}

func (handler *routeHandler) sync(c *gin.Context) {
	var request apiSyncRequest
	if !apiBindJSON(c, &request) {
		return
	}
	if !validAPIFingerprint(request.ExpectedFingerprint) {
		apiBadRequest(c, application.NewError(application.CodeValidationInvalid, nil))
		return
	}
	apis, err := handler.service.SyncRoutes(c.Request.Context(), request.ExpectedFingerprint)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	response := apiSyncResponse{}
	response.Data.Created = make([]apiInfo, len(apis))
	for index := range apis {
		response.Data.Created[index] = toAPIInfo(apis[index])
	}
	response.Data.Count = len(apis)
	apiMetaSuccess(c, response.Data)
}

func (handler *routeHandler) syncPermissions(c *gin.Context) {
	var request apiPermissionSyncRequest
	if !apiBindJSON(c, &request) {
		return
	}
	if !validAPIFingerprint(request.ExpectedFingerprint) {
		apiBadRequest(c, application.NewError(application.CodeValidationInvalid, nil))
		return
	}
	created, updated, err := handler.service.SyncPermissions(c.Request.Context(), request.ExpectedFingerprint)
	if err != nil {
		apiBadRequest(c, err)
		return
	}
	response := permissionSyncResponse{}
	response.Data.Created, response.Data.CreatedCount, response.Data.UpdatedAPI = created, len(created), updated
	apiMetaSuccess(c, response.Data)
}

func apiPathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
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
		httpresponse.WriteError(c, httpresponse.RequestInvalidDefinition(), err, nil)
		return false
	}
	return true
}
func requestFingerprint(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Expected-Fingerprint"))
}
func validAPIFingerprint(value string) bool {
	return configimpact.ValidFingerprint(value)
}
func validOptionalAPIFingerprint(value string) bool {
	return value == "" || configimpact.ValidFingerprint(value)
}

func apiBadRequest(c *gin.Context, err error) {
	writeAPIMetaError(c, err)
}

func toAPIInfo(api domain.API) apiInfo {
	return apiInfo{ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, PermissionCode: api.PermissionCode, Remark: api.Remark, Sort: api.Sort, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit, RouteState: api.RouteState}
}
