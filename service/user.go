package service

import (
	"context"
	"errors"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"
	"admin/request"
	"admin/utils"
)

type JWTConfig struct {
	Secret            string
	ExpireMins        int
	RefreshExpireMins int
}

// validatePassword 密码复杂度校验：至少 6 位 + 最少 3 种不同字符类型
func validatePassword(pw string) error {
	if len(pw) < 6 {
		return errors.New("密码长度不能少于 6 位")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	count := 0
	for _, ok := range []bool{hasUpper, hasLower, hasDigit, hasSpecial} {
		if ok {
			count++
		}
	}
	if count < 3 {
		return errors.New("密码必须包含大写字母、小写字母、数字、特殊符号中至少 3 种")
	}
	return nil
}

// Register 用户注册
func Register(req request.RegisterReq) (*request.UserInfo, error) {
	// 校验验证码
	if !VerifyCaptcha(req.CaptchaID, req.CaptchaCode) {
		return nil, errors.New("验证码错误或已过期")
	}

	// 校验密码复杂度
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// 查重
	var exist int64
	global.DB.Model(&model.User{}).Where("username = ? OR email = ?", req.Username, req.Email).Count(&exist)
	if exist > 0 {
		return nil, errors.New("用户名或邮箱已被注册")
	}

	// 密码加密
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	user := model.User{
		Username: req.Username,
		Password: string(hashed),
		Email:    req.Email,
		Nickname: req.Nickname,
		Role:     "user",
		Status:   1,
	}
	if err := global.DB.Create(&user).Error; err != nil {
		return nil, errors.New("创建用户失败: " + err.Error())
	}

	return &request.UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Email:    user.Email,
		Role:     user.Role,
		Status:   user.Status,
	}, nil
}

// Login 用户登录
func Login(req request.LoginReq, cfg JWTConfig) (*request.LoginResp, error) {
	// 校验验证码
	if !VerifyCaptcha(req.CaptchaID, req.CaptchaCode) {
		return nil, errors.New("验证码错误或已过期")
	}

	var user model.User
	if err := global.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户名或密码错误")
		}
		return nil, errors.New("查询用户失败: " + err.Error())
	}

	if user.Status != 1 {
		return nil, errors.New("账号已被禁用")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	accessToken, refreshToken, err := utils.GenerateToken(
		user.ID, user.Role,
		cfg.Secret, cfg.ExpireMins, cfg.RefreshExpireMins,
	)
	if err != nil {
		return nil, errors.New("生成 Token 失败: " + err.Error())
	}

	return &request.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: request.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Email:    user.Email,
			Role:     user.Role,
			Status:   user.Status,
		},
	}, nil
}

// UpdateSelf 普通用户修改自己的基础信息（不可改密码、用户名、角色）
func UpdateSelf(userID uint, req request.UpdateSelfReq) (*request.UserInfo, error) {
	updates := map[string]interface{}{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Avatar != "" {
		updates["avatar"] = req.Avatar
	}
	if req.Email != "" {
		var exist int64
		global.DB.Model(&model.User{}).Where("email = ? AND id != ?", req.Email, userID).Count(&exist)
		if exist > 0 {
			return nil, errors.New("邮箱已被占用")
		}
		updates["email"] = req.Email
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}
	if err := global.DB.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return nil, errors.New("修改失败")
	}
	return GetUserInfo(userID)
}

// ChangePassword 修改自己的密码（需验证旧密码 + 两次新密码一致 + 复杂度）
func ChangePassword(userID uint, req request.ChangePasswordReq) error {
	if req.NewPassword != req.ConfirmPassword {
		return errors.New("两次输入的新密码不一致")
	}
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}
	if req.OldPassword == req.NewPassword {
		return errors.New("新密码不能与旧密码相同")
	}

	var user model.User
	if err := global.DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return errors.New("旧密码错误")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}
	return global.DB.Model(&user).Update("password", string(hashed)).Error
}

// SetAvatar 设置/更新用户头像 URL
func SetAvatar(userID uint, avatarURL string) (*request.UserInfo, error) {
	if err := global.DB.Model(&model.User{}).Where("id = ?", userID).Update("avatar", avatarURL).Error; err != nil {
		return nil, errors.New("头像更新失败")
	}
	return GetUserInfo(userID)
}

// GetUserInfo 通过 ID 查询用户（脱敏）
func GetUserInfo(userID uint) (*request.UserInfo, error) {
	var user model.User
	if err := global.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, errors.New("查询用户失败: " + err.Error())
	}

	return &request.UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		Email:    user.Email,
		Role:     user.Role,
		Status:   user.Status,
	}, nil
}

// Logout 将 token 加入 Redis 黑名单，过期时间对齐 token 有效期
func Logout(tokenStr string, expireMins int) {
	global.Redis.Set(context.Background(), "blacklist:"+tokenStr, "1", time.Duration(expireMins)*time.Minute)
}
