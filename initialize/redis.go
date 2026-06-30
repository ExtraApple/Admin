package initialize

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"admin/global"

	"go.uber.org/zap"
)

// InitRedis 初始化 Redis 客户端并通过 Ping 验证连接。
func InitRedis(conf *Config) {
	global.Redis = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", conf.Redis.Host, conf.Redis.Port),
		Password: conf.Redis.Password,
		DB:       conf.Redis.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := global.Redis.Ping(ctx).Err(); err != nil {
		global.Logger.Fatal("redis connect failed", zap.Error(err))
	}
}
