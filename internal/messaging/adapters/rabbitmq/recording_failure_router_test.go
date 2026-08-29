package rabbitmq_test

import (
	"context"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type deadLetterRecorderFake struct {
	inputs []application.ConsumerDeadLetterInput
}

func (recorder *deadLetterRecorderFake) Record(_ context.Context, input application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error) {
	recorder.inputs = append(recorder.inputs, input)
	return application.ConsumerDeadLetter{ID: 1}, nil
}

type recordingFailurePublisherFake struct {
	retryAttempts []int
	deadEvents    []domain.MessageEvent
	failureCodes  []string
	observed      []int
}

func (publisher *recordingFailurePublisherFake) PublishRetry(_ context.Context, _ domain.MessageEvent, attempt int) error {
	publisher.retryAttempts = append(publisher.retryAttempts, attempt)
	return nil
}
func (publisher *recordingFailurePublisherFake) PublishDeadLetter(_ context.Context, event domain.MessageEvent, failureCode string, observed int) error {
	publisher.deadEvents = append(publisher.deadEvents, event)
	publisher.failureCodes = append(publisher.failureCodes, failureCode)
	publisher.observed = append(publisher.observed, observed)
	return nil
}

func TestRecordingFailureRouterPublishesConsumerDLQWithoutPersistingProjection(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-dlq", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}
	recorder := &deadLetterRecorderFake{}
	publisher := &recordingFailurePublisherFake{}
	router := messagingrabbitmq.NewRecordingFailureRouter(publisher)
	if err := router.PublishDeadLetter(context.Background(), event, "audience_capacity_exceeded", 100001); err != nil {
		t.Fatalf("PublishDeadLetter() = %v", err)
	}
	if len(recorder.inputs) != 0 || len(publisher.deadEvents) != 1 || publisher.deadEvents[0] != event || publisher.failureCodes[0] != "audience_capacity_exceeded" || publisher.observed[0] != 100001 {
		t.Fatalf("recorder=%#v publisher=%#v", recorder, publisher)
	}
}
