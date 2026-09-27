//go:build minio_integration

package application_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"admin/internal/files"
	adapter "admin/internal/files/adapters/gorm"
	storageadapter "admin/internal/files/adapters/objectstorage"
	"admin/internal/files/application"
	"admin/internal/files/domain"
	storage "admin/internal/platform/objectstorage"
	"admin/testsupport/testutil"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinIOMessageImageCleanupRecovery(t *testing.T) {
	endpoint, key, secret := os.Getenv("ADMIN_TEST_MINIO_ENDPOINT"), os.Getenv("ADMIN_TEST_MINIO_ACCESS_KEY"), os.Getenv("ADMIN_TEST_MINIO_SECRET_KEY")
	if endpoint == "" || key == "" || secret == "" {
		t.Fatal("MinIO integration credentials required")
	}
	secure, _ := strconv.ParseBool(os.Getenv("ADMIN_TEST_MINIO_SECURE"))
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(key, secret, ""), Secure: secure})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	original := fmt.Sprintf("c5-images-%d", time.Now().UnixNano())
	cold := original + "-cold"
	for _, bucket := range []string{original, cold} {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		c, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, bucket := range []string{original, cold} {
			for obj := range client.ListObjects(c, bucket, minio.ListObjectsOptions{Recursive: true}) {
				if obj.Err != nil {
					t.Error(obj.Err)
					continue
				}
				if e := client.RemoveObject(c, bucket, obj.Key, minio.RemoveObjectOptions{}); e != nil {
					t.Error(e)
				}
			}
			if e := client.RemoveBucket(c, bucket); e != nil {
				t.Error(e)
			}
		}
	})
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	provider := storage.NewStore(client)
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	s := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storageadapter.New(provider), Clock: application.ClockFunc(func() time.Time { return now })})
	name := "message-images/" + uuid.NewString() + ".png"
	for _, bucket := range []string{original, cold} {
		if _, err := provider.Put(ctx, storage.PutInput{Bucket: bucket, Name: name, Reader: bytes.NewReader([]byte("image")), Size: 5, ContentType: "image/png"}); err != nil {
			t.Fatal(err)
		}
	}
	file := domain.File{Purpose: "message_image", Bucket: original, ObjectName: name, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	// The original location is no longer configured. It must still be processed.
	result := s.CleanupMessageImage(ctx, file.ID, now, application.RotationConfig{Enabled: true, HotBucket: original + "-missing", ColdBucket: cold})
	if result.Status != "failed" {
		t.Fatalf("missing bucket treated as absent object: %+v", result)
	}
	jobs, err := r.FindMessageImageCleanupRetries(ctx, now.Add(time.Hour), application.CleanupCursor{UpperID: result.CleanupJobID}, 1)
	if err != nil || len(jobs) != 1 || jobs[0].Bucket != original {
		t.Fatalf("lost location: %+v %v", jobs, err)
	}
	// Original deletion already succeeded; retry must tolerate NoSuchKey there.
	now = now.Add(time.Hour)
	result = s.RetryMessageImageCleanup(ctx, jobs[0], now, application.RotationConfig{Enabled: true, ColdBucket: cold})
	if result.Status != "succeeded" {
		t.Fatalf("recover partial cleanup: %+v", result)
	}
	for _, bucket := range []string{original, cold} {
		if _, err := provider.Stat(ctx, bucket, name); err == nil {
			t.Fatalf("copy in %s remains", bucket)
		}
	}
	anonymous, err := minio.New(endpoint, &minio.Options{Secure: secure, Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	denied := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storageadapter.New(storage.NewStore(anonymous)), Clock: application.ClockFunc(func() time.Time { return now })})
	file.ID = 0
	file.ObjectName = "message-images/" + uuid.NewString() + ".png"
	if _, err := provider.Put(ctx, storage.PutInput{Bucket: original, Name: file.ObjectName, Reader: bytes.NewReader([]byte("keep")), Size: 4}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	result = denied.CleanupMessageImage(ctx, file.ID, now, application.RotationConfig{})
	if result.FailureCode != "message_image_cleanup_permission_denied" {
		t.Fatalf("permission rejection: %+v", result)
	}
	if _, err := provider.Stat(ctx, original, file.ObjectName); err != nil {
		t.Fatal("denied cleanup lost object")
	}
	slowClient, err := minio.New(endpoint, &minio.Options{Secure: secure, Region: "us-east-1", Creds: credentials.NewStaticV4(key, secret, ""), Transport: cleanupTimeoutTransport{}})
	if err != nil {
		t.Fatal(err)
	}
	slow := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storageadapter.New(storage.NewStore(slowClient)), Clock: application.ClockFunc(func() time.Time { return now })})
	jobs, err = r.FindMessageImageCleanupRetries(ctx, now.Add(time.Hour), application.CleanupCursor{UpperID: result.CleanupJobID}, 1)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("denied job lost: %+v %v", jobs, err)
	}
	now = now.Add(time.Hour)
	timed, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	result = slow.RetryMessageImageCleanup(timed, jobs[0], now, application.RotationConfig{})
	stop()
	if result.Status != "failed" || timed.Err() != context.DeadlineExceeded {
		t.Fatalf("timeout: %+v %v", result, timed.Err())
	}
	if _, err := provider.Stat(ctx, original, file.ObjectName); err != nil {
		t.Fatal("timeout lost object")
	}
	jobs, err = r.FindMessageImageCleanupRetries(ctx, now.Add(time.Hour), application.CleanupCursor{UpperID: result.CleanupJobID}, 1)
	if err != nil || len(jobs) != 1 || jobs[0].RetryCount != 1 {
		t.Fatalf("timeout lost recovery: %+v %v", jobs, err)
	}
}

// Inject a stalled network at the HTTP boundary without changing service logic.
type cleanupTimeoutTransport struct{}

func (cleanupTimeoutTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}
