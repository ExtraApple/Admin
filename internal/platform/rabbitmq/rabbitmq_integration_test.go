//go:build rabbitmq_integration

package rabbitmq_test

import (
	"os"
	"strconv"
	"testing"

	platformconfig "admin/internal/platform/config"
	"admin/internal/platform/rabbitmq"
)

func TestRabbitMQIntegrationOpensConfiguredBroker(t *testing.T) {
	port, err := strconv.Atoi(requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_PORT"))
	if err != nil || port < 1 {
		t.Fatalf("parse ADMIN_TEST_RABBITMQ_PORT: %q", os.Getenv("ADMIN_TEST_RABBITMQ_PORT"))
	}
	tlsEnabled, err := strconv.ParseBool(os.Getenv("ADMIN_TEST_RABBITMQ_TLS"))
	if err != nil && os.Getenv("ADMIN_TEST_RABBITMQ_TLS") != "" {
		t.Fatalf("parse ADMIN_TEST_RABBITMQ_TLS: %v", err)
	}
	connection, err := rabbitmq.Open(platformconfig.RabbitMQConfig{
		Host:                     requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_HOST"),
		Port:                     port,
		Username:                 requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_USERNAME"),
		Password:                 requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_PASSWORD"),
		VHost:                    requiredRabbitMQEnv(t, "ADMIN_TEST_RABBITMQ_VHOST"),
		TLS:                      tlsEnabled,
		CAFile:                   os.Getenv("ADMIN_TEST_RABBITMQ_CA_FILE"),
		ServerName:               os.Getenv("ADMIN_TEST_RABBITMQ_SERVER_NAME"),
		ConnectionTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("open configured RabbitMQ: %v", err)
	}
	defer connection.Close()
	if connection.IsClosed() {
		t.Fatal("RabbitMQ connection is closed immediately after opening")
	}
}

func requiredRabbitMQEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required for the explicit RabbitMQ integration gate", name)
	}
	return value
}
