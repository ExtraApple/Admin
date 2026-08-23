package rabbitmq

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	platformconfig "admin/internal/platform/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Open(conf platformconfig.RabbitMQConfig) (*amqp.Connection, error) {
	vhost := strings.TrimPrefix(conf.VHost, "/")
	endpoint := (&url.URL{
		Scheme:  "amqp",
		User:    url.UserPassword(conf.Username, conf.Password),
		Host:    net.JoinHostPort(conf.Host, strconv.Itoa(conf.Port)),
		Path:    "/" + vhost,
		RawPath: "/" + url.PathEscape(vhost),
	}).String()

	connection, err := amqp.Dial(endpoint)
	if err != nil {
		return nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	return connection, nil
}
