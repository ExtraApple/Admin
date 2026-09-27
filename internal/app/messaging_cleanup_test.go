package app_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"admin/internal/app"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/domain"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestBackgroundCleanupExpiresMissedAnnouncementWithoutBroker(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	client, err := minio.New("127.0.0.1:1", &minio.Options{Creds: credentials.NewStaticV4("probe", "secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()
	config := testAppConfig()
	config.Messaging.CleanupBatchSize = 1
	core, logs := observer.New(zap.InfoLevel)
	running, err := app.New(context.Background(), config, app.Resources{DB: db, Redis: redisClient, MinIO: client, Logger: zap.New(core)}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	publishAt, expiresAt := now.Add(-time.Hour), now.Add(-time.Minute)
	message := messaginggorm.Message{LogicalID: "missed-startup", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, Title: "Notice", BodyHTML: "<p>Body</p>", PublishAt: &publishAt, ExpiresAt: &expiresAt, AggregateVersion: 1}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	running.StartBackgroundJobs(context.Background())
	deadline := time.After(3 * time.Second)
	for {
		var current messaginggorm.Message
		if err := db.First(&current, message.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.Status == domain.MessageStatusExpired {
			break
		}
		select {
		case <-deadline:
			t.Fatal("background cleanup did not expire due announcement")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := running.Close(); err != nil {
		t.Fatal(err)
	}
	var events []messaginggorm.MessageOutbox
	if err := db.Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventName != domain.EventNameMessageExpired {
		t.Fatalf("events: %+v", events)
	}
	entries := logs.FilterMessage("messaging_cleanup_task_skipped").All()
	if len(entries) != 1 || entries[0].ContextMap()["failure_code"] != "rabbitmq_not_configured" || entries[0].ContextMap()["processed"] != int64(0) {
		t.Fatalf("missing broker skip log: %+v", entries)
	}
	started, finished := logs.FilterMessage("messaging_cleanup_started").All(), logs.FilterMessage("messaging_cleanup_finished").All()
	if len(started) != 1 || len(finished) != 1 || started[0].ContextMap()["run_id"] == "" || started[0].ContextMap()["run_id"] != finished[0].ContextMap()["run_id"] || entries[0].ContextMap()["run_id"] != started[0].ContextMap()["run_id"] {
		t.Fatalf("unlinked cleanup round: start=%+v finish=%+v skip=%+v", started, finished, entries)
	}
}

func TestBackgroundCleanupPublishesWithDisconnectedConfiguredBroker(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	client, err := minio.New("127.0.0.1:1", &minio.Options{Creds: credentials.NewStaticV4("probe", "secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()
	config := testAppConfig()
	config.Messaging.CleanupBatchSize = 1
	config.RabbitMQ.Host = "127.0.0.1"
	config.RabbitMQ.Port = 1
	var sink bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(&sink), zap.InfoLevel))
	running, err := app.New(context.Background(), config, app.Resources{DB: db, Redis: redisClient, MinIO: client, Logger: logger}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	publishAt := now.Add(-time.Minute)
	message := messaginggorm.Message{LogicalID: "disconnected-broker", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, Title: "Notice", BodyHTML: "<p>Body</p>", PublishAt: &publishAt, AggregateVersion: 1}
	if err := db.Create(&message).Error; err != nil {
		running.Close()
		t.Fatal(err)
	}
	running.StartBackgroundJobs(context.Background())
	deadline := time.After(3 * time.Second)
	for {
		var current messaginggorm.Message
		if err := db.First(&current, message.ID).Error; err != nil {
			running.Close()
			t.Fatal(err)
		}
		if current.Status == domain.MessageStatusPublished {
			break
		}
		select {
		case <-deadline:
			running.Close()
			t.Fatal("configured broker cleanup did not publish due announcement")
		case <-time.After(10 * time.Millisecond):
		}
	}
	var outboxes []messaginggorm.MessageOutbox
	if err := db.Find(&outboxes).Error; err != nil {
		t.Fatal(err)
	}
	if len(outboxes) != 1 || outboxes[0].EventName != domain.EventNameMessagePublished {
		t.Fatalf("published announcement must retain its Outbox event while broker is disconnected: %+v", outboxes)
	}
	if err := running.Close(); err != nil {
		t.Fatal(err)
	}
	output := sink.String()
	if !strings.Contains(output, `"msg":"messaging_cleanup_task_finished"`) || !strings.Contains(output, `"task":"publish_due_announcements"`) || !strings.Contains(output, `"status":"succeeded"`) {
		t.Fatalf("missing publish task summary in zap sink: %s", output)
	}
}

func TestBackgroundCleanupUsesConfiguredRotationForImageCleanup(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	client, err := minio.New("127.0.0.1:1", &minio.Options{Creds: credentials.NewStaticV4("probe", "secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()
	config := testAppConfig()
	config.Messaging.CleanupBatchSize = 1
	config.FileRotation.Enabled = true
	config.FileRotation.HotBucket = "hot-missing"
	config.FileRotation.ColdBucket = "cold-missing"
	var sink bytes.Buffer
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&sink), zap.InfoLevel))
	running, err := app.New(context.Background(), config, app.Resources{DB: db, Redis: redisClient, MinIO: client, Logger: logger}, app.Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(-time.Hour)
	file := map[string]any{"name": "image.png", "bucket": "original", "object_name": "message-images/00000000-0000-0000-0000-000000000001.png", "content_type": "image/png", "detected_content_type": "image/png", "size": 10, "uploader_id": 7, "purpose": "message_image", "binding_expires_at": expires, "validation_status": "validated"}
	if err := db.Table("files").Create(file).Error; err != nil {
		t.Fatal(err)
	}
	running.StartBackgroundJobs(context.Background())
	deadline := time.After(3 * time.Second)
	for {
		var job struct {
			LastErrorCode string
		}
		if err := db.Table("message_image_cleanup_jobs").First(&job).Error; err == nil && job.LastErrorCode != "" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("background cleanup did not record image storage failure")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := running.Close(); err != nil {
		t.Fatal(err)
	}
	output := sink.String()
	if !strings.Contains(output, `"msg":"messaging_cleanup_item_failed"`) || !strings.Contains(output, `"failure_code":"message_image_cleanup_storage_unavailable"`) {
		t.Fatalf("missing bounded image failure log: %s", output)
	}
}
