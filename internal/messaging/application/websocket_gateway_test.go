package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"admin/internal/messaging/application"
	websocket "github.com/coder/websocket"
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

func (connection *gatewayConnectionFake) Read(context.Context) (websocket.MessageType, []byte, error) {
	<-connection.closed
	return websocket.MessageText, nil, errors.New("connection closed")
}

func (connection *gatewayConnectionFake) Write(ctx context.Context, _ websocket.MessageType, data []byte) error {
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

func (connection *gatewayConnectionFake) Close(websocket.StatusCode, string) error {
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
		Accept: func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
			accepted = true
			return connection, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/user/messages/ws?ticket=opaque-ticket&cursor=cursor-1", nil)
	response := httptest.NewRecorder()
	if err := gateway.Upgrade(context.Background(), response, request, 7); err != nil {
		t.Fatalf("Upgrade() = %v", err)
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
	connection.Close(websocket.StatusNormalClosure, "test complete")
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
		Accept: func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
			return connection, nil
		},
	})
	if err := gateway.Upgrade(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws?ticket=opaque-ticket", nil), 7); err != nil {
		t.Fatalf("Upgrade() = %v", err)
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
		Accept: func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
			accepted = true
			return newGatewayConnectionFake(), nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/user/messages/ws?ticket=opaque-ticket", nil)
	if err := gateway.Upgrade(context.Background(), httptest.NewRecorder(), request, 7); !errors.Is(err, application.ErrWebSocketTicketInvalid) {
		t.Fatalf("invalid ticket error = %v", err)
	}
	if accepted {
		t.Fatal("invalid ticket reached WebSocket upgrade")
	}
}
