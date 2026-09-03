//go:build mysql_integration

package application_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	platformdatabase "admin/internal/platform/database"

	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type failingSnapshotStore struct {
	*messaginggorm.Repository
	calls int
}

func (store *failingSnapshotStore) PersistAudienceDeliveryBatch(ctx context.Context, batch application.AudienceDeliveryBatch) (bool, error) {
	store.calls++
	if store.calls == 2 {
		return false, errors.New("injected snapshot batch failure")
	}
	return store.Repository.PersistAudienceDeliveryBatch(ctx, batch)
}

func TestMySQLEventConsumerRollsBackEverySnapshotBatchBeforeRefresh(t *testing.T) {
	db := openMessagingMySQL(t)
	repository := messaginggorm.NewRepository(db)
	eventID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := domain.MessageEvent{EventID: eventID, EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: uniqueMessagingCopyID(), OrganizationID: 1, OccurredAt: now, AggregateVersion: 1}
	users := make([]uint, 501)
	identityUsers := make(map[uint]application.IdentityUser, len(users))
	for index := range users {
		users[index] = uint(index + 1)
		identityUsers[users[index]] = application.IdentityUser{ID: users[index], Enabled: true}
	}
	store := &failingSnapshotStore{Repository: repository}
	stream := &refreshStreamFake{}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{
		ConsumerName: "websocket", WorkerID: "mysql-failure-worker", Lease: time.Minute, BatchSize: 500,
		MaxAudienceUsers: 100000, Messages: consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 1, Type: domain.AudienceTypeOrganization}}}, Organizations: consumerOrganizationFake{members: map[uint][]uint{1: users}}, Identity: consumerIdentityFake{users: identityUsers},
		Store: store, Stream: stream, SnapshotTransactions: platformdatabase.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now }),
	})
	t.Cleanup(func() {
		db.Where("event_id = ?", eventID).Delete(&messaginggorm.MessageEventDelivery{})
		db.Where("event_id = ?", eventID).Delete(&messaginggorm.MessageEventConsumption{})
	})

	if _, err := consumer.Process(context.Background(), event); err == nil {
		t.Fatal("Process() accepted injected second-batch failure")
	}
	var deliveryCount int64
	if err := db.Model(&messaginggorm.MessageEventDelivery{}).Where("event_id = ?", eventID).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count rolled-back deliveries: %v", err)
	}
	if deliveryCount != 0 || len(stream.refreshes) != 0 {
		t.Fatalf("failed snapshot left deliveries=%d refreshes=%#v", deliveryCount, stream.refreshes)
	}
}

func TestMySQLEventConsumerCompetingClaimsProduceOneCompleteSnapshot(t *testing.T) {
	db := openMessagingMySQL(t)
	repository := messaginggorm.NewRepository(db)
	eventID := uuid.NewString()
	copyID := uniqueMessagingCopyID()
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := domain.MessageEvent{EventID: eventID, EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: copyID, OrganizationID: 1, OccurredAt: now, AggregateVersion: 1}
	if _, acquired, err := repository.ClaimEventConsumption(context.Background(), application.EventConsumptionClaim{ConsumerName: "websocket", Event: event, WorkerID: "seed-worker", Now: now, Lease: time.Second}); err != nil || !acquired {
		t.Fatalf("seed consumption claim = acquired:%t err:%v", acquired, err)
	}
	users := make([]uint, 501)
	identityUsers := make(map[uint]application.IdentityUser, len(users))
	for index := range users {
		users[index] = uint(index + 1)
		identityUsers[users[index]] = application.IdentityUser{ID: users[index], Enabled: true}
	}
	newConsumer := func(workerID string, stream *refreshStreamFake) *application.EventConsumer {
		return application.NewEventConsumer(application.EventConsumerConfig{
			ConsumerName: "websocket", WorkerID: workerID, Lease: time.Minute, BatchSize: 500, MaxAudienceUsers: 100000,
			Messages:      consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 1, Type: domain.AudienceTypeOrganization}}},
			Organizations: consumerOrganizationFake{members: map[uint][]uint{1: users}}, Identity: consumerIdentityFake{users: identityUsers},
			Store: repository, Stream: stream, SnapshotTransactions: platformdatabase.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now.Add(2 * time.Second) }),
		})
	}
	streamA, streamB := &refreshStreamFake{}, &refreshStreamFake{}
	consumerA, consumerB := newConsumer("worker-a", streamA), newConsumer("worker-b", streamB)
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Add(2)
	go func() { defer wait.Done(); _, err := consumerA.Process(context.Background(), event); results <- err }()
	go func() { defer wait.Done(); _, err := consumerB.Process(context.Background(), event); results <- err }()
	wait.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("competing Process() error: %v", err)
		}
	}
	var deliveryCount int64
	if err := db.Model(&messaginggorm.MessageEventDelivery{}).Where("event_id = ?", eventID).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count competing deliveries: %v", err)
	}
	if deliveryCount != int64(len(users)) {
		t.Fatalf("competing deliveries=%d, want %d", deliveryCount, len(users))
	}
	var completed int64
	if err := db.Model(&messaginggorm.MessageEventConsumption{}).Where("event_id = ? AND status = ?", eventID, domain.EventConsumptionStatusCompleted).Count(&completed).Error; err != nil {
		t.Fatalf("count completed consumption: %v", err)
	}
	if completed != 1 {
		t.Fatalf("completed consumptions=%d, want one", completed)
	}
	if len(streamA.refreshes)+len(streamB.refreshes) != len(users) {
		t.Fatalf("competing refreshes=%d, want %d", len(streamA.refreshes)+len(streamB.refreshes), len(users))
	}
	t.Cleanup(func() {
		db.Where("event_id = ?", eventID).Delete(&messaginggorm.MessageEventDelivery{})
		db.Where("event_id = ?", eventID).Delete(&messaginggorm.MessageEventConsumption{})
	})
}

func openMessagingMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required for the mandatory MySQL gate")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate messaging models: %v", err)
	}
	return db
}

func uniqueMessagingCopyID() uint { return uint(time.Now().UnixNano() % 2000000000) }
