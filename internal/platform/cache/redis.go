package cache

import (
	"fmt"

	platformconfig "admin/internal/platform/config"

	"github.com/redis/go-redis/v9"
)

func NewRedis(conf platformconfig.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", conf.Host, conf.Port),
		Password: conf.Password,
		DB:       conf.DB,
	})
}
