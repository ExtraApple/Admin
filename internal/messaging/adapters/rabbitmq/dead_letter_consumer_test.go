package rabbitmq_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type dlqRecorderFake struct {
	inputs []application.ConsumerDeadLetterInput
	err    error
}

func (recorder *dlqRecorderFake) Record(_ context.Context, input application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error) {
	recorder.inputs = append(recorder.inputs, input)
	if recorder.err != nil {
		return application.ConsumerDeadLetter{}, recorder.err
	}
	return application.ConsumerDeadLetter{ID: 1}, nil
}

func TestConsumerDeadLetterConsumerPersistsProjectionBeforeAck(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	recorder := &dlqRecorderFake{}
	consumer := messagingrabbitmq.NewConsumerDeadLetterConsumer(messagingrabbitmq.ConsumerDeadLetterConsumerConfig{
		Queue:         messagingrabbitmq.DeadLetterQueueName("websocket"),
		ConsumerName:  "websocket",
		OriginalQueue: messagingrabbitmq.ConsumerQueueName("websocket"),
		Channels:      consumerChannelFactoryFake{channel: channel},
		Recorder:      recorder,
	})
	body, err := json.Marshal(map[string]any{
		"event_id": "event-dlq-consume", "event_name": string(domain.EventNameMessagePublished), "event_version": 1,
		"message_copy_id": 41, "organization_id": 10, "occurred_at": now, "aggregate_version": 2,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	messages <- amqp.Delivery{DeliveryTag: 11, Headers: amqp.Table{
		messagingrabbitmq.RetryAttemptHeader:          int32(5),
		messagingrabbitmq.FailureCodeHeader:           "consumer_processing_failed",
		messagingrabbitmq.AudienceObservedCountHeader: int32(12),
	}, Body: body}
	close(messages)

	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(recorder.inputs) != 1 || recorder.inputs[0].ConsumerName != "websocket" || recorder.inputs[0].OriginalQueue != messagingrabbitmq.ConsumerQueueName("websocket") || recorder.inputs[0].RetryAttempt != 5 || recorder.inputs[0].FailureCode != "consumer_processing_failed" || recorder.inputs[0].AudienceObservedCount != 12 || len(channel.acked) != 1 || channel.acked[0] != 11 || len(channel.nacked) != 0 {
		t.Fatalf("recorder=%#v acked=%#v nacked=%#v", recorder.inputs, channel.acked, channel.nacked)
	}
}

func TestConsumerDeadLetterConsumerLeavesDeliveryUnackedWhenProjectionFails(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	recorder := &dlqRecorderFake{err: errors.New("database unavailable")}
	consumer := messagingrabbitmq.NewConsumerDeadLetterConsumer(messagingrabbitmq.ConsumerDeadLetterConsumerConfig{
		Queue:         messagingrabbitmq.DeadLetterQueueName("websocket"),
		ConsumerName:  "websocket",
		OriginalQueue: messagingrabbitmq.ConsumerQueueName("websocket"),
		Channels:      consumerChannelFactoryFake{channel: channel},
		Recorder:      recorder,
	})
	messages <- amqp.Delivery{DeliveryTag: 12, Headers: amqp.Table{
		messagingrabbitmq.RetryAttemptHeader: int32(5),
		messagingrabbitmq.FailureCodeHeader:  "consumer_processing_failed",
	}, Body: []byte(`{"event_id":"event-dlq-fail","event_name":"messaging.message.published.v1","event_version":1,"message_copy_id":41,"organization_id":10,"occurred_at":"2026-08-24T01:02:03Z","aggregate_version":2}`)}
	close(messages)

	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(recorder.inputs) != 1 || len(channel.acked) != 0 || len(channel.nacked) != 1 || channel.nacked[0] != 12 {
		t.Fatalf("recorder=%#v acked=%#v nacked=%#v", recorder.inputs, channel.acked, channel.nacked)
	}
}

func TestConsumerDeadLetterConsumerRejectsUncontrolledHeaders(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	recorder := &dlqRecorderFake{}
	consumer := messagingrabbitmq.NewConsumerDeadLetterConsumer(messagingrabbitmq.ConsumerDeadLetterConsumerConfig{
		Queue:         messagingrabbitmq.DeadLetterQueueName("websocket"),
		ConsumerName:  "websocket",
		OriginalQueue: messagingrabbitmq.ConsumerQueueName("websocket"),
		Channels:      consumerChannelFactoryFake{channel: channel},
		Recorder:      recorder,
	})
	messages <- amqp.Delivery{DeliveryTag: 13, Headers: amqp.Table{
		messagingrabbitmq.RetryAttemptHeader: int32(2),
		messagingrabbitmq.FailureCodeHeader:  "consumer_processing_failed",
	}, Body: []byte(`{"event_id":"event-dlq-invalid-header","event_name":"messaging.message.published.v1","event_version":1,"message_copy_id":41,"organization_id":10,"occurred_at":"2026-08-24T01:02:03Z","aggregate_version":2}`)}
	close(messages)

	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(recorder.inputs) != 0 || len(channel.acked) != 0 || len(channel.nacked) != 1 || channel.nacked[0] != 13 {
		t.Fatalf("recorder=%#v acked=%#v nacked=%#v", recorder.inputs, channel.acked, channel.nacked)
	}
}
func TestConsumerDeadLetterConsumerLogsControlledRecorderFailure(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	logger := &consumerRuntimeLoggerFake{}
	consumer := messagingrabbitmq.NewConsumerDeadLetterConsumer(messagingrabbitmq.ConsumerDeadLetterConsumerConfig{
		Queue:         messagingrabbitmq.DeadLetterQueueName("websocket"),
		ConsumerName:  "websocket",
		OriginalQueue: messagingrabbitmq.ConsumerQueueName("websocket"),
		Channels:      consumerChannelFactoryFake{channel: channel},
		Recorder:      &dlqRecorderFake{err: errors.New("database password=secret")},
		Logger:        logger,
	})
	messages <- amqp.Delivery{DeliveryTag: 14, Headers: amqp.Table{
		messagingrabbitmq.RetryAttemptHeader: int32(5),
		messagingrabbitmq.FailureCodeHeader:  "consumer_processing_failed",
	}, Body: []byte(`{"event_id":"event-dlq-log","event_name":"messaging.message.published.v1","event_version":1,"message_copy_id":41,"organization_id":10,"occurred_at":"2026-08-24T01:02:03Z","aggregate_version":2}`)}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(logger.records) == 0 {
		t.Fatal("dead-letter consumer did not emit runtime log")
	}
	for _, record := range logger.records {
		if record == "database password=secret" {
			t.Fatalf("runtime logger received raw error: %q", record)
		}
	}
}

func TestConsumerDeadLetterConsumerRecordsInvalidEventAsFingerprintOnlyProjection(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	recorder := &dlqRecorderFake{}
	consumer := messagingrabbitmq.NewConsumerDeadLetterConsumer(messagingrabbitmq.ConsumerDeadLetterConsumerConfig{Queue: messagingrabbitmq.DeadLetterQueueName("websocket"), ConsumerName: "websocket", OriginalQueue: messagingrabbitmq.ConsumerQueueName("websocket"), Channels: consumerChannelFactoryFake{channel: channel}, Recorder: recorder})
	body := []byte(`{"event_id":"leak-me","markdown":"secret","password":"secret"}`)
	digest := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(digest[:])
	messages <- amqp.Delivery{DeliveryTag: 15, Headers: amqp.Table{messagingrabbitmq.RetryAttemptHeader: int32(5), messagingrabbitmq.FailureCodeHeader: "message_event_invalid", messagingrabbitmq.InvalidEventHeader: true, messagingrabbitmq.InvalidEventFingerprintHeader: fingerprint}, Body: body}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(recorder.inputs) != 1 || !recorder.inputs[0].Invalid || recorder.inputs[0].Fingerprint != fingerprint || recorder.inputs[0].Event.EventID == "leak-me" || len(channel.acked) != 1 || len(channel.nacked) != 0 {
		t.Fatalf("invalid projection inputs=%#v acked=%#v nacked=%#v", recorder.inputs, channel.acked, channel.nacked)
	}
}
