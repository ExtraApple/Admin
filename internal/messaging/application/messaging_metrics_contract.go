package application

import (
	"context"
	"time"
)

// MessagingMetrics is the App-provided, best-effort operational observation
// seam. Its method never returns an error and must not influence Consumer DLQ
// persistence, RabbitMQ acknowledgement, or retry behavior.
type MessagingMetrics interface {
	RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)
}

// ConsumerDLQPendingObservation contains only the safe dimensions required to
// signal the first pending Consumer DLQ projection for a failure class.
type ConsumerDLQPendingObservation struct {
	ConsumerName     string
	FailureCode      string
	PendingCount     int64
	OldestPendingAge time.Duration
}
