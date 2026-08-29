package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin/internal/messaging/application"
)

type webSocketTicketStoreFake struct {
	key   string
	user  uint
	ttl   time.Duration
	owner uint
	found bool
	err   error
}

func (store *webSocketTicketStoreFake) StoreWebSocketTicket(_ context.Context, key string, userID uint, ttl time.Duration) error {
	store.key = key
	store.user = userID
	store.ttl = ttl
	return store.err
}
func (store *webSocketTicketStoreFake) ConsumeWebSocketTicket(context.Context, string) (uint, bool, error) {
	return store.owner, store.found, store.err
}

func TestWebSocketTicketServiceIssuesOpaqueSixtySecondUserBoundTicket(t *testing.T) {
	store := &webSocketTicketStoreFake{}
	service := application.NewWebSocketTicketService(store)
	ticket, err := service.Issue(context.Background(), 7)
	if err != nil || ticket == "" || store.key == "" || store.key == ticket || store.user != 7 || store.ttl != 60*time.Second {
		t.Fatalf("Issue() ticket=%q err=%v store=%#v", ticket, err, store)
	}
}

func TestWebSocketTicketServiceConsumesTicketExactlyOnceForItsAuthenticatedUser(t *testing.T) {
	store := &webSocketTicketStoreFake{owner: 7, found: true}
	service := application.NewWebSocketTicketService(store)
	if err := service.Consume(context.Background(), "opaque-ticket", 7); err != nil {
		t.Fatalf("Consume() = %v", err)
	}
	store.owner = 8
	if err := service.Consume(context.Background(), "opaque-ticket", 7); !errors.Is(err, application.ErrWebSocketTicketInvalid) {
		t.Fatalf("cross-user Consume() = %v, want invalid ticket", err)
	}
	store.found = false
	if err := service.Consume(context.Background(), "opaque-ticket", 7); !errors.Is(err, application.ErrWebSocketTicketInvalid) {
		t.Fatalf("missing Consume() = %v, want invalid ticket", err)
	}
}
