package navigation

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type seedMenu struct {
	Name           string
	Path           string
	ParentPath     string
	Component      string
	Icon           string
	PermissionCode string
	Sort           int
	Type           int
	Status         int
}

// SeedMenus restores Navigation-owned menus and their hierarchy.
func SeedMenus(tx *gorm.DB, lockPermissions func(context.Context, []string) error) (int, error) {
	created := 0
	menuIDs := map[string]uint{}
	items := defaultMenus()
	paths := make([]string, 0, len(items)*2)
	codes := make([]string, 0, len(items))
	for _, item := range items {
		codes = append(codes, strings.TrimSpace(item.PermissionCode))
		paths = append(paths, item.Path)
		if item.ParentPath != "" {
			paths = append(paths, item.ParentPath)
		}
	}
	if err := lockPermissions(tx.Statement.Context, codes); err != nil {
		return 0, err
	}
	// Seed may repair an existing hierarchy whose IDs do not follow definition
	// order. Acquire every existing target before processing that order.
	var targets []MenuModel
	if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("path IN ?", paths).Order("id asc").Find(&targets).Error; err != nil {
		return 0, err
	}
	for _, item := range items {
		parentID := uint(0)
		if item.ParentPath != "" {
			if id, ok := menuIDs[item.ParentPath]; ok {
				parentID = id
			} else {
				var parent MenuModel
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("path = ?", item.ParentPath).First(&parent).Error; err != nil {
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

func ensureSeedMenu(tx *gorm.DB, item seedMenu, parentID uint) (MenuModel, bool, error) {
	var menu MenuModel
	err := tx.Unscoped().Where("path = ?", item.Path).First(&menu).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return menu, false, err
	}
	ids := uniqueUintIDs([]uint{menu.ID, parentID})
	if len(ids) > 0 {
		var targets []MenuModel
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", ids).Order("id asc").Find(&targets).Error; err != nil {
			return menu, false, err
		}
		parentExists := parentID == 0
		for _, target := range targets {
			if target.ID == parentID && !target.DeletedAt.Valid {
				parentExists = true
			}
		}
		if !parentExists {
			return menu, false, ErrMenuNotFound
		}
	}
	// The first lookup only discovered lock IDs; do not use its RR snapshot as
	// the state to restore or attach after waiting on a lifecycle target.
	menu = MenuModel{}
	err = tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("path = ?", item.Path).First(&menu).Error
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
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&menu, menu.ID).Error; err != nil {
				return menu, false, err
			}
		}
		return menu, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return menu, false, err
	}
	path := strings.TrimSpace(item.Path)
	menu = MenuModel{ParentID: parentID, Name: item.Name, Path: &path, Component: item.Component, Icon: item.Icon, PermissionCode: item.PermissionCode, Sort: item.Sort, Type: item.Type, Status: item.Status}
	if err := tx.Create(&menu).Error; err != nil {
		return menu, false, err
	}
	return menu, true, nil
}

// SeedAdminRoleMenus grants the protected admin role every known menu.
func SeedAdminRoleMenus(tx *gorm.DB) (int, error) {
	var role struct{ ID uint }
	if err := tx.Table("roles").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("code = ? AND status = 1", "admin").First(&role).Error; err != nil {
		return 0, err
	}
	var menuIDs []uint
	if err := tx.Model(&MenuModel{}).Clauses(clause.Locking{Strength: "UPDATE"}).
		Order("id asc").Pluck("id", &menuIDs).Error; err != nil {
		return 0, err
	}
	created := 0
	for _, menuID := range menuIDs {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RoleMenuModel{RoleID: role.ID, MenuID: menuID})
		if result.Error != nil {
			return created, result.Error
		}
		if result.RowsAffected > 0 {
			created++
		}
	}
	return created, nil
}

func defaultMenus() []seedMenu {
	return []seedMenu{
		{Name: "系统管理", Path: "/system", Component: "Layout", Icon: "Settings", Sort: 10, Type: 1, Status: 1},
		{Name: "用户管理", Path: "/system/users", ParentPath: "/system", Component: "system/users/index", Icon: "Users", PermissionCode: "admin.users.get", Sort: 10, Type: 2, Status: 1},
		{Name: "角色管理", Path: "/system/roles", ParentPath: "/system", Component: "system/roles/index", Icon: "Shield", PermissionCode: "admin.roles.get", Sort: 20, Type: 2, Status: 1},
		{Name: "权限管理", Path: "/system/permissions", ParentPath: "/system", Component: "system/permissions/index", Icon: "KeyRound", PermissionCode: "admin.permissions.get", Sort: 30, Type: 2, Status: 1},
		{Name: "菜单管理", Path: "/system/menus", ParentPath: "/system", Component: "system/menus/index", Icon: "Menu", PermissionCode: "admin.menus.get", Sort: 40, Type: 2, Status: 1},
		{Name: "API 管理", Path: "/system/apis", ParentPath: "/system", Component: "system/apis/index", Icon: "Route", PermissionCode: "admin.apis.get", Sort: 50, Type: 2, Status: 1},
		{Name: "字典管理", Path: "/system/dicts", ParentPath: "/system", Component: "system/dicts/index", Icon: "BookOpen", PermissionCode: "admin.dict-types.get", Sort: 60, Type: 2, Status: 1},
		{Name: "组织管理", Path: "/system/organizations", ParentPath: "/system", Component: "system/organizations/index", Icon: "Network", PermissionCode: "admin.organizations.get", Sort: 70, Type: 2, Status: 1},
		{Name: "操作日志", Path: "/system/audit-logs", ParentPath: "/system", Component: "system/audit-logs/index", Icon: "FileClock", PermissionCode: "admin.audit-logs.get", Sort: 80, Type: 2, Status: 1},
		{Name: "文件管理", Path: "/system/files", ParentPath: "/system", Component: "system/files/index", Icon: "FolderOpen", PermissionCode: "admin.files.get", Sort: 90, Type: 2, Status: 1},
	}
}
