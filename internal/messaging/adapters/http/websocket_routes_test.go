package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"github.com/gin-gonic/gin"
)

type httpWebSocketTicketStoreFake struct {
	owner    uint
	found    bool
	stored   uint
	consumed int
}

func (store *httpWebSocketTicketStoreFake) StoreWebSocketTicket(_ context.Context, _ string, userID uint, _ time.Duration) error {
	store.stored = userID
	return nil
}
func (store *httpWebSocketTicketStoreFake) ConsumeWebSocketTicket(context.Context, string) (uint, bool, error) {
	store.consumed++
	return store.owner, store.found, nil
}

type httpWebSocketUserGuardFake struct {
	active  bool
	err     error
	checked uint
}

func (guard *httpWebSocketUserGuardFake) IsActive(_ context.Context, userID uint) (bool, error) {
	guard.checked = userID
	return guard.active, guard.err
}

type httpGatewayConnectionFake struct {
	closed chan struct{}
}

func newHTTPGatewayConnectionFake() *httpGatewayConnectionFake {
	return &httpGatewayConnectionFake{closed: make(chan struct{})}
}
func (connection *httpGatewayConnectionFake) Read(context.Context) ([]byte, error) {
	<-connection.closed
	return nil, errors.New("closed")
}
func (*httpGatewayConnectionFake) Write(context.Context, []byte) error {
	return nil
}
func (connection *httpGatewayConnectionFake) Close(string) error {
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
	descriptor := findMessageRoute(t, WebSocketRoutes(tickets, nil, "https://admin.example.test"), http.MethodPost, "/api/user/messages/ws-ticket")
	context, response := newGinRequest(t, http.MethodPost, "/api/user/messages/ws-ticket", "")
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if response.Code != http.StatusOK || response.Body.String() == "" || store.stored != 7 {
		t.Fatalf("ticket response=%d body=%s storedUser=%d", response.Code, response.Body.String(), store.stored)
	}
}

func TestWebSocketUpgradeHandlerRejectsInvalidTicketBeforeProtocolUpgrade(t *testing.T) {
	accepted := false
	store := &httpWebSocketTicketStoreFake{found: false}
	tickets := application.NewWebSocketTicketService(store)
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets: tickets,
		Hub:     application.NewRefreshHub(),
		Users:   &httpWebSocketUserGuardFake{active: true},
	})
	accept := func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
		accepted = true
		return newHTTPGatewayConnectionFake(), nil
	}
	descriptor := findMessageRoute(t, webSocketRoutes(tickets, gateway, "https://admin.example.test", accept), http.MethodGet, "/api/user/messages/ws")
	context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/ws?ticket=secret-ticket", "")
	descriptor.Handler(context)
	if response.Code != http.StatusUnauthorized || accepted || store.consumed != 1 || !strings.Contains(response.Body.String(), CodeTicketInvalid) || containsSensitiveTicket(response.Body.String()) {
		t.Fatalf("invalid upgrade response=%d accepted=%t consumed=%d body=%s", response.Code, accepted, store.consumed, response.Body.String())
	}
}

func TestWebSocketUpgradeHandlerPassesValidTicketToGateway(t *testing.T) {
	connection := newHTTPGatewayConnectionFake()
	accepted := false
	guard := &httpWebSocketUserGuardFake{active: true}
	tickets := application.NewWebSocketTicketService(&httpWebSocketTicketStoreFake{owner: 7, found: true})
	gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
		Tickets: tickets,
		Hub:     application.NewRefreshHub(),
		Users:   guard,
	})
	accept := func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
		accepted = true
		return connection, nil
	}
	descriptor := findMessageRoute(t, webSocketRoutes(tickets, gateway, "https://admin.example.test", accept), http.MethodGet, "/api/user/messages/ws")
	context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/ws?ticket=valid-ticket&cursor=cursor-1", "")
	descriptor.Handler(context)
	if !accepted || guard.checked != 7 || response.Body.Len() != 0 {
		t.Fatalf("valid upgrade accepted=%t checked=%d response=%d body=%s", accepted, guard.checked, response.Code, response.Body.String())
	}
	_ = connection.Close("test complete")
}

func TestWebSocketUpgradeHandlerAllowsOnlyExactConfiguredOrigin(t *testing.T) {
	for name, test := range map[string]struct {
		origin   string
		accepted bool
	}{
		"vite loopback proxy": {"http://localhost:15173", true},
		"untrusted origin":    {"https://evil.example.test", false},
		"same host mismatch":  {"https://admin.example.test.evil", false},
	} {
		t.Run(name, func(t *testing.T) {
			connection := newHTTPGatewayConnectionFake()
			accepted := false
			store := &httpWebSocketTicketStoreFake{owner: 7, found: true}
			tickets := application.NewWebSocketTicketService(store)
			gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
				Tickets: tickets,
				Hub:     application.NewRefreshHub(),
				Users:   &httpWebSocketUserGuardFake{active: true},
			})
			accept := func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
				accepted = true
				return connection, nil
			}
			descriptor := findMessageRoute(t, webSocketRoutes(tickets, gateway, "http://localhost:15173", accept), http.MethodGet, "/api/user/messages/ws")
			context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/ws?ticket=valid-ticket", "")
			context.Request.Header.Set("Origin", test.origin)
			descriptor.Handler(context)
			if accepted != test.accepted {
				t.Fatalf("accepted=%t want=%t response=%d body=%s", accepted, test.accepted, response.Code, response.Body.String())
			}
			if test.accepted {
				if response.Body.Len() != 0 || store.consumed != 1 {
					t.Fatalf("allowed response=%d body=%s consumed=%d", response.Code, response.Body.String(), store.consumed)
				}
				_ = connection.Close("test complete")
				return
			}
			if response.Code != http.StatusForbidden || store.consumed != 0 || !strings.Contains(response.Body.String(), CodePermissionDenied) {
				t.Fatalf("rejected response=%d consumed=%d body=%s", response.Code, store.consumed, response.Body.String())
			}
		})
	}
}

func TestWebSocketUpgradeHandlerRejectsDisabledOrUnverifiableUser(t *testing.T) {
	for name, test := range map[string]struct {
		guard  *httpWebSocketUserGuardFake
		status int
		code   string
	}{
		"disabled":        {&httpWebSocketUserGuardFake{active: false}, http.StatusForbidden, CodePermissionDenied},
		"dependency fail": {&httpWebSocketUserGuardFake{err: errors.New("identity unavailable")}, http.StatusServiceUnavailable, CodeWebSocketUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			accepted := false
			tickets := application.NewWebSocketTicketService(&httpWebSocketTicketStoreFake{owner: 7, found: true})
			gateway := application.NewWebSocketGateway(application.WebSocketGatewayConfig{
				Tickets: tickets,
				Hub:     application.NewRefreshHub(),
				Users:   test.guard,
			})
			descriptor := findMessageRoute(t, webSocketRoutes(tickets, gateway, "https://admin.example.test", func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error) {
				accepted = true
				return newHTTPGatewayConnectionFake(), nil
			}), http.MethodGet, "/api/user/messages/ws")
			context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/ws?ticket=valid-ticket", "")
			descriptor.Handler(context)
			if accepted || response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("guard response=%d accepted=%t body=%s", response.Code, accepted, response.Body.String())
			}
		})
	}
}

func containsSensitiveTicket(body string) bool {
	return strings.Contains(body, "secret-ticket")
}
