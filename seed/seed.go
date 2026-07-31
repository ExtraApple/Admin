package seed

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"admin/dto"
	"admin/global"
	"admin/initialize"
	"admin/model"
	"admin/service"

	"go.uber.org/zap"
)

func Run(conf *initialize.Config, routes []dto.SyncAPIItem) error {
	summary := seedSummary{}
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		if summary.Roles, err = seedRoles(tx); err != nil {
			return err
		}
		if summary.PermissionGroups, err = seedPermissionGroups(tx); err != nil {
			return err
		}
		if summary.DictTypes, summary.DictItems, err = seedDicts(tx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	apis, err := service.SyncAPIs(routes)
	if err != nil {
		return err
	}
	permissions, updatedAPI, err := service.SyncAPIPermissions()
	if err != nil {
		return err
	}
	summary.APIs = len(apis)
	summary.Permissions = len(permissions) + updatedAPI

	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		if summary.Menus, err = seedMenus(tx); err != nil {
			return err
		}
		if summary.RolePermissions, err = seedAdminRolePermissions(tx); err != nil {
			return err
		}
		if summary.RoleMenus, err = seedAdminRoleMenus(tx); err != nil {
			return err
		}
		if err = seedAdminUser(tx, conf); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	global.Logger.Info("seeds completed",
		zap.Int("roles", summary.Roles),
		zap.Int("permission_groups", summary.PermissionGroups),
		zap.Int("apis", summary.APIs),
		zap.Int("permissions", summary.Permissions),
		zap.Int("menus", summary.Menus),
		zap.Int("role_permissions", summary.RolePermissions),
		zap.Int("role_menus", summary.RoleMenus),
		zap.Int("dict_types", summary.DictTypes),
		zap.Int("dict_items", summary.DictItems),
	)
	return nil
}

func seedRoles(tx *gorm.DB) (int, error) {
	created := 0
	for _, item := range defaultRoles() {
		var role model.Role
		err := tx.Unscoped().Where("code = ?", item.Code).First(&role).Error
		if err == nil {
			updates := map[string]any{}
			if role.DeletedAt.Valid {
				updates["deleted_at"] = nil
			}
			if role.Status != item.Status {
				updates["status"] = item.Status
			}
			if role.DataScope == "" {
				updates["data_scope"] = item.DataScope
			}
			if len(updates) > 0 {
				if err := tx.Unscoped().Model(&role).Updates(updates).Error; err != nil {
					return created, err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, err
		}

		role = model.Role{
			Name:        item.Name,
			Code:        item.Code,
			Description: item.Description,
			Sort:        item.Sort,
			Status:      item.Status,
			DataScope:   item.DataScope,
		}
		if err := tx.Create(&role).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func seedPermissionGroups(tx *gorm.DB) (int, error) {
	created := 0
	for _, item := range defaultPermissionGroups() {
		var group model.PermissionGroup
		err := tx.Unscoped().Where("name = ?", item.Name).First(&group).Error
		if err == nil {
			if group.DeletedAt.Valid {
				if err := tx.Unscoped().Model(&group).Updates(map[string]any{
					"deleted_at": nil,
				}).Error; err != nil {
					return created, err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, err
		}

		if err := tx.Create(&model.PermissionGroup{Name: item.Name, Sort: item.Sort}).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func seedDicts(tx *gorm.DB) (int, int, error) {
	createdTypes := 0
	createdItems := 0
	for _, item := range defaultDictTypes() {
		var dictType model.DictType
		err := tx.Unscoped().Where("code = ?", item.Code).First(&dictType).Error
		if err == nil {
			if dictType.DeletedAt.Valid {
				if err := tx.Unscoped().Model(&dictType).Updates(map[string]any{
					"deleted_at": nil,
					"status":     1,
				}).Error; err != nil {
					return createdTypes, createdItems, err
				}
			}
		} else {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return createdTypes, createdItems, err
			}
			dictType = model.DictType{
				Name:   item.Name,
				Code:   item.Code,
				Remark: item.Remark,
				Sort:   item.Sort,
				Status: item.Status,
			}
			if err := tx.Create(&dictType).Error; err != nil {
				return createdTypes, createdItems, err
			}
			createdTypes++
		}

		for _, dictItem := range item.Items {
			var existingItem model.DictItem
			err := tx.Unscoped().Where("type_code = ? AND value = ?", item.Code, dictItem.Value).First(&existingItem).Error
			if err == nil {
				if existingItem.DeletedAt.Valid {
					if err := tx.Unscoped().Model(&existingItem).Updates(map[string]any{
						"deleted_at": nil,
						"status":     1,
					}).Error; err != nil {
						return createdTypes, createdItems, err
					}
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return createdTypes, createdItems, err
			}

			if err := tx.Create(&model.DictItem{
				TypeCode: item.Code,
				Label:    dictItem.Label,
				Value:    dictItem.Value,
				Remark:   dictItem.Remark,
				Sort:     dictItem.Sort,
				Status:   dictItem.Status,
			}).Error; err != nil {
				return createdTypes, createdItems, err
			}
			createdItems++
		}
	}
	return createdTypes, createdItems, nil
}

func seedMenus(tx *gorm.DB) (int, error) {
	created := 0
	menuIDs := map[string]uint{}
	for _, item := range defaultMenus() {
		parentID := uint(0)
		if item.ParentPath != "" {
			if id, ok := menuIDs[item.ParentPath]; ok {
				parentID = id
			} else {
				var parent model.Menu
				if err := tx.Where("path = ?", item.ParentPath).First(&parent).Error; err != nil {
					return created, err
				}
				parentID = parent.ID
			}
		}

		menu, wasCreated, err := ensureSeedMenu(tx, item, parentID)
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
		menuIDs[item.Path] = menu.ID
	}
	return created, nil
}

func ensureSeedMenu(tx *gorm.DB, item seedMenu, parentID uint) (model.Menu, bool, error) {
	var menu model.Menu
	err := tx.Unscoped().Where("path = ?", item.Path).First(&menu).Error
	if err == nil {
		updates := map[string]any{}
		if menu.DeletedAt.Valid {
			updates["deleted_at"] = nil
			updates["status"] = item.Status
		}
		if menu.ParentID == 0 && parentID != 0 {
			updates["parent_id"] = parentID
		}
		if menu.PermissionCode == "" && item.PermissionCode != "" {
			updates["permission_code"] = item.PermissionCode
		}
		if len(updates) > 0 {
			if err := tx.Unscoped().Model(&menu).Updates(updates).Error; err != nil {
				return menu, false, err
			}
			if err := tx.First(&menu, menu.ID).Error; err != nil {
				return menu, false, err
			}
		}
		return menu, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return menu, false, err
	}

	path := strings.TrimSpace(item.Path)
	menu = model.Menu{
		ParentID:       parentID,
		Name:           item.Name,
		Path:           &path,
		Component:      item.Component,
		Icon:           item.Icon,
		PermissionCode: item.PermissionCode,
		Sort:           item.Sort,
		Type:           item.Type,
		Status:         item.Status,
	}
	if err := tx.Create(&menu).Error; err != nil {
		return menu, false, err
	}
	return menu, true, nil
}

func seedAdminRolePermissions(tx *gorm.DB) (int, error) {
	adminRole, err := getRoleByCode(tx, "admin")
	if err != nil {
		return 0, err
	}

	var permissionIDs []uint
	if err := tx.Model(&model.Permission{}).Pluck("id", &permissionIDs).Error; err != nil {
		return 0, err
	}
	if len(permissionIDs) == 0 {
		return 0, nil
	}

	created := 0
	for _, permissionID := range permissionIDs {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.RolePermission{
			RoleID:       adminRole.ID,
			PermissionID: permissionID,
		})
		if result.Error != nil {
			return created, result.Error
		}
		if result.RowsAffected > 0 {
			created++
		}
	}
	return created, nil
}

func seedAdminRoleMenus(tx *gorm.DB) (int, error) {
	adminRole, err := getRoleByCode(tx, "admin")
	if err != nil {
		return 0, err
	}

	var menuIDs []uint
	if err := tx.Model(&model.Menu{}).Pluck("id", &menuIDs).Error; err != nil {
		return 0, err
	}
	if len(menuIDs) == 0 {
		return 0, nil
	}

	created := 0
	for _, menuID := range menuIDs {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.RoleMenu{
			RoleID: adminRole.ID,
			MenuID: menuID,
		})
		if result.Error != nil {
			return created, result.Error
		}
		if result.RowsAffected > 0 {
			created++
		}
	}
	return created, nil
}

func seedAdminUser(tx *gorm.DB, conf *initialize.Config) error {
	adminRole, err := getRoleByCode(tx, "admin")
	if err != nil {
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

	var user model.User
	err = tx.Unscoped().Where("username = ?", conf.Admin.Username).First(&user).Error
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

	user = model.User{
		Username: conf.Admin.Username,
		Password: string(hashed),
		Email:    conf.Admin.Email,
		Nickname: conf.Admin.Nickname,
		Role:     "user",
		Status:   1,
	}
	if err := tx.Create(&user).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.UserRole{
		UserID: user.ID,
		RoleID: adminRole.ID,
	}).Error
}

func restoreSeedAdminUser(tx *gorm.DB, user model.User, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return tx.Unscoped().Model(&user).Updates(map[string]any{
		"password":   string(hashed),
		"status":     1,
		"deleted_at": nil,
	}).Error
}

func getRoleByCode(tx *gorm.DB, code string) (model.Role, error) {
	var role model.Role
	err := tx.Where("code = ? AND status = 1", code).First(&role).Error
	return role, err
}

func hasActiveSuperAdmin(tx *gorm.DB, roleID uint) bool {
	var count int64
	tx.Table("users").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Where("user_roles.role_id = ? AND users.status = ? AND users.deleted_at IS NULL", roleID, 1).
		Count(&count)
	return count > 0
}

func hasUserRoleInTx(tx *gorm.DB, userID, roleID uint) bool {
	var count int64
	tx.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", userID, roleID).Count(&count)
	return count > 0
}
