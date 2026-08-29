package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	messagingapplication "admin/internal/messaging/application"
	messagingdomain "admin/internal/messaging/domain"
)

type readinessBrokerFake struct {
	healthy bool
	error   string
}

func (broker readinessBrokerFake) Healthy() bool         { return broker.healthy }
func (broker readinessBrokerFake) LastErrorCode() string { return broker.error }

type readinessOutboxFake struct {
	pending int64
	err     error
}

func (store readinessOutboxFake) ListOutboxes(context.Context, messagingapplication.OutboxListQuery) ([]messagingdomain.MessageOutbox, int64, error) {
	return nil, store.pending, store.err
}

func TestMessagingReadinessExposesOnlySafeBrokerAndOutboxState(t *testing.T) {
	readiness := newMessagingReadiness(true, readinessBrokerFake{healthy: false, error: "rabbitmq_unavailable"}, readinessOutboxFake{pending: 3})
	response := readiness.Snapshot(context.Background())
	if response.Status != "degraded" || response.Components.RabbitMQ.Status != "degraded" || response.Components.RabbitMQ.OutboxPending != 3 || response.Components.RabbitMQ.LastErrorCode != "rabbitmq_unavailable" {
		t.Fatalf("response = %#v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for _, forbidden := range []string{"127.0.0.1", "password", "credential", "consumer", "dead_letter", "timestamp"} {
		if string(encoded) != "" && contains(string(encoded), forbidden) {
			t.Fatalf("response exposes %q: %s", forbidden, encoded)
		}
	}
}

func TestMessagingReadinessTreatsDisabledBrokerAsReadyWithoutOutboxErrorLeak(t *testing.T) {
	readiness := newMessagingReadiness(false, nil, readinessOutboxFake{err: errors.New("database connection details")})
	response := readiness.Snapshot(context.Background())
	if response.Status != "ready" || response.Components.RabbitMQ.Status != "disabled" || response.Components.RabbitMQ.OutboxPending != 0 || response.Components.RabbitMQ.LastErrorCode != "" {
		t.Fatalf("response = %#v", response)
	}
}

func contains(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
