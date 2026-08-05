package gormadapter

import (
	"errors"

	authdomain "admin/internal/authorization/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type seedRole struct {
	Name        string
	Code        string
	Description string
	Sort        int
	Status      int
	DataScope   string
}

type seedPermissionGroup struct {
	Name string
	Sort int
}

// SeedFoundation restores the Authorization-owned roles and permission groups.
func SeedFoundation(tx *gorm.DB) (roles, permissionGroups int, err error) {
	roles, err = seedRoles(tx)
	if err != nil {
		return roles, permissionGroups, err
	}
	permissionGroups, err = seedPermissionGroups(tx)
	return roles, permissionGroups, err
}

func defaultRoles() []seedRole {
	return []seedRole{
		{Name: "超级管理员", Code: "admin", Description: "系统最高权限角色", Sort: 0, Status: 1, DataScope: string(authdomain.DataScopeAll)},
		{Name: "普通用户", Code: "user", Description: "系统默认普通用户角色", Sort: 100, Status: 1, DataScope: string(authdomain.DataScopeSelf)},
	}
}

func defaultPermissionGroups() []seedPermissionGroup {
	return []seedPermissionGroup{
		{Name: "auth", Sort: 1}, {Name: "user", Sort: 2}, {Name: "role", Sort: 3},
		{Name: "permission", Sort: 4}, {Name: "menu", Sort: 5}, {Name: "api", Sort: 6},
		{Name: "organization", Sort: 7}, {Name: "dict", Sort: 8}, {Name: "file", Sort: 9},
		{Name: "audit", Sort: 10}, {Name: "system", Sort: 99},
	}
}

func seedRoles(tx *gorm.DB) (int, error) {
	created := 0
	for _, item := range defaultRoles() {
		var role Role
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
		role = Role{Name: item.Name, Code: item.Code, Description: item.Description, Sort: item.Sort, Status: item.Status, DataScope: item.DataScope}
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
		var group PermissionGroup
		err := tx.Unscoped().Where("name = ?", item.Name).First(&group).Error
		if err == nil {
			if group.DeletedAt.Valid {
				if err := tx.Unscoped().Model(&group).Updates(map[string]any{"deleted_at": nil}).Error; err != nil {
					return created, err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, err
		}
		if err := tx.Create(&PermissionGroup{Name: item.Name, Sort: item.Sort}).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// SeedAdminRolePermissions grants the protected admin role every known permission.
func SeedAdminRolePermissions(tx *gorm.DB) (int, error) {
	adminRole, err := getRoleByCode(tx, "admin")
	if err != nil {
		return 0, err
	}
	var permissionIDs []uint
	if err := tx.Model(&Permission{}).Pluck("id", &permissionIDs).Error; err != nil {
		return 0, err
	}
	created := 0
	for _, permissionID := range permissionIDs {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RolePermission{RoleID: adminRole.ID, PermissionID: permissionID})
		if result.Error != nil {
			return created, result.Error
		}
		if result.RowsAffected > 0 {
			created++
		}
	}
	return created, nil
}

func getRoleByCode(tx *gorm.DB, code string) (Role, error) {
	var role Role
	err := tx.Where("code = ? AND status = 1", code).First(&role).Error
	return role, err
}
