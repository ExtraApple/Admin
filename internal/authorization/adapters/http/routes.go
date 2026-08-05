package httpadapter

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *application.Service
	routes  application.RoutePermissionSource
}

// Routes returns the complete Authorization-owned RBAC HTTP surface.
func Routes(service *application.Service, routeSource application.RoutePermissionSource) []routecatalog.Descriptor {
	handler := &Handler{service: service, routes: routeSource}
	return []routecatalog.Descriptor{
		authRoute(http.MethodGet, "/api/admin/roles", "List Roles", "admin.roles.get", handler.listRoles, nil, roleListResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles", "Create Role", "admin.roles.post", handler.createRole, createRoleRequest{}, roleResponse{}),
		authRoute(http.MethodPut, "/api/admin/roles/:id", "Update Role", "admin.roles.id.put", handler.updateRole, updateRoleRequest{}, roleResponse{}),
		authRoute(http.MethodDelete, "/api/admin/roles/:id", "Delete Role", "admin.roles.id.delete", handler.deleteRole, nil, successResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles/:id/users", "Assign Role Users", "admin.roles.id.users.post", handler.assignRoleUsers, assignUsersRequest{}, successResponse{}),
		authRoute(http.MethodGet, "/api/admin/roles/:id/users", "List Role Users", "admin.roles.id.users.get", handler.listRoleUsers, nil, userListResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles/:id/data-scope", "Assign Role Data Scope", "admin.roles.id.data-scope.post", handler.assignDataScope, assignDataScopeRequest{}, successResponse{}),
		authRoute(http.MethodGet, "/api/admin/roles/:id/data-scope", "Get Role Data Scope", "admin.roles.id.data-scope.get", handler.getDataScope, nil, dataScopeResponse{}),
		authRoute(http.MethodGet, "/api/admin/permissions", "List Permissions", "admin.permissions.get", handler.listPermissions, nil, permissionListResponse{}),
		authRoute(http.MethodPost, "/api/admin/permissions", "Create Permission", "admin.permissions.post", handler.createPermission, createPermissionRequest{}, permissionResponse{}),
		authRoute(http.MethodPut, "/api/admin/permissions/:id", "Update Permission", "admin.permissions.id.put", handler.updatePermission, updatePermissionRequest{}, permissionResponse{}),
		authRoute(http.MethodDelete, "/api/admin/permissions/:id", "Delete Permission", "admin.permissions.id.delete", handler.deletePermission, nil, successResponse{}),
		authRoute(http.MethodGet, "/api/admin/permission-codes", "List Permission Codes", "admin.permission-codes.get", handler.permissionCodes, nil, codesResponse{}),
		authRoute(http.MethodPost, "/api/admin/permissions/sync", "Sync Route Permissions", "admin.permissions.sync.post", handler.syncPermissions, nil, syncResponse{}),
		authRoute(http.MethodPost, "/api/admin/roles/:id/permissions", "Assign Role Permissions", "admin.roles.id.permissions.post", handler.assignPermissions, assignPermissionsRequest{}, successResponse{}),
		authRoute(http.MethodGet, "/api/admin/roles/:id/permissions", "List Role Permissions", "admin.roles.id.permissions.get", handler.rolePermissions, nil, permissionListEnvelope{}),
		authRoute(http.MethodGet, "/api/admin/permission-groups", "List Permission Groups", "admin.permission-groups.get", handler.listGroups, nil, groupListResponse{}),
		authRoute(http.MethodPost, "/api/admin/permission-groups", "Create Permission Group", "admin.permission-groups.post", handler.createGroup, createGroupRequest{}, groupResponse{}),
		authRoute(http.MethodPut, "/api/admin/permission-groups/:id", "Update Permission Group", "admin.permission-groups.id.put", handler.updateGroup, updateGroupRequest{}, groupResponse{}),
		authRoute(http.MethodDelete, "/api/admin/permission-groups/:id", "Delete Permission Group", "admin.permission-groups.id.delete", handler.deleteGroup, nil, successResponse{}),
	}
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
				http.StatusOK:         {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(responseSchema)},
				http.StatusBadRequest: {Description: "bad request", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(errorResponse{})},
			},
		},
	}
}

type createRoleRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=50"`
	Code        string `json:"code" binding:"required,min=1,max=50"`
	Description string `json:"description" binding:"max=255"`
	Sort        int    `json:"sort"`
	Status      int    `json:"status"`
	DataScope   string `json:"data_scope"`
}
type updateRoleRequest struct {
	Name        string `json:"name" binding:"max=50"`
	Code        string `json:"code" binding:"max=50"`
	Description string `json:"description" binding:"max=255"`
	Sort        *int   `json:"sort"`
	Status      *int   `json:"status"`
	DataScope   string `json:"data_scope"`
}
type assignUsersRequest struct {
	UserIDs []uint `json:"user_ids" binding:"required"`
}
type assignDataScopeRequest struct {
	DataScope       string `json:"data_scope" binding:"required"`
	OrganizationIDs []uint `json:"organization_ids"`
}
type createPermissionRequest struct {
	Name  string `json:"name" binding:"required,min=1,max=100"`
	Code  string `json:"code" binding:"required,min=1,max=100"`
	Group string `json:"group" binding:"max=50"`
	Sort  int    `json:"sort"`
}
type updatePermissionRequest struct {
	Name  string `json:"name" binding:"max=100"`
	Group string `json:"group" binding:"max=50"`
	Sort  *int   `json:"sort"`
}
type assignPermissionsRequest struct {
	PermissionIDs []uint `json:"permission_ids" binding:"required"`
}
type createGroupRequest struct {
	Name string `json:"name" binding:"required,min=1,max=50"`
	Sort int    `json:"sort"`
}
type updateGroupRequest struct {
	Name string `json:"name" binding:"max=50"`
	Sort *int   `json:"sort"`
}

type successResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}
type errorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}
type roleResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data roleInfo `json:"data"`
}
type roleListResponse struct {
	Code int      `json:"code"`
	Data roleList `json:"data"`
}
type roleList struct {
	List  []roleInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}
type roleInfo struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Sort        int    `json:"sort"`
	Status      int    `json:"status"`
	DataScope   string `json:"data_scope"`
}
type userListResponse struct {
	Code int        `json:"code"`
	Data []userInfo `json:"data"`
}
type userInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   int    `json:"status"`
}
type dataScopeResponse struct {
	Code int           `json:"code"`
	Data dataScopeInfo `json:"data"`
}
type dataScopeInfo struct {
	RoleID          uint   `json:"role_id"`
	DataScope       string `json:"data_scope"`
	OrganizationIDs []uint `json:"organization_ids"`
}
type permissionResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data permissionInfo `json:"data"`
}
type permissionListResponse struct {
	Code int            `json:"code"`
	Data permissionList `json:"data"`
}
type permissionListEnvelope struct {
	Code int              `json:"code"`
	Data []permissionInfo `json:"data"`
}
type permissionList struct {
	List  []permissionInfo `json:"list"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}
type permissionInfo struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Code  string `json:"code"`
	Group string `json:"group"`
	Sort  int    `json:"sort"`
}
type codesResponse struct {
	Code int      `json:"code"`
	Data []string `json:"data"`
}
type syncResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data syncData `json:"data"`
}
type syncData struct {
	Created []string `json:"created"`
	Count   int      `json:"count"`
}
type groupResponse struct {
	Code int       `json:"code"`
	Msg  string    `json:"msg"`
	Data groupInfo `json:"data"`
}
type groupListResponse struct {
	Code int       `json:"code"`
	Data groupList `json:"data"`
}
type groupList struct {
	List  []groupInfo `json:"list"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}
type groupInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

func (handler *Handler) listRoles(c *gin.Context) {
	page, size := queryPage(c)
	result, err := handler.service.ListRoles(c.Request.Context(), page, size)
	if err != nil {
		badRequest(c, err)
		return
	}
	list := make([]roleInfo, len(result.List))
	for i := range result.List {
		list[i] = roleInfoOf(result.List[i])
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": roleList{List: list, Total: result.Total, Page: result.Page, Size: result.Size}})
}
func (handler *Handler) createRole(c *gin.Context) {
	var req createRoleRequest
	if !bindJSON(c, &req) {
		return
	}
	role, err := handler.service.CreateRole(c.Request.Context(), application.CreateRoleRequest{Name: req.Name, Code: req.Code, Description: req.Description, Sort: req.Sort, Status: req.Status, DataScope: req.DataScope})
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": roleInfoOf(role)})
}
func (handler *Handler) updateRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req updateRoleRequest
	if !bindJSON(c, &req) {
		return
	}
	role, err := handler.service.UpdateRole(c.Request.Context(), id, application.UpdateRoleRequest{Name: req.Name, Code: req.Code, Description: req.Description, Sort: req.Sort, Status: req.Status, DataScope: req.DataScope})
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": roleInfoOf(role)})
}
func (handler *Handler) deleteRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeleteRole(c.Request.Context(), id); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}
func (handler *Handler) assignRoleUsers(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req assignUsersRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := handler.service.AssignUsersToRole(c.Request.Context(), id, req.UserIDs); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "分配成功"})
}
func (handler *Handler) listRoleUsers(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	users, err := handler.service.RoleUsers(c.Request.Context(), id)
	if err != nil {
		badRequest(c, err)
		return
	}
	response := make([]userInfo, len(users))
	for index, user := range users {
		response[index] = userInfo{
			ID: user.ID, Username: user.Username, Nickname: user.Nickname,
			Avatar: user.Avatar, Email: user.Email, Role: user.Role, Status: user.Status,
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": response})
}
func (handler *Handler) assignDataScope(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req assignDataScopeRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := handler.service.AssignRoleDataScope(c.Request.Context(), id, req.DataScope, req.OrganizationIDs); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "配置成功"})
}
func (handler *Handler) getDataScope(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	scope, err := handler.service.RoleDataScope(c.Request.Context(), id)
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dataScopeInfo{RoleID: scope.RoleID, DataScope: string(scope.DataScope), OrganizationIDs: scope.OrganizationIDs}})
}
func (handler *Handler) listPermissions(c *gin.Context) {
	page, size := queryPage(c)
	result, err := handler.service.ListPermissions(c.Request.Context(), page, size)
	if err != nil {
		badRequest(c, err)
		return
	}
	list := make([]permissionInfo, len(result.List))
	for i := range result.List {
		list[i] = permissionInfoOf(result.List[i])
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": permissionList{List: list, Total: result.Total, Page: result.Page, Size: result.Size}})
}
func (handler *Handler) createPermission(c *gin.Context) {
	var req createPermissionRequest
	if !bindJSON(c, &req) {
		return
	}
	permission, err := handler.service.CreatePermission(c.Request.Context(), application.CreatePermissionRequest{Name: req.Name, Code: req.Code, Group: req.Group, Sort: req.Sort})
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": permissionInfoOf(permission)})
}
func (handler *Handler) updatePermission(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req updatePermissionRequest
	if !bindJSON(c, &req) {
		return
	}
	permission, err := handler.service.UpdatePermission(c.Request.Context(), id, application.UpdatePermissionRequest{Name: req.Name, Group: req.Group, Sort: req.Sort})
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": permissionInfoOf(permission)})
}
func (handler *Handler) deletePermission(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeletePermission(c.Request.Context(), id); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}
func (handler *Handler) permissionCodes(c *gin.Context) {
	codes, err := handler.service.AllPermissionCodes(c.Request.Context())
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": codes})
}
func (handler *Handler) syncPermissions(c *gin.Context) {
	if handler.routes == nil {
		badRequest(c, errors.New("路由目录不可用"))
		return
	}
	routes, err := handler.routes.Routes(c.Request.Context())
	if err != nil {
		badRequest(c, err)
		return
	}
	result, err := handler.service.SyncPermissions(c.Request.Context(), routes)
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "同步成功", "data": syncData{Created: result.Created, Count: len(result.Created)}})
}
func (handler *Handler) assignPermissions(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req assignPermissionsRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := handler.service.AssignPermissionsToRole(c.Request.Context(), id, req.PermissionIDs); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "分配成功"})
}
func (handler *Handler) rolePermissions(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	permissions, err := handler.service.RolePermissions(c.Request.Context(), id)
	if err != nil {
		badRequest(c, err)
		return
	}
	result := make([]permissionInfo, len(permissions))
	for i := range permissions {
		result[i] = permissionInfoOf(permissions[i])
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": result})
}
func (handler *Handler) listGroups(c *gin.Context) {
	page, size := queryPage(c)
	result, err := handler.service.ListPermissionGroups(c.Request.Context(), page, size)
	if err != nil {
		badRequest(c, err)
		return
	}
	list := make([]groupInfo, len(result.List))
	for i := range result.List {
		list[i] = groupInfo{ID: result.List[i].ID, Name: result.List[i].Name, Sort: result.List[i].Sort}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": groupList{List: list, Total: result.Total, Page: result.Page, Size: result.Size}})
}
func (handler *Handler) createGroup(c *gin.Context) {
	var req createGroupRequest
	if !bindJSON(c, &req) {
		return
	}
	group, err := handler.service.CreatePermissionGroup(c.Request.Context(), req.Name, req.Sort)
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": groupInfo{ID: group.ID, Name: group.Name, Sort: group.Sort}})
}
func (handler *Handler) updateGroup(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req updateGroupRequest
	if !bindJSON(c, &req) {
		return
	}
	group, err := handler.service.UpdatePermissionGroup(c.Request.Context(), id, req.Name, req.Sort)
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": groupInfo{ID: group.ID, Name: group.Name, Sort: group.Sort}})
}
func (handler *Handler) deleteGroup(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeletePermissionGroup(c.Request.Context(), id); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

func roleInfoOf(role domain.Role) roleInfo {
	return roleInfo{ID: role.ID, Name: role.Name, Code: role.Code, Description: role.Description, Sort: role.Sort, Status: role.Status, DataScope: string(role.DataScope)}
}
func permissionInfoOf(permission domain.Permission) permissionInfo {
	return permissionInfo{ID: permission.ID, Name: permission.Name, Code: permission.Code.String(), Group: permission.Group, Sort: permission.Sort}
}
func queryPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	return page, size
}
func pathID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return 0, false
	}
	return uint(value), true
}
func bindJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return false
	}
	return true
}
func badRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
}
