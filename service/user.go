package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
	"admin/service/uploadsecurity"
	"admin/utils"
)

type JWTConfig struct {
	Secret            string
	ExpireMins        int
	RefreshExpireMins int
}

var (
	ErrAvatarFieldNotWritable = errors.New("头像只能通过专用接口修改")
	ErrRefreshTokenInvalid    = errors.New("Refresh Token 无效或已过期")
)

const maxFailures = 5

// Register 用户注册
func Register(req dto.RegisterReq) (*dto.UserInfo, error) {
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

	info := UserInfoFromModel(user)
	return &info, nil
}

// Login 用户登录
func Login(req dto.LoginReq, cfg JWTConfig) (*dto.LoginResp, error) {
	// 校验验证码
	if !VerifyCaptcha(req.CaptchaID, req.CaptchaCode) {
		return nil, errors.New("验证码错误或已过期")
	}

	// 检查是否被锁定
	if ttl, ok := isLocked(req.Username); ok {
		return nil, fmt.Errorf("账号已被锁定，请 %d 分钟后重试", ttl)
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
		key := "fail:" + req.Username
		failureCount, _ := global.Redis.Incr(context.Background(), key).Result()
		global.Redis.Expire(context.Background(), key, 24*time.Hour)

		count := int(failureCount)
		lockAfterFail(req.Username, count)
		return nil, fmt.Errorf("用户名或密码错误（剩余尝试: %d 次）", maxFailures-count)
	}

	accessVersionRepository := NewAccessVersionRepository(global.DB)
	tokenVersion, err := accessVersionRepository.EnsureVersion(user.ID)
	if err != nil {
		return nil, fmt.Errorf("初始化用户授权版本失败: %w", err)
	}

	// 登录成功，清除失败记录
	global.Redis.Del(context.Background(), "fail:"+req.Username, "lock:"+req.Username)

	// 查询用户关联的角色码
	var roles []string
	var ur []model.UserRole
	global.DB.Where("user_id = ?", user.ID).Find(&ur)
	if len(ur) > 0 {
		ids := make([]uint, len(ur))
		for i, r := range ur {
			ids[i] = r.RoleID
		}
		var rls []model.Role
		global.DB.Where("id IN ? AND status = 1", ids).Find(&rls)
		for _, r := range rls {
			roles = append(roles, r.Code)
		}
	}

	// 查询用户关联的权限码
	permissions := GetUserPermissions(user.ID)

	accessToken, refreshToken, err := utils.GenerateToken(
		user.ID, tokenVersion, roles, permissions,
		cfg.Secret, cfg.ExpireMins, cfg.RefreshExpireMins,
	)
	if err != nil {
		return nil, errors.New("生成 Token 失败: " + err.Error())
	}
	accessVersionRepository.recordSuccessfulTokenIssuance(user.ID)

	return &dto.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         UserInfoFromModel(user),
	}, nil
}

// RefreshTokens 使用有效 Refresh Token 签发新的 Access Token 和 Refresh Token。
func RefreshTokens(
	req dto.RefreshTokenReq,
	cfg JWTConfig,
) (*dto.RefreshTokenResp, error) {
	claims, err := utils.ParseToken(req.RefreshToken, cfg.Secret)
	if err != nil {
		return nil, ErrRefreshTokenInvalid
	}

	if global.Redis != nil {
		blacklisted, err := global.Redis.Exists(
			context.Background(),
			"blacklist:"+req.RefreshToken,
		).Result()
		if err != nil || blacklisted > 0 {
			return nil, ErrRefreshTokenInvalid
		}
	}

	currentVersion, err := currentUserTokenVersion(claims.UserID)
	if err != nil ||
		claims.TokenVersion <= 0 ||
		claims.TokenVersion != currentVersion {
		return nil, ErrRefreshTokenInvalid
	}

	accessToken, refreshToken, err := utils.GenerateToken(
		claims.UserID,
		currentVersion,
		claims.Roles,
		claims.Permissions,
		cfg.Secret,
		cfg.ExpireMins,
		cfg.RefreshExpireMins,
	)
	if err != nil {
		return nil, fmt.Errorf("生成 Token 失败: %w", err)
	}
	NewAccessVersionRepository(global.DB).
		recordSuccessfulTokenIssuance(claims.UserID)
	return &dto.RefreshTokenResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// isLocked 检查指定用户名是否处于登录锁定状态，并返回剩余分钟数。
func isLocked(username string) (int, bool) {
	val, err := global.Redis.Get(context.Background(), "lock:"+username).Result()
	if err != nil || val != "1" {
		return 0, false
	}

	ttl, _ := global.Redis.TTL(context.Background(), "lock:"+username).Result()
	mins := int(ttl.Minutes()) + 1
	return mins, true
}

// lockAfterFail 在失败次数达到阈值时设置账号锁定标记。
func lockAfterFail(username string, failures int) {
	lockMinutes := 0
	switch {
	case failures < maxFailures:
		return
	case failures < 10:
		lockMinutes = 1
	case failures < 15:
		lockMinutes = 5
	case failures < 20:
		lockMinutes = 15
	default:
		lockMinutes = 60
	}

	global.Redis.Set(context.Background(), "lock:"+username, "1", time.Duration(lockMinutes)*time.Minute)
	if failures >= 10 {
		global.Redis.Del(context.Background(), "fail:"+username)
	}
}

// UpdateSelf 普通用户修改自己的基础信息（不可改密码、用户名、角色）
func UpdateSelf(userID uint, req dto.UpdateSelfReq) (*dto.UserInfo, error) {
	if req.HasAvatarField() {
		return nil, ErrAvatarFieldNotWritable
	}

	updates := map[string]any{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Email != "" {
		var exist int64
		if err := global.DB.Model(&model.User{}).
			Where("email = ? AND id != ?", req.Email, userID).
			Count(&exist).Error; err != nil {
			return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
		}
		if exist > 0 {
			return nil, uploadsecurity.NewError(
				uploadsecurity.CodeRequestInvalid,
				errors.New("邮箱已被占用"),
			)
		}
		updates["email"] = req.Email
	}
	if len(updates) == 0 {
		return nil, uploadsecurity.NewError(
			uploadsecurity.CodeRequestInvalid,
			errors.New("无修改内容"),
		)
	}
	if err := global.DB.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	user, err := GetUserInfo(userID)
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return user, nil
}

// ChangePassword 修改自己的密码（需验证旧密码 + 两次新密码一致 + 复杂度）
func ChangePassword(userID uint, req dto.ChangePasswordReq) error {
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
	return global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password", string(hashed)).Error; err != nil {
			return err
		}
		_, err := NewAccessVersionRepository(tx).
			EnsureAndIncrement(tx, userID)
		return err
	})
}

// GetUserInfo 通过 ID 查询用户（脱敏）
func GetUserInfo(userID uint) (*dto.UserInfo, error) {
	var user model.User
	if err := global.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, errors.New("查询用户失败: " + err.Error())
	}

	info := UserInfoFromModel(user)
	return &info, nil
}

// Logout 将 token 加入 Redis 黑名单，过期时间对齐 token 有效期
func Logout(tokenStr string, expireMins int) {
	global.Redis.Set(context.Background(), "blacklist:"+tokenStr, "1", time.Duration(expireMins)*time.Minute)
}
