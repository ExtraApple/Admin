package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type gatewayConnectionFake struct {
	mu           sync.Mutex
	writes       []map[string]any
	writeCh      chan struct{}
	writeStarted chan struct{}
	closed       chan struct{}
	closeOnce    sync.Once
}

func newGatewayConnectionFake() *gatewayConnectionFake {
	return &gatewayConnectionFake{writeCh: make(chan struct{}, 10), closed: make(chan struct{})}
}

func (connection *gatewayConnectionFake) Read(context.Context) ([]byte, error) {
	<-connection.closed
	return nil, errors.New("connection closed")
}

func (connection *gatewayConnectionFake) Write(ctx context.Context, data []byte) error {
	if connection.writeStarted != nil {
		select {
		case connection.writeStarted <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
	var message map[string]any
	if err := json.Unmarshal(data, &message); err != nil {
		return err
	}
	connection.mu.Lock()
	connection.writes = append(connection.writes, message)
	connection.mu.Unlock()
	connection.writeCh <- struct{}{}
	return nil
}

func (connection *gatewayConnectionFake) Close(string) error {
	connection.closeOnce.Do(func() { close(connection.closed) })
	return nil
}

func (connection *gatewayConnectionFake) writtenMessages() []map[string]any {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return append([]map[string]any(nil), connection.writes...)
}

type gatewayRecoveryStoreFake struct {
	recovery application.RefreshRecovery
	userID   uint
	cursor   string
}

func (store *gatewayRecoveryStoreFake) ReplayRefresh(_ context.Context, userID uint, cursor string) (application.RefreshRecovery, error) {
	store.userID = userID
	store.cursor = cursor
	return store.recovery, nil
}

func TestWebSocketGatewayConsumesTicketBindsUserAndBootstrapsRecovery(t *testing.T) {
	ticketStore := &webSocketTicketStoreFake{owner: 7, found: true}
	connection := newGatewayConnectionFake()
	recovery := &gatewayRecoveryStoreFake{recovery: application.RefreshRecovery{Events: []application.RefreshEvent{{EventID: "event-1", Cursor: "cursor-2", MessageCopyID: 41, UserID: 7, AggregateVersion: 2}}}}
	accepted := false
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets:  application.NewWebSocketTicketService(ticketStore),
		Hub:      application.NewRefreshHub(),
		Recovery: recovery,
	})
	if err := gateway.Connect(context.Background(), application.WebSocketRequest{Ticket: "opaque-ticket", Cursor: "cursor-1"}, 7, func(context.Context) (application.WebSocketConnection, error) {
		accepted = true
		return connection, nil
	}); err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	select {
	case <-connection.writeCh:
	case <-time.After(time.Second):
		t.Fatal("recovery event was not written")
	}
	messages := connection.writtenMessages()
	if !accepted || recovery.userID != 7 || recovery.cursor != "cursor-1" || len(messages) != 1 || messages[0]["type"] != "inbox.refresh" || messages[0]["event_id"] != "event-1" || messages[0]["cursor"] != "cursor-2" || messages[0]["message_copy_id"] != float64(41) {
		t.Fatalf("gateway state accepted=%t recovery=%#v messages=%#v", accepted, recovery, messages)
	}
	for _, forbidden := range []string{"body", "html", "markdown", "token", "image", "url"} {
		if _, exists := messages[0][forbidden]; exists {
			t.Fatalf("gateway payload includes forbidden field %q: %#v", forbidden, messages[0])
		}
	}
	_ = connection.Close("test complete")
}

type transportNeutralGatewayConnectionFake struct{}

func (transportNeutralGatewayConnectionFake) Read(context.Context) ([]byte, error) {
	return nil, errors.New("closed")
}
func (transportNeutralGatewayConnectionFake) Write(context.Context, []byte) error { return nil }
func (transportNeutralGatewayConnectionFake) Close(string) error                  { return nil }

func TestWebSocketGatewayConnectUsesTransportNeutralAcceptor(t *testing.T) {
	accepted := false
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets: application.NewWebSocketTicketService(&webSocketTicketStoreFake{owner: 7, found: true}),
		Hub:     application.NewRefreshHub(),
	})
	err := gateway.Connect(context.Background(), application.WebSocketRequest{Ticket: "opaque-ticket"}, 7, func(context.Context) (application.WebSocketConnection, error) {
		accepted = true
		return transportNeutralGatewayConnectionFake{}, nil
	})
	if err != nil || !accepted {
		t.Fatalf("Connect() error=%v accepted=%t", err, accepted)
	}
}

func TestWebSocketGatewayClosesSlowConnectionWhenQueueFills(t *testing.T) {
	connection := newGatewayConnectionFake()
	connection.writeStarted = make(chan struct{}, 1)
	hub := application.NewRefreshHub()
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets:      application.NewWebSocketTicketService(&webSocketTicketStoreFake{owner: 7, found: true}),
		Hub:          hub,
		QueueSize:    1,
		WriteTimeout: 20 * time.Millisecond,
	})
	if err := gateway.Connect(context.Background(), application.WebSocketRequest{Ticket: "opaque-ticket"}, 7, func(context.Context) (application.WebSocketConnection, error) {
		return connection, nil
	}); err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	event := application.RefreshEvent{EventID: "event-queue", Cursor: "cursor-queue", MessageCopyID: 41, UserID: 7, AggregateVersion: 1}
	hub.Publish(context.Background(), event)
	select {
	case <-connection.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("slow write did not start")
	}
	hub.Publish(context.Background(), event)
	hub.Publish(context.Background(), event)
	select {
	case <-connection.closed:
	case <-time.After(time.Second):
		t.Fatal("slow connection was not closed")
	}
}

func TestWebSocketGatewayRejectsInvalidTicketBeforeUpgrade(t *testing.T) {
	accepted := false
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets: application.NewWebSocketTicketService(&webSocketTicketStoreFake{owner: 8, found: true}),
		Hub:     application.NewRefreshHub(),
	})
	if err := gateway.Connect(context.Background(), application.WebSocketRequest{Ticket: "opaque-ticket"}, 7, func(context.Context) (application.WebSocketConnection, error) {
		accepted = true
		return newGatewayConnectionFake(), nil
	}); !errors.Is(err, application.ErrWebSocketTicketInvalid) {
		t.Fatalf("invalid ticket error = %v", err)
	}
	if accepted {
		t.Fatal("invalid ticket reached WebSocket upgrade")
	}
}
