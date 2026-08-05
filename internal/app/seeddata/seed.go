package seeddata

import (
	"admin/internal/authorization/adapters/gorm"
	"admin/internal/dictionary"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/navigation"
	platformconfig "admin/internal/platform/config"

	"gorm.io/gorm"
)

// Foundation orchestrates module-provided foundational Seed capabilities. It
// owns ordering only; each module owns its data definitions and writes.
func Foundation(tx *gorm.DB) (Summary, error) {
	summary := Summary{}
	roles, groups, err := gormadapter.SeedFoundation(tx)
	if err != nil {
		return summary, err
	}
	summary.Roles, summary.PermissionGroups = roles, groups
	summary.DictTypes, summary.DictItems, err = dictionary.Seed(tx)
	return summary, err
}

// Finalize runs the dependency-ordered module Seed capabilities after API and
// permission synchronization has completed.
func Finalize(tx *gorm.DB, conf *platformconfig.Config) (Summary, error) {
	summary := Summary{}
	var err error
	if summary.Menus, err = navigation.SeedMenus(tx); err != nil {
		return summary, err
	}
	if summary.RolePermissions, err = gormadapter.SeedAdminRolePermissions(tx); err != nil {
		return summary, err
	}
	if summary.RoleMenus, err = navigation.SeedAdminRoleMenus(tx); err != nil {
		return summary, err
	}
	if err = identitygorm.SeedAdminUser(tx, conf); err != nil {
		return summary, err
	}
	return summary, nil
}
