package application

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"admin/internal/messaging/domain"
)

type ConsumerDeadLetterRecorderConfig struct {
	Store   ConsumerDeadLetterStore
	Metrics MessagingMetrics
	Clock   Clock
	Logger  RuntimeLogger
}

type ConsumerDeadLetterRecorder struct {
	store   ConsumerDeadLetterStore
	metrics MessagingMetrics
	clock   Clock
	logger  RuntimeLogger
}

func NewConsumerDeadLetterRecorder(config ConsumerDeadLetterRecorderConfig) *ConsumerDeadLetterRecorder {
	if config.Clock == nil {
		config.Clock = ClockFunc(time.Now)
	}
	if config.Logger == nil {
		config.Logger = discardRuntimeLogger{}
	}
	return &ConsumerDeadLetterRecorder{store: config.Store, metrics: config.Metrics, clock: config.Clock, logger: config.Logger}
}

func (recorder *ConsumerDeadLetterRecorder) Record(ctx context.Context, input ConsumerDeadLetterInput) (ConsumerDeadLetter, error) {
	if recorder == nil || recorder.store == nil || input.ConsumerName == "" || input.FailureCode == "" {
		return ConsumerDeadLetter{}, ErrMessagingDependency
	}
	if input.Invalid {
		if !validEventFingerprint(input.Fingerprint) {
			return ConsumerDeadLetter{}, ErrConsumerDeadLetterInvalid
		}
	} else if input.Event.EventID == "" {
		return ConsumerDeadLetter{}, ErrMessagingDependency
	}
	var before []ConsumerDeadLetter
	if recorder.metrics != nil {
		before, _ = recorder.pendingForFailure(ctx, input.ConsumerName, input.FailureCode)
	}
	recorded, err := recorder.store.RecordConsumerDeadLetter(ctx, input)
	if err != nil {
		recorder.logger.Warn("messaging_consumer_dlq_record_failed", runtimeField("stage", "record"), runtimeField("consumer", input.ConsumerName), runtimeField("event_id", input.Event.EventID), runtimeField("failure_code", input.FailureCode))
		return recorded, err
	}
	recorder.logger.Info("messaging_consumer_dlq_recorded", runtimeField("stage", "record"), runtimeField("consumer", input.ConsumerName), runtimeField("event_id", input.Event.EventID), runtimeField("failure_code", input.FailureCode), runtimeField("retry_attempt", input.RetryAttempt))
	if recorder.metrics == nil || len(before) != 0 {
		return recorded, nil
	}
	after, lookupErr := recorder.pendingForFailure(ctx, input.ConsumerName, input.FailureCode)
	if lookupErr != nil || len(after) == 0 {
		return recorded, nil
	}
	now := input.Now
	if now.IsZero() {
		now = recorder.clock.Now()
	}
	oldest := now.UTC()
	for _, item := range after {
		if item.Event.OccurredAt.Before(oldest) {
			oldest = item.Event.OccurredAt
		}
	}
	recorder.recordPendingObservation(ctx, ConsumerDLQPendingObservation{ConsumerName: input.ConsumerName, FailureCode: input.FailureCode, PendingCount: int64(len(after)), OldestPendingAge: now.UTC().Sub(oldest)})
	return recorded, nil
}

func (recorder *ConsumerDeadLetterRecorder) recordPendingObservation(ctx context.Context, observation ConsumerDLQPendingObservation) {
	defer func() {
		if recover() != nil {
			recorder.logger.Warn("messaging_consumer_dlq_metrics_failed", runtimeField("stage", "metrics"), runtimeField("consumer", observation.ConsumerName), runtimeField("failure_code", observation.FailureCode))
		}
	}()
	recorder.metrics.RecordConsumerDLQPending(ctx, observation)
	recorder.logger.Info("messaging_consumer_dlq_metrics_recorded", runtimeField("stage", "metrics"), runtimeField("consumer", observation.ConsumerName), runtimeField("failure_code", observation.FailureCode), runtimeField("pending_count", observation.PendingCount))
}

func (recorder *ConsumerDeadLetterRecorder) pendingForFailure(ctx context.Context, consumerName, failureCode string) ([]ConsumerDeadLetter, error) {
	items, _, err := recorder.store.ListConsumerDeadLetters(ctx, ConsumerDeadLetterListQuery{ConsumerName: consumerName, Statuses: []domain.ConsumerDLQStatus{domain.ConsumerDLQStatusPending}, Offset: 0, Limit: 0})
	if err != nil {
		return nil, err
	}
	filtered := make([]ConsumerDeadLetter, 0, len(items))
	for _, item := range items {
		if item.LastFailureCode == failureCode {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func validEventFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(strings.ToLower(value))
	return err == nil
}
