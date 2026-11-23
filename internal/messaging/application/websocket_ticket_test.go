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

func TestWebSocketTicketServiceConsumesTicketAndReturnsItsBoundUser(t *testing.T) {
	store := &webSocketTicketStoreFake{owner: 8, found: true}
	service := application.NewWebSocketTicketService(store)
	userID, err := service.Consume(context.Background(), "opaque-ticket")
	if err != nil || userID != 8 {
		t.Fatalf("Consume() user=%d err=%v", userID, err)
	}
	store.found = false
	if _, err := service.Consume(context.Background(), "opaque-ticket"); !errors.Is(err, application.ErrWebSocketTicketInvalid) {
		t.Fatalf("missing Consume() = %v, want invalid ticket", err)
	}
	store.found = true
	store.owner = 0
	if _, err := service.Consume(context.Background(), "opaque-ticket"); !errors.Is(err, application.ErrWebSocketTicketInvalid) {
		t.Fatalf("ownerless Consume() = %v, want invalid ticket", err)
	}
}

func TestWebSocketTicketServicePreservesStoreDependencyErrors(t *testing.T) {
	dependencyErr := errors.New("redis unavailable")
	service := application.NewWebSocketTicketService(&webSocketTicketStoreFake{err: dependencyErr})
	if _, err := service.Consume(context.Background(), "opaque-ticket"); !errors.Is(err, dependencyErr) {
		t.Fatalf("Consume() = %v, want dependency error", err)
	}
}
