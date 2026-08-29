//go:build rabbitmq_integration

package rabbitmq_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	"admin/internal/messaging/domain"
	platformconfig "admin/internal/platform/config"
	platformrabbitmq "admin/internal/platform/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQMessagingIntegrationDeclaresQuorumTopologyAndConfirmsPublish(t *testing.T) {
	config := integrationRabbitMQConfig(t)
	consumer := "integration-" + uuid.NewString()
	topology := messagingrabbitmq.Topology{Consumer: consumer, RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, DLQRetention: time.Hour}

	connection, err := platformrabbitmq.Open(config)
	if err != nil {
		t.Fatalf("open RabbitMQ: %v", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		t.Fatalf("open RabbitMQ topology channel: %v", err)
	}
	if err := messagingrabbitmq.DeclareTopology(channel, topology); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		t.Fatalf("declare messaging topology: %v", err)
	}
	_ = channel.Close()

	factory := messagingrabbitmq.NewResilientChannelFactory(func(ctx context.Context) (*amqp.Connection, error) {
		return platformrabbitmq.OpenContext(ctx, config)
	}, topology)
	defer factory.Close()
	publisher := messagingrabbitmq.NewPublisher(factory)
	event := domain.MessageEvent{EventID: "integration-" + uuid.NewString(), EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, AggregateVersion: 1, OccurredAt: time.Now().UTC()}
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish with RabbitMQ confirm: %v", err)
	}

	cleanupChannel, err := connection.Channel()
	if err != nil {
		t.Fatalf("open RabbitMQ cleanup channel: %v", err)
	}
	for _, queue := range integrationMessagingQueues(consumer) {
		if _, err := cleanupChannel.QueueDelete(queue, false, false, false); err != nil {
			t.Errorf("delete integration queue %q: %v", queue, err)
		}
	}
	_ = cleanupChannel.Close()
	_ = connection.Close()
}

func integrationRabbitMQConfig(t *testing.T) platformconfig.RabbitMQConfig {
	t.Helper()
	port, err := strconv.Atoi(requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_PORT"))
	if err != nil || port < 1 {
		t.Fatalf("parse ADMIN_TEST_RABBITMQ_PORT: %q", os.Getenv("ADMIN_TEST_RABBITMQ_PORT"))
	}
	tlsEnabled, err := strconv.ParseBool(os.Getenv("ADMIN_TEST_RABBITMQ_TLS"))
	if err != nil && os.Getenv("ADMIN_TEST_RABBITMQ_TLS") != "" {
		t.Fatalf("parse ADMIN_TEST_RABBITMQ_TLS: %v", err)
	}
	return platformconfig.RabbitMQConfig{Host: requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_HOST"), Port: port, Username: requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_USERNAME"), Password: requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_PASSWORD"), VHost: requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_VHOST"), TLS: tlsEnabled, CAFile: os.Getenv("ADMIN_TEST_RABBITMQ_CA_FILE"), ServerName: os.Getenv("ADMIN_TEST_RABBITMQ_SERVER_NAME"), ConnectionTimeoutSeconds: 5, ConfirmTimeoutSeconds: 10}
}

func requiredRabbitMQEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required for the explicit RabbitMQ integration gate", name)
	}
	return value
}

func integrationMessagingQueues(consumer string) []string {
	queues := []string{messagingrabbitmq.ConsumerQueueName(consumer)}
	for attempt := 1; attempt <= 5; attempt++ {
		queues = append(queues, messagingrabbitmq.RetryQueueName(consumer, attempt))
	}
	queues = append(queues, messagingrabbitmq.DeadLetterQueueName(consumer))
	for attempt := 2; attempt <= 5; attempt++ {
		queues = append(queues, messagingrabbitmq.ConsumerDeadLetterAttemptQueueName(consumer, attempt))
	}
	for attempt := 1; attempt <= 5; attempt++ {
		queues = append(queues, messagingrabbitmq.RecorderRetryQueueName(consumer, attempt))
	}
	queues = append(queues, messagingrabbitmq.RecorderAlertQueueName(consumer))
	return queues
}
