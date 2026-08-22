package httpadapter

import (
	"errors"

	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"

	"github.com/gin-gonic/gin"
)

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
	success(c, permissionList{List: list, Total: result.Total, Page: result.Page, Size: result.Size})
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
	success(c, permissionInfoOf(permission))
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
	success(c, permissionInfoOf(permission))
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
	success(c, nil)
}

func (handler *Handler) permissionCodes(c *gin.Context) {
	codes, err := handler.service.AllPermissionCodes(c.Request.Context())
	if err != nil {
		badRequest(c, err)
		return
	}
	success(c, codes)
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
	success(c, syncData{Created: result.Created, Count: len(result.Created)})
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
	success(c, nil)
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
	success(c, result)
}

func permissionInfoOf(permission domain.Permission) permissionInfo {
	return permissionInfo{ID: permission.ID, Name: permission.Name, Code: permission.Code.String(), Group: permission.Group, Sort: permission.Sort}
}
