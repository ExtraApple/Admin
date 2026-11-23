package redisadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
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

func TestWebSocketTicketConsumeTreatsRedisNilAsNotFound(t *testing.T) {
	for name, test := range map[string]struct {
		value any
		err   error
	}{
		"redis nil": {err: goredis.Nil},
		"nil value": {},
	} {
		t.Run(name, func(t *testing.T) {
			owner, found, err := parseConsumedWebSocketTicket(test.value, test.err)
			if err != nil || found || owner != 0 {
				t.Fatalf("parseConsumedWebSocketTicket() owner=%d found=%t err=%v", owner, found, err)
			}
		})
	}
}

func TestWebSocketTicketConsumePreservesDependencyErrorsAndRejectsMalformedOwners(t *testing.T) {
	dependencyErr := errors.New("redis unavailable")
	if _, found, err := parseConsumedWebSocketTicket(nil, dependencyErr); found || !errors.Is(err, dependencyErr) {
		t.Fatalf("dependency error found=%t err=%v", found, err)
	}
	for _, value := range []any{"", "0", "not-a-number", int64(7), "7"} {
		owner, found, err := parseConsumedWebSocketTicket(value, nil)
		if value == "7" {
			if err != nil || !found || owner != 7 {
				t.Fatalf("valid owner parsed as owner=%d found=%t err=%v", owner, found, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("malformed value %#v accepted as owner=%d found=%t", value, owner, found)
		}
	}
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
