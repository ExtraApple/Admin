package redisadapter

import (
	"context"
	"time"

	"admin/internal/identity/application"

	"github.com/redis/go-redis/v9"
)

const (
	captchaPrefix   = "captcha:"
	failurePrefix   = "fail:"
	lockPrefix      = "lock:"
	blacklistPrefix = "blacklist:"
)

type Store struct{ client *redis.Client }

func NewStore(client *redis.Client) *Store { return &Store{client: client} }

func (store *Store) Verify(ctx context.Context, id, answer string) bool {
	key := captchaPrefix + id
	value, err := store.client.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	// Delete on both matching and mismatching answers to preserve one-shot use.
	_ = store.client.Del(ctx, key).Err()
	return value == answer
}

func (store *Store) IsLocked(ctx context.Context, username string) (time.Duration, bool, error) {
	key := lockPrefix + username
	value, err := store.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if value != "1" {
		return 0, false, nil
	}
	ttl, err := store.client.TTL(ctx, key).Result()
	return ttl, err == nil, err
}

func (store *Store) RecordFailure(ctx context.Context, username string) (int, error) {
	key := failurePrefix + username
	count, err := store.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if err := store.client.Expire(ctx, key, 24*time.Hour).Err(); err != nil {
		return 0, err
	}
	return int(count), nil
}

func (store *Store) Lock(ctx context.Context, username string, duration time.Duration) error {
	return store.client.Set(ctx, lockPrefix+username, "1", duration).Err()
}

func (store *Store) Clear(ctx context.Context, username string) error {
	return store.client.Del(ctx, failurePrefix+username, lockPrefix+username).Err()
}

func (store *Store) Contains(ctx context.Context, token string) (bool, error) {
	count, err := store.client.Exists(ctx, blacklistPrefix+token).Result()
	return count > 0, err
}

func (store *Store) Add(ctx context.Context, token string, duration time.Duration) error {
	return store.client.Set(ctx, blacklistPrefix+token, "1", duration).Err()
}

var _ application.CaptchaVerifier = (*Store)(nil)
var _ application.LoginAttemptStore = (*Store)(nil)
var _ application.BlacklistStore = (*Store)(nil)
