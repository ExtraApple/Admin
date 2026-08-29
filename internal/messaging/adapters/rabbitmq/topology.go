package rabbitmq

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	EventsExchange        = "admin.events.v1"
	RetryExchange         = "admin.events.retry"
	DeadLetterExchange    = "admin.events.dlx"
	ReplayExchange        = "admin.events.replay"
	RecorderRetryExchange = "admin.events.recorder.retry"
	RecorderAlertExchange = "admin.events.recorder.alert"
)

type TopologyChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, arguments amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, arguments amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, arguments amqp.Table) error
}

type Topology struct {
	Consumer     string
	RetryDelays  []time.Duration
	DLQRetention time.Duration
}

func ConsumerQueueName(consumer string) string {
	return "admin.messaging." + consumer
}

func RetryQueueName(consumer string, attempt int) string {
	return ConsumerQueueName(consumer) + ".retry." + strconv.Itoa(attempt)
}

func DeadLetterQueueName(consumer string) string {
	return ConsumerQueueName(consumer) + ".dlq"
}

func ConsumerDeadLetterAttemptQueueName(consumer string, attempt int) string {
	if attempt <= 1 {
		return DeadLetterQueueName(consumer)
	}
	return DeadLetterQueueName(consumer) + ".recorder." + strconv.Itoa(attempt)
}

func RecorderRetryQueueName(consumer string, attempt int) string {
	return DeadLetterQueueName(consumer) + ".recorder.retry." + strconv.Itoa(attempt)
}

func RetryRoutingKey(consumer string, attempt int) string {
	return "messaging.retry." + consumer + "." + strconv.Itoa(attempt)
}

func RetryReturnRoutingKey(consumer string, attempt int) string {
	return RetryRoutingKey(consumer, attempt)
}

func RecorderRetryRoutingKey(consumer string, attempt int) string {
	return "consumer." + consumer + ".recorder.retry." + strconv.Itoa(attempt)
}

func RecorderAttemptRoutingKey(consumer string, attempt int) string {
	return "consumer." + consumer + ".recorder." + strconv.Itoa(attempt)
}

func RecorderAlertQueueName(consumer string) string {
	return DeadLetterQueueName(consumer) + ".recorder.alert"
}

func DeclareTopology(channel TopologyChannel, topology Topology) error {
	if channel == nil || strings.TrimSpace(topology.Consumer) == "" || len(topology.RetryDelays) != 5 || topology.DLQRetention <= 0 {
		return fmt.Errorf("invalid RabbitMQ messaging topology")
	}
	for _, delay := range topology.RetryDelays {
		if delay <= 0 || delay.Milliseconds() > int64(^uint32(0)>>1) {
			return fmt.Errorf("invalid RabbitMQ retry delay")
		}
	}
	if topology.DLQRetention.Milliseconds() > int64(^uint32(0)>>1) {
		return fmt.Errorf("invalid RabbitMQ dead-letter retention")
	}
	if err := channel.ExchangeDeclare(EventsExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare events exchange: %w", err)
	}
	if err := channel.ExchangeDeclare(RetryExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare retry exchange: %w", err)
	}
	if err := channel.ExchangeDeclare(DeadLetterExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}
	if err := channel.ExchangeDeclare(ReplayExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare replay exchange: %w", err)
	}
	if err := channel.ExchangeDeclare(RecorderRetryExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare Consumer dead-letter recorder retry exchange: %w", err)
	}
	if err := channel.ExchangeDeclare(RecorderAlertExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare Consumer dead-letter recorder alert exchange: %w", err)
	}

	consumerQueue := ConsumerQueueName(topology.Consumer)
	if _, err := channel.QueueDeclare(consumerQueue, true, false, false, false, amqp.Table{"x-queue-type": "quorum", "x-dead-letter-exchange": DeadLetterExchange}); err != nil {
		return fmt.Errorf("declare consumer queue: %w", err)
	}
	if err := channel.QueueBind(consumerQueue, "messaging.message.*.v1", EventsExchange, false, nil); err != nil {
		return fmt.Errorf("bind consumer lifecycle queue: %w", err)
	}
	if err := channel.QueueBind(consumerQueue, "messaging.retry."+topology.Consumer+".*", EventsExchange, false, nil); err != nil {
		return fmt.Errorf("bind consumer retry queue: %w", err)
	}
	if err := channel.QueueBind(consumerQueue, "consumer."+topology.Consumer, ReplayExchange, false, nil); err != nil {
		return fmt.Errorf("bind consumer replay queue: %w", err)
	}
	for index, delay := range topology.RetryDelays {
		attempt := index + 1
		queueName := RetryQueueName(topology.Consumer, attempt)
		arguments := amqp.Table{
			"x-queue-type":              "quorum",
			"x-message-ttl":             int32(delay.Milliseconds()),
			"x-dead-letter-exchange":    EventsExchange,
			"x-dead-letter-routing-key": RetryReturnRoutingKey(topology.Consumer, attempt),
		}
		if _, err := channel.QueueDeclare(queueName, true, false, false, false, arguments); err != nil {
			return fmt.Errorf("declare retry queue %d: %w", attempt, err)
		}
		if err := channel.QueueBind(queueName, RetryRoutingKey(topology.Consumer, attempt), RetryExchange, false, nil); err != nil {
			return fmt.Errorf("bind retry queue %d: %w", attempt, err)
		}
	}

	deadLetterQueue := DeadLetterQueueName(topology.Consumer)
	if _, err := channel.QueueDeclare(deadLetterQueue, true, false, false, false, amqp.Table{
		"x-queue-type":              "quorum",
		"x-message-ttl":             int32(topology.DLQRetention.Milliseconds()),
		"x-dead-letter-exchange":    RecorderRetryExchange,
		"x-dead-letter-routing-key": RecorderRetryRoutingKey(topology.Consumer, 1),
	}); err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	if err := channel.QueueBind(deadLetterQueue, "consumer."+topology.Consumer, DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue: %w", err)
	}
	for attempt := 2; attempt <= len(topology.RetryDelays); attempt++ {
		queueName := ConsumerDeadLetterAttemptQueueName(topology.Consumer, attempt)
		if _, err := channel.QueueDeclare(queueName, true, false, false, false, amqp.Table{
			"x-queue-type":              "quorum",
			"x-dead-letter-exchange":    RecorderRetryExchange,
			"x-dead-letter-routing-key": RecorderRetryRoutingKey(topology.Consumer, attempt),
		}); err != nil {
			return fmt.Errorf("declare Consumer dead-letter recorder queue %d: %w", attempt, err)
		}
		if err := channel.QueueBind(queueName, RecorderAttemptRoutingKey(topology.Consumer, attempt), DeadLetterExchange, false, nil); err != nil {
			return fmt.Errorf("bind Consumer dead-letter recorder queue %d: %w", attempt, err)
		}
	}
	for attempt, delay := range topology.RetryDelays {
		queueName := RecorderRetryQueueName(topology.Consumer, attempt+1)
		nextExchange := DeadLetterExchange
		nextRoutingKey := RecorderAttemptRoutingKey(topology.Consumer, attempt+2)
		if attempt+1 == len(topology.RetryDelays) {
			nextExchange = RecorderAlertExchange
			nextRoutingKey = "consumer." + topology.Consumer
		}
		arguments := amqp.Table{
			"x-queue-type":              "quorum",
			"x-message-ttl":             int32(delay.Milliseconds()),
			"x-dead-letter-exchange":    nextExchange,
			"x-dead-letter-routing-key": nextRoutingKey,
		}
		if _, err := channel.QueueDeclare(queueName, true, false, false, false, arguments); err != nil {
			return fmt.Errorf("declare Consumer dead-letter recorder retry queue %d: %w", attempt+1, err)
		}
		if err := channel.QueueBind(queueName, RecorderRetryRoutingKey(topology.Consumer, attempt+1), RecorderRetryExchange, false, nil); err != nil {
			return fmt.Errorf("bind Consumer dead-letter recorder retry queue %d: %w", attempt+1, err)
		}
	}
	alertQueue := RecorderAlertQueueName(topology.Consumer)
	if _, err := channel.QueueDeclare(alertQueue, true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
		return fmt.Errorf("declare Consumer dead-letter recorder alert queue: %w", err)
	}
	if err := channel.QueueBind(alertQueue, "consumer."+topology.Consumer, RecorderAlertExchange, false, nil); err != nil {
		return fmt.Errorf("bind Consumer dead-letter recorder alert queue: %w", err)
	}
	return nil
}
