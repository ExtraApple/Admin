package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ConnectionOpener func(context.Context) (*amqp.Connection, error)

type ResilientChannelFactory struct {
	open     ConnectionOpener
	topology Topology

	mu         sync.Mutex
	connection *amqp.Connection
}

func NewResilientChannelFactory(open ConnectionOpener, topology Topology) *ResilientChannelFactory {
	return &ResilientChannelFactory{open: open, topology: topology}
}

func (factory *ResilientChannelFactory) OpenPublisherChannel(ctx context.Context) (PublisherChannel, error) {
	connection, err := factory.connectionFor(ctx)
	if err != nil {
		return nil, err
	}
	channel, err := connection.Channel()
	if err != nil {
		return nil, fmt.Errorf("open RabbitMQ publisher channel: %w", err)
	}
	return amqpPublisherChannel{channel: channel}, nil
}

func (factory *ResilientChannelFactory) OpenConsumerChannel(ctx context.Context) (ConsumerChannel, error) {
	connection, err := factory.connectionFor(ctx)
	if err != nil {
		return nil, err
	}
	channel, err := connection.Channel()
	if err != nil {
		return nil, fmt.Errorf("open RabbitMQ Consumer channel: %w", err)
	}
	return amqpConsumerChannel{channel: channel}, nil
}

func (factory *ResilientChannelFactory) Close() error {
	if factory == nil {
		return nil
	}
	factory.mu.Lock()
	connection := factory.connection
	factory.connection = nil
	factory.mu.Unlock()
	if connection == nil || connection.IsClosed() {
		return nil
	}
	return connection.Close()
}

func (factory *ResilientChannelFactory) connectionFor(ctx context.Context) (*amqp.Connection, error) {
	if factory == nil || factory.open == nil {
		return nil, errors.New("RabbitMQ connection opener is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	factory.mu.Lock()
	defer factory.mu.Unlock()
	if factory.connection != nil && !factory.connection.IsClosed() {
		return factory.connection, nil
	}
	connection, err := factory.open(ctx)
	if err != nil {
		return nil, fmt.Errorf("open RabbitMQ connection: %w", err)
	}
	if connection == nil || connection.IsClosed() {
		return nil, errors.New("open RabbitMQ connection returned unavailable connection")
	}
	channel, err := connection.Channel()
	if err == nil {
		err = DeclareTopology(channel, factory.topology)
	}
	if closeErr := channelClose(channel); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("declare RabbitMQ messaging topology: %w", err)
	}
	factory.connection = connection
	return connection, nil
}

func channelClose(channel *amqp.Channel) error {
	if channel == nil {
		return nil
	}
	return channel.Close()
}

var _ PublisherChannelFactory = (*ResilientChannelFactory)(nil)
var _ ConsumerChannelFactory = (*ResilientChannelFactory)(nil)
