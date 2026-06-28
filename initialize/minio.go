package initialize

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"admin/global"

	"go.uber.org/zap"
)

func InitMinio(conf *Config) {
	endpoint := fmt.Sprintf("%s:%d", conf.Minio.Host, conf.Minio.Port)
	var err error
	global.Minio, err = minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.Minio.Username, conf.Minio.Password, ""),
		Secure: false,
	})
	if err != nil {
		global.Logger.Fatal("minio connect failed", zap.Error(err))
	}

	ctx := context.Background()
	buckets := []string{"image", "files"}
	if conf.FileRotation.Enabled {
		buckets = append(buckets, conf.FileRotation.ColdBucket)
	}
	for _, bucket := range buckets {
		exists, _ := global.Minio.BucketExists(ctx, bucket)
		if !exists {
			if err := global.Minio.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				global.Logger.Fatal("minio create bucket failed", zap.String("bucket", bucket), zap.Error(err))
			}
			global.Logger.Info("minio bucket created", zap.String("bucket", bucket))
		}
	}
}
