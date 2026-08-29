package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
	websocket "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type httpWebSocketTicketStoreFake struct {
	owner  uint
	found  bool
	stored uint
}

func (store *httpWebSocketTicketStoreFake) StoreWebSocketTicket(_ context.Context, _ string, userID uint, _ time.Duration) error {
	store.stored = userID
	return nil
}
func (store *httpWebSocketTicketStoreFake) ConsumeWebSocketTicket(context.Context, string) (uint, bool, error) {
	return store.owner, store.found, nil
}

type httpGatewayConnectionFake struct {
	closed chan struct{}
}

func newHTTPGatewayConnectionFake() *httpGatewayConnectionFake {
	return &httpGatewayConnectionFake{closed: make(chan struct{})}
}
func (connection *httpGatewayConnectionFake) Read(context.Context) (websocket.MessageType, []byte, error) {
	<-connection.closed
	return websocket.MessageText, nil, errors.New("closed")
}
func (*httpGatewayConnectionFake) Write(context.Context, websocket.MessageType, []byte) error {
	return nil
}
func (connection *httpGatewayConnectionFake) Close(websocket.StatusCode, string) error {
	select {
	case <-connection.closed:
	default:
		close(connection.closed)
	}
	return nil
}

func TestWebSocketTicketHandlerIssuesUserBoundTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &httpWebSocketTicketStoreFake{}
	tickets := application.NewWebSocketTicketService(store)
	descriptor := findMessageRoute(t, WebSocketRoutes(tickets, nil), http.MethodPost, "/api/user/messages/ws-ticket")
	context, response := newGinRequest(t, http.MethodPost, "/api/user/messages/ws-ticket", "")
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if response.Code != http.StatusOK || response.Body.String() == "" || store.stored != 7 {
		t.Fatalf("ticket response=%d body=%s storedUser=%d", response.Code, response.Body.String(), store.stored)
	}
}

func TestWebSocketUpgradeHandlerRejectsInvalidTicketBeforeProtocolUpgrade(t *testing.T) {
	accepted := false
	tickets := application.NewWebSocketTicketService(&httpWebSocketTicketStoreFake{owner: 8, found: true})
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets: tickets,
		Hub:     application.NewRefreshHub(),
		Accept: func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
			accepted = true
			return newHTTPGatewayConnectionFake(), nil
		},
	})
	descriptor := findMessageRoute(t, WebSocketRoutes(tickets, gateway), http.MethodGet, "/api/user/messages/ws")
	context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/ws?ticket=secret-ticket", "")
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if response.Code != http.StatusUnauthorized || accepted || containsSensitiveTicket(response.Body.String()) {
		t.Fatalf("invalid upgrade response=%d accepted=%t body=%s", response.Code, accepted, response.Body.String())
	}
}

func TestWebSocketUpgradeHandlerPassesValidTicketToGateway(t *testing.T) {
	connection := newHTTPGatewayConnectionFake()
	accepted := false
	tickets := application.NewWebSocketTicketService(&httpWebSocketTicketStoreFake{owner: 7, found: true})
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets: tickets,
		Hub:     application.NewRefreshHub(),
		Accept: func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
			accepted = true
			return connection, nil
		},
	})
	descriptor := findMessageRoute(t, WebSocketRoutes(tickets, gateway), http.MethodGet, "/api/user/messages/ws")
	context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/ws?ticket=valid-ticket&cursor=cursor-1", "")
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if !accepted || response.Body.Len() != 0 {
		t.Fatalf("valid upgrade accepted=%t response=%d body=%s", accepted, response.Code, response.Body.String())
	}
	_ = connection.Close(websocket.StatusNormalClosure, "test complete")
}

func containsSensitiveTicket(body string) bool {
	return strings.Contains(body, "secret-ticket")
}
