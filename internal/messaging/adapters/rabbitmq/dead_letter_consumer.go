package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ConsumerDeadLetterConsumer persists terminal Consumer failures before ACKing
// the RabbitMQ DLQ delivery. A persistence failure intentionally leaves the
// delivery unacknowledged so the broker can retry it through its DLQ policy.
type ConsumerDeadLetterConsumerConfig struct {
	Queue         string
	ConsumerName  string
	OriginalQueue string
	Channels      ConsumerChannelFactory
	Recorder      ConsumerDeadLetterRecorder
	Logger        application.RuntimeLogger
}

type ConsumerDeadLetterConsumer struct {
	queue         string
	consumerName  string
	originalQueue string
	channels      ConsumerChannelFactory
	recorder      ConsumerDeadLetterRecorder
	logger        application.RuntimeLogger
}

func NewConsumerDeadLetterConsumer(config ConsumerDeadLetterConsumerConfig) *ConsumerDeadLetterConsumer {
	return &ConsumerDeadLetterConsumer{queue: config.Queue, consumerName: config.ConsumerName, originalQueue: config.OriginalQueue, channels: config.Channels, recorder: config.Recorder, logger: config.Logger}
}

func (consumer *ConsumerDeadLetterConsumer) log(level, message string, fields ...application.RuntimeLogField) {
	if consumer == nil || consumer.logger == nil {
		return
	}
	if level == "warn" {
		consumer.logger.Warn(message, fields...)
		return
	}
	consumer.logger.Info(message, fields...)
}

func (consumer *ConsumerDeadLetterConsumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.queue == "" || consumer.consumerName == "" || consumer.originalQueue == "" || consumer.channels == nil || consumer.recorder == nil {
		return fmt.Errorf("invalid Consumer dead-letter configuration")
	}
	channel, err := consumer.channels.OpenConsumerChannel(ctx)
	if err != nil {
		consumer.log("warn", "messaging_rabbitmq_dlq_channel_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "channel_open"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_channel_unavailable"})
		return fmt.Errorf("open Consumer dead-letter channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Qos(1, 0, false); err != nil {
		consumer.log("warn", "messaging_rabbitmq_dlq_qos_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "qos"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_qos_failed"})
		return fmt.Errorf("set Consumer dead-letter prefetch: %w", err)
	}
	messages, err := channel.Consume(consumer.queue, consumer.consumerName+"-dlq-recorder", false, false, false, false, nil)
	if err != nil {
		consumer.log("warn", "messaging_rabbitmq_dlq_start_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "consume"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_consume_failed"})
		return fmt.Errorf("consume Consumer dead letters: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, open := <-messages:
			if !open {
				return nil
			}
			event, failureCode, retryAttempt, audienceObservedCount, valid := decodeDeadLetter(delivery)
			if !valid {
				consumer.log("warn", "messaging_rabbitmq_dlq_decode_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "stage", Value: "decode"}, application.RuntimeLogField{Key: "failure_code", Value: "consumer_dlq_payload_invalid"}, application.RuntimeLogField{Key: "retry_attempt", Value: retryAttempt})
				if err := channel.Nack(delivery.DeliveryTag, false, false); err != nil {
					return err
				}
				continue
			}
			if _, err := consumer.recorder.Record(ctx, application.ConsumerDeadLetterInput{ConsumerName: consumer.consumerName, Event: event, OriginalQueue: consumer.originalQueue, RetryAttempt: retryAttempt, FailureCode: failureCode, AudienceObservedCount: audienceObservedCount}); err != nil {
				consumer.log("warn", "messaging_rabbitmq_dlq_record_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: event.EventID}, application.RuntimeLogField{Key: "stage", Value: "record"}, application.RuntimeLogField{Key: "failure_code", Value: "consumer_dlq_record_failed"}, application.RuntimeLogField{Key: "retry_attempt", Value: retryAttempt})
				if nackErr := channel.Nack(delivery.DeliveryTag, false, false); nackErr != nil {
					return nackErr
				}
				continue
			}
			if err := channel.Ack(delivery.DeliveryTag, false); err != nil {
				consumer.log("warn", "messaging_rabbitmq_dlq_ack_failed", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: event.EventID}, application.RuntimeLogField{Key: "stage", Value: "ack"}, application.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_ack_failed"})
				return err
			}
			consumer.log("info", "messaging_rabbitmq_dlq_acknowledged", application.RuntimeLogField{Key: "consumer", Value: consumer.consumerName}, application.RuntimeLogField{Key: "event_id", Value: event.EventID}, application.RuntimeLogField{Key: "stage", Value: "ack"}, application.RuntimeLogField{Key: "retry_attempt", Value: retryAttempt})
		}
	}
}

func decodeDeadLetter(delivery amqp.Delivery) (domain.MessageEvent, string, int, int, bool) {
	var payload eventPayload
	if err := json.Unmarshal(delivery.Body, &payload); err != nil {
		return domain.MessageEvent{}, "", 0, 0, false
	}
	event := payload.toDomain()
	if err := domain.ValidateMessageEvent(event); err != nil {
		return domain.MessageEvent{}, "", 0, 0, false
	}
	retryAttempt, ok := headerIntValue(delivery.Headers, RetryAttemptHeader)
	if !ok || retryAttempt != terminalConsumerRetryAttempt {
		return domain.MessageEvent{}, "", 0, 0, false
	}
	failureCode, ok := stringHeader(delivery.Headers, FailureCodeHeader)
	if !ok || failureCode == "" {
		return domain.MessageEvent{}, "", 0, 0, false
	}
	audienceObservedCount, ok := nonNegativeIntHeader(delivery.Headers, AudienceObservedCountHeader)
	if !ok {
		return domain.MessageEvent{}, "", 0, 0, false
	}
	return event, failureCode, retryAttempt, audienceObservedCount, true
}

func headerIntValue(headers amqp.Table, key string) (int, bool) {
	if headers == nil {
		return 0, false
	}
	value, ok := headers[key]
	if !ok {
		return 0, false
	}
	return headerInt(value)
}

func stringHeader(headers amqp.Table, key string) (string, bool) {
	if headers == nil {
		return "", false
	}
	value, ok := headers[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func nonNegativeIntHeader(headers amqp.Table, key string) (int, bool) {
	if headers == nil {
		return 0, true
	}
	value, ok := headers[key]
	if !ok {
		return 0, true
	}
	attempt, valid := headerInt(value)
	return attempt, valid && attempt >= 0
}

func headerInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), int64(int(typed)) == typed
	case uint:
		return int(typed), uint(int(typed)) == typed
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), uint32(int(typed)) == typed
	case uint64:
		return int(typed), uint64(int(typed)) == typed
	default:
		return 0, false
	}
}
