package rabbitmq_test

import (
	"context"
	"testing"

	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
)

func TestConnectionConsumerChannelFactoryRejectsUnavailableConnection(t *testing.T) {
	factory := messagingrabbitmq.ConnectionConsumerChannelFactory{}
	channel, err := factory.OpenConsumerChannel(context.Background())
	if err == nil || channel != nil {
		t.Fatalf("OpenConsumerChannel() channel=%#v err=%v", channel, err)
	}
}
