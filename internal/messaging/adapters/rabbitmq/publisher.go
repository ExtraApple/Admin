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
	return publishConfirmedEvent(ctx, publisher.channels, EventsExchange, string(event.EventName), event, nil, "RabbitMQ event")
}

func publishConfirmedEvent(ctx context.Context, channels PublisherChannelFactory, exchange, routingKey string, event domain.MessageEvent, headers amqp.Table, description string) error {
	if err := domain.ValidateMessageEvent(event); err != nil {
		return fmt.Errorf("validate %s: %w", description, err)
	}
	channel, err := channels.OpenPublisherChannel(ctx)
	if err != nil {
		return fmt.Errorf("open %s publisher channel: %w", description, err)
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable %s publisher confirms: %w", description, err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	payload, err := json.Marshal(eventPayload{EventID: event.EventID, EventName: event.EventName, EventVersion: event.EventVersion, MessageCopyID: event.MessageCopyID, OrganizationID: event.OrganizationID, OccurredAt: event.OccurredAt.UTC(), AggregateVersion: event.AggregateVersion})
	if err != nil {
		return fmt.Errorf("encode %s: %w", description, err)
	}
	if err := channel.PublishWithContext(ctx, exchange, routingKey, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Headers: headers, Body: payload}); err != nil {
		return fmt.Errorf("publish %s: %w", description, err)
	}
	select {
	case confirmation, open := <-confirmations:
		if !open || !confirmation.Ack {
			return ErrPublisherConfirmRejected
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("await %s publisher confirm: %w", description, ctx.Err())
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
