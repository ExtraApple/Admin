package testutil

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// OpenIsolatedSQLite opens one temporary file-backed SQLite database per call.
// The connection is closed before testing.TempDir removes the database on Windows.
func OpenIsolatedSQLite(t testing.TB) *gorm.DB {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "test.sqlite")
	db, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated sqlite database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get isolated sqlite connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close isolated sqlite database: %v", err)
		}
	})

	return db
}
