package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const WebSocketTicketTTL = 60 * time.Second

var ErrWebSocketTicketInvalid = errors.New("messaging WebSocket ticket is invalid")

type WebSocketTicketStore interface {
	StoreWebSocketTicket(context.Context, string, uint, time.Duration) error
	ConsumeWebSocketTicket(context.Context, string) (uint, bool, error)
}

type WebSocketTicketService struct{ store WebSocketTicketStore }

func NewWebSocketTicketService(store WebSocketTicketStore) *WebSocketTicketService {
	return &WebSocketTicketService{store: store}
}

func (service *WebSocketTicketService) Issue(ctx context.Context, userID uint) (string, error) {
	if service == nil || service.store == nil || userID == 0 {
		return "", ErrWebSocketTicketInvalid
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate WebSocket ticket: %w", err)
	}
	ticket := base64.RawURLEncoding.EncodeToString(bytes)
	if err := service.store.StoreWebSocketTicket(ctx, webSocketTicketKey(ticket), userID, WebSocketTicketTTL); err != nil {
		return "", err
	}
	return ticket, nil
}

// Consume atomically exchanges a one-time ticket for the user bound at issue
// time. Empty, missing, expired, and already-consumed tickets are invalid; the
// caller cannot supply an external user identity.
func (service *WebSocketTicketService) Consume(ctx context.Context, ticket string) (uint, error) {
	if service == nil || service.store == nil || ticket == "" {
		return 0, ErrWebSocketTicketInvalid
	}
	userID, found, err := service.store.ConsumeWebSocketTicket(ctx, webSocketTicketKey(ticket))
	if err != nil {
		return 0, err
	}
	if !found || userID == 0 {
		return 0, ErrWebSocketTicketInvalid
	}
	return userID, nil
}

func webSocketTicketKey(ticket string) string {
	digest := sha256.Sum256([]byte(ticket))
	return "admin:messaging:ws-ticket:" + hex.EncodeToString(digest[:])
}
