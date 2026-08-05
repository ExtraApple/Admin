package gormadapter_test

import (
	"testing"

	"admin/testsupport/testutil"
	authgorm "admin/internal/authorization/adapters/gorm"
)

func TestModelsMigrateAuthorizationOwnedRBACTables(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(authgorm.Models()...); err != nil {
		t.Fatalf("migrate authorization models: %v", err)
	}
	for _, table := range []string{
		"roles", "user_roles", "role_data_scopes", "permissions", "role_permissions", "permission_groups", "user_access_versions",
	} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("authorization migration should own table %q", table)
		}
	}
}
