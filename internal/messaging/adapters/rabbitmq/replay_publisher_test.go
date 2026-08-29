package rabbitmq_test

import (
	"context"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/domain"
)

func TestConsumerReplayPublisherConfirmsMinimalReplayEvent(t *testing.T) {
	channel := &publisherChannelFake{}
	publisher := messagingrabbitmq.NewConsumerReplayPublisher(publisherChannelFactoryFake{channel: channel})
	event := domain.MessageEvent{EventID: "event-replay", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC), AggregateVersion: 2}
	if err := publisher.PublishConsumerReplay(context.Background(), "websocket", event); err != nil {
		t.Fatalf("PublishConsumerReplay() = %v", err)
	}
	if !channel.confirmEnabled || channel.exchange != messagingrabbitmq.ReplayExchange || channel.routingKey != "consumer.websocket" || channel.published.ContentType != "application/json" || channel.published.DeliveryMode == 0 {
		t.Fatalf("replay publish = %#v", channel)
	}
}
