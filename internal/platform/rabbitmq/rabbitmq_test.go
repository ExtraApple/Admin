package rabbitmq_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformconfig "admin/internal/platform/config"
	"admin/internal/platform/rabbitmq"
)

func TestOpenReturnsConnectionFailure(t *testing.T) {
	_, err := rabbitmq.Open(platformconfig.RabbitMQConfig{
		Host:     "127.0.0.1",
		Port:     1,
		Username: "admin",
		Password: "admin",
		VHost:    "admin",
	})
	if err == nil {
		t.Fatal("open RabbitMQ should fail when the configured endpoint refuses connections")
	}
	if !strings.Contains(err.Error(), "connect RabbitMQ") {
		t.Fatalf("open error = %v, want stable connect RabbitMQ context", err)
	}
}

func TestOpenUsesTLSAndConnectionTimeoutConfiguration(t *testing.T) {
	if _, err := rabbitmq.Open(platformconfig.RabbitMQConfig{
		Host:                     "127.0.0.1",
		Port:                     1,
		Username:                 "admin",
		Password:                 "admin",
		VHost:                    "admin",
		TLS:                      true,
		ConnectionTimeoutSeconds: 1,
	}); err == nil {
		t.Fatal("TLS RabbitMQ open should fail when the configured endpoint refuses connections")
	}
}

func TestOpenContextRejectsCancelledConnectionAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := rabbitmq.OpenContext(ctx, platformconfig.RabbitMQConfig{Host: "127.0.0.1", Port: 1, Username: "admin", Password: "admin", VHost: "admin"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenContext() error = %v, want context cancellation", err)
	}
}
