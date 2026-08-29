package rabbitmq_test

import (
	"context"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/domain"
)

func TestConsumerFailurePublisherConfirmsMinimalRetryAndDeadLetterEvents(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-1", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}

	retryChannel := &publisherChannelFake{}
	retryPublisher := messagingrabbitmq.NewConsumerFailurePublisher("websocket", publisherChannelFactoryFake{channel: retryChannel})
	if err := retryPublisher.PublishRetry(context.Background(), event, 1); err != nil {
		t.Fatalf("PublishRetry() = %v", err)
	}
	if !retryChannel.confirmEnabled || retryChannel.exchange != messagingrabbitmq.RetryExchange || retryChannel.routingKey != messagingrabbitmq.RetryRoutingKey("websocket", 1) || retryChannel.published.Headers[messagingrabbitmq.RetryAttemptHeader] != int32(1) {
		t.Fatalf("retry publish = %#v", retryChannel)
	}

	dlqChannel := &publisherChannelFake{}
	dlqPublisher := messagingrabbitmq.NewConsumerFailurePublisher("websocket", publisherChannelFactoryFake{channel: dlqChannel})
	if err := dlqPublisher.PublishDeadLetter(context.Background(), event, "audience_capacity_exceeded", 100001); err != nil {
		t.Fatalf("PublishDeadLetter() = %v", err)
	}
	if !dlqChannel.confirmEnabled || dlqChannel.exchange != messagingrabbitmq.DeadLetterExchange || dlqChannel.routingKey != "consumer.websocket" || dlqChannel.published.Headers["x-failure-code"] != "audience_capacity_exceeded" || dlqChannel.published.Headers["x-audience-observed-count"] != int32(100001) {
		t.Fatalf("dead-letter publish = %#v", dlqChannel)
	}
}
