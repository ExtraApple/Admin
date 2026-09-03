package rabbitmq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type ConsumerChannel interface {
	Qos(prefetchCount, prefetchSize int, global bool) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, arguments amqp.Table) (<-chan amqp.Delivery, error)
	Ack(tag uint64, multiple bool) error
	Nack(tag uint64, multiple, requeue bool) error
	Close() error
}

type ConsumerChannelFactory interface {
	OpenConsumerChannel(context.Context) (ConsumerChannel, error)
}

type EventProcessor interface {
	Process(context.Context, domain.MessageEvent) (application.EventConsumerResult, error)
}

type CompetingConsumerConfig struct {
	Queue        string
	ConsumerName string
	Channels     ConsumerChannelFactory
	Processor    EventProcessor
	Failures     FailureRouter
	Logger       application.RuntimeLogger
}

type CompetingConsumer struct {
	queue        string
	consumerName string
	channels     ConsumerChannelFactory
	processor    EventProcessor
	failures     FailureRouter
	logger       application.RuntimeLogger
}

func NewCompetingConsumer(config CompetingConsumerConfig) *CompetingConsumer {
	return &CompetingConsumer{queue: config.Queue, consumerName: config.ConsumerName, channels: config.Channels, processor: config.Processor, failures: config.Failures, logger: config.Logger}
}

func (consumer *CompetingConsumer) log(level, message string, fields ...application.RuntimeLogField) {
	if consumer == nil || consumer.logger == nil {
		return
	}
	if level == "warn" {
		consumer.logger.Warn(message, fields...)
		return
	}
	consumer.logger.Info(message, fields...)
}

func (consumer *CompetingConsumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.queue == "" || consumer.consumerName == "" || consumer.channels == nil || consumer.processor == nil {
		return fmt.Errorf("invalid competing Consumer configuration")
	}
	channel, err := consumer.channels.OpenConsumerChannel(ctx)
	if err != nil {
		consumer.log("warn", "messaging_rabbitmq_consumer_channel_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "channel_open"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_channel_unavailable"})
		return fmt.Errorf("open Consumer channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Qos(1, 0, false); err != nil {
		consumer.log("warn", "messaging_rabbitmq_consumer_qos_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "qos"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_qos_failed"})
		return fmt.Errorf("set Consumer prefetch: %w", err)
	}
	messages, err := channel.Consume(consumer.queue, consumer.consumerName, false, false, false, false, nil)
	if err != nil {
		consumer.log("warn", "messaging_rabbitmq_consumer_start_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "consume"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_consume_failed"})
		return fmt.Errorf("consume messaging events: %w", err)
	}
	consumer.log("info", "messaging_rabbitmq_consumer_started", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "consume"})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, open := <-messages:
			if !open {
				consumer.log("info", "messaging_rabbitmq_consumer_closed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "consume"})
				return nil
			}
			var event eventPayload
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				if routeErr := consumer.routeInvalidDelivery(ctx, channel, delivery, "message_event_invalid"); routeErr != nil {
					return routeErr
				}
				continue
			}
			messageEvent := event.toDomain()
			if err := domain.ValidateMessageEvent(messageEvent); err != nil {
				if routeErr := consumer.routeInvalidDelivery(ctx, channel, delivery, "message_event_invalid"); routeErr != nil {
					return routeErr
				}
				continue
			}
			result, processErr := consumer.processor.Process(ctx, messageEvent)
			if processErr != nil {
				retryAttempt, valid := controlledRetryAttempt(delivery.Headers)
				if !valid {
					retryAttempt = 0
				}
				failureCode := consumerFailureCode(processErr)
				consumer.log("warn", "messaging_rabbitmq_consumer_processing_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: messageEvent.EventID}, application.RuntimeLogField{Key: "stage", Value: "process"}, application.RuntimeLogField{Key: "failure_code", Value: failureCode}, application.RuntimeLogField{Key: "retry_attempt", Value: retryAttempt}, application.RuntimeLogField{Key: "audience_observed_count", Value: result.AudienceObservedCount})
				if consumer.failures == nil {
					if nackErr := channel.Nack(delivery.DeliveryTag, false, false); nackErr != nil {
						return nackErr
					}
					continue
				}
				if routeErr := RouteConsumerFailure(ctx, consumer.failures, messageEvent, delivery.Headers, failureCode, result.AudienceObservedCount); routeErr != nil {
					consumer.log("warn", "messaging_rabbitmq_consumer_failure_route_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: messageEvent.EventID}, application.RuntimeLogField{Key: "stage", Value: "failure_route"}, application.RuntimeLogField{Key: "failure_code", Value: "consumer_failure_route_failed"}, application.RuntimeLogField{Key: "retry_attempt", Value: retryAttempt})
					return fmt.Errorf("route Consumer failure: %w", routeErr)
				}
				consumer.log("info", "messaging_rabbitmq_consumer_failure_routed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: messageEvent.EventID}, application.RuntimeLogField{Key: "stage", Value: "failure_route"}, application.RuntimeLogField{Key: "failure_code", Value: failureCode}, application.RuntimeLogField{Key: "retry_attempt", Value: retryAttempt})
				if ackErr := channel.Ack(delivery.DeliveryTag, false); ackErr != nil {
					return ackErr
				}
				continue
			}
			if err := channel.Ack(delivery.DeliveryTag, false); err != nil {
				consumer.log("warn", "messaging_rabbitmq_consumer_ack_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: messageEvent.EventID}, application.RuntimeLogField{Key: "stage", Value: "ack"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_ack_failed"})
				return err
			}
			consumer.log("info", "messaging_rabbitmq_consumer_acknowledged", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: messageEvent.EventID}, application.RuntimeLogField{Key: "stage", Value: "ack"})
		}
	}
}

func consumerFailureCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrAudienceCapacityExceeded):
		return application.FailureAudienceCapacityExceeded
	case errors.Is(err, application.ErrLeaseNotHeld):
		return "consumer_lease_not_held"
	case errors.Is(err, domain.ErrMessageEventInvalid):
		return "message_event_invalid"
	default:
		return "consumer_processing_failed"
	}
}

func (payload eventPayload) toDomain() domain.MessageEvent {
	return domain.MessageEvent{EventID: payload.EventID, EventName: payload.EventName, EventVersion: payload.EventVersion, MessageCopyID: payload.MessageCopyID, OrganizationID: payload.OrganizationID, OccurredAt: payload.OccurredAt, AggregateVersion: payload.AggregateVersion}
}

func (consumer *CompetingConsumer) routeInvalidDelivery(ctx context.Context, channel ConsumerChannel, delivery amqp.Delivery, failureCode string) error {
	fingerprintBytes := sha256.Sum256(delivery.Body)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	attempt, valid := controlledRetryAttempt(delivery.Headers)
	if !valid {
		attempt = 0
	}
	if router, ok := consumer.failures.(InvalidFailureRouter); ok {
		if attempt >= 4 {
			if err := router.PublishInvalidDeadLetter(ctx, delivery.Body, fingerprint, failureCode); err != nil {
				return fmt.Errorf("route invalid event to dead letter: %w", err)
			}
		} else if err := router.PublishInvalidRetry(ctx, delivery.Body, fingerprint, attempt+1); err != nil {
			return fmt.Errorf("route invalid event to retry: %w", err)
		}
		consumer.log("info", "messaging_rabbitmq_invalid_event_routed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "invalid_event_route"}, application.RuntimeLogField{Key: "failure_code", Value: failureCode}, application.RuntimeLogField{Key: "retry_attempt", Value: attempt})
		return channel.Ack(delivery.DeliveryTag, false)
	}
	if err := channel.Nack(delivery.DeliveryTag, false, false); err != nil {
		return err
	}
	return nil
}
