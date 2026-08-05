package gormadapter

import (
	"errors"

	platformconfig "admin/internal/platform/config"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type seedRoleRecord struct{ gorm.Model }

type seedUserRoleRecord struct {
	UserID uint `gorm:"primaryKey"`
	RoleID uint `gorm:"primaryKey"`
}

func (seedRoleRecord) TableName() string     { return "roles" }
func (seedUserRoleRecord) TableName() string { return "user_roles" }

// SeedAdminUser provisions the configured Identity-owned administrator without
// exposing Authorization persistence models to the application composition root.
func SeedAdminUser(tx *gorm.DB, conf *platformconfig.Config) error {
	var adminRole seedRoleRecord
	if err := tx.Where("code = ? AND status = 1", "admin").First(&adminRole).Error; err != nil {
		return err
	}
	if hasActiveSuperAdmin(tx, adminRole.ID) {
		return nil
	}
	if conf.Admin.Username == "" {
		return errors.New("admin username is empty")
	}
	if conf.Admin.Password == "" {
		return errors.New("admin password is empty")
	}
	if conf.Admin.Email == "" {
		return errors.New("admin email is empty")
	}

	var user UserModel
	err := tx.Unscoped().Where("username = ?", conf.Admin.Username).First(&user).Error
	if err == nil {
		if !hasUserRoleInTx(tx, user.ID, adminRole.ID) {
			return errors.New("configured admin username already exists but is not bound to admin role")
		}
		return restoreSeedAdminUser(tx, user, conf.Admin.Password)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(conf.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user = UserModel{Username: conf.Admin.Username, Password: string(hashed), Email: conf.Admin.Email, Nickname: conf.Admin.Nickname, Role: "user", Status: 1}
	if err := tx.Create(&user).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seedUserRoleRecord{UserID: user.ID, RoleID: adminRole.ID}).Error
}

func restoreSeedAdminUser(tx *gorm.DB, user UserModel, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return tx.Unscoped().Model(&user).Updates(map[string]any{"password": string(hashed), "status": 1, "deleted_at": nil}).Error
}

func hasActiveSuperAdmin(tx *gorm.DB, roleID uint) bool {
	var count int64
	tx.Table("users").Joins("JOIN user_roles ON user_roles.user_id = users.id").Where("user_roles.role_id = ? AND users.status = ? AND users.deleted_at IS NULL", roleID, 1).Count(&count)
	return count > 0
}

func hasUserRoleInTx(tx *gorm.DB, userID, roleID uint) bool {
	var count int64
	tx.Model(&seedUserRoleRecord{}).Where("user_id = ? AND role_id = ?", userID, roleID).Count(&count)
	return count > 0
}
