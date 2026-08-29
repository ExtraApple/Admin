package rabbitmq

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	platformconfig "admin/internal/platform/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Open(conf platformconfig.RabbitMQConfig) (*amqp.Connection, error) {
	return OpenContext(context.Background(), conf)
}

func OpenContext(ctx context.Context, conf platformconfig.RabbitMQConfig) (*amqp.Connection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vhost := strings.TrimPrefix(conf.VHost, "/")
	scheme := "amqp"
	if conf.TLS {
		scheme = "amqps"
	}
	endpoint := (&url.URL{
		Scheme:  scheme,
		User:    url.UserPassword(conf.Username, conf.Password),
		Host:    net.JoinHostPort(conf.Host, strconv.Itoa(conf.Port)),
		Path:    "/" + vhost,
		RawPath: "/" + url.PathEscape(vhost),
	}).String()

	dialTimeout := time.Duration(conf.ConnectionTimeoutSeconds) * time.Second
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	clientTLS, err := tlsConfig(conf)
	if err != nil {
		return nil, fmt.Errorf("configure RabbitMQ TLS: %w", err)
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	connection, err := amqp.DialConfig(endpoint, amqp.Config{
		TLSClientConfig: clientTLS,
		Dial: func(network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	return connection, nil
}

func tlsConfig(conf platformconfig.RabbitMQConfig) (*tls.Config, error) {
	if !conf.TLS {
		return nil, nil
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: conf.ServerName}
	if conf.CAFile == "" {
		return config, nil
	}
	data, err := os.ReadFile(conf.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("CA file contains no certificates")
	}
	config.RootCAs = pool
	return config, nil
}
