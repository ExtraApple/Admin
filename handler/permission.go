package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/service"
)

type PermissionHandler struct {
	Engine *gin.Engine
}

// ListPermissions 分页查询权限列表。
func (h *PermissionHandler) ListPermissions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	list, total, err := service.GetAllPermissions(page, size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.PermissionListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// CreatePermission 创建新的权限码记录。
func (h *PermissionHandler) CreatePermission(c *gin.Context) {
	var req dto.CreatePermissionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	perm, err := service.CreatePermission(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": perm})
}

// UpdatePermission 修改权限展示信息，不修改权限码本身。
func (h *PermissionHandler) UpdatePermission(c *gin.Context) {
	permID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req dto.UpdatePermissionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	perm, err := service.UpdatePermission(uint(permID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": perm})
}

// DeletePermission 删除权限并清理角色权限关联。
func (h *PermissionHandler) DeletePermission(c *gin.Context) {
	permID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeletePermission(uint(permID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// AssignPermissions 为指定角色全量替换权限集合。
func (h *PermissionHandler) AssignPermissions(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req dto.AssignPermsToRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if err := service.AssignPermissionsToRole(uint(roleID), req.PermissionIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "分配成功"})
}

// GetRolePermissions 查询指定角色拥有的权限列表。
func (h *PermissionHandler) GetRolePermissions(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	perms, err := service.GetRolePermissions(uint(roleID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": perms})
}

// GetPermissionCodes 获取所有权限码，供前端按钮级权限控制使用。
func (h *PermissionHandler) GetPermissionCodes(c *gin.Context) {
	codes := service.GetPermissionCodes()
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": codes})
}

// SyncPermissions 扫描 Gin 已注册路由并创建缺失的权限码。
func (h *PermissionHandler) SyncPermissions(c *gin.Context) {
	routes := h.Engine.Routes()

	routeList := make([]map[string]string, len(routes))
	for i, r := range routes {
		routeList[i] = map[string]string{
			"method": r.Method,
			"path":   r.Path,
		}
	}

	created, err := service.SyncPermissions(routeList)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "同步成功", "data": gin.H{
		"created": created,
		"count":   len(created),
	}})
}

// ListPermGroups 分页查询权限分组列表。
func (h *PermissionHandler) ListPermGroups(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	list, total, err := service.GetAllPermGroups(page, size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.PermGroupListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// CreatePermGroup 创建权限分组。
func (h *PermissionHandler) CreatePermGroup(c *gin.Context) {
	var req dto.CreatePermGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	group, err := service.CreatePermGroup(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": group})
}

// UpdatePermGroup 修改权限分组名称或排序。
func (h *PermissionHandler) UpdatePermGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req dto.UpdatePermGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	group, err := service.UpdatePermGroup(uint(groupID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": group})
}

// DeletePermGroup 删除指定权限分组。
func (h *PermissionHandler) DeletePermGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeletePermGroup(uint(groupID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}
