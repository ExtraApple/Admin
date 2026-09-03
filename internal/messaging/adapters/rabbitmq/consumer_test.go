package rabbitmq_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type competingConsumerChannelFake struct {
	prefetch int
	messages chan amqp.Delivery
	acked    []uint64
	nacked   []uint64
}

func (channel *competingConsumerChannelFake) Qos(prefetchCount, _ int, _ bool) error {
	channel.prefetch = prefetchCount
	return nil
}
func (channel *competingConsumerChannelFake) Consume(string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error) {
	return channel.messages, nil
}
func (channel *competingConsumerChannelFake) Ack(tag uint64, _ bool) error {
	channel.acked = append(channel.acked, tag)
	return nil
}
func (channel *competingConsumerChannelFake) Nack(tag uint64, _ bool, _ bool) error {
	channel.nacked = append(channel.nacked, tag)
	return nil
}
func (channel *competingConsumerChannelFake) Close() error { return nil }

type eventProcessorFake struct {
	events []domain.MessageEvent
	result application.EventConsumerResult
	err    error
}

func (processor *eventProcessorFake) Process(_ context.Context, event domain.MessageEvent) (application.EventConsumerResult, error) {
	processor.events = append(processor.events, event)
	if processor.result == (application.EventConsumerResult{}) {
		processor.result.Completed = processor.err == nil
	}
	return processor.result, processor.err
}

type consumerChannelFactoryFake struct {
	channel messagingrabbitmq.ConsumerChannel
}

func (factory consumerChannelFactoryFake) OpenConsumerChannel(context.Context) (messagingrabbitmq.ConsumerChannel, error) {
	return factory.channel, nil
}

type consumerFailureRouterFake struct {
	retryEvents []domain.MessageEvent
	attempts    []int
	deadEvents  []domain.MessageEvent
}

func (router *consumerFailureRouterFake) PublishRetry(_ context.Context, event domain.MessageEvent, attempt int) error {
	router.retryEvents = append(router.retryEvents, event)
	router.attempts = append(router.attempts, attempt)
	return nil
}

func (router *consumerFailureRouterFake) PublishDeadLetter(_ context.Context, event domain.MessageEvent, _ string, _ int) error {
	router.deadEvents = append(router.deadEvents, event)
	return nil
}

type invalidConsumerFailureRouterFake struct {
	retryBodies       [][]byte
	retryFingerprints []string
	retryAttempts     []int
	deadBodies        [][]byte
	deadFingerprints  []string
	deadFailureCodes  []string
}

func (router *invalidConsumerFailureRouterFake) PublishInvalidRetry(_ context.Context, body []byte, fingerprint string, attempt int) error {
	router.retryBodies = append(router.retryBodies, append([]byte(nil), body...))
	router.retryFingerprints = append(router.retryFingerprints, fingerprint)
	router.retryAttempts = append(router.retryAttempts, attempt)
	return nil
}
func (router *invalidConsumerFailureRouterFake) PublishInvalidDeadLetter(_ context.Context, body []byte, fingerprint, failureCode string) error {
	router.deadBodies = append(router.deadBodies, append([]byte(nil), body...))
	router.deadFingerprints = append(router.deadFingerprints, fingerprint)
	router.deadFailureCodes = append(router.deadFailureCodes, failureCode)
	return nil
}

func TestCompetingConsumerRoutesInvalidEventThroughControlledRetryWithoutProcessing(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	processor := &eventProcessorFake{}
	router := &invalidConsumerFailureRouterFake{}
	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: processor, Failures: router})
	body := []byte(`{"event_id":"invalid","body":"do-not-persist"}`)
	messages <- amqp.Delivery{DeliveryTag: 15, Body: body}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(router.retryBodies) != 1 || string(router.retryBodies[0]) != string(body) || router.retryAttempts[0] != 1 || len(router.deadBodies) != 0 || len(processor.events) != 0 || len(channel.acked) != 1 || channel.acked[0] != 15 {
		t.Fatalf("invalid event routing router=%#v processor=%#v channel=%#v", router, processor, channel)
	}
	if len(router.retryFingerprints[0]) != 64 {
		t.Fatalf("invalid event fingerprint=%q, want SHA-256", router.retryFingerprints[0])
	}
}
func (*invalidConsumerFailureRouterFake) PublishRetry(context.Context, domain.MessageEvent, int) error {
	return nil
}
func (*invalidConsumerFailureRouterFake) PublishDeadLetter(context.Context, domain.MessageEvent, string, int) error {
	return nil
}

func TestCompetingConsumerSetsPrefetchOneAndAcksProcessedMinimalEvent(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	processor := &eventProcessorFake{}
	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: processor})
	body, err := json.Marshal(map[string]any{"event_id": "event-1", "event_name": string(domain.EventNameMessagePublished), "event_version": 1, "message_copy_id": 41, "organization_id": 10, "occurred_at": time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC), "aggregate_version": 1})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	messages <- amqp.Delivery{DeliveryTag: 7, Body: body}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if channel.prefetch != 1 || len(channel.acked) != 1 || channel.acked[0] != 7 || len(channel.nacked) != 0 || len(processor.events) != 1 || processor.events[0].EventID != "event-1" {
		t.Fatalf("consumer state prefetch=%d acked=%#v nacked=%#v events=%#v", channel.prefetch, channel.acked, channel.nacked, processor.events)
	}
}

func TestCompetingConsumerNacksMalformedEventsAndRoutesProcessingFailuresThroughControlledRetry(t *testing.T) {
	messages := make(chan amqp.Delivery, 2)
	channel := &competingConsumerChannelFake{messages: messages}
	processor := &eventProcessorFake{err: context.DeadlineExceeded}
	router := &consumerFailureRouterFake{}

	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: processor, Failures: router})
	messages <- amqp.Delivery{DeliveryTag: 8, Body: []byte("not-json")}
	messages <- amqp.Delivery{DeliveryTag: 9, Body: []byte(`{"event_id":"event-2","event_name":"messaging.message.published.v1","event_version":1,"message_copy_id":41,"organization_id":10,"occurred_at":"2026-08-24T01:02:03Z","aggregate_version":1}`)}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(channel.nacked) != 1 || channel.nacked[0] != 8 || len(channel.acked) != 1 || channel.acked[0] != 9 || len(router.retryEvents) != 1 || router.retryEvents[0].EventID != "event-2" || len(router.attempts) != 1 || router.attempts[0] != 1 || len(router.deadEvents) != 0 {
		t.Fatalf("consumer state acked=%#v nacked=%#v retries=%#v attempts=%#v dead=%#v", channel.acked, channel.nacked, router.retryEvents, router.attempts, router.deadEvents)
	}
}
func TestCompetingConsumerRoutesFourthInvalidAttemptToDeadLetterAndAcks(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	router := &invalidConsumerFailureRouterFake{}
	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: &eventProcessorFake{}, Failures: router})
	body := []byte(`{"event_id":"bad","secret":"do-not-persist"}`)
	messages <- amqp.Delivery{DeliveryTag: 16, Headers: amqp.Table{messagingrabbitmq.RetryAttemptHeader: int32(4), messagingrabbitmq.InvalidEventHeader: true}, Body: body}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(router.retryBodies) != 0 || len(router.deadBodies) != 1 || string(router.deadBodies[0]) != string(body) || router.deadFailureCodes[0] != "message_event_invalid" || len(channel.acked) != 1 || channel.acked[0] != 16 {
		t.Fatalf("terminal invalid routing router=%#v channel=%#v", router, channel)
	}
}

func TestCompetingConsumerRejectsInvalidEventBeforeProcessing(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	processor := &eventProcessorFake{}
	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: processor, Failures: &consumerFailureRouterFake{}})
	messages <- amqp.Delivery{DeliveryTag: 10, Body: []byte(`{"event_id":"event-invalid"}`)}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(channel.nacked) != 1 || channel.nacked[0] != 10 || len(channel.acked) != 0 || len(processor.events) != 0 {
		t.Fatalf("consumer state acked=%#v nacked=%#v events=%#v", channel.acked, channel.nacked, processor.events)
	}
}

func TestCompetingConsumerRecordsTerminalFailureBeforeAcknowledgingDelivery(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	processor := &eventProcessorFake{result: application.EventConsumerResult{AudienceObservedCount: 100001}, err: domain.ErrAudienceCapacityExceeded}
	recorder := &deadLetterRecorderFake{}
	publisher := &recordingFailurePublisherFake{}
	router := messagingrabbitmq.NewRecordingFailureRouter(publisher)
	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: processor, Failures: router})
	body, err := json.Marshal(map[string]any{"event_id": "event-terminal", "event_name": string(domain.EventNameMessagePublished), "event_version": 1, "message_copy_id": 41, "organization_id": 10, "occurred_at": now, "aggregate_version": 1})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	messages <- amqp.Delivery{DeliveryTag: 11, Headers: amqp.Table{messagingrabbitmq.RetryAttemptHeader: int32(4)}, Body: body}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(channel.acked) != 1 || channel.acked[0] != 11 || len(channel.nacked) != 0 || len(recorder.inputs) != 0 || len(publisher.deadEvents) != 1 || len(publisher.retryAttempts) != 0 {
		t.Fatalf("consumer=%#v recorder=%#v publisher=%#v", channel, recorder, publisher)
	}
}

type consumerRuntimeLoggerFake struct {
	records []string
}

func (logger *consumerRuntimeLoggerFake) Info(message string, _ ...application.RuntimeLogField) {
	logger.records = append(logger.records, message)
}
func (logger *consumerRuntimeLoggerFake) Warn(message string, _ ...application.RuntimeLogField) {
	logger.records = append(logger.records, message)
}

func TestCompetingConsumerLogsControlledProcessingFailureWithoutRawBrokerError(t *testing.T) {
	messages := make(chan amqp.Delivery, 1)
	channel := &competingConsumerChannelFake{messages: messages}
	processor := &eventProcessorFake{err: errors.New("broker password=secret")}
	logger := &consumerRuntimeLoggerFake{}
	consumer := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: "admin.messaging.websocket", ConsumerName: "websocket", Channels: consumerChannelFactoryFake{channel: channel}, Processor: processor, Failures: &consumerFailureRouterFake{}, Logger: logger})
	body := []byte(`{"event_id":"event-rabbit-log","event_name":"messaging.message.published.v1","event_version":1,"message_copy_id":41,"organization_id":10,"occurred_at":"2026-08-24T01:02:03Z","aggregate_version":1}`)
	messages <- amqp.Delivery{DeliveryTag: 12, Body: body}
	close(messages)
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(logger.records) == 0 {
		t.Fatal("rabbitmq consumer did not emit runtime log")
	}
	for _, record := range logger.records {
		if record == "broker password=secret" {
			t.Fatalf("runtime logger received raw error: %q", record)
		}
	}
}
