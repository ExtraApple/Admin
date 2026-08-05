package cache_test

import (
	"testing"

	"admin/internal/platform/cache"
	platformconfig "admin/internal/platform/config"
)

func TestNewRedisPreservesExistingConnectionOptions(t *testing.T) {
	client := cache.NewRedis(platformconfig.RedisConfig{
		Host:     "redis.internal",
		Port:     6381,
		Password: "redis-secret",
		DB:       4,
	})
	t.Cleanup(func() { _ = client.Close() })

	options := client.Options()
	if options.Addr != "redis.internal:6381" || options.Password != "redis-secret" || options.DB != 4 {
		t.Fatalf("Redis options changed: addr=%q password=%q db=%d", options.Addr, options.Password, options.DB)
	}
}
