package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	platformcache "admin/internal/platform/cache"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	platformlogging "admin/internal/platform/logging"
	platformobjectstorage "admin/internal/platform/objectstorage"

	"go.uber.org/zap"
)

type openedPlatform struct {
	resources Resources
	closers   []func() error
}

type platformOpener func(context.Context, platformconfig.Config) (openedPlatform, error)

func Build(ctx context.Context, conf platformconfig.Config, options Options) (*Application, error) {
	return buildWithOpener(ctx, conf, options, openPlatform)
}

func BuildFromPath(ctx context.Context, configPath string, options Options) (*Application, error) {
	return buildFromPathWithOpener(ctx, configPath, options, openPlatform)
}

func buildFromPathWithOpener(ctx context.Context, configPath string, options Options, opener platformOpener) (*Application, error) {
	conf, err := platformconfig.Load(configPath)
	if err != nil {
		return nil, err
	}
	return buildWithOpener(ctx, conf, options, opener)
}

func buildWithOpener(ctx context.Context, conf platformconfig.Config, options Options, opener platformOpener) (*Application, error) {
	platform, err := opener(ctx, conf)
	if err != nil {
		return nil, err
	}
	application, err := New(ctx, conf, platform.resources, options)
	if err != nil {
		return nil, errors.Join(err, closePlatform(platform.closers))
	}
	application.closers = platform.closers
	return application, nil
}

func openPlatform(ctx context.Context, conf platformconfig.Config) (openedPlatform, error) {
	logger, err := platformlogging.New(conf.Logger)
	if err != nil {
		return openedPlatform{}, fmt.Errorf("create logger: %w", err)
	}
	platform := openedPlatform{
		resources: Resources{Logger: logger.Logger},
		closers:   []func() error{logger.Close},
	}
	fail := func(component string, err error) (openedPlatform, error) {
		logger.Error(component+" initialization failed", zap.Error(err))
		return openedPlatform{}, errors.Join(err, closePlatform(platform.closers))
	}

	db, err := platformdatabase.OpenMySQL(conf.Mysql)
	if err != nil {
		return fail("MySQL", err)
	}
	platform.resources.DB = db
	sqlDB, err := db.DB()
	if err != nil {
		return fail("MySQL", fmt.Errorf("access MySQL connection: %w", err))
	}
	platform.closers = append(platform.closers, sqlDB.Close)

	redisClient := platformcache.NewRedis(conf.Redis)
	platform.resources.Redis = redisClient
	platform.closers = append(platform.closers, redisClient.Close)
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = redisClient.Ping(pingContext).Err()
	cancel()
	if err != nil {
		return fail("Redis", fmt.Errorf("ping Redis: %w", err))
	}

	minioClient, err := platformobjectstorage.NewMinIO(conf.Minio)
	if err != nil {
		return fail("MinIO", err)
	}
	platform.resources.MinIO = minioClient
	buckets := []string{"image", "files"}
	if conf.FileRotation.Enabled {
		buckets = append(buckets, conf.FileRotation.ColdBucket)
	}
	if err := platformobjectstorage.EnsureBuckets(ctx, minioClient, logger.Logger, buckets); err != nil {
		return fail("MinIO", err)
	}
	logger.Info("platform initialized", zap.String("mysql_host", conf.Mysql.Host), zap.String("redis_host", conf.Redis.Host), zap.String("minio_host", conf.Minio.Host))
	return platform, nil
}

func closePlatform(closers []func() error) error {
	var result error
	for index := len(closers) - 1; index >= 0; index-- {
		result = errors.Join(result, closers[index]())
	}
	return result
}
