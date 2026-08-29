package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type ConsumerReplayPublisher struct {
	channels PublisherChannelFactory
}

func NewConsumerReplayPublisher(channels PublisherChannelFactory) *ConsumerReplayPublisher {
	return &ConsumerReplayPublisher{channels: channels}
}

func (publisher *ConsumerReplayPublisher) PublishConsumerReplay(ctx context.Context, consumerName string, event domain.MessageEvent) error {
	if publisher == nil || publisher.channels == nil || consumerName == "" {
		return fmt.Errorf("Consumer replay publisher is unavailable")
	}
	if err := domain.ValidateMessageEvent(event); err != nil {
		return fmt.Errorf("validate Consumer replay event: %w", err)
	}
	channel, err := publisher.channels.OpenPublisherChannel(ctx)
	if err != nil {
		return fmt.Errorf("open Consumer replay publisher channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable Consumer replay publisher confirms: %w", err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	payload, err := json.Marshal(eventPayload{EventID: event.EventID, EventName: event.EventName, EventVersion: event.EventVersion, MessageCopyID: event.MessageCopyID, OrganizationID: event.OrganizationID, OccurredAt: event.OccurredAt.UTC(), AggregateVersion: event.AggregateVersion})
	if err != nil {
		return fmt.Errorf("encode Consumer replay event: %w", err)
	}
	if err := channel.PublishWithContext(ctx, ReplayExchange, "consumer."+consumerName, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: payload}); err != nil {
		return fmt.Errorf("publish Consumer replay event: %w", err)
	}
	select {
	case confirmation, open := <-confirmations:
		if !open || !confirmation.Ack {
			return ErrPublisherConfirmRejected
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("await Consumer replay publisher confirm: %w", ctx.Err())
	}
}
