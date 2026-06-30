package handler

import (
	"net/http"
	"strconv"

	"admin/dto"
	"admin/service"

	"github.com/gin-gonic/gin"
)

type OrganizationHandler struct{}

// ListOrganizations 分页查询组织列表，支持关键字和状态筛选。
func (h *OrganizationHandler) ListOrganizations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	status := parseOptionalInt(c.Query("status"))

	list, total, err := service.GetOrganizations(page, size, c.Query("keyword"), status)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.OrganizationListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// GetOrganizationTree 返回组织的树形结构。
func (h *OrganizationHandler) GetOrganizationTree(c *gin.Context) {
	tree, err := service.GetOrganizationTree()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": tree})
}

// CreateOrganization 创建组织节点，并校验父节点和编码唯一性。
func (h *OrganizationHandler) CreateOrganization(c *gin.Context) {
	var req dto.CreateOrganizationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	organization, err := service.CreateOrganization(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": organization})
}

// UpdateOrganization 修改组织节点，并防止形成循环父子关系。
func (h *OrganizationHandler) UpdateOrganization(c *gin.Context) {
	orgID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var req dto.UpdateOrganizationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	organization, err := service.UpdateOrganization(uint(orgID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": organization})
}

// DeleteOrganization 删除没有子节点的组织。
func (h *OrganizationHandler) DeleteOrganization(c *gin.Context) {
	orgID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeleteOrganization(uint(orgID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// AssignUsers 覆盖指定组织的成员列表。
func (h *OrganizationHandler) AssignUsers(c *gin.Context) {
	orgID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var req dto.AssignUsersToOrganizationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	if err := service.AssignUsersToOrganization(uint(orgID), req.UserIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "分配成功"})
}

// GetUsers 查询指定组织下的成员列表。
func (h *OrganizationHandler) GetUsers(c *gin.Context) {
	orgID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	users, err := service.GetOrganizationUsers(uint(orgID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": users})
}
