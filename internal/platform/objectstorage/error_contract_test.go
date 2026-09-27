package objectstorage_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	filesstorage "admin/internal/files/adapters/objectstorage"
	filesapplication "admin/internal/files/application"
	storage "admin/internal/platform/objectstorage"
	"admin/internal/uploadsecurity"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestStoragePermissionDeniedRemainsDistinctFromMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>Denied</Message></Error>`))
	}))
	defer server.Close()
	client, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{Creds: credentials.NewStaticV4("probe", "probe-secret", ""), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = storage.NewStore(client).Stat(context.Background(), "probe-bucket", "image.png")
	if !errors.Is(err, storage.ErrPermissionDenied) || errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("permission classification: %v", err)
	}
	err = filesstorage.New(storage.NewStore(client)).Delete(context.Background(), "probe-bucket", "image.png")
	if code, _ := uploadsecurity.CodeOf(err); code != uploadsecurity.CodeStorageUnavailable || !errors.Is(err, filesapplication.ErrStoragePermissionDenied) {
		t.Fatalf("Files permission classification changed HTTP compatibility: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = filesstorage.New(storage.NewStore(client)).Delete(ctx, "probe-bucket", "image.png")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Files lost cancellation identity: %v", err)
	}
}
