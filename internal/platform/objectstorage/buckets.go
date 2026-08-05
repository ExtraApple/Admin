package objectstorage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
)

func EnsureBuckets(ctx context.Context, client *minio.Client, logger *zap.Logger, buckets []string) error {
	for _, bucket := range buckets {
		if bucket == "" {
			return fmt.Errorf("MinIO bucket name is empty")
		}
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("check MinIO bucket %q: %w", bucket, err)
		}
		if exists {
			continue
		}
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create MinIO bucket %q: %w", bucket, err)
		}
		logger.Info("minio bucket created", zap.String("bucket", bucket))
	}
	return nil
}
