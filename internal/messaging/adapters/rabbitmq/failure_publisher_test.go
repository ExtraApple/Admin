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

func TestConsumerFailurePublisherPreservesInvalidBodyOnlyThroughRetriesAndTerminalDLQ(t *testing.T) {
	body := []byte(`{"event_id":"bad","secret":"do-not-persist"}`)
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	retryChannel := &publisherChannelFake{}
	publisher := messagingrabbitmq.NewConsumerFailurePublisher("websocket", publisherChannelFactoryFake{channel: retryChannel})
	if err := publisher.PublishInvalidRetry(context.Background(), body, fingerprint, 1); err != nil {
		t.Fatalf("PublishInvalidRetry() = %v", err)
	}
	if string(retryChannel.published.Body) != string(body) || retryChannel.published.Headers[messagingrabbitmq.InvalidEventHeader] != true || retryChannel.published.Headers[messagingrabbitmq.InvalidEventFingerprintHeader] != fingerprint || retryChannel.published.Headers[messagingrabbitmq.RetryAttemptHeader] != int32(1) {
		t.Fatalf("invalid retry publish = %#v", retryChannel)
	}
	deadChannel := &publisherChannelFake{}
	deadPublisher := messagingrabbitmq.NewConsumerFailurePublisher("websocket", publisherChannelFactoryFake{channel: deadChannel})
	if err := deadPublisher.PublishInvalidDeadLetter(context.Background(), body, fingerprint, "message_event_invalid"); err != nil {
		t.Fatalf("PublishInvalidDeadLetter() = %v", err)
	}
	if deadChannel.exchange != messagingrabbitmq.DeadLetterExchange || string(deadChannel.published.Body) != string(body) || deadChannel.published.Headers[messagingrabbitmq.InvalidEventHeader] != true || deadChannel.published.Headers[messagingrabbitmq.RetryAttemptHeader] != int32(5) {
		t.Fatalf("invalid dead-letter publish = %#v", deadChannel)
	}
}

func TestConsumerFailurePublisherAllowsEmptyInvalidBody(t *testing.T) {
	fingerprint := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	channel := &publisherChannelFake{}
	publisher := messagingrabbitmq.NewConsumerFailurePublisher("websocket", publisherChannelFactoryFake{channel: channel})
	if err := publisher.PublishInvalidDeadLetter(context.Background(), nil, fingerprint, "message_event_invalid"); err != nil {
		t.Fatalf("PublishInvalidDeadLetter() = %v", err)
	}
	if len(channel.published.Body) != 0 || channel.published.Headers[messagingrabbitmq.InvalidEventHeader] != true {
		t.Fatalf("empty invalid body publish = %#v", channel)
	}
}
