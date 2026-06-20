package global

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"github.com/minio/minio-go/v7"
)

var (
	DB    *gorm.DB
	Redis *redis.Client
	Minio *minio.Client
)
