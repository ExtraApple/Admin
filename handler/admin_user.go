package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/request"
	"admin/service"
)

type AdminUserHandler struct{}

// ========== 管理端 ==========

// --- 获取用户列表 ---		
func (h *AdminUserHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	list, total, err := service.GetAllUsers(page, size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": request.UserListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// --- 删除用户 ---
func (h *AdminUserHandler) DeleteUser(c *gin.Context) {
	operatorID := c.GetUint("userID")
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeleteUserByAdmin(uint(operatorID), uint(targetID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// --- 管理员修改用户 ---
func (h *AdminUserHandler) UpdateUser(c *gin.Context) {
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req request.AdminUpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	user, err := service.UpdateUserByAdmin(c.GetUint("userID"), uint(targetID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": user})
}

// --- 切换用户状态 ---
func (h *AdminUserHandler) ToggleStatus(c *gin.Context) {
	operatorID := c.GetUint("userID")
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	newStatus, err := service.ToggleUserStatus(uint(operatorID), uint(targetID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	msg := "已禁用"
	if newStatus == 1 {
		msg = "已启用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": msg})
}
