//go:build mysql_integration

package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/routecatalog"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestSyncCatalogAPIPersistsPublicNeedAuthZeroOnMySQL(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required for the mysql_integration gate")
	}
	if _, err := mysql.ParseDSN(dsn); err != nil {
		t.Fatalf("parse ADMIN_TEST_MYSQL_DSN: %v", err)
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL connection: %v", err)
	}
	if err := db.AutoMigrate(apigorm.Models()...); err != nil {
		t.Fatalf("migrate API metadata: %v", err)
	}

	suffix := uuid.NewString()
	publicPath := fmt.Sprintf("/api/a3-mysql-public-%s", suffix)
	protectedPath := fmt.Sprintf("/api/a3-mysql-protected-%s", suffix)
	t.Cleanup(func() {
		if err := db.Unscoped().Where("path IN ?", []string{publicPath, protectedPath}).Delete(&apigorm.API{}).Error; err != nil {
			t.Errorf("clean MySQL API metadata: %v", err)
		}
		_ = connection.Close()
	})

	err = syncCatalogAPIs(context.Background(), db, []routecatalog.Descriptor{
		a3SeedDescriptor(http.MethodGet, publicPath, routecatalog.Public, ""),
		a3SeedDescriptor(http.MethodPost, protectedPath, routecatalog.PermissionControlled, "admin.a3.mysql.protected"),
	})
	if err != nil {
		t.Fatalf("sync Catalog APIs on MySQL: %v", err)
	}

	var public, protected apigorm.API
	if err := db.Where("method = ? AND path = ?", http.MethodGet, publicPath).First(&public).Error; err != nil {
		t.Fatalf("read Public MySQL metadata: %v", err)
	}
	if err := db.Where("method = ? AND path = ?", http.MethodPost, protectedPath).First(&protected).Error; err != nil {
		t.Fatalf("read protected MySQL metadata: %v", err)
	}
	if public.NeedAuth != 0 || public.PermissionCode != "" {
		t.Fatalf("Public MySQL metadata = need_auth:%d permission_code:%q, want 0 and empty", public.NeedAuth, public.PermissionCode)
	}
	if protected.NeedAuth != 1 || protected.PermissionCode != "admin.a3.mysql.protected" {
		t.Fatalf("protected MySQL metadata = need_auth:%d permission_code:%q", protected.NeedAuth, protected.PermissionCode)
	}
}
