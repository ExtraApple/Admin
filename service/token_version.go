package service

import (
	"errors"
	"sort"

	"gorm.io/gorm"

	"admin/global"
	"admin/model"
)

// currentUserTokenVersion 在确认用户存在且启用后读取 Authorization 拥有的版本。
func currentUserTokenVersion(userID uint) (int, error) {
	var user model.User
	if err := global.DB.Select("id", "status").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errors.New("用户不存在")
		}
		return 0, errors.New("查询用户状态失败")
	}
	if user.Status != 1 {
		return 0, errors.New("账号已被禁用")
	}

	version, err := NewAccessVersionRepository(global.DB).CurrentVersion(userID)
	if err != nil {
		return 0, err
	}
	return version, nil
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

func incrementAccessVersionsForUsers(
	tx *gorm.DB,
	userIDs ...uint,
) error {
	userIDs = uniqueUintIDs(userIDs)
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	repository := NewAccessVersionRepository(tx)
	for _, userID := range userIDs {
		if _, err := repository.EnsureAndIncrement(userID); err != nil {
			return err
		}
	}
	return nil
}

func incrementAccessVersionsForRole(tx *gorm.DB, roleID uint) error {
	var userIDs []uint
	if err := tx.Model(&model.UserRole{}).
		Where("role_id = ?", roleID).
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	return incrementAccessVersionsForUsers(tx, userIDs...)
}

func incrementAccessVersionsForRoles(
	tx *gorm.DB,
	roleIDs []uint,
) error {
	roleIDs = uniqueUintIDs(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}
	var userIDs []uint
	if err := tx.Model(&model.UserRole{}).
		Where("role_id IN ?", roleIDs).
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	return incrementAccessVersionsForUsers(tx, userIDs...)
}

func incrementAllAccessVersions(tx *gorm.DB) error {
	var userIDs []uint
	if err := tx.Model(&model.User{}).
		Pluck("id", &userIDs).Error; err != nil {
		return err
	}
	return incrementAccessVersionsForUsers(tx, userIDs...)
}
