package testutil_test

import (
	"database/sql"
	"strings"
	"testing"

	"admin/initialize/testutil"
)

type databaseIsolationProbe struct {
	ID uint `gorm:"primaryKey"`
}

func TestOpenIsolatedSQLiteSeparatesEachDatabase(t *testing.T) {
	first := testutil.OpenIsolatedSQLite(t)
	second := testutil.OpenIsolatedSQLite(t)

	if err := first.AutoMigrate(&databaseIsolationProbe{}); err != nil {
		t.Fatalf("migrate first sqlite database: %v", err)
	}
	if second.Migrator().HasTable(&databaseIsolationProbe{}) {
		t.Fatal("separate helper calls should not share sqlite schema")
	}
}

func TestOpenIsolatedSQLiteClosesConnectionAfterTestCleanup(t *testing.T) {
	var connection *sql.DB

	t.Run("database lifecycle", func(t *testing.T) {
		db := testutil.OpenIsolatedSQLite(t)

		var err error
		connection, err = db.DB()
		if err != nil {
			t.Fatalf("get sqlite connection: %v", err)
		}
		if err := connection.Ping(); err != nil {
			t.Fatalf("ping open sqlite connection: %v", err)
		}
	})

	err := connection.Ping()
	if err == nil {
		t.Fatal("sqlite connection should be closed after test cleanup")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed connection error: got %q", err)
	}
}
