package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	filesapplication "admin/internal/files/application"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	messagingapplication "admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
	"admin/testsupport/testutil"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

type scheduleLogger struct {
	info func(string, ...messagingapplication.RuntimeLogField)
}

func (l scheduleLogger) Info(message string, fields ...messagingapplication.RuntimeLogField) {
	if l.info != nil {
		l.info(message, fields...)
	}
}
func (scheduleLogger) Warn(string, ...messagingapplication.RuntimeLogField) {}

func scheduleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatal(err)
	}
	return db
}

func scheduleMessage(t *testing.T, db *gorm.DB, status domain.MessageStatus, publishAt, expiresAt *time.Time) uint {
	t.Helper()
	message := messaginggorm.Message{LogicalID: uuid.NewString(), OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: status, Title: "Notice", BodyHTML: "<p>Notice</p>", PublishAt: publishAt, ExpiresAt: expiresAt, AggregateVersion: 1}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	return message.ID
}

func scheduleService(db *gorm.DB) *messagingapplication.Service {
	return messagingapplication.NewService(messagingapplication.Dependencies{Messages: messaginggorm.NewRepository(db), Transactions: platformdatabase.NewTransactionRunner(db)})
}

func scheduleRound(ctx context.Context, db *gorm.DB, logger messagingapplication.RuntimeLogger, batch int, broker bool, publishScan, expireScan *messagingapplication.AnnouncementScan) {
	runMessagingCleanupRound(ctx, nil, scheduleService(db), messaginggorm.NewRepository(db), logger, "scheduler-regression", batch, broker, filesapplication.RotationConfig{}, &filesapplication.MessageImageCleanupScan{}, publishScan, expireScan)
}

func scheduleStatus(t *testing.T, db *gorm.DB, id uint) domain.MessageStatus {
	t.Helper()
	var message messaginggorm.Message
	if err := db.First(&message, id).Error; err != nil {
		t.Fatal(err)
	}
	return message.Status
}

func TestMessagingCleanupParentCancellationSkipsLaterTasks(t *testing.T) {
	db := scheduleDB(t)
	now := time.Now().UTC()
	publishAt := now.Add(-time.Hour)
	id := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var skipped, started []string
	logger := scheduleLogger{info: func(message string, fields ...messagingapplication.RuntimeLogField) {
		var task string
		for _, field := range fields {
			if field.Key == "task" {
				task, _ = field.Value.(string)
			}
		}
		switch message {
		case "messaging_cleanup_task_started":
			started = append(started, task)
			if task == "cleanup_message_images" {
				cancel()
			}
		case "messaging_cleanup_task_skipped":
			skipped = append(skipped, task)
		}
	}}
	scheduleRound(ctx, db, logger, 1, true, &messagingapplication.AnnouncementScan{}, &messagingapplication.AnnouncementScan{})
	if len(started) != 3 || started[0] != "cleanup_audience_deliveries" || started[1] != "cleanup_consumer_dead_letters" || started[2] != "cleanup_message_images" {
		t.Fatalf("tasks started after cancellation: %v", started)
	}
	if len(skipped) != 2 || skipped[0] != "publish_due_announcements" || skipped[1] != "expire_due_announcements" {
		t.Fatalf("later tasks not skipped: %v", skipped)
	}
	if got := scheduleStatus(t, db, id); got != domain.MessageStatusScheduled {
		t.Fatalf("canceled round changed announcement: %s", got)
	}
}

func TestMessagingCleanupRetainsLegacyDeletionRulesAndLimits(t *testing.T) {
	db := scheduleDB(t)
	now := time.Now().UTC()
	old, recent := now.Add(-31*24*time.Hour), now.Add(-29*24*time.Hour)
	for index := range 501 {
		delivery := messaginggorm.MessageEventDelivery{ConsumerName: "websocket", EventID: fmt.Sprintf("old-%d", index), MessageCopyID: 1, UserID: uint(index + 1), ExpiresAt: old}
		if err := db.Create(&delivery).Error; err != nil {
			t.Fatal(err)
		}
		letter := messaginggorm.MessageConsumerDeadLetter{ConsumerName: "websocket", EventID: fmt.Sprintf("final-%d", index), OriginalQueue: "websocket.dlq", Status: domain.ConsumerDLQStatusDiscarded, FinalizedAt: &old}
		if err := db.Create(&letter).Error; err != nil {
			t.Fatal(err)
		}
	}
	pending := messaginggorm.MessageConsumerDeadLetter{ConsumerName: "websocket", EventID: "pending", OriginalQueue: "websocket.dlq", Status: domain.ConsumerDLQStatusPending, FinalizedAt: &old}
	recentFinal := messaginggorm.MessageConsumerDeadLetter{ConsumerName: "websocket", EventID: "recent", OriginalQueue: "websocket.dlq", Status: domain.ConsumerDLQStatusDiscarded, FinalizedAt: &recent}
	if err := db.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&recentFinal).Error; err != nil {
		t.Fatal(err)
	}
	notDue := messaginggorm.MessageEventDelivery{ConsumerName: "websocket", EventID: "not-due", MessageCopyID: 1, UserID: 1, ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&notDue).Error; err != nil {
		t.Fatal(err)
	}
	scheduleRound(context.Background(), db, scheduleLogger{}, 1, false, &messagingapplication.AnnouncementScan{}, &messagingapplication.AnnouncementScan{})
	var deliveries, letters int64
	if err := db.Model(&messaginggorm.MessageEventDelivery{}).Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&messaginggorm.MessageConsumerDeadLetter{}).Count(&letters).Error; err != nil {
		t.Fatal(err)
	}
	if deliveries != 2 || letters != 3 {
		t.Fatalf("legacy deletion exceeded 500 or removed protected rows: deliveries=%d letters=%d", deliveries, letters)
	}
	for _, id := range []uint{pending.ID, recentFinal.ID} {
		var remaining messaginggorm.MessageConsumerDeadLetter
		if err := db.First(&remaining, id).Error; err != nil {
			t.Fatalf("protected dead letter %d removed: %v", id, err)
		}
	}
	var remaining messaginggorm.MessageEventDelivery
	if err := db.First(&remaining, notDue.ID).Error; err != nil {
		t.Fatalf("future delivery removed: %v", err)
	}
}

func TestMessagingCleanupAnnouncementBatchAdvancesPastFailuresAndFixedUpper(t *testing.T) {
	db := scheduleDB(t)
	now := time.Now().UTC()
	publishAt := now.Add(-time.Hour)
	bad := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, nil)
	good := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, nil)
	third := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, nil)
	future := now.Add(time.Hour)
	_ = scheduleMessage(t, db, domain.MessageStatusScheduled, &future, nil) // Ineligible high ID fixes the scan ceiling beyond the eligible page.
	// A failed Outbox insert is a genuine transition failure; the next copy must still run.
	if err := db.Exec(fmt.Sprintf("CREATE TRIGGER fail_first_publish BEFORE INSERT ON message_outboxes WHEN NEW.message_copy_id = %d BEGIN SELECT RAISE(FAIL, 'outbox rejected'); END", bad)).Error; err != nil {
		t.Fatal(err)
	}
	service := scheduleService(db)
	repository := messaginggorm.NewRepository(db)
	publishScan, expireScan := &messagingapplication.AnnouncementScan{}, &messagingapplication.AnnouncementScan{}
	core, logs := observer.New(zap.InfoLevel)
	logger := messagingRuntimeLogger{logger: zap.New(core)}
	round := func() {
		runMessagingCleanupRound(context.Background(), nil, service, repository, logger, "scheduler-regression", 1, true, filesapplication.RotationConfig{}, &filesapplication.MessageImageCleanupScan{}, publishScan, expireScan)
	}
	round()
	if got := scheduleStatus(t, db, bad); got != domain.MessageStatusScheduled {
		t.Fatalf("failed copy committed without outbox: %s", got)
	}
	if got := scheduleStatus(t, db, good); got != domain.MessageStatusScheduled {
		t.Fatalf("batch cap did not hold: %s", got)
	}
	later := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, nil)
	round()
	if got := scheduleStatus(t, db, good); got != domain.MessageStatusPublished {
		t.Fatalf("bad low-ID copy starved later copy: %s", got)
	}
	if got := scheduleStatus(t, db, later); got != domain.MessageStatusScheduled {
		t.Fatalf("new ID breached fixed scan upper bound: %s", got)
	}
	round()
	if got := scheduleStatus(t, db, third); got != domain.MessageStatusPublished {
		t.Fatalf("original upper candidate was skipped: %s", got)
	}
	if got := scheduleStatus(t, db, later); got != domain.MessageStatusScheduled {
		t.Fatalf("new ID breached fixed scan upper bound: %s", got)
	}
	round() // Exhaust old fixed upper; do not start a new cycle in this round.
	if got := scheduleStatus(t, db, later); got != domain.MessageStatusScheduled {
		t.Fatalf("fixed upper did not defer inserted copy: %s", got)
	}
	round() // New cycle retries the failed copy first.
	if got := scheduleStatus(t, db, bad); got != domain.MessageStatusScheduled {
		t.Fatalf("first-page failure unexpectedly succeeded: %s", got)
	}
	round()
	if got := scheduleStatus(t, db, later); got != domain.MessageStatusPublished {
		t.Fatalf("fixed upper did not refresh after wrap: %s", got)
	}
	var published int64
	if err := db.Model(&messaginggorm.MessageOutbox{}).Where("event_name = ?", domain.EventNameMessagePublished).Count(&published).Error; err != nil || published != 3 {
		t.Fatalf("published events=%d err=%v", published, err)
	}
	var failedRuns int
	for _, entry := range logs.FilterMessage("messaging_cleanup_task_finished").All() {
		fields := entry.ContextMap()
		if fields["task"] == "publish_due_announcements" && fields["failed"] == int64(1) {
			failedRuns++
		}
		if fields["task"] == "publish_due_announcements" && fields["processed"].(int64) > 1 {
			t.Fatalf("publish batch exceeded one: %+v", fields)
		}
	}
	if failedRuns != 2 {
		t.Fatalf("expected two attempts of persistent failed copy, got %d", failedRuns)
	}
}

func TestMessagingCleanupStartsOnceAndRetriesImageInNextControlledRound(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	config := platformconfig.Config{}
	config.Jwt.Secret = "schedule-test-signing-secret"
	config.FileUpload.MaxSizeMB = 50
	config.FileUpload.AvatarMaxSizeMB = 2
	config.FileUpload.DownloadURLExpireSeconds = 300
	config.Messaging.CleanupBatchSize = 1
	minioClient, err := minio.New("127.0.0.1:1", &minio.Options{Creds: credentials.NewStaticV4("probe", "secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()
	core, logs := observer.New(zap.InfoLevel)
	resources := Resources{DB: db, Redis: redisClient, MinIO: minioClient, Logger: zap.New(core)}
	running, err := New(context.Background(), config, resources, Options{Seed: func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	publishAt, expiresAt := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(-time.Minute)
	first := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, &expiresAt)
	second := scheduleMessage(t, db, domain.MessageStatusScheduled, &publishAt, &expiresAt)
	image := map[string]any{"name": "image.png", "bucket": "original", "object_name": "message-images/00000000-0000-0000-0000-000000000001.png", "content_type": "image/png", "detected_content_type": "image/png", "size": 10, "uploader_id": 7, "purpose": "message_image", "binding_expires_at": expiresAt, "validation_status": "validated"}
	if err := db.Table("files").Create(image).Error; err != nil {
		t.Fatal(err)
	}
	running.StartBackgroundJobs(context.Background())
	running.StartBackgroundJobs(context.Background())
	deadline := time.Now().Add(3 * time.Second)
	for len(logs.FilterMessage("messaging_cleanup_finished").All()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(logs.FilterMessage("messaging_cleanup_finished").All()); got != 1 {
		t.Fatalf("cleanup startup rounds=%d, want one", got)
	}
	if got := scheduleStatus(t, db, first); got != domain.MessageStatusExpired {
		t.Fatalf("first startup copy not expired: %s", got)
	}
	if got := scheduleStatus(t, db, second); got != domain.MessageStatusScheduled {
		t.Fatalf("overlapping startup round exceeded batch cap: %s", got)
	}
	var queued struct {
		ID         uint
		RetryCount uint
		Status     string
	}
	if err := db.Table("message_image_cleanup_jobs").First(&queued).Error; err != nil || queued.RetryCount != 0 || queued.Status != "pending" {
		t.Fatalf("initial image attempt: %+v, %v", queued, err)
	}
	if finished := logs.FilterMessage("messaging_cleanup_finished").All(); len(finished) != 1 || finished[0].ContextMap()["status"] != "failed" {
		t.Fatalf("image failure did not isolate and report failed round: %+v", finished)
	}
	var outbox []messaginggorm.MessageOutbox
	if err := db.Where("message_copy_id = ?", first).Find(&outbox).Error; err != nil || len(outbox) != 1 || outbox[0].EventName != domain.EventNameMessageExpired {
		t.Fatalf("expiration event after image failure: %+v, %v", outbox, err)
	}
	// Advance retry eligibility in the isolated queue; no hour-long sleep.
	if err := db.Table("message_image_cleanup_jobs").Where("id = ?", queued.ID).Update("next_retry_at", time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)).Error; err != nil {
		t.Fatal(err)
	}
	files, err := newFilesComposition(resources, config)
	if err != nil {
		t.Fatal(err)
	}
	runMessagingCleanupRound(context.Background(), files.service, scheduleService(db), messaginggorm.NewRepository(db), messagingRuntimeLogger{logger: resources.Logger}, "scheduler-regression", 1, false, filesapplication.RotationConfig{}, &filesapplication.MessageImageCleanupScan{}, &messagingapplication.AnnouncementScan{}, &messagingapplication.AnnouncementScan{})
	if err := db.Table("message_image_cleanup_jobs").First(&queued).Error; err != nil || queued.RetryCount != 1 || queued.Status != "pending" {
		t.Fatalf("retry after startup image failure: %+v, %v", queued, err)
	}
	if got := scheduleStatus(t, db, second); got != domain.MessageStatusExpired {
		t.Fatalf("second controlled round failed to expire remaining copy: %s", got)
	}
}
