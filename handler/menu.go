package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/service"
)

type MenuHandler struct{}

// ListMenus 返回全部菜单的树形结构。
func (h *MenuHandler) ListMenus(c *gin.Context) {
	menus, err := service.GetMenuTree()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": menus})
}

// CreateMenu 创建菜单或目录节点。
func (h *MenuHandler) CreateMenu(c *gin.Context) {
	var req dto.CreateMenuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	menu, err := service.CreateMenu(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": menu})
}

// UpdateMenu 修改菜单信息，并校验路径冲突和父级关系。
func (h *MenuHandler) UpdateMenu(c *gin.Context) {
	menuID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var req dto.UpdateMenuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	menu, err := service.UpdateMenu(uint(menuID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": menu})
}

// DeleteMenu 删除无子节点的菜单，并清理角色菜单关联。
func (h *MenuHandler) DeleteMenu(c *gin.Context) {
	menuID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeleteMenu(uint(menuID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// AssignRoleMenus 为指定角色全量替换菜单授权。
func (h *MenuHandler) AssignRoleMenus(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var req dto.AssignMenusToRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if err := service.AssignMenusToRole(uint(roleID), req.MenuIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "分配成功"})
}

// GetRoleMenus 查询指定角色已授权的菜单树。
func (h *MenuHandler) GetRoleMenus(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	menus, err := service.GetRoleMenus(uint(roleID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": menus})
}

// SyncMenus 根据前端路由元数据批量创建缺失菜单。
func (h *MenuHandler) SyncMenus(c *gin.Context) {
	var req dto.SyncMenusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	created, err := service.SyncMenus(req.Routes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "同步成功", "data": gin.H{"created": created}})
}
