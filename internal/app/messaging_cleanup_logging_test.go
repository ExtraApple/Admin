package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	files "admin/internal/files"
	filesadapter "admin/internal/files/adapters/gorm"
	filesapplication "admin/internal/files/application"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/uploadsecurity"
	"admin/testsupport/testutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

type cleanupLoggingStorage struct {
	objects      map[string]bool
	deleteErr    error
	cancelOnStat int
	blockOnStat  int
	cancel       context.CancelFunc
	stats        int
}

func (s *cleanupLoggingStorage) Put(context.Context, filesapplication.ObjectInput) error { return nil }
func (s *cleanupLoggingStorage) Open(context.Context, string, string) (io.ReadCloser, error) {
	return nil, uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil)
}
func (s *cleanupLoggingStorage) List(context.Context, string, filesapplication.ListOptions) ([]filesapplication.Object, error) {
	return nil, nil
}
func (s *cleanupLoggingStorage) Move(context.Context, string, string, string) error { return nil }
func (s *cleanupLoggingStorage) Stat(ctx context.Context, bucket, name string) (filesapplication.Object, error) {
	s.stats++
	if s.cancelOnStat > 0 && s.stats == s.cancelOnStat {
		s.cancel()
		return filesapplication.Object{}, context.Canceled
	}
	if s.blockOnStat > 0 && s.stats == s.blockOnStat {
		<-ctx.Done()
		return filesapplication.Object{}, ctx.Err()
	}
	if s.objects[bucket+"/"+name] {
		return filesapplication.Object{Name: name}, nil
	}
	return filesapplication.Object{}, uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil)
}
func (s *cleanupLoggingStorage) Delete(_ context.Context, bucket, name string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.objects, bucket+"/"+name)
	return nil
}

func newCleanupLoggingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(append(files.Models(), messaginggorm.Models()...)...); err != nil {
		t.Fatal(err)
	}
	return db
}

func cleanupSink(t *testing.T) (*zap.Logger, *bytes.Buffer) {
	t.Helper()
	var sink bytes.Buffer
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&sink), zap.InfoLevel))
	t.Cleanup(func() { _ = logger.Sync() })
	return logger, &sink
}

func cleanupFile(t *testing.T, db *gorm.DB, object string) files.File {
	t.Helper()
	expired := time.Now().UTC().Add(-time.Hour)
	file := files.File{Name: "notice.png", Bucket: "files", ObjectName: object, ContentType: "image/png", Purpose: "message_image", BindingExpiresAt: &expired, ValidationStatus: files.FileValidationStatusValidated}
	if err := db.Create(&file).Error; err != nil {
		t.Fatal(err)
	}
	return file
}

func runImageCleanupRound(t *testing.T, db *gorm.DB, storage filesapplication.ObjectStorage, logger *zap.Logger, ctx context.Context, batch int) {
	t.Helper()
	service := filesapplication.NewService(filesapplication.Dependencies{Repository: filesadapter.NewRepository(db), MessageImages: filesadapter.NewRepository(db), Storage: storage})
	runMessagingCleanupRound(ctx, service, nil, nil, messagingRuntimeLogger{logger: logger}, "worker-fixed", batch, false, filesapplication.RotationConfig{}, &filesapplication.MessageImageCleanupScan{}, nil, nil)
}

func decodeCleanupLogs(t *testing.T, sink *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(sink.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log %q: %v", line, err)
		}
		if entry["msg"] == "messaging_cleanup_task_finished" || entry["msg"] == "messaging_cleanup_task_skipped" || entry["msg"] == "messaging_cleanup_finished" {
			processed, ok := entry["processed"].(float64)
			if !ok {
				t.Fatalf("missing processed count: %#v", entry)
			}
			succeeded, successOK := entry["succeeded"].(float64)
			skipped, skipOK := entry["skipped"].(float64)
			failed, failOK := entry["failed"].(float64)
			duration, durationOK := entry["duration_ms"].(float64)
			if !successOK || !skipOK || !failOK || processed != succeeded+skipped+failed || !durationOK || duration < 0 || duration != float64(int64(duration)) {
				t.Fatalf("invalid cleanup summary: %#v", entry)
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func findCleanupLog(entries []map[string]any, message, task string) map[string]any {
	for _, entry := range entries {
		if entry["msg"] == message && (task == "" || entry["task"] == task) {
			return entry
		}
	}
	return nil
}

func TestMessagingCleanupLoggingUsesRealJSONSinkAndSafeSummaries(t *testing.T) {
	db := newCleanupLoggingDB(t)
	logger, sink := cleanupSink(t)
	object := "message-images/8b4b8a3e-22d7-46d8-9dbf-2f7021d4f5c0.png"
	cleanupFile(t, db, object)
	storage := &cleanupLoggingStorage{objects: map[string]bool{}}
	runImageCleanupRound(t, db, storage, logger, context.Background(), 1)
	_ = logger.Sync()
	entries := decodeCleanupLogs(t, sink)
	started := findCleanupLog(entries, "messaging_cleanup_started", "")
	finished := findCleanupLog(entries, "messaging_cleanup_finished", "messaging-cleanup")
	imageFinished := findCleanupLog(entries, "messaging_cleanup_task_finished", "cleanup_message_images")
	if started == nil || finished == nil || imageFinished == nil {
		t.Fatalf("missing cleanup summaries: %#v", entries)
	}
	if started["worker_id"] != "worker-fixed" || started["run_id"] == "" || finished["run_id"] != started["run_id"] || imageFinished["run_id"] != started["run_id"] {
		t.Fatalf("round linkage: start=%#v finish=%#v image=%#v", started, finished, imageFinished)
	}
	if _, err := time.Parse(time.RFC3339Nano, started["run_at"].(string)); err != nil {
		t.Fatalf("run_at is not RFC3339Nano: %#v (%v)", started["run_at"], err)
	}
	if !strings.HasSuffix(started["run_at"].(string), "Z") {
		t.Fatalf("run_at is not UTC: %#v", started["run_at"])
	}
	if _, ok := imageFinished["duration_ms"].(float64); !ok {
		t.Fatalf("duration_ms is not encoded as JSON number: %#v", imageFinished["duration_ms"])
	}
	if imageFinished["processed"] != float64(1) || imageFinished["succeeded"] != float64(1) || imageFinished["skipped"] != float64(0) || imageFinished["failed"] != float64(0) {
		t.Fatalf("NoSuchKey success summary: %#v", imageFinished)
	}
	for _, entry := range entries {
		if _, exists := entry["bucket"]; exists {
			t.Fatalf("bucket leaked into runtime log: %#v", entry)
		}
		if _, exists := entry["object_name"]; exists {
			t.Fatalf("object name leaked into runtime log: %#v", entry)
		}
		encoded, _ := json.Marshal(entry)
		for _, sensitive := range []string{"message-images/8b4b8a3e-22d7-46d8-9dbf-2f7021d4f5c0.png", "files", "database unavailable", "password=secret"} {
			if bytes.Contains(encoded, []byte(sensitive)) {
				t.Fatalf("sensitive value %q leaked: %s", sensitive, encoded)
			}
		}
	}
}

func TestMessagingCleanupLoggingKeepsZeroOnQueryFailureAndConfiguredSkip(t *testing.T) {
	db := newCleanupLoggingDB(t)
	if err := db.Migrator().DropTable(&files.MessageImageCleanupJob{}); err != nil {
		t.Fatal(err)
	}
	logger, sink := cleanupSink(t)
	imageService := filesapplication.NewService(filesapplication.Dependencies{Repository: filesadapter.NewRepository(db), MessageImages: filesadapter.NewRepository(db), Storage: &cleanupLoggingStorage{objects: map[string]bool{}}})
	// The dropped queue table makes the image candidate query fail without attempting a row.
	runMessagingCleanupRound(context.Background(), imageService, nil, nil, messagingRuntimeLogger{logger: logger}, "worker-query", 1, false, filesapplication.RotationConfig{}, &filesapplication.MessageImageCleanupScan{}, nil, nil)
	_ = logger.Sync()
	entries := decodeCleanupLogs(t, sink)
	for _, task := range []string{"cleanup_audience_deliveries", "cleanup_consumer_dead_letters", "cleanup_message_images", "expire_due_announcements"} {
		entry := findCleanupLog(entries, "messaging_cleanup_task_finished", task)
		if entry == nil {
			t.Fatalf("missing task failure summary %q: %#v", task, entries)
		}
		if entry["processed"] != float64(0) {
			t.Fatalf("query/dependency failure counted processed rows: %q %#v", task, entry)
		}
	}
	skipped := findCleanupLog(entries, "messaging_cleanup_task_skipped", "publish_due_announcements")
	if skipped == nil || skipped["failure_code"] != "rabbitmq_not_configured" || skipped["processed"] != float64(0) || skipped["succeeded"] != float64(0) || skipped["skipped"] != float64(0) || skipped["failed"] != float64(0) {
		t.Fatalf("configured broker skip summary: %#v", skipped)
	}
}

func TestMessagingCleanupLoggingKeepsCompletedCountOnTimeout(t *testing.T) {
	db := newCleanupLoggingDB(t)
	logger, sink := cleanupSink(t)
	first := "message-images/40000000-0000-0000-0000-000000000001.png"
	second := "message-images/40000000-0000-0000-0000-000000000002.png"
	cleanupFile(t, db, first)
	cleanupFile(t, db, second)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	storage := &cleanupLoggingStorage{objects: map[string]bool{"files/" + first: true}, blockOnStat: 2}
	runImageCleanupRound(t, db, storage, logger, ctx, 4)
	_ = logger.Sync()
	entries := decodeCleanupLogs(t, sink)
	image := findCleanupLog(entries, "messaging_cleanup_task_finished", "cleanup_message_images")
	round := findCleanupLog(entries, "messaging_cleanup_finished", "messaging-cleanup")
	if image == nil || image["status"] != "timed_out" || image["timed_out"] != true || image["processed"] != float64(2) || image["succeeded"] != float64(1) || image["failed"] != float64(1) {
		t.Fatalf("timeout dropped completed outcome: %#v", image)
	}
	if round == nil || round["status"] != "timed_out" || round["timed_out"] != true || round["processed"].(float64) < image["processed"].(float64) {
		t.Fatalf("timeout round summary lost task counts: %#v", round)
	}
}

func TestMessagingCleanupLoggingPreservesCanceledPartialCountsAndStableFailureCodes(t *testing.T) {
	db := newCleanupLoggingDB(t)
	logger, sink := cleanupSink(t)
	firstObject := "message-images/10000000-0000-0000-0000-000000000001.png"
	secondObject := "message-images/10000000-0000-0000-0000-000000000002.png"
	thirdObject := "message-images/10000000-0000-0000-0000-000000000003.png"
	cleanupFile(t, db, firstObject)
	cleanupFile(t, db, secondObject)
	cleanupFile(t, db, thirdObject)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	storage := &cleanupLoggingStorage{objects: map[string]bool{"files/" + firstObject: true}, cancelOnStat: 2, cancel: cancel}
	runImageCleanupRound(t, db, storage, logger, ctx, 6)
	_ = logger.Sync()
	entries := decodeCleanupLogs(t, sink)
	imageFinished := findCleanupLog(entries, "messaging_cleanup_task_finished", "cleanup_message_images")
	if imageFinished == nil || imageFinished["processed"] != float64(2) || imageFinished["succeeded"] != float64(1) || imageFinished["failed"] != float64(1) {
		t.Fatalf("canceled partial result was not retained: %#v", imageFinished)
	}
	if imageFinished["processed"] != imageFinished["succeeded"].(float64)+imageFinished["skipped"].(float64)+imageFinished["failed"].(float64) {
		t.Fatalf("canceled result violates processed decomposition: %#v", imageFinished)
	}
	failure := findCleanupLog(entries, "messaging_cleanup_item_failed", "cleanup_message_images")
	if failure == nil || failure["failure_code"] != "message_image_cleanup_canceled" {
		t.Fatalf("canceled item did not emit stable failure code: %#v", failure)
	}

	// A storage error leaves the first job pending; the second same-path file
	// then exercises the stable hash-conflict code through real registration.
	db2 := newCleanupLoggingDB(t)
	logger2, sink2 := cleanupSink(t)
	shared := "message-images/20000000-0000-0000-0000-000000000001.png"
	cleanupFile(t, db2, shared)
	runImageCleanupRound(t, db2, &cleanupLoggingStorage{objects: map[string]bool{"files/" + shared: true}, deleteErr: errors.New("storage backend host=10.0.0.9 password=secret")}, logger2, context.Background(), 4)
	second := cleanupFile(t, db2, shared)
	runImageCleanupRound(t, db2, &cleanupLoggingStorage{objects: map[string]bool{}}, logger2, context.Background(), 4)
	_ = logger2.Sync()
	entries2 := decodeCleanupLogs(t, sink2)
	seen := map[string]bool{}
	for _, entry := range entries2 {
		if entry["msg"] == "messaging_cleanup_item_failed" {
			if code, ok := entry["failure_code"].(string); ok {
				seen[code] = true
			}
		}
		encoded, _ := json.Marshal(entry)
		if bytes.Contains(encoded, []byte("10.0.0.9")) || bytes.Contains(encoded, []byte("password=secret")) {
			t.Fatalf("storage error leaked: %s", encoded)
		}
	}
	if !seen["message_image_cleanup_storage_unavailable"] || !seen["message_image_cleanup_hash_conflict"] || len(seen) != 2 {
		t.Fatalf("unexpected failure codes: %#v", seen)
	}
	var hashLog map[string]any
	for _, entry := range entries2 {
		if entry["failure_code"] == "message_image_cleanup_hash_conflict" {
			hashLog = entry
		}
	}
	if hashLog == nil || hashLog["level"] != "warn" || hashLog["file_id"] != float64(second.ID) || hashLog["stage"] != "register" {
		t.Fatalf("hash collision log omitted File Record ID: %#v", hashLog)
	}
	if _, exists := hashLog["cleanup_job_id"]; exists {
		t.Fatalf("registration failure forged a queue ID: %#v", hashLog)
	}
}

func TestMessagingCleanupLoggingEmitsScopeAndDeadCodesWithoutSensitiveDetails(t *testing.T) {
	db := newCleanupLoggingDB(t)
	logger, sink := cleanupSink(t)
	now := time.Now().UTC()
	bad := files.MessageImageCleanupJob{FileID: 901, Bucket: "secret-bucket", ObjectName: "untrusted/private-key", ObjectKeyHash: strings.Repeat("a", 64), Status: "pending", RetryCount: 1, NextRetryAt: ptrCleanupTime(now.Add(-time.Minute))}
	dead := files.MessageImageCleanupJob{FileID: 902, Bucket: "secret-bucket", ObjectName: "message-images/30000000-0000-0000-0000-000000000001.png", ObjectKeyHash: strings.Repeat("b", 64), Status: "pending", RetryCount: 24, NextRetryAt: ptrCleanupTime(now.Add(-time.Minute))}
	if err := db.Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&dead).Error; err != nil {
		t.Fatal(err)
	}
	storage := &cleanupLoggingStorage{objects: map[string]bool{}}
	runImageCleanupRound(t, db, storage, logger, context.Background(), 2)
	_ = logger.Sync()
	entries := decodeCleanupLogs(t, sink)
	deadEntry := findCleanupLog(entries, "messaging_cleanup_dead_lettered", "cleanup_message_images")
	if deadEntry == nil || deadEntry["level"] != "warn" || deadEntry["cleanup_job_id"] != float64(dead.ID) || deadEntry["file_id"] != float64(dead.FileID) || deadEntry["retry_attempt"] != float64(24) || deadEntry["stage"] != "dead" || deadEntry["failure_code"] != "message_image_cleanup_retry_exhausted" {
		t.Fatalf("dead signal omitted stable IDs or retry attempt: %#v", deadEntry)
	}
	codes := map[string]bool{}
	for _, entry := range entries {
		if entry["msg"] == "messaging_cleanup_item_failed" || entry["msg"] == "messaging_cleanup_dead_lettered" {
			if code, ok := entry["failure_code"].(string); ok {
				codes[code] = true
			}
		}
		encoded, _ := json.Marshal(entry)
		for _, forbidden := range []string{"secret-bucket", "untrusted/private-key"} {
			if bytes.Contains(encoded, []byte(forbidden)) {
				t.Fatalf("cleanup location leaked: %s", encoded)
			}
		}
	}
	if !codes["message_image_cleanup_scope_untrusted"] || !codes["message_image_cleanup_retry_exhausted"] {
		t.Fatalf("scope/dead failure codes: %#v", codes)
	}
}

func ptrCleanupTime(value time.Time) *time.Time { return &value }

var _ filesapplication.ObjectStorage = (*cleanupLoggingStorage)(nil)
