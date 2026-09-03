package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type deadLetterStoreFake struct {
	items   []application.ConsumerDeadLetter
	listErr error
}

func (store *deadLetterStoreFake) RecordConsumerDeadLetter(_ context.Context, input application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error) {
	for index := range store.items {
		if store.items[index].ConsumerName == input.ConsumerName && store.items[index].Event.EventID == input.Event.EventID && store.items[index].Fingerprint == input.Fingerprint {
			store.items[index].LastFailureCode = input.FailureCode
			store.items[index].AudienceObservedCount = input.AudienceObservedCount
			return store.items[index], nil
		}
	}
	item := application.ConsumerDeadLetter{ID: uint(len(store.items) + 1), ConsumerName: input.ConsumerName, Event: input.Event, OriginalQueue: input.OriginalQueue, RetryAttempt: input.RetryAttempt, Status: domain.ConsumerDLQStatusPending, LastFailureCode: input.FailureCode, AudienceObservedCount: input.AudienceObservedCount, Invalid: input.Invalid, Fingerprint: input.Fingerprint, Replayable: !input.Invalid}
	store.items = append(store.items, item)
	return item, nil
}
func (store *deadLetterStoreFake) ListConsumerDeadLetters(_ context.Context, query application.ConsumerDeadLetterListQuery) ([]application.ConsumerDeadLetter, int64, error) {
	if store.listErr != nil {
		return nil, 0, store.listErr
	}
	result := make([]application.ConsumerDeadLetter, 0)
	for _, item := range store.items {
		if len(query.Statuses) > 0 && item.Status != query.Statuses[0] {
			continue
		}
		if query.ConsumerName != "" && item.ConsumerName != query.ConsumerName {
			continue
		}
		result = append(result, item)
	}
	return result, int64(len(result)), nil
}
func (store *deadLetterStoreFake) ClaimConsumerDeadLetterReplay(context.Context, application.ConsumerDeadLetterReplayClaim) (application.ConsumerDeadLetter, bool, error) {
	panic("unused")
}
func (store *deadLetterStoreFake) MarkConsumerDeadLetterReplayed(context.Context, application.ConsumerDeadLetterReplayResult) (bool, error) {
	panic("unused")
}
func (store *deadLetterStoreFake) ReturnConsumerDeadLetterPending(context.Context, application.ConsumerDeadLetterReplayResult) (bool, error) {
	panic("unused")
}
func (store *deadLetterStoreFake) DiscardConsumerDeadLetter(context.Context, uint, time.Time) (bool, error) {
	panic("unused")
}
func (store *deadLetterStoreFake) CleanupFinalConsumerDeadLetters(context.Context, time.Time, int) (int64, error) {
	panic("unused")
}

type dlqMetricsFake struct {
	observations []application.ConsumerDLQPendingObservation
}

func (metrics *dlqMetricsFake) RecordConsumerDLQPending(_ context.Context, observation application.ConsumerDLQPendingObservation) {
	metrics.observations = append(metrics.observations, observation)
}

type panickingDLQMetrics struct{}

func (panickingDLQMetrics) RecordConsumerDLQPending(context.Context, application.ConsumerDLQPendingObservation) {
	panic("metrics adapter failed")
}

func TestConsumerDeadLetterRecorderPersistsBeforeBestEffortFirstPendingObservation(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &deadLetterStoreFake{}
	metrics := &dlqMetricsFake{}
	recorder := application.NewConsumerDeadLetterRecorder(application.ConsumerDeadLetterRecorderConfig{Store: store, Metrics: metrics, Clock: application.ClockFunc(func() time.Time { return now })})
	event := domain.MessageEvent{EventID: "event-dlq", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now.Add(-5 * time.Minute), AggregateVersion: 1}
	if _, err := recorder.Record(context.Background(), application.ConsumerDeadLetterInput{ConsumerName: "websocket", Event: event, OriginalQueue: "admin.messaging.websocket.dlq", RetryAttempt: 5, FailureCode: "MSG_PROCESS_FAILED", Now: now, AudienceObservedCount: 12}); err != nil {
		t.Fatalf("Record() = %v", err)
	}
	if len(store.items) != 1 || len(metrics.observations) != 1 || metrics.observations[0].ConsumerName != "websocket" || metrics.observations[0].FailureCode != "MSG_PROCESS_FAILED" || metrics.observations[0].PendingCount != 1 || metrics.observations[0].OldestPendingAge != 5*time.Minute {
		t.Fatalf("store=%#v observations=%#v", store.items, metrics.observations)
	}
	if _, err := recorder.Record(context.Background(), application.ConsumerDeadLetterInput{ConsumerName: "websocket", Event: event, OriginalQueue: "admin.messaging.websocket.dlq", RetryAttempt: 5, FailureCode: "MSG_PROCESS_FAILED", Now: now, AudienceObservedCount: 12}); err != nil {
		t.Fatalf("duplicate Record() = %v", err)
	}
	if len(metrics.observations) != 1 {
		t.Fatalf("duplicate record emitted observation: %#v", metrics.observations)
	}
}

func TestConsumerDeadLetterRecorderPersistsWhenObservationFails(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-best-effort", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	for _, test := range []struct {
		name    string
		store   *deadLetterStoreFake
		metrics application.MessagingMetrics
	}{
		{name: "pending lookup unavailable", store: &deadLetterStoreFake{listErr: context.DeadlineExceeded}, metrics: &dlqMetricsFake{}},
		{name: "metrics panics", store: &deadLetterStoreFake{}, metrics: panickingDLQMetrics{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := application.NewConsumerDeadLetterRecorder(application.ConsumerDeadLetterRecorderConfig{Store: test.store, Metrics: test.metrics, Clock: application.ClockFunc(func() time.Time { return now })})
			if _, err := recorder.Record(context.Background(), application.ConsumerDeadLetterInput{ConsumerName: "websocket", Event: event, OriginalQueue: "admin.messaging.websocket", RetryAttempt: 5, FailureCode: "consumer_processing_failed", Now: now}); err != nil {
				t.Fatalf("Record() = %v", err)
			}
			if len(test.store.items) != 1 {
				t.Fatalf("recorded items = %#v", test.store.items)
			}
		})
	}
}

func TestConsumerDeadLetterRecorderAcceptsInvalidEventAsNonReplayableSafeProjection(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &deadLetterStoreFake{}
	recorder := application.NewConsumerDeadLetterRecorder(application.ConsumerDeadLetterRecorderConfig{Store: store, Clock: application.ClockFunc(func() time.Time { return now })})
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	item, err := recorder.Record(context.Background(), application.ConsumerDeadLetterInput{ConsumerName: "websocket", OriginalQueue: "admin.messaging.websocket", RetryAttempt: 5, FailureCode: "message_event_invalid", Invalid: true, Fingerprint: fingerprint, Now: now})
	if err != nil || !item.Invalid || item.Fingerprint != fingerprint || item.Replayable {
		t.Fatalf("invalid dead letter = %#v, err=%v", item, err)
	}
}
