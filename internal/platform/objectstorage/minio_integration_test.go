//go:build minio_integration

package objectstorage_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	platformstorage "admin/internal/platform/objectstorage"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinIOIntegrationObjectLifecycle(t *testing.T) {
	endpoint := os.Getenv("ADMIN_TEST_MINIO_ENDPOINT")
	accessKey := os.Getenv("ADMIN_TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("ADMIN_TEST_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Fatal("ADMIN_TEST_MINIO_ENDPOINT, ADMIN_TEST_MINIO_ACCESS_KEY and ADMIN_TEST_MINIO_SECRET_KEY are required for the mandatory MinIO gate")
	}
	secure, err := strconv.ParseBool(os.Getenv("ADMIN_TEST_MINIO_SECURE"))
	if err != nil && os.Getenv("ADMIN_TEST_MINIO_SECURE") != "" {
		t.Fatalf("parse ADMIN_TEST_MINIO_SECURE: %v", err)
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure})
	if err != nil {
		t.Fatalf("create MinIO integration client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bucket := fmt.Sprintf("admin-gate-%d", time.Now().UnixNano())
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("create MinIO integration bucket: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for object := range client.ListObjects(cleanupContext, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err != nil {
				t.Errorf("list MinIO cleanup objects: %v", object.Err)
				continue
			}
			if err := client.RemoveObject(cleanupContext, bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				t.Errorf("remove MinIO cleanup object %s: %v", object.Key, err)
			}
		}
		if err := client.RemoveBucket(cleanupContext, bucket); err != nil {
			t.Errorf("remove MinIO integration bucket: %v", err)
		}
	})

	payload := []byte("admin object storage integration payload")
	const name = "integration/nested/object.txt"
	store := platformstorage.NewStore(client)
	put, err := store.Put(ctx, platformstorage.PutInput{Bucket: bucket, Name: name, Reader: bytes.NewReader(payload), Size: int64(len(payload)), ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if put.Name != name || put.Size != int64(len(payload)) {
		t.Fatalf("put info = %+v, want name %q and size %d", put, name, len(payload))
	}
	stat, err := store.Stat(ctx, bucket, name)
	if err != nil {
		t.Fatalf("stat object: %v", err)
	}
	if stat.Name != name || stat.ContentType != "text/plain" {
		t.Fatalf("stat info = %+v, want stored object metadata", stat)
	}
	reader, err := store.Open(ctx, bucket, name)
	if err != nil {
		t.Fatalf("open object: %v", err)
	}
	content, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read object: read=%v close=%v", readErr, closeErr)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("object content = %q, want %q", content, payload)
	}
	objects, err := store.List(ctx, bucket, platformstorage.ListOptions{Prefix: "integration/", Recursive: true})
	if err != nil {
		t.Fatalf("list objects: %v", err)
	}
	if len(objects) != 1 || objects[0].Name != name {
		t.Fatalf("listed objects = %+v, want only %q", objects, name)
	}
	if err := store.Delete(ctx, bucket, name); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if _, err := store.Stat(ctx, bucket, name); !errors.Is(err, platformstorage.ErrObjectNotFound) {
		t.Fatalf("stat deleted object error = %v, want ErrObjectNotFound", err)
	}
}
