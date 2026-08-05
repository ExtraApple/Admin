//go:build mysql_integration

package testutil_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/identity"
	platformdatabase "admin/internal/platform/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type mysqlTransactionGateRow struct {
	ID    uint64 `gorm:"primaryKey"`
	Value int
}

func TestMySQLAppMigrationUsesOnlyCurrentAuthorizationSchema(t *testing.T) {
	db := openMySQLBehaviorGate(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("run App migration against MySQL: %v", err)
	}
	if !db.Migrator().HasTable(&authgorm.UserAccessVersion{}) {
		t.Fatal("App migration did not create user_access_versions")
	}
	if !db.Migrator().HasColumn(&authgorm.UserAccessVersion{}, "version") {
		t.Fatal("App migration did not create user_access_versions.version")
	}
	if db.Migrator().HasColumn(&identity.User{}, "token_version") {
		t.Fatal("App migration restored retired users.token_version")
	}
	if db.Migrator().HasTable("access_version_migration_states") {
		t.Fatal("App migration restored retired access_version_migration_states")
	}
}

func TestMySQLConcurrentAccessVersionTransactionsSerializeIncrements(t *testing.T) {
	db := openMySQLBehaviorGate(t)
	if err := db.AutoMigrate(&authgorm.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate access version model: %v", err)
	}
	userID := uint(time.Now().UnixNano())
	if err := db.Where("user_id = ?", userID).Delete(&authgorm.UserAccessVersion{}).Error; err != nil {
		t.Fatalf("clear access version fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Where("user_id = ?", userID).Delete(&authgorm.UserAccessVersion{}).Error; err != nil {
			t.Errorf("delete access version fixture: %v", err)
		}
	})

	versions := authgorm.NewAccessVersions(db)
	runner := platformdatabase.NewTransactionRunner(db)
	const workers = 8
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsByWorker <- runner.Run(context.Background(), func(ctx context.Context) error {
				_, err := versions.EnsureAndIncrement(ctx, userID)
				return err
			})
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent access version increment: %v", err)
		}
	}
	current, err := versions.Current(context.Background(), userID)
	if err != nil {
		t.Fatalf("read concurrent access version: %v", err)
	}
	if current != workers+1 {
		t.Fatalf("access version = %d, want %d after %d serialized increments", current, workers+1, workers)
	}
}

func TestMySQLTransactionRunnerRetriesDeadlockAndRollsBackAttempt(t *testing.T) {
	db := openMySQLBehaviorGate(t)
	if err := db.AutoMigrate(&mysqlTransactionGateRow{}); err != nil {
		t.Fatalf("migrate transaction gate row: %v", err)
	}
	rowID := uint64(time.Now().UnixNano())
	t.Cleanup(func() {
		if err := db.Delete(&mysqlTransactionGateRow{}, "id = ?", rowID).Error; err != nil {
			t.Errorf("delete transaction gate row: %v", err)
		}
	})

	var attempts atomic.Int32
	runner := platformdatabase.NewTransactionRunner(db)
	err := runner.Run(context.Background(), func(ctx context.Context) error {
		attempt := attempts.Add(1)
		if err := platformdatabase.FromContext(ctx, db).Create(&mysqlTransactionGateRow{ID: rowID, Value: int(attempt)}).Error; err != nil {
			return err
		}
		if attempt < 3 {
			return &mysqldriver.MySQLError{Number: 1213, Message: "integration deadlock retry"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run retrying MySQL transaction: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("transaction attempts = %d, want 3", attempts.Load())
	}
	var persisted mysqlTransactionGateRow
	if err := db.First(&persisted, "id = ?", rowID).Error; err != nil {
		t.Fatalf("read committed transaction gate row: %v", err)
	}
	if persisted.Value != 3 {
		t.Fatalf("persisted attempt value = %d, want only committed attempt 3", persisted.Value)
	}
}

func openMySQLBehaviorGate(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required for the mandatory MySQL gate")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL behavior gate database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("access MySQL behavior gate connection: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close MySQL behavior gate database: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping MySQL behavior gate database: %v", err)
	}
	if sqlDB.Stats().MaxOpenConnections == 1 {
		t.Fatal("MySQL behavior gate requires multiple concurrent connections")
	}
	return db
}
