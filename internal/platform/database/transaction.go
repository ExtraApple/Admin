package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"gorm.io/gorm"
)

const maxTransactionRetries = 2

type transactionContextKey struct{}

// TransactionRunner owns the outermost GORM transaction and joins an existing
// transaction carried by Context. A retryable outer operation can run up to
// three times, so it must contain database work only. Callers run cache cleanup
// and other external side effects only after Run returns nil.
type TransactionRunner struct {
	db *gorm.DB
}

func NewTransactionRunner(db *gorm.DB) *TransactionRunner {
	return &TransactionRunner{db: db}
}

// Run commits one outer transaction, or joins the transaction already in ctx.
// Only outer MySQL deadlock and lock-wait failures are retried.
func (runner *TransactionRunner) Run(ctx context.Context, operation func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := transactionFromContext(ctx); ok {
		return operation(ctx)
	}

	var err error
	for attempt := 0; attempt <= maxTransactionRetries; attempt++ {
		err = runner.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return operation(context.WithValue(ctx, transactionContextKey{}, tx))
		})
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == maxTransactionRetries || !isRetryableMySQLError(err) {
			return err
		}

		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

// RunRepeatableRead commits one non-retryable transaction at MySQL's
// REPEATABLE READ isolation and carries its connection through Context.
// The callback must contain database work only; callers perform external side
// effects after this method returns successfully.
func (runner *TransactionRunner) RunRepeatableRead(ctx context.Context, operation func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := transactionFromContext(ctx); ok {
		return operation(ctx)
	}
	tx := runner.db.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return tx.Error
	}
	transactionCtx := context.WithValue(ctx, transactionContextKey{}, tx)
	if err := operation(transactionCtx); err != nil {
		_ = tx.Rollback().Error
		return err
	}
	return tx.Commit().Error
}

func isRetryableMySQLError(err error) bool {
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) {
		return false
	}
	return mysqlError.Number == 1205 || mysqlError.Number == 1213
}

// FromContext returns the current transaction for GORM adapters, falling back
// to the adapter's base connection outside a transaction.
func FromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := transactionFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}

func transactionFromContext(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(*gorm.DB)
	return tx, ok
}
