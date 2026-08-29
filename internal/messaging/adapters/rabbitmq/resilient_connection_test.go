package rabbitmq_test

import (
	"context"
	"errors"
	"testing"
	"time"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type connectionOpenerFake struct {
	calls int
	err   error
}

func (opener *connectionOpenerFake) Open(context.Context) (*amqp.Connection, error) {
	opener.calls++
	return nil, opener.err
}

func TestResilientChannelFactoryDefersBrokerConnectionUntilAChannelIsNeeded(t *testing.T) {
	opener := &connectionOpenerFake{err: errors.New("broker unavailable")}
	factory := messagingrabbitmq.NewResilientChannelFactory(opener.Open, messagingrabbitmq.Topology{Consumer: "websocket", RetryDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}, DLQRetention: 7 * 24 * time.Hour})
	if opener.calls != 0 {
		t.Fatalf("constructor opened broker %d times", opener.calls)
	}
	channel, err := factory.OpenPublisherChannel(context.Background())
	if err == nil || channel != nil || opener.calls != 1 {
		t.Fatalf("OpenPublisherChannel() channel=%#v err=%v calls=%d", channel, err, opener.calls)
	}
	if err := factory.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
}
