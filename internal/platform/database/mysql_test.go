package database_test

import (
	"strings"
	"testing"

	platformconfig "admin/internal/platform/config"
	"admin/internal/platform/database"
)

func TestOpenMySQLReturnsConnectionFailure(t *testing.T) {
	_, err := database.OpenMySQL(platformconfig.MySQLConfig{
		Host:     "127.0.0.1",
		Port:     1,
		User:     "admin",
		Password: "secret",
		DB:       "admin",
	})
	if err == nil {
		t.Fatal("open MySQL should fail when the configured endpoint refuses connections")
	}
	if !strings.Contains(err.Error(), "open mysql") {
		t.Fatalf("open error = %v, want stable open mysql context", err)
	}
}
