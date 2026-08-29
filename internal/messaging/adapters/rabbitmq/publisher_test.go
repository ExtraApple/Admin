package rabbitmq_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type publisherChannelFake struct {
	confirmEnabled bool
	published      amqp.Publishing
	exchange       string
	routingKey     string
	confirmations  chan amqp.Confirmation
}

func (channel *publisherChannelFake) Confirm(bool) error {
	channel.confirmEnabled = true
	return nil
}

func (channel *publisherChannelFake) NotifyPublish(confirmations chan amqp.Confirmation) chan amqp.Confirmation {
	channel.confirmations = confirmations
	return confirmations
}

func (channel *publisherChannelFake) PublishWithContext(_ context.Context, exchange, key string, _ bool, _ bool, publishing amqp.Publishing) error {
	channel.exchange = exchange
	channel.routingKey = key
	channel.published = publishing
	channel.confirmations <- amqp.Confirmation{Ack: true}
	return nil
}

func (channel *publisherChannelFake) Close() error { return nil }

type publisherChannelFactoryFake struct {
	channel messagingrabbitmq.PublisherChannel
}

func (factory publisherChannelFactoryFake) OpenPublisherChannel(context.Context) (messagingrabbitmq.PublisherChannel, error) {
	return factory.channel, nil
}

func TestPublisherUsesConfirmsAndMinimalEventPayload(t *testing.T) {
	channel := &publisherChannelFake{}
	publisher := messagingrabbitmq.NewPublisher(publisherChannelFactoryFake{channel: channel})
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-1", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 9, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish() = %v", err)
	}
	if !channel.confirmEnabled || channel.routingKey != string(event.EventName) || channel.published.ContentType != "application/json" || channel.published.DeliveryMode != amqp.Persistent {
		t.Fatalf("publish channel state = %#v", channel)
	}
	var payload map[string]any
	if err := json.Unmarshal(channel.published.Body, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) != 7 || payload["event_id"] != event.EventID || payload["event_name"] != string(event.EventName) || payload["message_copy_id"] != float64(event.MessageCopyID) || payload["organization_id"] != float64(event.OrganizationID) || payload["aggregate_version"] != float64(event.AggregateVersion) {
		t.Fatalf("minimal payload = %#v", payload)
	}
	for _, forbidden := range []string{"markdown", "html", "body", "token", "url", "image"} {
		if _, present := payload[forbidden]; present {
			t.Fatalf("payload exposes forbidden %q: %#v", forbidden, payload)
		}
	}
}
