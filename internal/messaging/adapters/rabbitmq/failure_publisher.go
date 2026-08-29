package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	FailureCodeHeader            = "x-failure-code"
	AudienceObservedCountHeader  = "x-audience-observed-count"
	terminalConsumerRetryAttempt = 5
)

type ConsumerFailurePublisher struct {
	consumerName string
	channels     PublisherChannelFactory
}

func NewConsumerFailurePublisher(consumerName string, channels PublisherChannelFactory) *ConsumerFailurePublisher {
	return &ConsumerFailurePublisher{consumerName: consumerName, channels: channels}
}

func (publisher *ConsumerFailurePublisher) PublishRetry(ctx context.Context, event domain.MessageEvent, attempt int) error {
	if attempt < 1 || attempt > terminalConsumerRetryAttempt {
		return fmt.Errorf("invalid Consumer retry attempt")
	}
	return publisher.publish(ctx, RetryExchange, RetryRoutingKey(publisher.consumerName, attempt), event, amqp.Table{RetryAttemptHeader: int32(attempt)})
}

func (publisher *ConsumerFailurePublisher) PublishDeadLetter(ctx context.Context, event domain.MessageEvent, failureCode string, audienceObservedCount int) error {
	if failureCode == "" || audienceObservedCount < 0 {
		return fmt.Errorf("invalid Consumer dead-letter payload")
	}
	return publisher.publish(ctx, DeadLetterExchange, "consumer."+publisher.consumerName, event, amqp.Table{RetryAttemptHeader: int32(terminalConsumerRetryAttempt), FailureCodeHeader: failureCode, AudienceObservedCountHeader: int32(audienceObservedCount)})
}

func (publisher *ConsumerFailurePublisher) publish(ctx context.Context, exchange, routingKey string, event domain.MessageEvent, headers amqp.Table) error {
	if publisher == nil || publisher.consumerName == "" || publisher.channels == nil {
		return fmt.Errorf("Consumer failure publisher is unavailable")
	}
	if err := domain.ValidateMessageEvent(event); err != nil {
		return fmt.Errorf("validate Consumer failure event: %w", err)
	}
	channel, err := publisher.channels.OpenPublisherChannel(ctx)
	if err != nil {
		return fmt.Errorf("open Consumer failure publisher channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable Consumer failure publisher confirms: %w", err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	payload, err := json.Marshal(eventPayload{EventID: event.EventID, EventName: event.EventName, EventVersion: event.EventVersion, MessageCopyID: event.MessageCopyID, OrganizationID: event.OrganizationID, OccurredAt: event.OccurredAt.UTC(), AggregateVersion: event.AggregateVersion})
	if err != nil {
		return fmt.Errorf("encode Consumer failure event: %w", err)
	}
	if err := channel.PublishWithContext(ctx, exchange, routingKey, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Headers: headers, Body: payload}); err != nil {
		return fmt.Errorf("publish Consumer failure event: %w", err)
	}
	select {
	case confirmation, open := <-confirmations:
		if !open || !confirmation.Ack {
			return ErrPublisherConfirmRejected
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("await Consumer failure publisher confirm: %w", ctx.Err())
	}
}
