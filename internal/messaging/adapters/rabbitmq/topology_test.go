package rabbitmq_test

import (
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type topologyOperation struct {
	kind, name, exchange, routingKey string
	arguments                        amqp.Table
}

type topologyChannelFake struct {
	operations []topologyOperation
}

func (channel *topologyChannelFake) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, arguments amqp.Table) error {
	channel.operations = append(channel.operations, topologyOperation{kind: kind, name: name, arguments: arguments})
	return nil
}

func (channel *topologyChannelFake) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, arguments amqp.Table) (amqp.Queue, error) {
	channel.operations = append(channel.operations, topologyOperation{kind: "queue", name: name, arguments: arguments})
	return amqp.Queue{Name: name}, nil
}

func (channel *topologyChannelFake) QueueBind(name, key, exchange string, noWait bool, arguments amqp.Table) error {
	channel.operations = append(channel.operations, topologyOperation{kind: "bind", name: name, exchange: exchange, routingKey: key, arguments: arguments})
	return nil
}

func TestDeclareTopologyCreatesDurableQuorumLifecycleRetryAndReplayResources(t *testing.T) {
	channel := &topologyChannelFake{}
	topology := messagingrabbitmq.Topology{Consumer: "websocket", RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, DLQRetention: 7 * 24 * time.Hour}
	if err := messagingrabbitmq.DeclareTopology(channel, topology); err != nil {
		t.Fatalf("DeclareTopology() = %v", err)
	}
	if !containsTopologyOperation(channel.operations, topologyOperation{kind: "topic", name: messagingrabbitmq.EventsExchange}) || !containsTopologyOperation(channel.operations, topologyOperation{kind: "topic", name: messagingrabbitmq.RetryExchange}) || !containsTopologyOperation(channel.operations, topologyOperation{kind: "topic", name: messagingrabbitmq.DeadLetterExchange}) || !containsTopologyOperation(channel.operations, topologyOperation{kind: "direct", name: messagingrabbitmq.ReplayExchange}) {
		t.Fatalf("exchange declarations = %#v", channel.operations)
	}
	mainQueue := messagingrabbitmq.ConsumerQueueName("websocket")
	main, found := topologyOperationFor(channel.operations, "queue", mainQueue)
	if !found || main.arguments["x-queue-type"] != "quorum" || main.arguments["x-dead-letter-exchange"] != messagingrabbitmq.DeadLetterExchange {
		t.Fatalf("main queue declaration = %#v", main)
	}
	if !containsTopologyOperation(channel.operations, topologyOperation{kind: "bind", name: mainQueue, exchange: messagingrabbitmq.EventsExchange, routingKey: "messaging.message.*.v1"}) || !containsTopologyOperation(channel.operations, topologyOperation{kind: "bind", name: mainQueue, exchange: messagingrabbitmq.EventsExchange, routingKey: "messaging.retry.websocket.*"}) || !containsTopologyOperation(channel.operations, topologyOperation{kind: "bind", name: mainQueue, exchange: messagingrabbitmq.ReplayExchange, routingKey: "consumer.websocket"}) {
		t.Fatalf("main queue bindings = %#v", channel.operations)
	}
	for attempt, delay := range topology.RetryDelays {
		queueName := messagingrabbitmq.RetryQueueName("websocket", attempt+1)
		queue, found := topologyOperationFor(channel.operations, "queue", queueName)
		if !found || queue.arguments["x-queue-type"] != "quorum" || queue.arguments["x-message-ttl"] != int32(delay.Milliseconds()) || queue.arguments["x-dead-letter-exchange"] != messagingrabbitmq.EventsExchange || queue.arguments["x-dead-letter-routing-key"] != messagingrabbitmq.RetryReturnRoutingKey("websocket", attempt+1) {
			t.Fatalf("retry queue %d declaration = %#v", attempt+1, queue)
		}
		if !containsTopologyOperation(channel.operations, topologyOperation{kind: "bind", name: queueName, exchange: messagingrabbitmq.RetryExchange, routingKey: messagingrabbitmq.RetryRoutingKey("websocket", attempt+1)}) {
			t.Fatalf("retry queue %d binding is missing", attempt+1)
		}
	}
	dlq, found := topologyOperationFor(channel.operations, "queue", messagingrabbitmq.DeadLetterQueueName("websocket"))
	if !found || dlq.arguments["x-queue-type"] != "quorum" || dlq.arguments["x-message-ttl"] != int32((7*24*time.Hour).Milliseconds()) {
		t.Fatalf("dead-letter queue declaration = %#v", dlq)
	}

	if !containsTopologyOperation(channel.operations, topologyOperation{kind: "topic", name: messagingrabbitmq.RecorderRetryExchange}) || !containsTopologyOperation(channel.operations, topologyOperation{kind: "direct", name: messagingrabbitmq.RecorderAlertExchange}) {
		t.Fatalf("recorder exchanges = %#v", channel.operations)
	}
	for attempt, delay := range topology.RetryDelays {
		queueName := messagingrabbitmq.RecorderRetryQueueName("websocket", attempt+1)
		queue, found := topologyOperationFor(channel.operations, "queue", queueName)
		expectedExchange := messagingrabbitmq.DeadLetterExchange
		if attempt+1 == len(topology.RetryDelays) {
			expectedExchange = messagingrabbitmq.RecorderAlertExchange
		}
		if !found || queue.arguments["x-queue-type"] != "quorum" || queue.arguments["x-message-ttl"] != int32(delay.Milliseconds()) || queue.arguments["x-dead-letter-exchange"] != expectedExchange || !containsTopologyOperation(channel.operations, topologyOperation{kind: "bind", name: queueName, exchange: messagingrabbitmq.RecorderRetryExchange, routingKey: messagingrabbitmq.RecorderRetryRoutingKey("websocket", attempt+1)}) {
			t.Fatalf("recorder retry queue %d = %#v", attempt+1, queue)
		}
	}
	alert, found := topologyOperationFor(channel.operations, "queue", messagingrabbitmq.RecorderAlertQueueName("websocket"))
	if !found || alert.arguments["x-queue-type"] != "quorum" {
		t.Fatalf("recorder alert queue = %#v", alert)
	}
}

func topologyOperationFor(operations []topologyOperation, kind, name string) (topologyOperation, bool) {
	for _, operation := range operations {
		if operation.kind == kind && operation.name == name {
			return operation, true
		}
	}
	return topologyOperation{}, false
}

func containsTopologyOperation(operations []topologyOperation, expected topologyOperation) bool {
	for _, operation := range operations {
		if operation.kind == expected.kind && operation.name == expected.name && operation.exchange == expected.exchange && operation.routingKey == expected.routingKey {
			return true
		}
	}
	return false
}
