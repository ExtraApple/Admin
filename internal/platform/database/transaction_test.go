package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"admin/internal/platform/database"
	"admin/testsupport/testutil"

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

func TestTransactionRunnerAfterCommitWaitsForOutermostCommit(t *testing.T) {
	for _, repeatableRead := range []bool{false, true} {
		name := "Run"
		if repeatableRead {
			name = "RunRepeatableRead"
		}
		t.Run(name, func(t *testing.T) {
			db := testutil.OpenIsolatedSQLite(t)
			if err := db.AutoMigrate(&transactionRecord{}); err != nil {
				t.Fatalf("migrate transaction record: %v", err)
			}
			runner := database.NewTransactionRunner(db)
			run := runner.Run
			if repeatableRead {
				run = runner.RunRepeatableRead
			}
			var observations [][]transactionRecord
			observeAfterCommit := func(ctx context.Context) {
				runner.AfterCommit(ctx, func() {
					observationContext, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()
					var records []transactionRecord
					if err := database.FromContext(observationContext, db).Order("id").Find(&records).Error; err != nil {
						t.Errorf("read committed records through captured context: %v", err)
					}
					observations = append(observations, records)
					var committed []transactionRecord
					if err := db.WithContext(observationContext).Order("id").Find(&committed).Error; err != nil {
						t.Errorf("read committed records independently of transaction context: %v", err)
					}
					if len(records) != len(committed) {
						t.Errorf("captured-context records = %#v, independently committed records = %#v", records, committed)
						return
					}
					for index, record := range records {
						if record != committed[index] {
							t.Errorf("captured-context record = %#v, independently committed record = %#v", record, committed[index])
						}
					}
				})
			}

			err := run(context.Background(), func(ctx context.Context) error {
				if err := database.FromContext(ctx, db).Create(&transactionRecord{Name: "outer-before"}).Error; err != nil {
					return err
				}
				observeAfterCommit(ctx)
				if err := runner.Run(ctx, func(nestedContext context.Context) error {
					if err := database.FromContext(nestedContext, db).Create(&transactionRecord{Name: "nested-run"}).Error; err != nil {
						return err
					}
					observeAfterCommit(nestedContext)
					return nil
				}); err != nil {
					return err
				}
				if err := runner.RunRepeatableRead(ctx, func(nestedContext context.Context) error {
					if err := database.FromContext(nestedContext, db).Create(&transactionRecord{Name: "nested-repeatable-read"}).Error; err != nil {
						return err
					}
					observeAfterCommit(nestedContext)
					return nil
				}); err != nil {
					return err
				}
				if len(observations) != 0 {
					return fmt.Errorf("callbacks observed records before outermost commit: %#v", observations)
				}
				return database.FromContext(ctx, db).Create(&transactionRecord{Name: "outer-after"}).Error
			})
			if err != nil {
				t.Fatalf("commit outer transaction: %v", err)
			}

			if len(observations) != 3 {
				t.Fatalf("committed observations = %#v, want outer and both nested observations", observations)
			}
			wantNames := []string{"outer-before", "nested-run", "nested-repeatable-read", "outer-after"}
			for callback, records := range observations {
				if len(records) != len(wantNames) {
					t.Fatalf("callback %d records = %#v, want all outermost committed records", callback, records)
				}
				for index, wantName := range wantNames {
					if records[index].Name != wantName {
						t.Errorf("callback %d record %d = %#v, want name %q", callback, index, records[index], wantName)
					}
				}
			}
		})
	}
}

func TestTransactionRunnerAfterCommitDiscardsRolledBackEffects(t *testing.T) {
	for _, repeatableRead := range []bool{false, true} {
		name := "Run"
		if repeatableRead {
			name = "RunRepeatableRead"
		}
		t.Run(name, func(t *testing.T) {
			db := testutil.OpenIsolatedSQLite(t)
			if err := db.AutoMigrate(&transactionRecord{}); err != nil {
				t.Fatalf("migrate transaction record: %v", err)
			}
			runner := database.NewTransactionRunner(db)
			run := runner.Run
			if repeatableRead {
				run = runner.RunRepeatableRead
			}
			effectDirectory := t.TempDir()
			registerEffect := func(ctx context.Context, name string) {
				runner.AfterCommit(ctx, func() {
					if err := os.WriteFile(filepath.Join(effectDirectory, name), []byte("transaction committed"), 0600); err != nil {
						t.Errorf("write external commit notification: %v", err)
					}
				})
			}
			rollback := errors.New("rollback outer transaction after nested success")

			err := run(context.Background(), func(ctx context.Context) error {
				if err := database.FromContext(ctx, db).Create(&transactionRecord{Name: "rolled-back-outer"}).Error; err != nil {
					return err
				}
				registerEffect(ctx, "outer")
				if err := runner.Run(ctx, func(nestedContext context.Context) error {
					if err := database.FromContext(nestedContext, db).Create(&transactionRecord{Name: "rolled-back-nested"}).Error; err != nil {
						return err
					}
					registerEffect(nestedContext, "nested")
					return nil
				}); err != nil {
					return err
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatalf("outer transaction error = %v, want %v", err, rollback)
			}
			for _, name := range []string{"outer", "nested"} {
				if _, err := os.Stat(filepath.Join(effectDirectory, name)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("rolled-back %s notification stat error = %v, want no external effect", name, err)
				}
			}
			var records []transactionRecord
			if err := db.Find(&records).Error; err != nil {
				t.Fatalf("list records after rollback: %v", err)
			}
			if len(records) != 0 {
				t.Fatalf("records after rollback = %#v, want no outer or nested writes", records)
			}
		})
	}
}

func TestTransactionRunnerAfterCommitDiscardsRetryAttemptEffects(t *testing.T) {
	for _, errorNumber := range []uint16{1205, 1213} {
		t.Run(fmt.Sprintf("MySQL%d", errorNumber), func(t *testing.T) {
			db := testutil.OpenIsolatedSQLite(t)
			if err := db.AutoMigrate(&transactionRecord{}); err != nil {
				t.Fatalf("migrate transaction record: %v", err)
			}
			runner := database.NewTransactionRunner(db)
			effectDirectory := t.TempDir()
			attempts := 0

			err := runner.Run(context.Background(), func(ctx context.Context) error {
				attempts++
				attempt := attempts
				if err := database.FromContext(ctx, db).Create(&transactionRecord{Name: fmt.Sprintf("attempt-%d", attempt)}).Error; err != nil {
					return err
				}
				runner.AfterCommit(ctx, func() {
					var records []transactionRecord
					if err := database.FromContext(ctx, db).Order("id").Find(&records).Error; err != nil {
						t.Errorf("read committed retry records through captured context: %v", err)
					}
					payload, err := json.Marshal(records)
					if err != nil {
						t.Errorf("encode committed retry observation: %v", err)
						return
					}
					if err := os.WriteFile(filepath.Join(effectDirectory, fmt.Sprintf("attempt-%d.json", attempt)), payload, 0600); err != nil {
						t.Errorf("write external retry observation: %v", err)
					}
				})
				if attempt < 3 {
					return &mysqldriver.MySQLError{Number: errorNumber, Message: "synthetic retryable transaction failure"}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("commit retried transaction: %v", err)
			}
			if attempts != 3 {
				t.Fatalf("transaction attempts = %d, want two failed attempts and one commit", attempts)
			}
			for _, attempt := range []int{1, 2} {
				if _, err := os.Stat(filepath.Join(effectDirectory, fmt.Sprintf("attempt-%d.json", attempt))); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("failed attempt %d observation stat error = %v, want no external effect", attempt, err)
				}
			}
			payload, err := os.ReadFile(filepath.Join(effectDirectory, "attempt-3.json"))
			if err != nil {
				t.Fatalf("read committed external observation: %v", err)
			}
			var observed []transactionRecord
			if err := json.Unmarshal(payload, &observed); err != nil {
				t.Fatalf("decode committed external observation: %v", err)
			}
			if len(observed) != 1 || observed[0].Name != "attempt-3" {
				t.Fatalf("external committed observation = %#v, want only final attempt", observed)
			}
			var committed []transactionRecord
			if err := db.Find(&committed).Error; err != nil {
				t.Fatalf("list committed retry records: %v", err)
			}
			if len(committed) != 1 || committed[0] != observed[0] {
				t.Fatalf("committed retry records = %#v, want externally observed record %#v", committed, observed[0])
			}
		})
	}
}
