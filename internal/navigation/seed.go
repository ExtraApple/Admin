package navigation

import (
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
func SeedMenus(tx *gorm.DB) (int, error) {
	created := 0
	menuIDs := map[string]uint{}
	for _, item := range defaultMenus() {
		parentID := uint(0)
		if item.ParentPath != "" {
			if id, ok := menuIDs[item.ParentPath]; ok {
				parentID = id
			} else {
				var parent MenuModel
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

func ensureSeedMenu(tx *gorm.DB, item seedMenu, parentID uint) (MenuModel, bool, error) {
	var menu MenuModel
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
	menu = MenuModel{ParentID: parentID, Name: item.Name, Path: &path, Component: item.Component, Icon: item.Icon, PermissionCode: item.PermissionCode, Sort: item.Sort, Type: item.Type, Status: item.Status}
	if err := tx.Create(&menu).Error; err != nil {
		return menu, false, err
	}
	return menu, true, nil
}

// SeedAdminRoleMenus grants the protected admin role every known menu.
func SeedAdminRoleMenus(tx *gorm.DB) (int, error) {
	var role struct{ ID uint }
	if err := tx.Table("roles").Where("code = ? AND status = 1", "admin").First(&role).Error; err != nil {
		return 0, err
	}
	var menuIDs []uint
	if err := tx.Model(&MenuModel{}).Pluck("id", &menuIDs).Error; err != nil {
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
