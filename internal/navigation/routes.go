package navigation

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"admin/internal/platform/configimpact"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type menuHTTPHandler struct{ service *Service }

type menuCreateRequest struct {
	ParentID            uint   `json:"parent_id"`
	Name                string `json:"name" binding:"required,min=1,max=100"`
	Path                string `json:"path" binding:"max=255"`
	Component           string `json:"component" binding:"max=255"`
	Icon                string `json:"icon" binding:"max=100"`
	PermissionCode      string `json:"permission_code" binding:"max=100"`
	Sort                int    `json:"sort"`
	Type                *int   `json:"type"`
	Status              *int   `json:"status"`
	ExpectedFingerprint string `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type menuUpdateRequest struct {
	ParentID            *uint   `json:"parent_id"`
	Name                *string `json:"name" binding:"omitempty,min=1,max=100"`
	Path                *string `json:"path" binding:"omitempty,max=255"`
	Component           *string `json:"component" binding:"omitempty,max=255"`
	Icon                *string `json:"icon" binding:"omitempty,max=100"`
	PermissionCode      *string `json:"permission_code" binding:"omitempty,max=100"`
	Sort                *int    `json:"sort"`
	Type                *int    `json:"type"`
	Status              *int    `json:"status"`
	ExpectedFingerprint string  `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type assignMenuRequest struct {
	MenuIDs []uint `json:"menu_ids" binding:"required"`
}
type assignAPIsRequest struct {
	APIIDs              []uint `json:"api_ids"`
	PermissionCode      string `json:"permission_code" binding:"max=100"`
	Clear               bool   `json:"clear"`
	ExpectedFingerprint string `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
}
type syncMenuItemRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Path           string `json:"path" binding:"required,max=255"`
	Component      string `json:"component" binding:"max=255"`
	Icon           string `json:"icon" binding:"max=100"`
	PermissionCode string `json:"permission_code" binding:"max=100"`
	ParentPath     string `json:"parent_path" binding:"max=255"`
	Sort           int    `json:"sort"`
	Type           *int   `json:"type"`
}
type syncMenusRequest struct {
	Routes              []syncMenuItemRequest `json:"routes" binding:"required"`
	ExpectedFingerprint string                `json:"expected_fingerprint" binding:"omitempty,len=64,hexadecimal"`
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
type impactPreviewRequest struct {
	Operation string          `json:"operation" binding:"required,max=40"`
	Candidate json.RawMessage `json:"candidate" binding:"required"`
}
type impactPreviewResponse struct {
	Data ImpactPreview `json:"data"`
}
type menuImpactUpdateCandidate struct {
	MenuID         uint    `json:"menu_id"`
	ParentID       *uint   `json:"parent_id"`
	Name           *string `json:"name"`
	Path           *string `json:"path"`
	Component      *string `json:"component"`
	Icon           *string `json:"icon"`
	PermissionCode *string `json:"permission_code"`
	Sort           *int    `json:"sort"`
	Type           *int    `json:"type"`
	Status         *int    `json:"status"`
}
type menuImpactDeleteCandidate struct {
	MenuID uint `json:"menu_id"`
}
type menuImpactBindCandidate struct {
	MenuID         uint   `json:"menu_id"`
	APIIDs         []uint `json:"api_ids"`
	PermissionCode string `json:"permission_code"`
}
type menuImpactClearCandidate struct {
	MenuID uint `json:"menu_id"`
}
type menuImpactSyncCandidate struct {
	Routes []syncMenuItemRequest `json:"routes"`
}

func Routes(service *Service) []routecatalog.Descriptor {
	handler := &menuHTTPHandler{service: service}
	update := navigationRoute(http.MethodPut, "/api/admin/menus/:id", "Update Menu", "admin.menus.id.put", handler.updateMenu, menuUpdateRequest{}, menuResponse{})
	update.OpenAPI.Description = "未提交的字段保持原值；显式空字符串可清空 icon、component，path 是否允许为空由菜单 type 约束，name 不得为空。JSON null 沿既有解析表示未提交。permission_code 继续独立绑定码契约，不表示解绑。完成现有校验后，字段与适用派生事实均同值才不提升版本；真实展示或联动修复仍原子失效，空更新仍拒绝。"
	syncMenus := navigationRoute(http.MethodPost, "/api/admin/menus/sync", "Sync Menus", "admin.menus.sync.post", handler.syncMenus, syncMenusRequest{}, syncMenusResponse{})
	syncMenus.OpenAPI.Description = "按既有路径及父级解析规则创建缺失菜单；合法全skip且无派生写入时返回created为0，不提升用户版本。"
	bindAPIs := navigationRoute(http.MethodPost, "/api/admin/menus/:id/apis", "Assign Menu APIs", "admin.menus.id.apis.post", handler.assignAPIs, assignAPIsRequest{}, nil)
	bindAPIs.OpenAPI.Description = "先校验全部原始API ID、存在、启用和认证前提；目标集合、最终码及既有规则维护的权限和授权事实均无变化才不重建关联、不提升版本。缺失权限或直接码漂移等修复仍原子提交并处理版本。"
	roleMenus := navigationRoute(http.MethodPost, "/api/admin/roles/:id/menus", "Assign Role Menus", "admin.roles.id.menus.post", handler.assignRoleMenus, assignMenuRequest{}, nil)
	roleMenus.OpenAPI.Description = "先满足角色保护及全部原始菜单引用前提，再比较实际显式配置；合法等价集合成功且不重建关联、不提升版本，不按运行时可见性裁剪配置。"
	impactPreview := navigationRoute(http.MethodPost, "/api/admin/menus/impact-preview", "Preview Menu Impact", "admin.menus.get", handler.impactPreview, impactPreviewRequest{}, impactPreviewResponse{})
	impactPreview.OpenAPI.Description = "只读计算菜单变更的规范化目标、受影响菜单/API/角色/用户摘要和无状态 fingerprint；不会写入配置、权限、授权版本或缓存。"
	deleteMenu := navigationRoute(http.MethodDelete, "/api/admin/menus/:id", "Delete Menu", "admin.menus.id.delete", handler.deleteMenu, nil, nil)
	deleteMenu.OpenAPI.Headers = []routecatalog.Header{{Name: "X-Expected-Fingerprint", Description: "Fingerprint returned by the menu impact preview.", Required: true}}
	return []routecatalog.Descriptor{
		impactPreview,
		navigationRoute(http.MethodGet, "/api/admin/menus", "List Menus", "admin.menus.get", handler.listMenus, nil, menuListResponse{}),
		navigationRoute(http.MethodPost, "/api/admin/menus", "Create Menu", "admin.menus.post", handler.createMenu, menuCreateRequest{}, menuResponse{}),
		update,
		deleteMenu,
		syncMenus,
		bindAPIs,
		navigationRoute(http.MethodGet, "/api/admin/menus/:id/apis", "List Menu APIs", "admin.menus.id.apis.get", handler.listAPIs, nil, menuAPIResponse{}),
		roleMenus,
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
func (handler *menuHTTPHandler) impactPreview(c *gin.Context) {
	var request impactPreviewRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	input, err := decodeMenuImpactInput(request)
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	preview, err := handler.service.PreviewImpact(c.Request.Context(), input)
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, preview)
}

func decodeMenuImpactInput(request impactPreviewRequest) (ImpactPreviewInput, error) {
	if strings.TrimSpace(string(request.Candidate)) == "" || strings.TrimSpace(string(request.Candidate)) == "null" {
		return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
	}
	operation := ImpactOperation(strings.TrimSpace(request.Operation))
	switch operation {
	case ImpactCreateMenu:
		var candidate menuCreateRequest
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil {
			return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
		}
		menuType, err := optionalMenuType(candidate.Type)
		if err != nil {
			return ImpactPreviewInput{}, err
		}
		return ImpactPreviewInput{Operation: operation, Create: CreateInput{ParentID: candidate.ParentID, Name: candidate.Name, Path: candidate.Path, Component: candidate.Component, Icon: candidate.Icon, PermissionCode: candidate.PermissionCode, Sort: candidate.Sort, Type: menuType, Status: candidate.Status}}, nil
	case ImpactUpdateMenu:
		var candidate menuImpactUpdateCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.MenuID == 0 {
			return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
		}
		return ImpactPreviewInput{Operation: operation, MenuID: candidate.MenuID, Update: UpdateInput{ParentID: candidate.ParentID, Name: candidate.Name, Path: candidate.Path, Component: candidate.Component, Icon: candidate.Icon, PermissionCode: candidate.PermissionCode, Sort: candidate.Sort, Type: candidate.Type, Status: candidate.Status}}, nil
	case ImpactDeleteMenu:
		var candidate menuImpactDeleteCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.MenuID == 0 {
			return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
		}
		return ImpactPreviewInput{Operation: operation, MenuID: candidate.MenuID}, nil
	case ImpactBindAPIs:
		var candidate menuImpactBindCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.MenuID == 0 {
			return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
		}
		return ImpactPreviewInput{Operation: operation, MenuID: candidate.MenuID, APIIDs: candidate.APIIDs, PermissionCode: candidate.PermissionCode}, nil
	case ImpactClearAPIs:
		var candidate menuImpactClearCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil || candidate.MenuID == 0 {
			return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
		}
		return ImpactPreviewInput{Operation: operation, MenuID: candidate.MenuID}, nil
	case ImpactSyncMenus:
		var candidate menuImpactSyncCandidate
		if err := json.Unmarshal(request.Candidate, &candidate); err != nil {
			return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
		}
		routes := make([]SyncItem, len(candidate.Routes))
		for index, item := range candidate.Routes {
			menuType, err := optionalMenuType(item.Type)
			if err != nil {
				return ImpactPreviewInput{}, err
			}
			routes[index] = SyncItem{Name: item.Name, Path: item.Path, Component: item.Component, Icon: item.Icon, PermissionCode: item.PermissionCode, ParentPath: item.ParentPath, Sort: item.Sort, Type: menuType}
		}
		return ImpactPreviewInput{Operation: operation, SyncItems: routes}, nil
	default:
		return ImpactPreviewInput{}, NewError(CodeValidationInvalid, nil)
	}
}
func (handler *menuHTTPHandler) createMenu(c *gin.Context) {
	var request menuCreateRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	if !validNavigationFingerprint(request.ExpectedFingerprint) {
		navigationBadRequest(c, NewError(CodeValidationInvalid, nil))
		return
	}
	menuType, err := optionalMenuType(request.Type)
	if err != nil {
		navigationBadRequest(c, err)
		return
	}
	menu, err := handler.service.CreateMenu(c.Request.Context(), CreateInput{ParentID: request.ParentID, Name: request.Name, Path: request.Path, Component: request.Component, Icon: request.Icon, PermissionCode: request.PermissionCode, Sort: request.Sort, Type: menuType, Status: request.Status, ExpectedFingerprint: request.ExpectedFingerprint})
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
	if !validNavigationFingerprint(request.ExpectedFingerprint) {
		navigationBadRequest(c, NewError(CodeValidationInvalid, nil))
		return
	}
	menu, err := handler.service.UpdateMenu(c.Request.Context(), id, UpdateInput{ParentID: request.ParentID, Name: request.Name, Path: request.Path, Component: request.Component, Icon: request.Icon, PermissionCode: request.PermissionCode, Sort: request.Sort, Type: request.Type, Status: request.Status, ExpectedFingerprint: request.ExpectedFingerprint})
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
	expectedFingerprint := requestFingerprint(c)
	if !validNavigationFingerprint(expectedFingerprint) {
		navigationBadRequest(c, NewError(CodeValidationInvalid, nil))
		return
	}
	if err := handler.service.DeleteMenu(c.Request.Context(), id, expectedFingerprint); err != nil {
		navigationBadRequest(c, err)
		return
	}
	navigationSuccess(c, nil)
}

func requestFingerprint(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Expected-Fingerprint"))
}
func (handler *menuHTTPHandler) syncMenus(c *gin.Context) {
	var request syncMenusRequest
	if !navigationBindJSON(c, &request) {
		return
	}
	if !validNavigationFingerprint(request.ExpectedFingerprint) {
		navigationBadRequest(c, NewError(CodeValidationInvalid, nil))
		return
	}
	routes := make([]SyncItem, len(request.Routes))
	for index, item := range request.Routes {
		menuType, err := optionalMenuType(item.Type)
		if err != nil {
			navigationBadRequest(c, err)
			return
		}
		routes[index] = SyncItem{Name: item.Name, Path: item.Path, Component: item.Component, Icon: item.Icon, PermissionCode: item.PermissionCode, ParentPath: item.ParentPath, Sort: item.Sort, Type: menuType}
	}
	created, err := handler.service.SyncMenus(c.Request.Context(), routes, request.ExpectedFingerprint)
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
	if !validNavigationFingerprint(request.ExpectedFingerprint) || !validAPIBindingRequest(request) {
		navigationBadRequest(c, NewError(CodeValidationInvalid, nil))
		return
	}
	if request.Clear {
		if err := handler.service.ClearAPIs(c.Request.Context(), id, request.ExpectedFingerprint); err != nil {
			navigationBadRequest(c, err)
			return
		}
	} else if err := handler.service.AssignAPIs(c.Request.Context(), id, request.APIIDs, request.PermissionCode, request.ExpectedFingerprint); err != nil {
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
func optionalMenuType(value *int) (int, error) {
	if value == nil {
		return 0, nil
	}
	if *value < 1 || *value > 3 {
		return 0, NewError(CodeValidationInvalid, nil)
	}
	return *value, nil
}

func validNavigationFingerprint(value string) bool {
	return configimpact.ValidFingerprint(value)
}
func validAPIBindingRequest(request assignAPIsRequest) bool {
	if request.Clear {
		return len(request.APIIDs) == 0 && strings.TrimSpace(request.PermissionCode) == ""
	}
	return len(request.APIIDs) > 0
}
func navigationBadRequest(c *gin.Context, err error) {
	navigationError(c, err)
}
