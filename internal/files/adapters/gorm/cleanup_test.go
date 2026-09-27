package gormadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin/internal/files"
	gormadapter "admin/internal/files/adapters/gorm"
	"admin/internal/files/application"
	"admin/internal/files/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

func TestCleanupCandidatesRespectEligibilityAndCursor(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	repository := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	inputs := []domain.File{
		{Purpose: "message_image", BindingExpiresAt: &past},
		{Purpose: "message_image", BindingExpiresAt: &future},
		{Purpose: "message_image", BindingExpiresAt: &past, LogicalMessageID: "bound"},
		{Purpose: "managed_file", BindingExpiresAt: &past},
		{Purpose: "message_image", BindingExpiresAt: &past},
		{Purpose: "message_image", BindingExpiresAt: &past},
	}
	for i := range inputs {
		inputs[i].Bucket = "files"
		if err := repository.CreateMessageImage(ctx, &inputs[i]); err != nil {
			t.Fatal(err)
		}
	}
	page, err := repository.FindMessageImageCleanupCandidates(ctx, now, application.CleanupCursor{AfterID: inputs[0].ID, UpperID: inputs[4].ID}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].ID != inputs[4].ID {
		t.Fatalf("eligible bounded candidates = %+v", page)
	}
}

func TestCleanupRegistrationRetainsFileAndReusesJob(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, created, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil || !created || job.FileID != file.ID || job.ObjectName != file.ObjectName {
		t.Fatalf("register: %+v %v %v", job, created, err)
	}
	duplicate, created, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now.Add(time.Minute))
	if err != nil || created || duplicate.ID != job.ID || !duplicate.NextRetryAt.Equal(*job.NextRetryAt) {
		t.Fatalf("repeat registration reset job: %+v %v %v", duplicate, created, err)
	}
	var retained files.File
	if err := db.First(&retained, file.ID).Error; err != nil {
		t.Fatalf("registration deleted file before storage: %v", err)
	}
	page, err := r.FindMessageImageCleanupCandidates(ctx, now, application.CleanupCursor{UpperID: file.ID}, 10)
	if err != nil || len(page) != 0 {
		t.Fatalf("registered image selected again: %+v %v", page, err)
	}
}

func TestCleanupRegistrationRollsBackAndRejectsSharedLocation(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	first := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &first); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback requested")
	err := platformdatabase.NewTransactionRunner(db).Run(ctx, func(tx context.Context) error {
		if _, _, err := r.RegisterMessageImageCleanup(tx, first.ID, now, now); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	page, err := r.FindMessageImageCleanupCandidates(ctx, now, application.CleanupCursor{UpperID: first.ID}, 10)
	if err != nil || len(page) != 1 {
		t.Fatalf("rollback lost candidate: %+v %v", page, err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, first.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = 0
	if err := r.CreateMessageImage(ctx, &second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.RegisterMessageImageCleanup(ctx, second.ID, now, now); !errors.Is(err, application.ErrStateConflict) || !errors.Is(err, application.ErrMessageImageCleanupHashConflict) {
		t.Fatalf("shared object accepted: %v", err)
	}
	if err := db.Model(&files.MessageImageCleanupJob{}).Where("id = ?", job.ID).Update("object_key_hash", "different").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.RegisterMessageImageCleanup(ctx, first.ID, now, now); !errors.Is(err, application.ErrStateConflict) || !errors.Is(err, application.ErrMessageImageCleanupHashConflict) {
		t.Fatalf("inconsistent path hash accepted: %v", err)
	}
}

func TestMessageImageBindingRejectsStaleRequestTime(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "expired.png", UploaderID: 7, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	err := r.BindMessageImages(ctx, application.MessageImageBindRequest{ActorID: 7, MessageLogicalID: "logical", ImageIDs: []uint{file.ID}, Now: now.Add(-time.Hour)})
	if !errors.Is(err, application.ErrStateConflict) {
		t.Fatalf("stale request bound expired image: %v", err)
	}
}

func TestMessageImageBindingIsAtomicAndRejectsQueuedImages(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	future := time.Now().UTC().Add(time.Hour)
	first := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "one.png", UploaderID: 7, BindingExpiresAt: &future}
	second := first
	second.ObjectName = "two.png"
	second.UploaderID = 8
	if err := r.CreateMessageImage(ctx, &first); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateMessageImage(ctx, &second); err != nil {
		t.Fatal(err)
	}
	request := application.MessageImageBindRequest{ActorID: 7, MessageLogicalID: "logical", ImageIDs: []uint{second.ID, first.ID}}
	if err := r.BindMessageImages(ctx, request); !errors.Is(err, application.ErrStateConflict) {
		t.Fatalf("wrong owner accepted: %v", err)
	}
	loaded, err := r.FindMessageImage(ctx, first.ID)
	if err != nil || loaded.LogicalMessageID != "" {
		t.Fatalf("partial binding survived: %+v %v", loaded, err)
	}
	job := files.MessageImageCleanupJob{FileID: first.ID, Bucket: "files", ObjectName: first.ObjectName, ObjectKeyHash: "queued", Status: "dead"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	request.ImageIDs = []uint{first.ID}
	if err := r.BindMessageImages(ctx, request); !errors.Is(err, application.ErrStateConflict) {
		t.Fatalf("dead cleanup image bound: %v", err)
	}
}

func TestMessageImageReadRejectsRegisteredCleanup(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "image.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FindMessageImage(ctx, file.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FindMessageImage(ctx, file.ID); !errors.Is(err, application.ErrFileNotFound) {
		t.Fatalf("queued image remains readable: %v", err)
	}
}

func TestCleanupRetryReservationRejectsStaleAttempt(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "retry.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	attemptAt := now.Add(time.Hour)
	reserved, ok, err := r.ReserveMessageImageCleanupRetry(ctx, job, attemptAt, attemptAt)
	if err != nil || !ok || reserved.RetryCount != 1 || reserved.NextRetryAt == nil || !reserved.NextRetryAt.Equal(attemptAt.Add(time.Hour)) {
		t.Fatalf("reserve: %+v %v %v", reserved, ok, err)
	}
	if _, ok, err := r.ReserveMessageImageCleanupRetry(ctx, job, attemptAt, attemptAt); err != nil || ok {
		t.Fatalf("stale attempt reserved twice: %v %v", ok, err)
	}
	if _, ok, err := r.ReserveMessageImageCleanupRetry(ctx, reserved, attemptAt, attemptAt); err != nil || ok {
		t.Fatalf("future retry ran early: %v %v", ok, err)
	}
}

func TestCleanupCompletionCannotBeResurrectedByLateFailure(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "complete.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := r.CompleteMessageImageCleanup(ctx, job); err != nil || !ok {
		t.Fatalf("complete: %v %v", ok, err)
	}
	if ok, err := r.FailMessageImageCleanup(ctx, job, "storage_unavailable"); err != nil || ok {
		t.Fatalf("late failure recreated job: %v %v", ok, err)
	}
	var count int64
	if err := db.Unscoped().Model(&files.File{}).Where("id = ?", file.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("completed file not physically deleted")
	}
	if ok, err := r.CompleteMessageImageCleanup(ctx, job); err != nil || ok {
		t.Fatalf("duplicate completion: %v %v", ok, err)
	}
}

func TestCleanupLastRetryCrashBecomesDeadWithoutAnotherReservation(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "last.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := uint(1); attempt <= 24; attempt++ {
		now = now.Add(time.Hour)
		var ok bool
		job, ok, err = r.ReserveMessageImageCleanupRetry(ctx, job, now, now)
		if err != nil || !ok || job.RetryCount != attempt {
			t.Fatalf("attempt %d: %+v %v %v", attempt, job, ok, err)
		}
	}
	if ok, err := r.DeadLetterExhaustedMessageImageCleanup(ctx, job, now); err != nil || ok {
		t.Fatalf("live last attempt terminated early: %v %v", ok, err)
	}
	now = now.Add(time.Hour)
	if _, ok, err := r.ReserveMessageImageCleanupRetry(ctx, job, now, now); err != nil || ok {
		t.Fatalf("reserved attempt 25: %v %v", ok, err)
	}
	if ok, err := r.DeadLetterExhaustedMessageImageCleanup(ctx, job, now); err != nil || !ok {
		t.Fatalf("crashed last attempt not terminated: %v %v", ok, err)
	}
	var stored files.MessageImageCleanupJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "dead" || stored.NextRetryAt != nil || stored.RetryCount != 24 {
		t.Fatalf("terminal state: %+v", stored)
	}
}

func TestCleanupCompletionRollbackKeepsRecoverableJob(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "rollback.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("commit rejected")
	err = platformdatabase.NewTransactionRunner(db).Run(ctx, func(tx context.Context) error {
		if _, err := r.CompleteMessageImageCleanup(tx, job); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if ok, err := r.CompleteMessageImageCleanup(ctx, job); err != nil || !ok {
		t.Fatalf("rolled-back cleanup cannot recover: %v %v", ok, err)
	}
}

func TestCleanupCompletionRejectsZeroIdentity(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	if _, err := r.CompleteMessageImageCleanup(context.Background(), application.MessageImageCleanupJob{}); !errors.Is(err, application.ErrStateConflict) {
		t.Fatalf("zero identity accepted: %v", err)
	}
}

func TestCleanupFailureCannotOverwriteNewAttemptAndLastFailureIsDead(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "failed.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	initial := job
	for attempt := uint(1); attempt <= 24; attempt++ {
		now = now.Add(time.Hour)
		var ok bool
		job, ok, err = r.ReserveMessageImageCleanupRetry(ctx, job, now, now)
		if err != nil || !ok {
			t.Fatalf("reserve %d: %v %v", attempt, ok, err)
		}
	}
	if ok, err := r.FailMessageImageCleanup(ctx, initial, "old_failure"); err != nil || ok {
		t.Fatalf("stale failure overwrote attempt: %v %v", ok, err)
	}
	if ok, err := r.FailMessageImageCleanup(ctx, job, "storage_unavailable"); err != nil || !ok {
		t.Fatalf("last failure: %v %v", ok, err)
	}
	var stored files.MessageImageCleanupJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "dead" || stored.NextRetryAt != nil || stored.LastErrorCode != "storage_unavailable" {
		t.Fatalf("last failure state: %+v", stored)
	}
	if ok, err := r.CompleteMessageImageCleanup(ctx, job); err != nil || !ok {
		t.Fatalf("successful overlapping deletion cannot finish dead job: %v %v", ok, err)
	}
}

func TestCleanupRegistrationRejectsHashCollisionAndPreservesDead(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "collision.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	job, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&files.MessageImageCleanupJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": "dead", "retry_count": 24, "next_retry_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	reused, created, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now)
	if err != nil || created || reused.Status != "dead" || reused.RetryCount != 24 {
		t.Fatalf("dead task reset: %+v %v %v", reused, created, err)
	}
	if err := db.Model(&files.MessageImageCleanupJob{}).Where("id = ?", job.ID).Update("object_name", "different.png").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.RegisterMessageImageCleanup(ctx, file.ID, now, now); !errors.Is(err, application.ErrStateConflict) || !errors.Is(err, application.ErrMessageImageCleanupHashConflict) {
		t.Fatalf("same hash different path accepted: %v", err)
	}
}

func TestCleanupRegistrationRejectsDistinctConflictsOnBothUniqueIndexes(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	first := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/first.png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &first); err != nil {
		t.Fatal(err)
	}
	firstJob, _, err := r.RegisterMessageImageCleanup(ctx, first.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = 0
	second.ObjectName = "message-images/second.png"
	if err := r.CreateMessageImage(ctx, &second); err != nil {
		t.Fatal(err)
	}
	secondJob, _, err := r.RegisterMessageImageCleanup(ctx, second.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&files.File{}).Where("id = ?", first.ID).Update("object_name", second.ObjectName).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.RegisterMessageImageCleanup(ctx, first.ID, now, now); !errors.Is(err, application.ErrMessageImageCleanupHashConflict) {
		t.Fatalf("separate file and path index matches accepted: %v", err)
	}
	var count int64
	if err := db.Model(&files.MessageImageCleanupJob{}).Where("id IN ?", []uint{firstJob.ID, secondJob.ID}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("conflicting jobs were modified: %d %v", count, err)
	}
}

func TestRotationExcludesUnboundAndQueuedMessageImages(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	inputs := []domain.File{
		{Purpose: "managed_file"},
		{Purpose: "message_image", LogicalMessageID: "bound"},
		{Purpose: "message_image", BindingExpiresAt: &past},
		{Purpose: "message_image", LogicalMessageID: "queued"},
	}
	for i := range inputs {
		inputs[i].Bucket = "files"
		inputs[i].CreatedAt = past
		if err := r.CreateMessageImage(ctx, &inputs[i]); err != nil {
			t.Fatal(err)
		}
	}
	job := files.MessageImageCleanupJob{FileID: inputs[3].ID, Bucket: "files", ObjectName: "queued", ObjectKeyHash: "queued", Status: "dead"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	got, err := r.FindRotationCandidates(ctx, now, "files", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != inputs[0].ID || got[1].ID != inputs[1].ID {
		t.Fatalf("unsafe rotation candidates: %+v", got)
	}
}

func TestCleanupRetryCandidatesRespectDeadlineAndScanBounds(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := gormadapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	jobs := []files.MessageImageCleanupJob{
		{FileID: 1, Bucket: "files", ObjectName: "a", ObjectKeyHash: "a", Status: "dead"},
		{FileID: 2, Bucket: "files", ObjectName: "b", ObjectKeyHash: "b", Status: "pending", NextRetryAt: &future},
		{FileID: 3, Bucket: "files", ObjectName: "c", ObjectKeyHash: "c", Status: "pending", NextRetryAt: &past},
		{FileID: 4, Bucket: "files", ObjectName: "d", ObjectKeyHash: "d", Status: "pending", NextRetryAt: &past},
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	upper, err := r.MessageImageCleanupUpperID(ctx, true)
	if err != nil || upper != jobs[3].ID {
		t.Fatalf("retry upper bound: %d %v", upper, err)
	}
	got, err := r.FindMessageImageCleanupRetries(ctx, now, application.CleanupCursor{AfterID: jobs[0].ID, UpperID: jobs[2].ID}, 1)
	if err != nil || len(got) != 1 || got[0].ID != jobs[2].ID {
		t.Fatalf("due retry page: %+v %v", got, err)
	}
}
