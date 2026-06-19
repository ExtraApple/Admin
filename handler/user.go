package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"admin/request"
	"admin/service"
)

type UserHandler struct {
	JwtCfg service.JWTConfig
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	var req request.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	user, err := service.Register(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "注册成功", "data": user})
}

// Login 用户登录
func (h *UserHandler) Login(c *gin.Context) {
	var req request.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	resp, err := service.Login(req, h.JwtCfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "登录成功", "data": resp})
}

// UpdateSelf 修改自己的基础信息
func (h *UserHandler) UpdateSelf(c *gin.Context) {
	var req request.UpdateSelfReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	user, err := service.UpdateSelf(c.GetUint("userID"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": user})
}

// ChangePassword 修改自己的密码
func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req request.ChangePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if err := service.ChangePassword(c.GetUint("userID"), req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "密码修改成功"})
}

// GetInfo 获取当前用户信息（需 JWT 中间件）
func (h *UserHandler) GetInfo(c *gin.Context) {
	userID, _ := c.Get("userID")

	user, err := service.GetUserInfo(userID.(uint))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": user})
}
