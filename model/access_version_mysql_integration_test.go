//go:build mysql_integration

package model_test

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"admin/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestMySQLAccessVersionAutoMigrateSchemaAndSoftDelete(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required for the mandatory MySQL gate")
	}

	tablePrefix := fmt.Sprintf("it_av_%d_", time.Now().UnixNano())
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: tablePrefix,
		},
	})
	if err != nil {
		t.Fatalf("open MySQL integration database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL integration connection: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close MySQL integration database: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := db.Migrator().DropTable(
			&model.UserAccessVersion{},
			&model.User{},
		); err != nil {
			t.Errorf("drop isolated MySQL integration tables: %v", err)
		}
	})

	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("auto migrate access version storage in MySQL: %v", err)
	}
	for _, migratedModel := range []any{
		&model.User{},
		&model.UserAccessVersion{},
	} {
		if !db.Migrator().HasTable(migratedModel) {
			t.Fatalf("MySQL AutoMigrate did not create %T", migratedModel)
		}
	}

	userAccessVersionTable := db.NamingStrategy.TableName("UserAccessVersion")
	migrationStateTable := db.NamingStrategy.TableName("AccessVersionMigrationState")
	var primaryKeyColumns int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND index_name = 'PRIMARY'
		  AND column_name = 'user_id'
		  AND non_unique = 0
	`, userAccessVersionTable).Scan(&primaryKeyColumns).Error; err != nil {
		t.Fatalf("inspect MySQL primary key %s.user_id: %v", userAccessVersionTable, err)
	}
	if primaryKeyColumns != 1 {
		t.Fatalf(
			"MySQL primary key %s.user_id: got %d matching index columns, want 1",
			userAccessVersionTable,
			primaryKeyColumns,
		)
	}
	if db.Migrator().HasTable(migrationStateTable) {
		t.Fatalf("retired MySQL table %s was auto-migrated", migrationStateTable)
	}

	var cascadingForeignKeys int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.referential_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = ?
		  AND delete_rule = 'CASCADE'
	`, userAccessVersionTable).Scan(&cascadingForeignKeys).Error; err != nil {
		t.Fatalf("inspect MySQL user access version foreign keys: %v", err)
	}
	if cascadingForeignKeys != 0 {
		t.Fatalf("user access version table has %d ON DELETE CASCADE foreign keys", cascadingForeignKeys)
	}

	user := model.User{
		Username: fmt.Sprintf("soft-delete-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("soft-delete-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create MySQL user: %v", err)
	}
	version := model.UserAccessVersion{
		UserID:  user.ID,
		Version: 3,
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create MySQL user access version: %v", err)
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatalf("soft delete MySQL user: %v", err)
	}

	var activeUser model.User
	if err := db.First(&activeUser, user.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("soft-deleted MySQL user lookup: got %v, want record not found", err)
	}
	var deletedUser model.User
	if err := db.Unscoped().First(&deletedUser, user.ID).Error; err != nil {
		t.Fatalf("read soft-deleted MySQL user unscoped: %v", err)
	}
	var retained model.UserAccessVersion
	if err := db.First(&retained, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("MySQL access version should remain after user soft delete: %v", err)
	}
	if retained.Version != version.Version {
		t.Fatalf("retained MySQL version: got %d, want %d", retained.Version, version.Version)
	}
}
