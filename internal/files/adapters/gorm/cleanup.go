package gormadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"admin/internal/files"
	"admin/internal/files/application"
	"admin/internal/files/domain"
	platformdatabase "admin/internal/platform/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) FindMessageImageCleanupCandidates(ctx context.Context, now time.Time, cursor application.CleanupCursor, limit int) ([]domain.File, error) {
	if limit < 1 || limit > 1000 {
		return nil, application.ErrStateConflict
	}
	var records []files.File
	err := r.connection(ctx).
		Where("purpose = ? AND (logical_message_id IS NULL OR logical_message_id = '') AND binding_expires_at <= ?", "message_image", now.UTC()).
		Where("id > ? AND id <= ?", cursor.AfterID, cursor.UpperID).
		Where("NOT EXISTS (SELECT 1 FROM message_image_cleanup_jobs AS job WHERE job.file_id = files.id)").
		Order("id ASC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.File, len(records))
	for i := range records {
		result[i] = toDomain(records[i])
	}
	return result, nil
}

func (r *Repository) RegisterMessageImageCleanup(ctx context.Context, fileID uint, now, attemptedAt time.Time) (application.MessageImageCleanupJob, bool, error) {
	var job files.MessageImageCleanupJob
	created := false
	err := platformdatabase.NewTransactionRunner(r.db).Run(ctx, func(tx context.Context) error {
		created = false
		job = files.MessageImageCleanupJob{}
		db := r.connection(tx)
		var file files.File
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&file, fileID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrFileNotFound
			}
			return err
		}
		if file.Purpose != "message_image" || file.LogicalMessageID != "" || file.BindingExpiresAt == nil || file.BindingExpiresAt.After(now) {
			return application.ErrStateConflict
		}
		digest := sha256.Sum256([]byte(file.Bucket + "\x00" + file.ObjectName))
		key := hex.EncodeToString(digest[:])
		var matches []files.MessageImageCleanupJob
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("file_id = ? OR object_key_hash = ?", file.ID, key).Limit(2).Find(&matches).Error; err != nil {
			return err
		}
		if len(matches) != 0 {
			if len(matches) != 1 || matches[0].FileID != file.ID || matches[0].Bucket != file.Bucket || matches[0].ObjectName != file.ObjectName || matches[0].ObjectKeyHash != key {
				return errors.Join(application.ErrStateConflict, application.ErrMessageImageCleanupHashConflict)
			}
			job = matches[0]
			return nil
		}
		next := attemptedAt.UTC().Add(time.Hour)
		job = files.MessageImageCleanupJob{FileID: file.ID, Bucket: file.Bucket, ObjectName: file.ObjectName, ObjectKeyHash: key, Status: "pending", NextRetryAt: &next, CreatedAt: attemptedAt.UTC(), UpdatedAt: attemptedAt.UTC()}
		if err := db.Create(&job).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return application.MessageImageCleanupJob{}, false, err
	}
	return cleanupJobValue(job), created, nil
}

func cleanupJobValue(job files.MessageImageCleanupJob) application.MessageImageCleanupJob {
	return application.MessageImageCleanupJob{ID: job.ID, FileID: job.FileID, Bucket: job.Bucket, ObjectName: job.ObjectName, ObjectKeyHash: job.ObjectKeyHash, Status: job.Status, RetryCount: job.RetryCount, NextRetryAt: job.NextRetryAt, LastErrorCode: job.LastErrorCode}
}

func (r *Repository) ReserveMessageImageCleanupRetry(ctx context.Context, job application.MessageImageCleanupJob, now, attemptedAt time.Time) (application.MessageImageCleanupJob, bool, error) {
	if job.ID == 0 || job.Status != "pending" || job.NextRetryAt == nil || job.NextRetryAt.After(now) || job.RetryCount >= 24 {
		return application.MessageImageCleanupJob{}, false, nil
	}
	next := attemptedAt.UTC().Add(time.Hour)
	result := r.connection(ctx).Model(&files.MessageImageCleanupJob{}).
		Where("id = ? AND status = ? AND retry_count = ? AND next_retry_at = ? AND next_retry_at <= ?", job.ID, "pending", job.RetryCount, job.NextRetryAt, now.UTC()).
		Updates(map[string]any{"retry_count": job.RetryCount + 1, "next_retry_at": next})
	if result.Error != nil || result.RowsAffected == 0 {
		return application.MessageImageCleanupJob{}, false, result.Error
	}
	job.RetryCount++
	job.NextRetryAt = &next
	return job, true, nil
}

func (r *Repository) FailMessageImageCleanup(ctx context.Context, job application.MessageImageCleanupJob, code string) (bool, error) {
	if job.ID == 0 || job.NextRetryAt == nil {
		return false, application.ErrStateConflict
	}
	updates := map[string]any{"last_error_code": code}
	if job.RetryCount >= 24 {
		updates["status"] = "dead"
		updates["next_retry_at"] = nil
	}
	result := r.connection(ctx).Model(&files.MessageImageCleanupJob{}).
		Where("id = ? AND status = ? AND retry_count = ? AND next_retry_at = ?", job.ID, "pending", job.RetryCount, job.NextRetryAt).
		Updates(updates)
	return result.RowsAffected != 0, result.Error
}

func (r *Repository) CompleteMessageImageCleanup(ctx context.Context, expected application.MessageImageCleanupJob) (bool, error) {
	if expected.ID == 0 || expected.FileID == 0 {
		return false, application.ErrStateConflict
	}
	completed := false
	err := platformdatabase.NewTransactionRunner(r.db).Run(ctx, func(tx context.Context) error {
		completed = false
		db := r.connection(tx)
		var file files.File
		fileErr := db.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&file, expected.FileID).Error
		if fileErr != nil && !errors.Is(fileErr, gorm.ErrRecordNotFound) {
			return fileErr
		}
		var job files.MessageImageCleanupJob
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, expected.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if job.FileID != expected.FileID || job.Bucket != expected.Bucket || job.ObjectName != expected.ObjectName || job.ObjectKeyHash != expected.ObjectKeyHash {
			return application.ErrStateConflict
		}
		if fileErr == nil {
			if file.Purpose != "message_image" || file.LogicalMessageID != "" || file.Bucket != job.Bucket || file.ObjectName != job.ObjectName {
				return application.ErrStateConflict
			}
			if err := db.Unscoped().Delete(&file).Error; err != nil {
				return err
			}
		}
		if err := db.Delete(&job).Error; err != nil {
			return err
		}
		completed = true
		return nil
	})
	return completed && err == nil, err
}

func (r *Repository) DeadLetterExhaustedMessageImageCleanup(ctx context.Context, job application.MessageImageCleanupJob, now time.Time) (bool, error) {
	if job.ID == 0 || job.RetryCount != 24 || job.NextRetryAt == nil || job.NextRetryAt.After(now) {
		return false, nil
	}
	result := r.connection(ctx).Model(&files.MessageImageCleanupJob{}).
		Where("id = ? AND status = ? AND retry_count = ? AND next_retry_at = ? AND next_retry_at <= ?", job.ID, "pending", 24, job.NextRetryAt, now.UTC()).
		Updates(map[string]any{"status": "dead", "next_retry_at": nil, "last_error_code": "message_image_cleanup_retry_exhausted"})
	return result.RowsAffected != 0, result.Error
}

func (r *Repository) MessageImageCleanupUpperID(ctx context.Context, retries bool) (uint, error) {
	var upper uint
	query := r.connection(ctx).Model(&files.File{})
	if retries {
		query = r.connection(ctx).Model(&files.MessageImageCleanupJob{})
	}
	err := query.Select("COALESCE(MAX(id), 0)").Scan(&upper).Error
	return upper, err
}

func (r *Repository) FindMessageImageCleanupRetries(ctx context.Context, now time.Time, cursor application.CleanupCursor, limit int) ([]application.MessageImageCleanupJob, error) {
	if limit < 1 || limit > 1000 {
		return nil, application.ErrStateConflict
	}
	var records []files.MessageImageCleanupJob
	err := r.connection(ctx).Where("status = ? AND next_retry_at <= ? AND id > ? AND id <= ?", "pending", now.UTC(), cursor.AfterID, cursor.UpperID).
		Order("id ASC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	result := make([]application.MessageImageCleanupJob, len(records))
	for i := range records {
		result[i] = cleanupJobValue(records[i])
	}
	return result, nil
}

func (r *Repository) MessageImageCleanupLocationConflicts(ctx context.Context, job application.MessageImageCleanupJob, buckets []string) (bool, error) {
	var other files.File
	err := r.connection(ctx).Where("id <> ? AND bucket IN ? AND object_name = ?", job.FileID, buckets, job.ObjectName).Take(&other).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}
