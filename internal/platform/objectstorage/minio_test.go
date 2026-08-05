package objectstorage_test

import (
	"testing"

	platformconfig "admin/internal/platform/config"
	"admin/internal/platform/objectstorage"
)

func TestNewMinIOPreservesExistingEndpoint(t *testing.T) {
	client, err := objectstorage.NewMinIO(platformconfig.MinIOConfig{
		Host:     "minio.internal",
		Port:     9001,
		Username: "minio-user",
		Password: "minio-secret",
	})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}

	endpoint := client.EndpointURL()
	if endpoint.Scheme != "http" || endpoint.Host != "minio.internal:9001" {
		t.Fatalf("MinIO endpoint changed: %s", endpoint)
	}
}
