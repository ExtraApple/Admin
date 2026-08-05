package app

import (
	"context"
	"time"

	filesmodule "admin/internal/files"
	filesgorm "admin/internal/files/adapters/gorm"
	filesstorage "admin/internal/files/adapters/objectstorage"
	filesapplication "admin/internal/files/application"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	platformobjectstorage "admin/internal/platform/objectstorage"
	"admin/internal/routecatalog"
	"admin/internal/uploadsecurity"

	"go.uber.org/zap"
)

type filesComposition struct {
	service *filesapplication.Service
	routes  []routecatalog.Descriptor
	jobs    []BackgroundJob
}

func newFilesComposition(resources Resources, config platformconfig.Config) (filesComposition, error) {
	signer, err := filesapplication.NewHMACSignerFromJWTSecret(config.Jwt.Secret)
	if err != nil {
		return filesComposition{}, err
	}
	provider := platformobjectstorage.NewStore(resources.MinIO)
	storage := filesstorage.New(provider)
	service := filesapplication.NewService(filesapplication.Dependencies{
		Repository:               filesgorm.NewRepository(resources.DB),
		Storage:                  storage,
		Validator:                uploadsecurity.NewManagedFileValidator(),
		Transactions:             platformdatabase.NewTransactionRunner(resources.DB),
		Signer:                   signer,
		DownloadURLExpireSeconds: config.FileUpload.DownloadURLExpireSeconds,
	})
	maxUploadBytes := int64(config.FileUpload.MaxSizeMB) * 1024 * 1024
	composition := filesComposition{service: service, routes: filesmodule.Routes(service, maxUploadBytes)}
	if config.FileRotation.Enabled {
		rotation := filesapplication.RotationConfig{
			Enabled: true, Days: config.FileRotation.Days,
			HotBucket: config.FileRotation.HotBucket, ColdBucket: config.FileRotation.ColdBucket,
			BatchSize: config.FileRotation.BatchSize,
		}
		composition.jobs = []BackgroundJob{{Name: "file-rotation", Run: func(ctx context.Context) {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					resources.Logger.Info("file rotation started", zap.String("hot_bucket", rotation.HotBucket), zap.String("cold_bucket", rotation.ColdBucket))
					result, err := service.Rotate(ctx, rotation)
					if err != nil {
						resources.Logger.Error("file rotation query failed", zap.Error(err))
						continue
					}
					if result.Examined == 0 {
						resources.Logger.Info("file rotation no files")
						continue
					}
					for _, failure := range result.Failures {
						resources.Logger.Error("file rotation item failed", zap.Error(failure))
					}
					resources.Logger.Info("file rotation finished", zap.Int("success", result.Moved), zap.Int("total", result.Examined))
				}
			}
		}}}
	}
	return composition, nil
}
