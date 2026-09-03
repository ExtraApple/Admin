package rabbitmq

import (
	"context"
	"fmt"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type ConsumerDeadLetterRecorder interface {
	Record(context.Context, application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error)
}

type RecordingFailureRouter struct {
	publisher FailureRouter
}

func NewRecordingFailureRouter(publisher FailureRouter) *RecordingFailureRouter {
	return &RecordingFailureRouter{publisher: publisher}
}

func (router *RecordingFailureRouter) PublishRetry(ctx context.Context, event domain.MessageEvent, attempt int) error {
	if router == nil || router.publisher == nil {
		return fmt.Errorf("Consumer failure publisher is unavailable")
	}
	return router.publisher.PublishRetry(ctx, event, attempt)
}

func (router *RecordingFailureRouter) PublishDeadLetter(ctx context.Context, event domain.MessageEvent, failureCode string, audienceObservedCount int) error {
	if router == nil || router.publisher == nil {
		return fmt.Errorf("Consumer dead-letter publisher is unavailable")
	}
	return router.publisher.PublishDeadLetter(ctx, event, failureCode, audienceObservedCount)
}

func (router *RecordingFailureRouter) PublishInvalidRetry(ctx context.Context, body []byte, fingerprint string, attempt int) error {
	invalidRouter, ok := router.publisher.(InvalidFailureRouter)
	if !ok {
		return fmt.Errorf("Consumer invalid-event failure publisher is unavailable")
	}
	return invalidRouter.PublishInvalidRetry(ctx, body, fingerprint, attempt)
}

func (router *RecordingFailureRouter) PublishInvalidDeadLetter(ctx context.Context, body []byte, fingerprint, failureCode string) error {
	invalidRouter, ok := router.publisher.(InvalidFailureRouter)
	if !ok {
		return fmt.Errorf("Consumer invalid-event failure publisher is unavailable")
	}
	return invalidRouter.PublishInvalidDeadLetter(ctx, body, fingerprint, failureCode)
}
