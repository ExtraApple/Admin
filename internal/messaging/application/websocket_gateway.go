package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	websocket "github.com/coder/websocket"
)

const (
	DefaultWebSocketQueueSize = 512
	WebSocketWriteTimeout     = 5 * time.Second
)

var (
	ErrWebSocketGatewayUnavailable = errors.New("messaging WebSocket gateway is unavailable")
	ErrWebSocketQueueFull          = errors.New("messaging WebSocket write queue is full")
)

// WebSocketConnection is the minimal protocol surface used by the gateway.
// Keeping it separate from the concrete library type makes queue and close
// behavior testable without a network listener.
type WebSocketConnection interface {
	Read(context.Context) (websocket.MessageType, []byte, error)
	Write(context.Context, websocket.MessageType, []byte) error
	Close(websocket.StatusCode, string) error
}

// WebSocketAcceptFunc is injectable for application tests; production uses the
// coder/websocket RFC 6455 upgrader.
type WebSocketAcceptFunc func(http.ResponseWriter, *http.Request) (WebSocketConnection, error)

type WebSocketGatewayConfig struct {
	Tickets      *WebSocketTicketService
	Hub          *RefreshHub
	Recovery     RefreshRecoveryStore
	QueueSize    int
	WriteTimeout time.Duration
	Accept       WebSocketAcceptFunc
}

type WebSocketGateway struct {
	tickets      *WebSocketTicketService
	hub          *RefreshHub
	recovery     RefreshRecoveryStore
	queueSize    int
	writeTimeout time.Duration
	accept       WebSocketAcceptFunc

	mu       sync.Mutex
	sessions map[*webSocketSession]struct{}
}

func NewWebSocketGateway(config WebSocketGatewayConfig) *WebSocketGateway {
	if config.QueueSize < 1 {
		config.QueueSize = DefaultWebSocketQueueSize
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = WebSocketWriteTimeout
	}
	if config.Accept == nil {
		config.Accept = acceptCoderWebSocket
	}
	return &WebSocketGateway{
		tickets: config.Tickets, hub: config.Hub, recovery: config.Recovery,
		queueSize: config.QueueSize, writeTimeout: config.WriteTimeout, accept: config.Accept,
		sessions: make(map[*webSocketSession]struct{}),
	}
}

// Upgrade consumes the short-lived ticket before accepting an RFC 6455
// connection, then binds the connection to that ticket's authenticated user.
// The optional cursor is replayed through the durable recovery store. All
// writes are serialized by a bounded per-connection queue.
func (gateway *WebSocketGateway) Upgrade(ctx context.Context, writer http.ResponseWriter, request *http.Request, userID uint) error {
	if gateway == nil || gateway.tickets == nil || gateway.hub == nil || gateway.accept == nil || request == nil || writer == nil {
		return ErrWebSocketGatewayUnavailable
	}
	if userID == 0 {
		return ErrWebSocketTicketInvalid
	}
	ticket := request.URL.Query().Get("ticket")
	if ticket == "" {
		return ErrWebSocketTicketInvalid
	}
	if err := gateway.tickets.Consume(ctx, ticket, userID); err != nil {
		return err
	}

	var recovery RefreshRecovery
	cursor := request.URL.Query().Get("cursor")
	if cursor != "" && gateway.recovery != nil {
		var err error
		recovery, err = gateway.recovery.ReplayRefresh(ctx, userID, cursor)
		if err != nil {
			return err
		}
	}
	connection, err := gateway.accept(writer, request)
	if err != nil {
		return fmt.Errorf("upgrade messaging WebSocket: %w", err)
	}
	if connection == nil {
		return ErrWebSocketGatewayUnavailable
	}
	session := newWebSocketSession(connection, gateway.queueSize, gateway.writeTimeout)
	var unregister func()
	unregister = gateway.hub.Register(userID, session)
	session.setOnClose(func() {
		unregister()
		gateway.removeSession(session)
	})
	gateway.addSession(session)
	session.start()
	if recovery.FullRefresh {
		if err := session.enqueue(webSocketFrame{Type: "inbox.full_refresh_required"}); err != nil {
			session.close(websocket.StatusGoingAway, "recovery queue unavailable")
			return err
		}
	} else {
		for _, event := range recovery.Events {
			if err := session.enqueue(refreshFrame(event)); err != nil {
				session.close(websocket.StatusGoingAway, "recovery queue unavailable")
				return err
			}
		}
	}
	return nil
}

// Close terminates all currently managed connections without blocking on any
// client. Existing Redis stream events remain available for reconnect.
func (gateway *WebSocketGateway) Close() error {
	if gateway == nil {
		return nil
	}
	gateway.mu.Lock()
	sessions := make([]*webSocketSession, 0, len(gateway.sessions))
	for session := range gateway.sessions {
		sessions = append(sessions, session)
	}
	gateway.mu.Unlock()
	for _, session := range sessions {
		session.close(websocket.StatusGoingAway, "gateway closed")
	}
	return nil
}

func acceptCoderWebSocket(writer http.ResponseWriter, request *http.Request) (WebSocketConnection, error) {
	return websocket.Accept(writer, request, nil)
}

func (gateway *WebSocketGateway) addSession(session *webSocketSession) {
	gateway.mu.Lock()
	gateway.sessions[session] = struct{}{}
	gateway.mu.Unlock()
}

func (gateway *WebSocketGateway) removeSession(session *webSocketSession) {
	gateway.mu.Lock()
	delete(gateway.sessions, session)
	gateway.mu.Unlock()
}

type webSocketFrame struct {
	Type             string `json:"type"`
	EventID          string `json:"event_id,omitempty"`
	Cursor           string `json:"cursor,omitempty"`
	MessageCopyID    uint   `json:"message_copy_id,omitempty"`
	UserID           uint   `json:"user_id,omitempty"`
	AggregateVersion uint64 `json:"aggregate_version,omitempty"`
}

func refreshFrame(event RefreshEvent) webSocketFrame {
	return webSocketFrame{Type: "inbox.refresh", EventID: event.EventID, Cursor: event.Cursor, MessageCopyID: event.MessageCopyID, UserID: event.UserID, AggregateVersion: event.AggregateVersion}
}

type webSocketSession struct {
	connection   WebSocketConnection
	queue        chan webSocketFrame
	writeTimeout time.Duration
	done         chan struct{}
	onClose      func()
	onCloseMu    sync.Mutex
	closeOnce    sync.Once
}

func newWebSocketSession(connection WebSocketConnection, queueSize int, writeTimeout time.Duration) *webSocketSession {
	return &webSocketSession{connection: connection, queue: make(chan webSocketFrame, queueSize), writeTimeout: writeTimeout, done: make(chan struct{})}
}

func (session *webSocketSession) start() {
	go session.writeLoop()
	go session.readLoop()
}

func (session *webSocketSession) Send(ctx context.Context, event RefreshEvent) error {
	if session == nil || session.connection == nil {
		return ErrWebSocketGatewayUnavailable
	}
	frame := refreshFrame(event)
	select {
	case <-session.done:
		return ErrWebSocketGatewayUnavailable
	default:
	}
	select {
	case session.queue <- frame:
		return nil
	case <-session.done:
		return ErrWebSocketGatewayUnavailable
	case <-ctx.Done():
		return ctx.Err()
	default:
		session.close(websocket.StatusGoingAway, "write queue full")
		return ErrWebSocketQueueFull
	}
}

func (session *webSocketSession) enqueue(frame webSocketFrame) error {
	select {
	case <-session.done:
		return ErrWebSocketGatewayUnavailable
	case session.queue <- frame:
		return nil
	default:
		return ErrWebSocketQueueFull
	}
}

func (session *webSocketSession) writeLoop() {
	defer session.close(websocket.StatusNormalClosure, "")
	for {
		select {
		case <-session.done:
			return
		case frame := <-session.queue:
			payload, err := json.Marshal(frame)
			if err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), session.writeTimeout)
			err = session.connection.Write(ctx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (session *webSocketSession) readLoop() {
	for {
		if _, _, err := session.connection.Read(context.Background()); err != nil {
			session.close(websocket.StatusNormalClosure, "client disconnected")
			return
		}
	}
}

func (session *webSocketSession) setOnClose(onClose func()) {
	if session == nil {
		return
	}
	session.onCloseMu.Lock()
	session.onClose = onClose
	session.onCloseMu.Unlock()
	select {
	case <-session.done:
		if onClose != nil {
			onClose()
		}
	default:
	}
}

func (session *webSocketSession) close(status websocket.StatusCode, reason string) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		close(session.done)
		_ = session.connection.Close(status, reason)
		session.onCloseMu.Lock()
		onClose := session.onClose
		session.onCloseMu.Unlock()
		if onClose != nil {
			onClose()
		}
	})
}

var _ WebSocketConnection = (*websocket.Conn)(nil)
var _ RefreshSink = (*webSocketSession)(nil)
