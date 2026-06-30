package initialize

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"

	"go.uber.org/zap"
)

// InitSuperAdmin ensures the new RBAC system always has one enabled admin user.
func InitSuperAdmin(conf *Config) {
	adminRole := ensureAdminRole()
	if hasEnabledSuperAdmin(adminRole.ID) {
		return
	}

	user := ensureAdminUser(conf, adminRole.ID)
	ensureAdminUserRole(user.ID, adminRole.ID)
	global.Logger.Info("super admin initialized", zap.String("username", user.Username))
}

func ensureAdminRole() model.Role {
	var role model.Role
	err := global.DB.Unscoped().Where("code = ?", "admin").First(&role).Error
	if err == nil {
		if role.Status != 1 || role.DeletedAt.Valid {
			updates := map[string]any{
				"status":     1,
				"deleted_at": nil,
			}
			if err := global.DB.Unscoped().Model(&role).Updates(updates).Error; err != nil {
				global.Logger.Fatal("restore admin role failed", zap.Error(err))
			}
			global.DB.First(&role, role.ID)
		}
		return role
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Fatal("query admin role failed", zap.Error(err))
	}

	role = model.Role{
		Name:        "超级管理员",
		Code:        "admin",
		Description: "系统最高权限角色",
		Sort:        0,
		Status:      1,
	}
	if err := global.DB.Create(&role).Error; err != nil {
		global.Logger.Fatal("create admin role failed", zap.Error(err))
	}
	return role
}

func hasEnabledSuperAdmin(roleID uint) bool {
	var count int64
	global.DB.Table("users").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Where("user_roles.role_id = ? AND users.status = ? AND users.deleted_at IS NULL", roleID, 1).
		Count(&count)
	return count > 0
}

func ensureAdminUser(conf *Config, roleID uint) model.User {
	if conf.Admin.Username == "" {
		global.Logger.Fatal("admin username is empty")
	}
	if conf.Admin.Password == "" {
		global.Logger.Fatal("admin password is empty")
	}
	if conf.Admin.Email == "" {
		global.Logger.Fatal("admin email is empty")
	}

	var user model.User
	err := global.DB.Unscoped().Where("username = ?", conf.Admin.Username).First(&user).Error
	if err == nil {
		if !hasUserRole(user.ID, roleID) {
			global.Logger.Fatal("configured admin username already exists but is not bound to admin role", zap.String("username", conf.Admin.Username))
		}
		restoreAdminUser(user, conf.Admin.Password)
		global.DB.First(&user, user.ID)
		return user
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Fatal("query configured admin user failed", zap.Error(err))
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(conf.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		global.Logger.Fatal("hash admin password failed", zap.Error(err))
	}

	user = model.User{
		Username:     conf.Admin.Username,
		Password:     string(hashed),
		Email:        conf.Admin.Email,
		Nickname:     conf.Admin.Nickname,
		Role:         "user",
		Status:       1,
		TokenVersion: 1,
	}
	if err := global.DB.Create(&user).Error; err != nil {
		global.Logger.Fatal("create admin user failed", zap.Error(err))
	}
	return user
}

func restoreAdminUser(user model.User, password string) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		global.Logger.Fatal("hash admin password failed", zap.Error(err))
	}

	updates := map[string]any{
		"password":      string(hashed),
		"status":        1,
		"deleted_at":    nil,
		"token_version": gorm.Expr("COALESCE(token_version, 0) + ?", 1),
	}
	if err := global.DB.Unscoped().Model(&user).Updates(updates).Error; err != nil {
		global.Logger.Fatal("restore admin user failed", zap.Error(err))
	}
}

func hasUserRole(userID, roleID uint) bool {
	var count int64
	global.DB.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", userID, roleID).Count(&count)
	return count > 0
}

func ensureAdminUserRole(userID, roleID uint) {
	if hasUserRole(userID, roleID) {
		return
	}
	if err := global.DB.Create(&model.UserRole{UserID: userID, RoleID: roleID}).Error; err != nil {
		global.Logger.Fatal("bind admin role failed", zap.Error(err))
	}
}
