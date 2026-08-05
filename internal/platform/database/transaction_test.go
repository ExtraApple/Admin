package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/platform/database"

	mysqldriver "github.com/go-sql-driver/mysql"
)

type transactionRecord struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func TestTransactionRunnerCommitsAndRollsBackThroughContext(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&transactionRecord{}); err != nil {
		t.Fatalf("migrate transaction record: %v", err)
	}
	runner := database.NewTransactionRunner(db)

	if err := runner.Run(context.Background(), func(ctx context.Context) error {
		return database.FromContext(ctx, db).Create(&transactionRecord{Name: "committed"}).Error
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	rollback := errors.New("rollback requested")
	err := runner.Run(context.Background(), func(ctx context.Context) error {
		if err := database.FromContext(ctx, db).Create(&transactionRecord{Name: "rolled-back"}).Error; err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v, want %v", err, rollback)
	}

	var records []transactionRecord
	if err := db.Order("id").Find(&records).Error; err != nil {
		t.Fatalf("list transaction records: %v", err)
	}
	if len(records) != 1 || records[0].Name != "committed" {
		t.Fatalf("transaction records = %#v, want only committed record", records)
	}
}

func TestTransactionRunnerRetriesMySQLDeadlocksWithFreshTransactions(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&transactionRecord{}); err != nil {
		t.Fatalf("migrate transaction record: %v", err)
	}
	runner := database.NewTransactionRunner(db)
	attempts := 0

	err := runner.Run(context.Background(), func(ctx context.Context) error {
		attempts++
		if err := database.FromContext(ctx, db).Create(&transactionRecord{Name: fmt.Sprintf("attempt-%d", attempts)}).Error; err != nil {
			return err
		}
		if attempts < 3 {
			return &mysqldriver.MySQLError{Number: 1213, Message: "deadlock found"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run retried transaction: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("transaction attempts = %d, want 3", attempts)
	}

	var records []transactionRecord
	if err := db.Find(&records).Error; err != nil {
		t.Fatalf("list transaction records: %v", err)
	}
	if len(records) != 1 || records[0].Name != "attempt-3" {
		t.Fatalf("transaction records = %#v, want only final attempt", records)
	}
}

func TestTransactionRunnerNestedCapabilityJoinsOuterTransaction(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&transactionRecord{}); err != nil {
		t.Fatalf("migrate transaction record: %v", err)
	}
	runner := database.NewTransactionRunner(db)
	rollback := errors.New("rollback outer transaction")
	nestedAttempts := 0

	err := runner.Run(context.Background(), func(ctx context.Context) error {
		if err := runner.Run(ctx, func(nestedContext context.Context) error {
			nestedAttempts++
			return database.FromContext(nestedContext, db).Create(&transactionRecord{Name: "nested"}).Error
		}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("outer transaction error = %v, want %v", err, rollback)
	}
	if nestedAttempts != 1 {
		t.Fatalf("nested capability attempts = %d, want 1", nestedAttempts)
	}

	var count int64
	if err := db.Model(&transactionRecord{}).Count(&count).Error; err != nil {
		t.Fatalf("count transaction records: %v", err)
	}
	if count != 0 {
		t.Fatalf("nested capability committed independently; record count = %d", count)
	}
}

func TestTransactionRunnerDoesNotRetryCancellationOrNonMySQLErrors(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	runner := database.NewTransactionRunner(db)

	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledCalls := 0
	err := runner.Run(cancelledContext, func(context.Context) error {
		cancelledCalls++
		return nil
	})
	if !errors.Is(err, context.Canceled) || cancelledCalls != 0 {
		t.Fatalf("cancelled run = (%v, %d calls), want context.Canceled and zero calls", err, cancelledCalls)
	}

	nonMySQLError := errors.New("application validation failed")
	applicationCalls := 0
	err = runner.Run(context.Background(), func(context.Context) error {
		applicationCalls++
		return nonMySQLError
	})
	if !errors.Is(err, nonMySQLError) || applicationCalls != 1 {
		t.Fatalf("application run = (%v, %d calls), want original error and one call", err, applicationCalls)
	}
}
