package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type outboxStoreFake struct {
	claimed    []domain.MessageOutbox
	claim      application.OutboxClaim
	marked     []application.OutboxLease
	failures   []application.OutboxFailure
	claimError error
}

func (store *outboxStoreFake) ClaimOutbox(_ context.Context, claim application.OutboxClaim) ([]domain.MessageOutbox, error) {
	store.claim = claim
	return store.claimed, store.claimError
}

func (store *outboxStoreFake) RenewOutboxLease(context.Context, application.OutboxLease) (bool, error) {
	return true, nil
}

func (store *outboxStoreFake) MarkOutboxPublished(_ context.Context, lease application.OutboxLease) (bool, error) {
	store.marked = append(store.marked, lease)
	return true, nil
}

func (store *outboxStoreFake) RecordOutboxFailure(_ context.Context, failure application.OutboxFailure) (bool, error) {
	store.failures = append(store.failures, failure)
	return true, nil
}

func (store *outboxStoreFake) ReplayOutbox(context.Context, uint) (bool, error) {
	panic("unused")
}

func (store *outboxStoreFake) ListOutboxes(context.Context, application.OutboxListQuery) ([]domain.MessageOutbox, int64, error) {
	panic("unused")
}

type outboxPublisherFake struct {
	events []domain.MessageEvent
	err    error
}

func (publisher *outboxPublisherFake) Publish(_ context.Context, event domain.MessageEvent) error {
	publisher.events = append(publisher.events, event)
	return publisher.err
}

func TestOutboxWorkerPublishesClaimedEventWithConfirmDeadline(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-1", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 9, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	store := &outboxStoreFake{claimed: []domain.MessageOutbox{{ID: 8, Event: event, Status: domain.OutboxStatusPublishing, WorkerID: "worker-a"}}}
	publisher := &outboxPublisherFake{}
	worker := application.NewOutboxWorker(application.OutboxWorkerConfig{Store: store, Publisher: publisher, WorkerID: "worker-a", Lease: 30 * time.Second, ConfirmTimeout: 10 * time.Second, RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, Clock: application.ClockFunc(func() time.Time { return now })})

	published, err := worker.RunOnce(context.Background(), 10)
	if err != nil || published != 1 || len(publisher.events) != 1 || publisher.events[0] != event || len(store.marked) != 1 || store.marked[0].ID != 8 || store.marked[0].WorkerID != "worker-a" || store.claim.Lease != 30*time.Second {
		t.Fatalf("RunOnce() published=%d err=%v events=%#v marked=%#v claim=%#v", published, err, publisher.events, store.marked, store.claim)
	}
}

func TestOutboxWorkerRecordsFixedBackoffAndFifthFailureDead(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	retryDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	for name, retryAttempt := range map[string]int{"first failure": 0, "fifth failure": 4} {
		t.Run(name, func(t *testing.T) {
			store := &outboxStoreFake{claimed: []domain.MessageOutbox{{ID: 8, Event: domain.MessageEvent{EventID: "event-1", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 9, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}, Status: domain.OutboxStatusPublishing, RetryAttempt: retryAttempt, WorkerID: "worker-a"}}}
			worker := application.NewOutboxWorker(application.OutboxWorkerConfig{Store: store, Publisher: &outboxPublisherFake{err: errors.New("broker unavailable")}, WorkerID: "worker-a", Lease: 30 * time.Second, ConfirmTimeout: 10 * time.Second, RetryDelays: retryDelays, Clock: application.ClockFunc(func() time.Time { return now })})
			published, err := worker.RunOnce(context.Background(), 1)
			if err != nil || published != 0 || len(store.failures) != 1 {
				t.Fatalf("RunOnce() published=%d err=%v failures=%#v", published, err, store.failures)
			}
			failure := store.failures[0]
			if failure.FailureCode != application.OutboxFailureCodePublishFailed || failure.Dead != (retryAttempt == 4) {
				t.Fatalf("failure = %#v", failure)
			}
			if retryAttempt == 0 && (failure.RetryAt == nil || !failure.RetryAt.Equal(now.Add(time.Second))) {
				t.Fatalf("first retry = %#v", failure.RetryAt)
			}
			if retryAttempt == 4 && failure.RetryAt != nil {
				t.Fatalf("fifth failure retry = %#v, want nil", failure.RetryAt)
			}
		})
	}
}

func TestOutboxWorkerClassifiesConfirmTimeoutSeparately(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &outboxStoreFake{claimed: []domain.MessageOutbox{{ID: 8, Event: domain.MessageEvent{EventID: "event-timeout", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 9, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}, Status: domain.OutboxStatusPublishing, WorkerID: "worker-a"}}}
	worker := application.NewOutboxWorker(application.OutboxWorkerConfig{Store: store, Publisher: &outboxPublisherFake{err: context.DeadlineExceeded}, WorkerID: "worker-a", Lease: 30 * time.Second, ConfirmTimeout: 10 * time.Second, RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, Clock: application.ClockFunc(func() time.Time { return now })})
	if _, err := worker.RunOnce(context.Background(), 1); err != nil {
		t.Fatalf("RunOnce() = %v", err)
	}
	if len(store.failures) != 1 || store.failures[0].FailureCode != application.OutboxFailureCodeConfirmTimeout {
		t.Fatalf("failures = %#v", store.failures)
	}
}

type recoveringOutboxStoreFake struct {
	outboxStoreFake
	queued bool
}

func (store *recoveringOutboxStoreFake) ClaimOutbox(_ context.Context, claim application.OutboxClaim) ([]domain.MessageOutbox, error) {
	store.claim = claim
	if !store.queued {
		return nil, nil
	}
	return append([]domain.MessageOutbox(nil), store.claimed...), nil
}

func (store *recoveringOutboxStoreFake) RecordOutboxFailure(_ context.Context, failure application.OutboxFailure) (bool, error) {
	store.failures = append(store.failures, failure)
	if len(store.claimed) > 0 {
		store.claimed[0].Status = domain.OutboxStatusPending
		store.claimed[0].RetryAttempt++
	}
	return true, nil
}

func (store *recoveringOutboxStoreFake) MarkOutboxPublished(_ context.Context, lease application.OutboxLease) (bool, error) {
	store.marked = append(store.marked, lease)
	if len(store.claimed) > 0 {
		store.claimed[0].Status = domain.OutboxStatusPublished
	}
	return true, nil
}

type recoveringOutboxPublisherFake struct {
	events []domain.MessageEvent
	errors []error
}

func (publisher *recoveringOutboxPublisherFake) Publish(_ context.Context, event domain.MessageEvent) error {
	publisher.events = append(publisher.events, event)
	if len(publisher.errors) == 0 {
		return nil
	}
	err := publisher.errors[0]
	publisher.errors = publisher.errors[1:]
	return err
}

func TestOutboxWorkerRetainsEventWhenRabbitMQIsUnavailableAndPublishesAfterRecovery(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-recovery", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 9, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	store := &recoveringOutboxStoreFake{outboxStoreFake: outboxStoreFake{claimed: []domain.MessageOutbox{{ID: 8, Event: event, Status: domain.OutboxStatusPending}}}, queued: true}
	publisher := &recoveringOutboxPublisherFake{errors: []error{errors.New("RabbitMQ unavailable")}}
	config := application.OutboxWorkerConfig{Store: store, Publisher: publisher, WorkerID: "worker-a", Lease: 30 * time.Second, ConfirmTimeout: 10 * time.Second, RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, Clock: application.ClockFunc(func() time.Time { return now })}
	worker := application.NewOutboxWorker(config)
	if published, err := worker.RunOnce(context.Background(), 1); err != nil || published != 0 || len(store.failures) != 1 || store.claimed[0].Status != domain.OutboxStatusPending || len(publisher.events) != 1 {
		t.Fatalf("failed RunOnce() published=%d err=%v failures=%#v outbox=%#v events=%#v", published, err, store.failures, store.claimed, publisher.events)
	}
	if published, err := worker.RunOnce(context.Background(), 1); err != nil || published != 1 || len(store.marked) != 1 || store.claimed[0].Status != domain.OutboxStatusPublished || len(publisher.events) != 2 || publisher.events[1] != event {
		t.Fatalf("recovered RunOnce() published=%d err=%v marked=%#v outbox=%#v events=%#v", published, err, store.marked, store.claimed, publisher.events)
	}
}
