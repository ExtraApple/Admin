package files_test

import (
	"strings"
	"testing"
	"time"

	"admin/internal/files"
	"admin/testsupport/testutil"
)

func TestMessageImageCleanupJobPreservesLocationAndRejectsDuplicateFile(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&files.MessageImageCleanupJob{}); err != nil {
		t.Fatal(err)
	}
	next := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	job := files.MessageImageCleanupJob{
		FileID: 17, Bucket: strings.Repeat("b", 100), ObjectName: strings.Repeat("n", 500),
		ObjectKeyHash: strings.Repeat("a", 64), Status: "pending", NextRetryAt: &next,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	var restored files.MessageImageCleanupJob
	if err := db.First(&restored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if restored.Bucket != job.Bucket || restored.ObjectName != job.ObjectName || restored.NextRetryAt == nil || !restored.NextRetryAt.Equal(next) {
		t.Fatalf("cleanup lost its recoverable location or schedule: %+v", restored)
	}
	duplicate := files.MessageImageCleanupJob{FileID: 17, Bucket: "another", ObjectName: "another", ObjectKeyHash: strings.Repeat("b", 64), Status: "pending"}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("same file accepted a second cleanup job")
	}
}
