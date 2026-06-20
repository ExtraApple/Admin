package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"admin/initialize"
	"admin/request"
	"admin/service"
	"admin/utils"
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

// Logout 退出登录
func (h *UserHandler) Logout(c *gin.Context) {
	tokenStr := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	service.Logout(tokenStr, h.JwtCfg.ExpireMins)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "退出成功"})
}

// UploadAvatar 上传/修改头像
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请选择文件"})
		return
	}
	if file.Size > 2*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "头像大小不能超过 2MB"})
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "头像仅支持 jpg / jpeg / png / webp 格式"})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "文件打开失败"})
		return
	}
	defer f.Close()

	userID := c.GetUint("userID")
	prefix := "avatars/" + strconv.FormatUint(uint64(userID), 10) + "/"
	objName, err := utils.UploadStream("image", prefix, ext, file.Header.Get("Content-Type"), f, file.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "头像上传失败: " + err.Error()})
		return
	}

	// 只保留最近 2 个头像，删除更旧的
	utils.CleanOldFiles("image", prefix, 2)

	conf := initialize.InitConfig()
	avatarURL := fmt.Sprintf("http://%s:%d/image/%s", conf.Minio.Host, conf.Minio.Port, objName)
	user, err := service.SetAvatar(c.GetUint("userID"), avatarURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "头像上传成功", "data": user})
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
