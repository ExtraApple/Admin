package service

import (
	"errors"

	"gorm.io/gorm"

	"admin/global"
	"admin/model"
)

// currentUserTokenVersion 查询用户当前 Token 版本，并在缺省时补齐初始版本。
func currentUserTokenVersion(userID uint) (int, error) {
	var user model.User
	if err := global.DB.Select("id", "status", "token_version").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errors.New("用户不存在")
		}
		return 0, errors.New("查询用户Token版本失败")
	}
	if user.Status != 1 {
		return 0, errors.New("账号已被禁用")
	}
	if user.TokenVersion <= 0 {
		user.TokenVersion = 1
		global.DB.Model(&model.User{}).Where("id = ?", userID).Update("token_version", user.TokenVersion)
	}
	return user.TokenVersion, nil
}

// IsTokenVersionValid 校验请求 Token 中的版本号是否仍然有效。
func IsTokenVersionValid(userID uint, tokenVersion int) error {
	currentVersion, err := currentUserTokenVersion(userID)
	if err != nil {
		return err
	}
	if tokenVersion <= 0 || tokenVersion != currentVersion {
		return errors.New("Token已失效，请重新登录")
	}
	return nil
}

// revokeTokensForUsers 提升指定用户的 Token 版本，使其既有 Token 失效。
func revokeTokensForUsers(userIDs ...uint) {
	userIDs = uniqueUintIDs(userIDs)
	if len(userIDs) == 0 {
		return
	}
	global.DB.Model(&model.User{}).Where("id IN ?", userIDs).UpdateColumn("token_version", gorm.Expr("COALESCE(token_version, 0) + ?", 1))
}

// revokeTokensForRole 提升指定角色下所有用户的 Token 版本。
func revokeTokensForRole(roleID uint) {
	var userIDs []uint
	global.DB.Model(&model.UserRole{}).Where("role_id = ?", roleID).Pluck("user_id", &userIDs)
	revokeTokensForUsers(userIDs...)
}

// revokeTokensForRoles 提升多个角色下所有用户的 Token 版本。
func revokeTokensForRoles(roleIDs []uint) {
	roleIDs = uniqueUintIDs(roleIDs)
	if len(roleIDs) == 0 {
		return
	}
	var userIDs []uint
	global.DB.Model(&model.UserRole{}).Where("role_id IN ?", roleIDs).Pluck("user_id", &userIDs)
	revokeTokensForUsers(userIDs...)
}

// revokeAllUserTokens 提升全部用户的 Token 版本。
func revokeAllUserTokens() {
	global.DB.Model(&model.User{}).UpdateColumn("token_version", gorm.Expr("COALESCE(token_version, 0) + ?", 1))
}
