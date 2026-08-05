package objectstorage

import (
	"fmt"

	platformconfig "admin/internal/platform/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func NewMinIO(conf platformconfig.MinIOConfig) (*minio.Client, error) {
	endpoint := fmt.Sprintf("%s:%d", conf.Host, conf.Port)
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.Username, conf.Password, ""),
		Secure: false,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}
	return client, nil
}
