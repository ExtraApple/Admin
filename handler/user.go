package handler

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/model"
	"admin/service"
	"admin/service/uploadsecurity"
)

type AvatarManager interface {
	UploadWithResult(
		ctx context.Context,
		input service.UploadAvatarInput,
	) (*service.UploadAvatarResult, error)
	RestoreDefault(ctx context.Context, userID uint) (*model.User, error)
}

type AvatarContentOpener interface {
	Open(ctx context.Context, userID uint) service.AvatarContent
}

type UserProfileUpdater interface {
	Update(
		ctx context.Context,
		userID uint,
		req dto.UpdateSelfReq,
	) (*dto.UserInfo, error)
}

type UserHandler struct {
	JwtCfg               service.JWTConfig
	Avatars              AvatarManager
	AvatarContents       AvatarContentOpener
	ProfileUpdates       UserProfileUpdater
	AvatarMaxUploadBytes int64
}

// toStringSlice 从 Gin Context 中取出的任意值安全转换为字符串切片。
func toStringSlice(v any) []string {
	list, ok := v.([]string)
	if !ok {
		return []string{}
	}
	return list
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	var req dto.RegisterReq
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
	var req dto.LoginReq
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

// Refresh 使用 Refresh Token 换取新的 Token 对。
func (h *UserHandler) Refresh(c *gin.Context) {
	var req dto.RefreshTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401,
			"msg":  service.ErrRefreshTokenInvalid.Error(),
		})
		return
	}

	resp, err := service.RefreshTokens(req, h.JwtCfg)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401,
			"msg":  service.ErrRefreshTokenInvalid.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "刷新成功", "data": resp})
}

// UpdateSelf 修改自己的基础信息
func (h *UserHandler) UpdateSelf(c *gin.Context) {
	var req dto.UpdateSelfReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			err,
		))
		return
	}
	if req.HasAvatarField() {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeAvatarFieldNotWritable,
			service.ErrAvatarFieldNotWritable,
		))
		return
	}

	var (
		user *dto.UserInfo
		err  error
	)
	if h.ProfileUpdates != nil {
		user, err = h.ProfileUpdates.Update(
			c.Request.Context(),
			c.GetUint("userID"),
			req,
		)
	} else {
		user, err = service.UpdateSelf(c.GetUint("userID"), req)
	}
	if err != nil {
		writeUploadError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": user})
}

// ChangePassword 修改自己的密码
func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordReq
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
	file, cleanup, err := parseSingleUpload(
		c,
		h.AvatarMaxUploadBytes,
		avatarMultipartOverheadBytes,
	)
	defer cleanup()
	if err != nil {
		setUploadRejectionAudit(
			c,
			uploadsecurity.PurposeAvatar,
			nil,
			err,
		)
		writeUploadError(c, err)
		return
	}

	f, err := file.Open()
	if err != nil {
		classifiedErr := uploadsecurity.NewError(
			uploadsecurity.CodeUploadBodyInvalid,
			err,
		)
		setUploadRejectionAudit(
			c,
			uploadsecurity.PurposeAvatar,
			file,
			classifiedErr,
		)
		writeUploadError(c, classifiedErr)
		return
	}
	defer f.Close()

	if h.Avatars == nil {
		classifiedErr := uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		)
		setUploadRejectionAudit(
			c,
			uploadsecurity.PurposeAvatar,
			file,
			classifiedErr,
		)
		writeUploadError(c, classifiedErr)
		return
	}

	result, err := h.Avatars.UploadWithResult(c.Request.Context(), service.UploadAvatarInput{
		UserID:      c.GetUint("userID"),
		FileName:    file.Filename,
		ContentType: file.Header.Get("Content-Type"),
		Size:        file.Size,
		MaxBytes:    h.AvatarMaxUploadBytes,
		Reader:      f,
	})
	if err != nil {
		setUploadRejectionAudit(
			c,
			uploadsecurity.PurposeAvatar,
			file,
			err,
		)
		writeUploadError(c, err)
		return
	}
	if result == nil || result.User == nil {
		classifiedErr := uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		)
		setUploadRejectionAudit(
			c,
			uploadsecurity.PurposeAvatar,
			file,
			classifiedErr,
		)
		writeUploadError(c, classifiedErr)
		return
	}

	c.Set(service.UploadAuditMetadataContextKey, service.UploadAuditMetadata{
		Purpose:          string(uploadsecurity.PurposeAvatar),
		FileName:         result.FileName,
		FileSize:         result.FileSize,
		DeclaredMIME:     file.Header.Get("Content-Type"),
		DetectedMIME:     result.DetectedMIME,
		ValidationResult: service.UploadValidationAccepted,
		PolicyVersion:    result.PolicyVersion,
	})

	info := service.UserInfoFromModel(*result.User)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "头像上传成功", "data": info})
}

// RestoreDefaultAvatar 恢复系统默认头像。
func (h *UserHandler) RestoreDefaultAvatar(c *gin.Context) {
	if h.Avatars == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}

	user, err := h.Avatars.RestoreDefault(
		c.Request.Context(),
		c.GetUint("userID"),
	)
	if err != nil {
		writeUploadError(c, err)
		return
	}
	if user == nil {
		writeUploadError(c, uploadsecurity.NewError(
			uploadsecurity.CodeInternalError,
			nil,
		))
		return
	}

	info := service.UserInfoFromModel(*user)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已恢复默认头像", "data": info})
}

// GetAvatar 公开读取可信用户头像；无效、缺失或不可用时返回默认 PNG。
func (h *UserHandler) GetAvatar(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 || h.AvatarContents == nil {
		writeAvatarContent(c, service.DefaultAvatarContent(), "public, max-age=300")
		return
	}

	content := h.AvatarContents.Open(c.Request.Context(), uint(userID))
	writeAvatarContent(c, content, "public, max-age=300")
}

// GetDefaultAvatar 公开读取应用内置的默认 PNG。
func (h *UserHandler) GetDefaultAvatar(c *gin.Context) {
	writeAvatarContent(c, service.DefaultAvatarContent(), "public, max-age=86400")
}

func writeAvatarContent(
	c *gin.Context,
	content service.AvatarContent,
	cacheControl string,
) {
	if content.Reader == nil ||
		(content.ContentType != "image/jpeg" && content.ContentType != "image/png") {
		if content.Reader != nil {
			_ = content.Reader.Close()
		}
		content = service.DefaultAvatarContent()
	}
	defer content.Reader.Close()

	c.Header("Content-Type", content.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", cacheControl)
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		_ = c.Error(err)
	}
}

// InitialContext 获取初始上下文（用户信息 + 角色 + 权限 + 菜单）
func (h *UserHandler) InitialContext(c *gin.Context) {
	user, err := service.GetUserInfo(c.GetUint("userID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	roles, _ := c.Get("roles")
	permissions, _ := c.Get("permissions")
	menus, _ := service.GetUserMenus(c.GetUint("userID"))

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": dto.InitialContextResp{
			User:        *user,
			Roles:       toStringSlice(roles),
			Permissions: toStringSlice(permissions),
			Menus:       menus,
		},
	})
}

// GetInfo 获取当前用户信息
func (h *UserHandler) GetInfo(c *gin.Context) {
	user, err := service.GetUserInfo(c.GetUint("userID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": user})
}
