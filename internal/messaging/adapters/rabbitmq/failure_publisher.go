package rabbitmq

import (
	"context"
	"encoding/hex"
	"fmt"

	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	FailureCodeHeader             = "x-failure-code"
	AudienceObservedCountHeader   = "x-audience-observed-count"
	InvalidEventHeader            = "x-invalid-event"
	InvalidEventFingerprintHeader = "x-event-fingerprint"
	terminalConsumerRetryAttempt  = 5
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

func (publisher *ConsumerFailurePublisher) PublishInvalidRetry(ctx context.Context, body []byte, fingerprint string, attempt int) error {
	if !validFingerprint(fingerprint) || attempt < 1 || attempt >= terminalConsumerRetryAttempt {
		return fmt.Errorf("invalid Consumer invalid-event retry payload")
	}
	return publisher.publishRaw(ctx, RetryExchange, RetryRoutingKey(publisher.consumerName, attempt), body, amqp.Table{RetryAttemptHeader: int32(attempt), InvalidEventHeader: true, InvalidEventFingerprintHeader: fingerprint}, "Consumer invalid-event retry")
}

func (publisher *ConsumerFailurePublisher) PublishInvalidDeadLetter(ctx context.Context, body []byte, fingerprint, failureCode string) error {
	if !validFingerprint(fingerprint) || failureCode == "" {
		return fmt.Errorf("invalid Consumer invalid-event dead-letter payload")
	}
	return publisher.publishRaw(ctx, DeadLetterExchange, "consumer."+publisher.consumerName, body, amqp.Table{RetryAttemptHeader: int32(terminalConsumerRetryAttempt), FailureCodeHeader: failureCode, AudienceObservedCountHeader: int32(0), InvalidEventHeader: true, InvalidEventFingerprintHeader: fingerprint}, "Consumer invalid-event dead letter")
}

func (publisher *ConsumerFailurePublisher) publish(ctx context.Context, exchange, routingKey string, event domain.MessageEvent, headers amqp.Table) error {
	if publisher == nil || publisher.consumerName == "" || publisher.channels == nil {
		return fmt.Errorf("Consumer failure publisher is unavailable")
	}
	return publishConfirmedEvent(ctx, publisher.channels, exchange, routingKey, event, headers, "Consumer failure event")
}

func (publisher *ConsumerFailurePublisher) publishRaw(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table, description string) error {
	if publisher == nil || publisher.consumerName == "" || publisher.channels == nil {
		return fmt.Errorf("Consumer failure publisher is unavailable")
	}
	channel, err := publisher.channels.OpenPublisherChannel(ctx)
	if err != nil {
		return fmt.Errorf("open %s publisher channel: %w", description, err)
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable %s publisher confirms: %w", description, err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	if err := channel.PublishWithContext(ctx, exchange, routingKey, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Headers: headers, Body: append([]byte(nil), body...)}); err != nil {
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

func validFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
