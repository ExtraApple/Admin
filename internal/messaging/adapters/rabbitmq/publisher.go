package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

var ErrPublisherConfirmRejected = errors.New("RabbitMQ publisher confirm rejected")

type PublisherChannel interface {
	Confirm(noWait bool) error
	NotifyPublish(confirmations chan amqp.Confirmation) chan amqp.Confirmation
	PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error
	Close() error
}

type PublisherChannelFactory interface {
	OpenPublisherChannel(context.Context) (PublisherChannel, error)
}

type Publisher struct {
	channels PublisherChannelFactory
}

func NewPublisher(channels PublisherChannelFactory) *Publisher {
	return &Publisher{channels: channels}
}

func (publisher *Publisher) Publish(ctx context.Context, event domain.MessageEvent) error {
	if publisher == nil || publisher.channels == nil {
		return errors.New("RabbitMQ publisher is unavailable")
	}
	if err := domain.ValidateMessageEvent(event); err != nil {
		return fmt.Errorf("validate RabbitMQ event: %w", err)
	}
	channel, err := publisher.channels.OpenPublisherChannel(ctx)
	if err != nil {
		return fmt.Errorf("open RabbitMQ publisher channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable RabbitMQ publisher confirms: %w", err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	payload, err := json.Marshal(eventPayload{EventID: event.EventID, EventName: event.EventName, EventVersion: event.EventVersion, MessageCopyID: event.MessageCopyID, OrganizationID: event.OrganizationID, OccurredAt: event.OccurredAt.UTC(), AggregateVersion: event.AggregateVersion})
	if err != nil {
		return fmt.Errorf("encode RabbitMQ event: %w", err)
	}
	if err := channel.PublishWithContext(ctx, EventsExchange, string(event.EventName), true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: payload}); err != nil {
		return fmt.Errorf("publish RabbitMQ event: %w", err)
	}
	select {
	case confirmation, open := <-confirmations:
		if !open || !confirmation.Ack {
			return ErrPublisherConfirmRejected
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("await RabbitMQ publisher confirm: %w", ctx.Err())
	}
}

type eventPayload struct {
	EventID          string           `json:"event_id"`
	EventName        domain.EventName `json:"event_name"`
	EventVersion     uint             `json:"event_version"`
	MessageCopyID    uint             `json:"message_copy_id"`
	OrganizationID   uint             `json:"organization_id"`
	OccurredAt       time.Time        `json:"occurred_at"`
	AggregateVersion uint64           `json:"aggregate_version"`
}
