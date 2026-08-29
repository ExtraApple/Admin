package redisadapter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"admin/internal/messaging/application"
	goredis "github.com/redis/go-redis/v9"
)

const consumeWebSocketTicketLua = `
local userID = redis.call('GET', KEYS[1])
if not userID then
  return false
end
redis.call('DEL', KEYS[1])
return userID
`

type webSocketTicketBackend interface {
	Store(context.Context, string, uint, time.Duration) error
	Consume(context.Context, string) (uint, bool, error)
}

type redisWebSocketTicketBackend struct{ client *goredis.Client }

func (backend redisWebSocketTicketBackend) Store(ctx context.Context, key string, userID uint, ttl time.Duration) error {
	if backend.client == nil {
		return fmt.Errorf("Redis client is unavailable")
	}
	return backend.client.Set(ctx, key, strconv.FormatUint(uint64(userID), 10), ttl).Err()
}

func (backend redisWebSocketTicketBackend) Consume(ctx context.Context, key string) (uint, bool, error) {
	if backend.client == nil {
		return 0, false, fmt.Errorf("Redis client is unavailable")
	}
	value, err := backend.client.Eval(ctx, consumeWebSocketTicketLua, []string{key}).Result()
	if err != nil {
		return 0, false, err
	}
	if value == nil {
		return 0, false, nil
	}
	text, ok := value.(string)
	if !ok {
		return 0, false, fmt.Errorf("invalid Redis WebSocket ticket value")
	}
	userID, err := strconv.ParseUint(text, 10, 0)
	if err != nil || userID == 0 {
		return 0, false, fmt.Errorf("invalid Redis WebSocket ticket value")
	}
	return uint(userID), true, nil
}

type WebSocketTicketStore struct{ backend webSocketTicketBackend }

func NewWebSocketTicketStore(client *goredis.Client) *WebSocketTicketStore {
	return newWebSocketTicketStore(redisWebSocketTicketBackend{client: client})
}

func newWebSocketTicketStore(backend webSocketTicketBackend) *WebSocketTicketStore {
	return &WebSocketTicketStore{backend: backend}
}

func (store *WebSocketTicketStore) StoreWebSocketTicket(ctx context.Context, key string, userID uint, ttl time.Duration) error {
	if store == nil || store.backend == nil || key == "" || userID == 0 || ttl <= 0 {
		return application.ErrWebSocketTicketInvalid
	}
	return store.backend.Store(ctx, key, userID, ttl)
}

func (store *WebSocketTicketStore) ConsumeWebSocketTicket(ctx context.Context, key string) (uint, bool, error) {
	if store == nil || store.backend == nil || key == "" {
		return 0, false, application.ErrWebSocketTicketInvalid
	}
	return store.backend.Consume(ctx, key)
}

var _ application.WebSocketTicketStore = (*WebSocketTicketStore)(nil)
