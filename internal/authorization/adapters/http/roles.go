package httpadapter

import (
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"

	"github.com/gin-gonic/gin"
)

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
	success(c, roleList{List: list, Total: result.Total, Page: result.Page, Size: result.Size})
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
	success(c, roleInfoOf(role))
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
	success(c, roleInfoOf(role))
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
	success(c, nil)
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
	success(c, nil)
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
	success(c, response)
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
	success(c, nil)
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
	success(c, dataScopeInfo{RoleID: scope.RoleID, DataScope: string(scope.DataScope), OrganizationIDs: scope.OrganizationIDs})
}

func roleInfoOf(role domain.Role) roleInfo {
	return roleInfo{ID: role.ID, Name: role.Name, Code: role.Code, Description: role.Description, Sort: role.Sort, Status: role.Status, DataScope: string(role.DataScope)}
}
