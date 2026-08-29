package rabbitmq

import (
	"context"
	"fmt"

	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const RetryAttemptHeader = "x-retry-attempt"

type FailureRouter interface {
	PublishRetry(context.Context, domain.MessageEvent, int) error
	PublishDeadLetter(context.Context, domain.MessageEvent, string, int) error
}

func RouteConsumerFailure(ctx context.Context, router FailureRouter, event domain.MessageEvent, headers amqp.Table, failureCode string, audienceObservedCount int) error {
	if router == nil || failureCode == "" {
		return fmt.Errorf("invalid Consumer failure router")
	}
	attempt, valid := controlledRetryAttempt(headers)
	if !valid || attempt >= 4 {
		return router.PublishDeadLetter(ctx, event, failureCode, audienceObservedCount)
	}
	return router.PublishRetry(ctx, event, attempt+1)
}

func controlledRetryAttempt(headers amqp.Table) (int, bool) {
	if headers == nil {
		return 0, true
	}
	value, ok := headers[RetryAttemptHeader]
	if !ok {
		return 0, true
	}
	var attempt int
	switch typed := value.(type) {
	case int:
		attempt = typed
	case int8:
		attempt = int(typed)
	case int16:
		attempt = int(typed)
	case int32:
		attempt = int(typed)
	case int64:
		attempt = int(typed)
	case uint:
		attempt = int(typed)
	case uint8:
		attempt = int(typed)
	case uint16:
		attempt = int(typed)
	case uint32:
		attempt = int(typed)
	case uint64:
		if typed > uint64(^uint(0)>>1) {
			return 0, false
		}
		attempt = int(typed)
	default:
		return 0, false
	}
	return attempt, attempt >= 0 && attempt <= 4
}
