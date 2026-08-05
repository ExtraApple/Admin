package testsupport

import (
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
)

func TestExitVersionAutoMigrateDoesNotDeclareLegacyTokenVersionColumn(
	t *testing.T,
) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate exit-version database: %v", err)
	}
	if db.Migrator().HasColumn(&identity.User{}, "token_version") {
		t.Fatal("exit-version AutoMigrate still declares users.token_version")
	}
	if !db.Migrator().HasTable(&authgorm.UserAccessVersion{}) {
		t.Fatal("exit-version AutoMigrate did not create user_access_versions")
	}
	if db.Migrator().HasTable("access_version_migration_states") {
		t.Fatal("exit-version AutoMigrate recreated access_version_migration_states")
	}
}
