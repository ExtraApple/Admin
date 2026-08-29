package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type messagingMetricsFake struct {
	observations []application.ConsumerDLQPendingObservation
}

func (fake *messagingMetricsFake) RecordConsumerDLQPending(_ context.Context, observation application.ConsumerDLQPendingObservation) {
	fake.observations = append(fake.observations, observation)
}

var _ application.MessagingMetrics = (*messagingMetricsFake)(nil)

func TestMessagingMetricsContractCarriesOnlyPendingDLQDimensions(t *testing.T) {
	metrics := &messagingMetricsFake{}
	metrics.RecordConsumerDLQPending(context.Background(), application.ConsumerDLQPendingObservation{ConsumerName: "websocket", FailureCode: "MSG_RETRY_EXHAUSTED", PendingCount: 3, OldestPendingAge: 2 * time.Minute})
	if len(metrics.observations) != 1 {
		t.Fatalf("observations = %#v", metrics.observations)
	}
	observation := metrics.observations[0]
	if observation.ConsumerName != "websocket" || observation.FailureCode != "MSG_RETRY_EXHAUSTED" || observation.PendingCount != 3 || observation.OldestPendingAge != 2*time.Minute {
		t.Fatalf("observation = %#v", observation)
	}
}
