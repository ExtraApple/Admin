package redisadapter

import (
	"context"
	"testing"
	"time"
)

type ticketBackendFake struct {
	storedKey string
	storedID  uint
	storedTTL time.Duration
	owner     uint
	found     bool
}

func (backend *ticketBackendFake) Store(_ context.Context, key string, userID uint, ttl time.Duration) error {
	backend.storedKey = key
	backend.storedID = userID
	backend.storedTTL = ttl
	return nil
}
func (backend *ticketBackendFake) Consume(context.Context, string) (uint, bool, error) {
	return backend.owner, backend.found, nil
}

func TestWebSocketTicketStorePersistsHashKeyAndDelegatesAtomicConsume(t *testing.T) {
	backend := &ticketBackendFake{owner: 7, found: true}
	store := newWebSocketTicketStore(backend)
	if err := store.StoreWebSocketTicket(context.Background(), "admin:messaging:ws-ticket:digest", 7, 60*time.Second); err != nil {
		t.Fatalf("StoreWebSocketTicket() = %v", err)
	}
	owner, found, err := store.ConsumeWebSocketTicket(context.Background(), "admin:messaging:ws-ticket:digest")
	if err != nil || !found || owner != 7 || backend.storedKey != "admin:messaging:ws-ticket:digest" || backend.storedID != 7 || backend.storedTTL != 60*time.Second {
		t.Fatalf("ticket state owner=%d found=%t err=%v backend=%#v", owner, found, err, backend)
	}
}
