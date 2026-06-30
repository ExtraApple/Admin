package service

import (
	"admin/global"
	"admin/model"
)

func isAdminUser(user model.User) bool {
	var count int64
	global.DB.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ? AND roles.status = ?", user.ID, "admin", 1).
		Count(&count)
	return count > 0
}
