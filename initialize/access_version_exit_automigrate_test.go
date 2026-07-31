package initialize

import (
	"testing"

	"admin/initialize/testutil"
	"admin/model"
)

func TestExitVersionAutoMigrateDoesNotDeclareLegacyTokenVersionColumn(
	t *testing.T,
) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := migrateDatabase(db); err != nil {
		t.Fatalf("migrate exit-version database: %v", err)
	}
	if db.Migrator().HasColumn(&model.User{}, "token_version") {
		t.Fatal("exit-version AutoMigrate still declares users.token_version")
	}
	if !db.Migrator().HasTable(&model.UserAccessVersion{}) {
		t.Fatal("exit-version AutoMigrate did not create user_access_versions")
	}
	if db.Migrator().HasTable("access_version_migration_states") {
		t.Fatal("exit-version AutoMigrate recreated access_version_migration_states")
	}
}
