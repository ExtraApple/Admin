//go:build mysql_integration

package gormadapter_test

import (
	"context"
	"os"
	"testing"
	"time"

	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestMySQLClaimOutboxSkipsLockedEarlierRow(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required for the mandatory MySQL gate")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&messaginggorm.MessageOutbox{}); err != nil {
		t.Fatalf("migrate outbox: %v", err)
	}
	now := time.Now().UTC()
	lockedID := "skipa-" + now.Format("20060102150405.000000000")
	availableID := "skipb-" + now.Format("20060102150405.000000000")
	records := []messaginggorm.MessageOutbox{
		{EventID: lockedID, EventName: domain.EventNameMessageCreated, EventVersion: 1, MessageCopyID: 910001, OrganizationID: 1, AggregateVersion: 1, OccurredAt: now, Status: domain.OutboxStatusPending},
		{EventID: availableID, EventName: domain.EventNameMessageCreated, EventVersion: 1, MessageCopyID: 910002, OrganizationID: 1, AggregateVersion: 1, OccurredAt: now, Status: domain.OutboxStatusPending},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("create outboxes: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Where("event_id IN ?", []string{lockedID, availableID}).Delete(&messaginggorm.MessageOutbox{}).Error; err != nil {
			t.Errorf("cleanup outboxes: %v", err)
		}
	})

	locked := make(chan struct{})
	release := make(chan struct{})
	lockErr := make(chan error, 1)
	go func() {
		lockErr <- db.Transaction(func(tx *gorm.DB) error {
			var record messaginggorm.MessageOutbox
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ?", lockedID).First(&record).Error; err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-lockErr:
		t.Fatalf("lock earlier outbox: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out acquiring earlier outbox lock")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	claimed, err := messaginggorm.NewRepository(db).ClaimOutbox(ctx, application.OutboxClaim{WorkerID: "mysql-worker", Now: now.Add(time.Second), Lease: time.Minute, Limit: 1})
	close(release)
	if lockErr := <-lockErr; lockErr != nil {
		t.Fatalf("release earlier outbox lock: %v", lockErr)
	}
	if err != nil {
		t.Fatalf("ClaimOutbox() while earlier row locked: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Event.EventID != availableID {
		t.Fatalf("ClaimOutbox() = %#v, want available %q", claimed, availableID)
	}
}
