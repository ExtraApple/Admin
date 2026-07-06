package service

import (
	"admin/global"
	"admin/model"
)

// isAdminUser 判断用户是否绑定了启用状态的 admin 角色。
func isAdminUser(user model.User) bool {
	var count int64
	global.DB.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ? AND roles.status = ?", user.ID, "admin", 1).
		Count(&count)
	return count > 0
}
