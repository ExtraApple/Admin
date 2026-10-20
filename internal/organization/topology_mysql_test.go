//go:build mysql_integration

package organization_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMySQLLockedOrganizationParentsOverridesEarlierRepeatableReadSnapshot(t *testing.T) {
	configuration, err := mysqldriver.ParseDSN(os.Getenv("ADMIN_TEST_MYSQL_DSN"))
	if err != nil || configuration.DBName == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN must identify a disposable test database")
	}
	configuration.DBName = ""
	server, err := gorm.Open(mysql.Open(configuration.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to the MySQL test server")
	}
	serverConnection, err := server.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverConnection.Close() })
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("organization_topology_%x", suffix)
	if err := server.Exec("CREATE DATABASE `" + databaseName + "`").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Exec("DROP DATABASE `" + databaseName + "`").Error; err != nil {
			t.Error(err)
		}
	})
	configuration.DBName = databaseName
	db, err := gorm.Open(mysql.Open(configuration.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to the owned MySQL test schema")
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	connection.SetMaxOpenConns(2)
	if err := db.AutoMigrate(&organization.Unit{}); err != nil {
		t.Fatal(err)
	}
	units := []organization.Unit{
		{Model: gorm.Model{ID: 30}, ParentID: 10, Name: "Child", Code: "child", Status: 1},
		{Model: gorm.Model{ID: 10}, Name: "Root A", Code: "root-a", Status: 1},
		{Model: gorm.Model{ID: 20}, Name: "Root B", Code: "root-b", Status: 1},
	}
	if err := db.Create(&units).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := organization.NewGORMRepository(db)
	before := []organization.OrganizationParent{{ID: 10}, {ID: 20}, {ID: 30, ParentID: 10}}
	after := []organization.OrganizationParent{{ID: 10}, {ID: 20}, {ID: 30, ParentID: 20}}
	err = platformdatabase.NewTransactionRunner(db).RunRepeatableRead(ctx, func(tx context.Context) error {
		parents, err := repository.OrganizationParents(tx)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(parents, before) {
			return fmt.Errorf("initial ordered topology = %v; want %v", parents, before)
		}
		// The ordinary read above establishes the transaction's RR snapshot.
		// This context deliberately uses a separate, autocommitted connection.
		if err := repository.UpdateUnit(ctx, 30, map[string]any{"parent_id": uint(20)}); err != nil {
			return err
		}
		parents, err = repository.OrganizationParents(tx)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(parents, before) {
			return fmt.Errorf("consistent topology = %v; want earlier snapshot %v", parents, before)
		}
		parents, err = repository.LockOrganizationParents(tx)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(parents, after) {
			return fmt.Errorf("locked current topology = %v; want committed topology %v", parents, after)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	parents, err := repository.OrganizationParents(ctx)
	if err != nil || !reflect.DeepEqual(parents, after) {
		t.Fatalf("ordinary topology outside transaction = %v, %v; want %v", parents, err, after)
	}
}
