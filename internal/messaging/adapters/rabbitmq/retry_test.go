package rabbitmq_test

import (
	"context"
	"testing"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type failureRouterFake struct {
	retryAttempt int
	dead         bool
	failureCode  string
}

func (router *failureRouterFake) PublishRetry(_ context.Context, _ domain.MessageEvent, attempt int) error {
	router.retryAttempt = attempt
	return nil
}
func (router *failureRouterFake) PublishDeadLetter(_ context.Context, _ domain.MessageEvent, failureCode string, _ int) error {
	router.dead = true
	router.failureCode = failureCode
	return nil
}

func TestConsumerFailureRoutingUsesFiveControlledRetryAttempts(t *testing.T) {
	event := domain.MessageEvent{EventID: "event-retry", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, AggregateVersion: 1}
	for _, test := range []struct {
		name      string
		headers   amqp.Table
		wantRetry int
		wantDead  bool
	}{
		{name: "first retry", headers: amqp.Table{}, wantRetry: 1},
		{name: "fifth failure", headers: amqp.Table{"x-retry-attempt": int32(4)}, wantDead: true},
		{name: "invalid header cannot bypass dead letter", headers: amqp.Table{"x-retry-attempt": "invalid"}, wantDead: true},
		{name: "string retry attempt cannot bypass dead letter", headers: amqp.Table{"x-retry-attempt": "1"}, wantDead: true},
		{name: "out of range cannot bypass dead letter", headers: amqp.Table{"x-retry-attempt": int32(9)}, wantDead: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := &failureRouterFake{}
			if err := messagingrabbitmq.RouteConsumerFailure(context.Background(), router, event, test.headers, "MSG_PROCESS_FAILED", 12); err != nil {
				t.Fatalf("RouteConsumerFailure() = %v", err)
			}
			if router.retryAttempt != test.wantRetry || router.dead != test.wantDead {
				t.Fatalf("router state retry=%d dead=%t", router.retryAttempt, router.dead)
			}
			if test.wantDead && router.failureCode != "MSG_PROCESS_FAILED" {
				t.Fatalf("dead-letter failure code = %q", router.failureCode)
			}
		})
	}
}
