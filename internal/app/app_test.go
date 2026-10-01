package app_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/app"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestApplicationShutdownCancelsBackgroundJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenIsolatedSQLite(t)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("test", "test", ""), Secure: false})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}
	logger := zap.NewNop()
	jobStarted := make(chan struct{})
	jobStopped := make(chan struct{})

	application, err := app.New(context.Background(), testAppConfig(), app.Resources{
		DB: db, Redis: redisClient, MinIO: minioClient, Logger: logger,
	}, app.Options{
		Seed: func(_ context.Context, _ platformconfig.Config, snapshot []routecatalog.Descriptor) error {
			return nil
		},
		BackgroundJobs: []app.BackgroundJob{{Name: "test", Run: func(ctx context.Context) {
			close(jobStarted)
			<-ctx.Done()
			close(jobStopped)
		}}},
	})
	if err != nil {
		t.Fatalf("assemble App: %v", err)
	}

	application.StartBackgroundJobs(context.Background())
	select {
	case <-jobStarted:
	case <-time.After(time.Second):
		t.Fatal("background job did not start")
	}

	if err := application.Close(); err != nil {
		t.Fatalf("close App: %v", err)
	}
	select {
	case <-jobStopped:
	case <-time.After(time.Second):
		t.Fatal("background job did not stop")
	}
}
