//go:build redis_integration

package redisadapter

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisIntegrationIdentityStateLifecycle(t *testing.T) {
	address := os.Getenv("ADMIN_TEST_REDIS_ADDR")
	if address == "" {
		t.Fatal("ADMIN_TEST_REDIS_ADDR is required for the mandatory Redis gate")
	}
	database := 15
	if configured := os.Getenv("ADMIN_TEST_REDIS_DB"); configured != "" {
		parsed, err := strconv.Atoi(configured)
		if err != nil {
			t.Fatalf("parse ADMIN_TEST_REDIS_DB: %v", err)
		}
		database = parsed
	}
	client := redis.NewClient(&redis.Options{Addr: address, Password: os.Getenv("ADMIN_TEST_REDIS_PASSWORD"), DB: database})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Redis integration client: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis integration server: %v", err)
	}

	suffix := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	captchaID, mismatchID := "captcha-"+suffix, "mismatch-"+suffix
	username, token := "user-"+suffix, "token-"+suffix
	keys := []string{
		captchaPrefix + captchaID,
		captchaPrefix + mismatchID,
		failurePrefix + username,
		lockPrefix + username,
		blacklistPrefix + token,
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := client.Del(cleanupContext, keys...).Err(); err != nil {
			t.Errorf("clean Redis integration keys: %v", err)
		}
	})

	store := NewStore(client)
	if err := client.Set(ctx, captchaPrefix+captchaID, "246810", time.Minute).Err(); err != nil {
		t.Fatalf("seed captcha: %v", err)
	}
	if !store.Verify(ctx, captchaID, "246810") {
		t.Fatal("matching captcha was rejected")
	}
	if store.Verify(ctx, captchaID, "246810") {
		t.Fatal("captcha was not consumed after the first verification")
	}
	if err := client.Set(ctx, captchaPrefix+mismatchID, "135790", time.Minute).Err(); err != nil {
		t.Fatalf("seed mismatching captcha: %v", err)
	}
	if store.Verify(ctx, mismatchID, "wrong") || store.Verify(ctx, mismatchID, "135790") {
		t.Fatal("mismatching captcha was accepted or not consumed")
	}

	for expected := 1; expected <= 2; expected++ {
		count, err := store.RecordFailure(ctx, username)
		if err != nil {
			t.Fatalf("record login failure %d: %v", expected, err)
		}
		if count != expected {
			t.Fatalf("failure count = %d, want %d", count, expected)
		}
	}
	if err := store.Lock(ctx, username, time.Minute); err != nil {
		t.Fatalf("lock identity: %v", err)
	}
	if ttl, locked, err := store.IsLocked(ctx, username); err != nil || !locked || ttl <= 0 {
		t.Fatalf("locked identity state = (%s, %v, %v), want positive TTL and locked", ttl, locked, err)
	}
	if err := store.Clear(ctx, username); err != nil {
		t.Fatalf("clear login state: %v", err)
	}
	if _, locked, err := store.IsLocked(ctx, username); err != nil || locked {
		t.Fatalf("cleared identity remains locked: locked=%v err=%v", locked, err)
	}

	if err := store.Add(ctx, token, time.Minute); err != nil {
		t.Fatalf("blacklist token: %v", err)
	}
	if contains, err := store.Contains(ctx, token); err != nil || !contains {
		t.Fatalf("blacklist lookup = (%v, %v), want present", contains, err)
	}
}
