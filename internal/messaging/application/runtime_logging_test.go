package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type runtimeLogRecord struct {
	level   string
	message string
	fields  []application.RuntimeLogField
}

type runtimeLoggerFake struct {
	records []runtimeLogRecord
}

func (logger *runtimeLoggerFake) Info(message string, fields ...application.RuntimeLogField) {
	logger.records = append(logger.records, runtimeLogRecord{level: "info", message: message, fields: fields})
}
func (logger *runtimeLoggerFake) Warn(message string, fields ...application.RuntimeLogField) {
	logger.records = append(logger.records, runtimeLogRecord{level: "warn", message: message, fields: fields})
}

type loggingOutboxStoreFake struct {
	outbox domain.MessageOutbox
}

func (store *loggingOutboxStoreFake) ClaimOutbox(context.Context, application.OutboxClaim) ([]domain.MessageOutbox, error) {
	return []domain.MessageOutbox{store.outbox}, nil
}
func (*loggingOutboxStoreFake) RenewOutboxLease(context.Context, application.OutboxLease) (bool, error) {
	return true, nil
}
func (*loggingOutboxStoreFake) MarkOutboxPublished(context.Context, application.OutboxLease) (bool, error) {
	return false, errors.New("database password=secret")
}
func (*loggingOutboxStoreFake) RecordOutboxFailure(context.Context, application.OutboxFailure) (bool, error) {
	return true, nil
}
func (*loggingOutboxStoreFake) ReplayOutbox(context.Context, uint) (bool, error) { return true, nil }
func (*loggingOutboxStoreFake) ListOutboxes(context.Context, application.OutboxListQuery) ([]domain.MessageOutbox, int64, error) {
	return nil, 0, nil
}

type loggingPublisherFake struct{}

func (loggingPublisherFake) Publish(context.Context, domain.MessageEvent) error { return nil }

func TestOutboxWorkerLogsStablePublishLifecycleWithoutRawErrors(t *testing.T) {
	logger := &runtimeLoggerFake{}
	store := &loggingOutboxStoreFake{outbox: domain.MessageOutbox{ID: 4, Event: domain.MessageEvent{EventID: "event-4", EventName: domain.EventNameMessagePublished, MessageCopyID: 41, OrganizationID: 10, EventVersion: 1, AggregateVersion: 1, OccurredAt: time.Now().UTC()}}}
	worker := application.NewOutboxWorker(application.OutboxWorkerConfig{Store: store, Publisher: loggingPublisherFake{}, WorkerID: "worker-a", RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, Logger: logger})
	if _, err := worker.RunOnce(context.Background(), 1); err == nil {
		t.Fatal("RunOnce() unexpectedly succeeded")
	}
	if len(logger.records) == 0 {
		t.Fatal("outbox worker did not emit runtime log")
	}
	for _, record := range logger.records {
		if record.message == "database password=secret" {
			t.Fatalf("runtime logger received raw error: %#v", record)
		}
	}
}

func TestDeadLetterRecorderLogsMetricsAdapterFailureWithoutBlocking(t *testing.T) {
	logger := &runtimeLoggerFake{}
	recorder := application.NewConsumerDeadLetterRecorder(application.ConsumerDeadLetterRecorderConfig{Store: &deadLetterStoreFake{}, Metrics: panickingDLQMetrics{}, Logger: logger})
	if _, err := recorder.Record(context.Background(), application.ConsumerDeadLetterInput{ConsumerName: "websocket", Event: domain.MessageEvent{EventID: "event-dlq-log", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, AggregateVersion: 1, OccurredAt: time.Now().UTC()}, RetryAttempt: 5, FailureCode: "consumer_processing_failed", Now: time.Now().UTC()}); err != nil {
		t.Fatalf("Record() = %v", err)
	}
	if len(logger.records) == 0 {
		t.Fatal("metrics adapter failure was not logged")
	}
}
func TestEventConsumerLogsControlledLeaseFailure(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	logger := &runtimeLoggerFake{}
	store := &consumerStoreFake{claim: application.EventConsumption{ConsumerName: "websocket", EventID: "event-fence-log", MessageCopyID: 41, SnapshotFence: 2, Status: "snapshotting"}, acquired: true, failRenewAt: 1}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{ConsumerName: "websocket", WorkerID: "worker-a", Messages: consumerMessageStoreFake{}, Organizations: consumerOrganizationFake{}, Identity: consumerIdentityFake{}, Store: store, Stream: &refreshStreamFake{}, Logger: logger, Clock: application.ClockFunc(func() time.Time { return now })})
	_, _ = consumer.Process(context.Background(), domain.MessageEvent{EventID: "event-fence-log", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, AggregateVersion: 1, OccurredAt: now})
	if len(logger.records) == 0 {
		t.Fatal("event consumer did not emit runtime log")
	}
}

func TestConsumerDeadLetterReplayLogsControlledFailure(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	logger := &runtimeLoggerFake{}
	store := &consumerDeadLetterReplayStoreFake{claimed: application.ConsumerDeadLetter{ID: 8, ConsumerName: "websocket", Event: domain.MessageEvent{EventID: "event-replay-log", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, AggregateVersion: 1, OccurredAt: now}, ReplayLeaseFence: 4}, acquired: true}
	service := application.NewConsumerDeadLetterReplayService(application.ConsumerDeadLetterReplayConfig{Store: store, Publisher: &consumerDeadLetterReplayPublisherFake{err: errors.New("broker password=secret")}, WorkerID: "worker-a", Logger: logger, Clock: application.ClockFunc(func() time.Time { return now })})
	_, _, _ = service.Replay(context.Background(), 8)
	if len(logger.records) == 0 {
		t.Fatal("dead-letter replay did not emit runtime log")
	}
	for _, record := range logger.records {
		if record.message == "broker password=secret" {
			t.Fatalf("runtime logger received raw error: %#v", record)
		}
	}
}
