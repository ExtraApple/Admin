package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"admin/testsupport/testutil"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestBuildOwnsPlatformConstructionAndClosure(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	opened := 0
	closed := make([]string, 0, 2)
	opener := func(_ context.Context, _ platformconfig.Config) (openedPlatform, error) {
		opened++
		return openedPlatform{
			resources: Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()},
			closers: []func() error{
				func() error { closed = append(closed, "first"); return nil },
				func() error { closed = append(closed, "second"); return nil },
			},
		}, nil
	}

	config := platformconfig.Config{}
	config.Jwt.Secret = "build-app-test-file-signing-secret"
	config.FileUpload.MaxSizeMB = 50
	config.FileUpload.AvatarMaxSizeMB = 2
	config.FileUpload.DownloadURLExpireSeconds = 300
	application, err := buildWithOpener(context.Background(), config, Options{
		Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil },
	}, opener)
	if err != nil {
		t.Fatalf("build App: %v", err)
	}
	if opened != 1 {
		t.Fatalf("Platform open count = %d, want 1", opened)
	}
	if err := application.Close(); err != nil {
		t.Fatalf("close App: %v", err)
	}
	if len(closed) != 2 || closed[0] != "second" || closed[1] != "first" {
		t.Fatalf("Platform close order = %v, want reverse construction order", closed)
	}
}

func TestBuildFromPathUsesAppComposition(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := []byte("jwt:\n  secret: build-from-path-secret\nfile_upload:\n  max_size_mb: 50\n  avatar_max_size_mb: 2\n  download_url_expire_seconds: 300\n")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	opened := 0
	opener := func(_ context.Context, _ platformconfig.Config) (openedPlatform, error) {
		opened++
		return openedPlatform{resources: Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.NewNop()}}, nil
	}
	application, err := buildFromPathWithOpener(context.Background(), configPath, Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }}, opener)
	if err != nil {
		t.Fatalf("build App from path: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	if opened != 1 {
		t.Fatalf("Platform open count = %d, want 1", opened)
	}
}

func TestOpenPlatformLogsMySQLInitializationFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "platform.log")
	conf := platformconfig.Config{}
	conf.Logger.Output = logPath
	conf.Logger.Level = "error"
	conf.Logger.Format = "json"
	conf.Mysql.Host = "127.0.0.1"
	conf.Mysql.Port = 1
	conf.Mysql.User = "root"
	conf.Mysql.DB = "admin"

	_, err := openPlatform(context.Background(), conf)
	if err == nil {
		t.Fatal("openPlatform succeeded against an unreachable MySQL port")
	}
	content, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read initialization failure log: %v", readErr)
	}
	if !strings.Contains(string(content), "MySQL initialization failed") || !strings.Contains(string(content), "error") {
		t.Fatalf("initialization failure log = %q, want component and error fields", content)
	}
}
