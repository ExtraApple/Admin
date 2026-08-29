package rabbitmq

import (
	"context"
	"errors"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConnectionPublisherChannelFactory adapts one live AMQP connection without
// exposing its channel to Messaging application code. Reconnection policy is
// owned by the composition root/factory that supplies the connection.
type ConnectionPublisherChannelFactory struct {
	Connection *amqp.Connection
}

func (factory ConnectionPublisherChannelFactory) OpenPublisherChannel(context.Context) (PublisherChannel, error) {
	if factory.Connection == nil || factory.Connection.IsClosed() {
		return nil, errors.New("RabbitMQ connection is unavailable")
	}
	channel, err := factory.Connection.Channel()
	if err != nil {
		return nil, err
	}
	return amqpPublisherChannel{channel: channel}, nil
}

type amqpPublisherChannel struct {
	channel *amqp.Channel
}

func (channel amqpPublisherChannel) Confirm(noWait bool) error {
	return channel.channel.Confirm(noWait)
}

func (channel amqpPublisherChannel) NotifyPublish(confirmations chan amqp.Confirmation) chan amqp.Confirmation {
	return channel.channel.NotifyPublish(confirmations)
}

func (channel amqpPublisherChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, publishing amqp.Publishing) error {
	return channel.channel.PublishWithContext(ctx, exchange, key, mandatory, immediate, publishing)
}

func (channel amqpPublisherChannel) Close() error {
	return channel.channel.Close()
}

type ConnectionConsumerChannelFactory struct {
	Connection *amqp.Connection
}

func (factory ConnectionConsumerChannelFactory) OpenConsumerChannel(context.Context) (ConsumerChannel, error) {
	if factory.Connection == nil || factory.Connection.IsClosed() {
		return nil, errors.New("RabbitMQ connection is unavailable")
	}
	channel, err := factory.Connection.Channel()
	if err != nil {
		return nil, err
	}
	return amqpConsumerChannel{channel: channel}, nil
}

type amqpConsumerChannel struct {
	channel *amqp.Channel
}

func (channel amqpConsumerChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	return channel.channel.Qos(prefetchCount, prefetchSize, global)
}

func (channel amqpConsumerChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, arguments amqp.Table) (<-chan amqp.Delivery, error) {
	return channel.channel.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, arguments)
}

func (channel amqpConsumerChannel) Ack(tag uint64, multiple bool) error {
	return channel.channel.Ack(tag, multiple)
}

func (channel amqpConsumerChannel) Nack(tag uint64, multiple, requeue bool) error {
	return channel.channel.Nack(tag, multiple, requeue)
}

func (channel amqpConsumerChannel) Close() error {
	return channel.channel.Close()
}
