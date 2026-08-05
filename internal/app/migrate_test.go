package app_test

import (
	"testing"

	"admin/testsupport/testutil"
	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/app"
	"admin/internal/audit"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/dictionary"
	"admin/internal/files"
	"admin/internal/identity"
	"admin/internal/navigation"
	internalorganization "admin/internal/organization"
)

func TestMigrateCollectsExistingModels(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate application database: %v", err)
	}

	wantModels := []any{
		identity.User{},
		authgorm.UserAccessVersion{},
		authgorm.Role{},
		authgorm.UserRole{},
		authgorm.RoleDataScope{},
		authgorm.Permission{},
		authgorm.RolePermission{},
		authgorm.PermissionGroup{},
		files.File{},
		navigation.MenuModel{},
		navigation.RoleMenuModel{},
		navigation.MenuAPIModel{},
		audit.AuditLog{},
		audit.AuditLogArchive{},
		dictionary.Type{},
		dictionary.Item{},
		internalorganization.Unit{},
		internalorganization.Membership{},
		apigorm.API{},
	}
	for _, registeredModel := range wantModels {
		if !db.Migrator().HasTable(registeredModel) {
			t.Fatalf("migration did not create table for %T", registeredModel)
		}
	}
}
