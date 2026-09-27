//go:build mysql_integration

package gormadapter_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"admin/internal/files"
	adapter "admin/internal/files/adapters/gorm"
	"admin/internal/files/application"
	"admin/internal/files/domain"
	database "admin/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMySQLCleanupConcurrentRegistrationAndRetry(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "probe", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired, UploaderID: 7}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Where("file_id = ?", file.ID).Delete(&files.MessageImageCleanupJob{})
		db.Unscoped().Delete(&files.File{}, file.ID)
	}()
	type result struct {
		job application.MessageImageCleanupJob
		ok  bool
		err error
	}
	run := func(fn func() result) []result {
		start := make(chan struct{})
		outcomes := make(chan result, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; outcomes <- fn() }()
		}
		close(start)
		wg.Wait()
		close(outcomes)
		var results []result
		for value := range outcomes {
			results = append(results, value)
		}
		return results
	}
	registrations := run(func() result {
		j, ok, e := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
		return result{j, ok, e}
	})
	winners := 0
	for _, v := range registrations {
		if v.err != nil {
			t.Fatal(v.err)
		}
		if v.ok {
			winners++
		}
	}
	if winners != 1 || registrations[0].job.ID != registrations[1].job.ID {
		t.Fatalf("registration competition: %+v", registrations)
	}
	job := registrations[0].job
	retries := run(func() result {
		j, ok, e := r.ReserveMessageImageCleanupRetry(ctx, job, now.Add(time.Hour), now.Add(time.Hour))
		return result{j, ok, e}
	})
	winners = 0
	for _, v := range retries {
		if v.err != nil {
			t.Fatal(v.err)
		}
		if v.ok {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("retry competition: %+v", retries)
	}
	if err := r.BindMessageImages(ctx, application.MessageImageBindRequest{ActorID: 7, MessageLogicalID: uuid.NewString(), ImageIDs: []uint{file.ID}}); !errors.Is(err, application.ErrStateConflict) {
		t.Fatalf("registered image bound: %v", err)
	}
	rollback := errors.New("rollback")
	if err := database.NewTransactionRunner(db).Run(ctx, func(tx context.Context) error {
		if _, err := r.CompleteMessageImageCleanup(tx, job); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if ok, err := r.CompleteMessageImageCleanup(ctx, job); err != nil || !ok {
		t.Fatalf("completion recovery: %v %v", ok, err)
	}
}

func TestMySQLBindingAndCleanupSerializeAtFileRecord(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	for _, bindingFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "binding-first", false: "cleanup-first"}[bindingFirst], func(t *testing.T) {
			r := adapter.NewRepository(db)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			now := time.Now().UTC().Truncate(time.Millisecond)
			deadline := now.Add(time.Hour)
			file := domain.File{Purpose: "message_image", Bucket: "probe", ObjectName: "message-images/" + uuid.NewString() + ".png", UploaderID: 7, BindingExpiresAt: &deadline}
			if err := r.CreateMessageImage(ctx, &file); err != nil {
				t.Fatal(err)
			}
			defer func() {
				db.Where("file_id = ?", file.ID).Delete(&files.MessageImageCleanupJob{})
				db.Unscoped().Delete(&files.File{}, file.ID)
			}()
			bind := func(c context.Context) error {
				return r.BindMessageImages(c, application.MessageImageBindRequest{ActorID: 7, MessageLogicalID: uuid.NewString(), ImageIDs: []uint{file.ID}})
			}
			register := func(c context.Context) error {
				_, _, e := r.RegisterMessageImageCleanup(c, file.ID, deadline, deadline)
				return e
			}
			first, second := bind, register
			if !bindingFirst {
				first, second = register, bind
			}
			held := make(chan struct{})
			release := make(chan struct{})
			firstDone := make(chan error, 1)
			go func() {
				firstDone <- database.NewTransactionRunner(db).Run(ctx, func(tx context.Context) error {
					if e := first(tx); e != nil {
						return e
					}
					close(held)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			select {
			case <-held:
			case e := <-firstDone:
				t.Fatal(e)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			// A bounded contender must wait for the first transaction, not observe old eligibility.
			waiting, stop := context.WithTimeout(ctx, 100*time.Millisecond)
			e := second(waiting)
			stop()
			close(release)
			if e == nil {
				t.Fatal("contender bypassed uncommitted file lock")
			}
			if e := <-firstDone; e != nil {
				t.Fatal(e)
			}
			if e := second(ctx); !errors.Is(e, application.ErrStateConflict) {
				t.Fatalf("losing operation accepted after commit: %v", e)
			}
		})
	}
}
