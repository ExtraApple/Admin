package rabbitmq_test

import (
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
