package initialize

import (
	"context"
	"fmt"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"admin/global"
)

func InitMinio(conf *Config) {
	endpoint := fmt.Sprintf("%s:%d", conf.Minio.Host, conf.Minio.Port)
	var err error
	global.Minio, err = minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.Minio.Username, conf.Minio.Password, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalf("MinIO connect failed: %v", err)
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
				log.Fatalf("MinIO create bucket %s failed: %v", bucket, err)
			}
			log.Printf("MinIO bucket %s created", bucket)
		}
	}
}
