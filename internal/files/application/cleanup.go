package application

import (
	"context"
	"errors"
	"path"
	"slices"
	"strings"
	"time"

	"admin/internal/uploadsecurity"
	"github.com/google/uuid"
)

func (s *Service) CleanupMessageImage(ctx context.Context, fileID uint, now time.Time, rotation RotationConfig) CleanupItemResult {
	result := CleanupItemResult{FileID: fileID, Status: "failed", Stage: "register"}
	if s == nil || s.deps.MessageImages == nil || s.deps.Storage == nil {
		result.FailureCode = "message_image_cleanup_dependency_missing"
		return result
	}
	job, created, err := s.deps.MessageImages.RegisterMessageImageCleanup(ctx, fileID, now, s.deps.Clock.Now().UTC())
	if err != nil {
		result.FailureCode = "message_image_cleanup_register_failed"
		if errors.Is(err, ErrMessageImageCleanupHashConflict) {
			result.FailureCode = "message_image_cleanup_hash_conflict"
		}
		return result
	}
	result.CleanupJobID = job.ID
	if !created {
		result.Status = "skipped"
		return result
	}
	return s.deleteCleanupObject(ctx, job, rotation)
}

func (s *Service) RetryMessageImageCleanup(ctx context.Context, job MessageImageCleanupJob, now time.Time, rotation RotationConfig) CleanupItemResult {
	result := CleanupItemResult{FileID: job.FileID, CleanupJobID: job.ID, RetryAttempt: job.RetryCount, Status: "failed", Stage: "reserve"}
	if s == nil || s.deps.MessageImages == nil || s.deps.Storage == nil {
		result.FailureCode = "message_image_cleanup_dependency_missing"
		return result
	}
	if job.RetryCount == 24 {
		changed, err := s.deps.MessageImages.DeadLetterExhaustedMessageImageCleanup(ctx, job, now)
		if err != nil {
			result.FailureCode = "message_image_cleanup_persistence_failed"
			return result
		}
		if !changed {
			result.Status = "skipped"
			return result
		}
		result.Stage = "dead"
		result.FailureCode = "message_image_cleanup_retry_exhausted"
		return result
	}
	reserved, ok, err := s.deps.MessageImages.ReserveMessageImageCleanupRetry(ctx, job, now, s.deps.Clock.Now().UTC())
	if err != nil {
		result.FailureCode = "message_image_cleanup_persistence_failed"
		return result
	}
	if !ok {
		result.Status = "skipped"
		return result
	}
	return s.deleteCleanupObject(ctx, reserved, rotation)
}

func (s *Service) deleteCleanupObject(ctx context.Context, job MessageImageCleanupJob, rotation RotationConfig) CleanupItemResult {
	result := CleanupItemResult{FileID: job.FileID, CleanupJobID: job.ID, RetryAttempt: job.RetryCount, Status: "failed", Stage: "storage"}
	buckets := []string{job.Bucket}
	if rotation.Enabled {
		for _, bucket := range []string{rotation.HotBucket, rotation.ColdBucket} {
			if bucket != "" && !slices.Contains(buckets, bucket) {
				buckets = append(buckets, bucket)
			}
		}
	}
	code := ""
	name := strings.TrimPrefix(job.ObjectName, "message-images/")
	ext := path.Ext(name)
	id, err := uuid.Parse(strings.TrimSuffix(name, ext))
	if job.Bucket == "" || !strings.HasPrefix(job.ObjectName, "message-images/") || err != nil || id == uuid.Nil || id.String()+ext != name || (ext != ".png" && ext != ".jpg" && ext != ".webp") {
		code = "message_image_cleanup_scope_untrusted"
	} else if conflict, err := s.deps.MessageImages.MessageImageCleanupLocationConflicts(ctx, job, buckets); err != nil {
		code = "message_image_cleanup_persistence_failed"
	} else if conflict {
		code = "message_image_cleanup_scope_untrusted"
	}
	if code == "" {
		for _, bucket := range buckets {
			if ctx.Err() != nil {
				code = "message_image_cleanup_canceled"
				break
			}
			_, err := s.deps.Storage.Stat(ctx, bucket, job.ObjectName)
			if c, _ := uploadsecurity.CodeOf(err); c == uploadsecurity.CodeStorageObjectNotFound {
				continue
			}
			if err == nil {
				err = s.deps.Storage.Delete(ctx, bucket, job.ObjectName)
			}
			if c, _ := uploadsecurity.CodeOf(err); c == uploadsecurity.CodeStorageObjectNotFound {
				continue
			}
			if err != nil {
				code = cleanupStorageFailure(err)
				break
			}
		}
	}
	if code != "" {
		result.FailureCode = code
		if ctx.Err() != nil {
			return result
		}
		changed, err := s.deps.MessageImages.FailMessageImageCleanup(ctx, job, code)
		if err != nil {
			result.Stage = "record_failure"
			result.FailureCode = "message_image_cleanup_persistence_failed"
			return result
		}
		if !changed {
			result.Status = "skipped"
			return result
		}
		if job.RetryCount == 24 {
			result.Stage = "dead"
		}
		return result
	}
	result.Stage = "complete"
	changed, err := s.deps.MessageImages.CompleteMessageImageCleanup(ctx, job)
	if err != nil {
		result.FailureCode = "message_image_cleanup_persistence_failed"
		return result
	}
	result.Status = "skipped"
	if changed {
		result.Status = "succeeded"
	}
	return result
}

func cleanupStorageFailure(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "message_image_cleanup_timeout"
	case errors.Is(err, context.Canceled):
		return "message_image_cleanup_canceled"
	case errors.Is(err, ErrStoragePermissionDenied):
		return "message_image_cleanup_permission_denied"
	default:
		return "message_image_cleanup_storage_unavailable"
	}
}

// MessageImageCleanupScan is owned by the caller and retained across rounds.
type MessageImageCleanupScan struct {
	New, Retry CleanupCursor
	NewFirst   bool
}

func (s *Service) CleanupMessageImages(ctx context.Context, now time.Time, limit int, scan *MessageImageCleanupScan, rotation RotationConfig) (CleanupResult, error) {
	result := CleanupResult{}
	if s == nil || s.deps.MessageImages == nil || s.deps.Storage == nil || scan == nil || limit < 1 || limit > 1000 {
		return result, ErrStateConflict
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, entry := range []struct {
		cursor *CleanupCursor
		retry  bool
	}{{&scan.New, false}, {&scan.Retry, true}} {
		if entry.cursor.UpperID == 0 {
			upper, err := s.deps.MessageImages.MessageImageCleanupUpperID(ctx, entry.retry)
			if err != nil {
				return result, err
			}
			entry.cursor.UpperID = upper
		}
	}
	exhausted := [2]bool{}
	process := func(retry bool, budget int) error {
		index := 0
		cursor := &scan.New
		if retry {
			index = 1
			cursor = &scan.Retry
		}
		if budget == 0 || exhausted[index] {
			return nil
		}
		if cursor.UpperID == 0 {
			exhausted[index] = true
			return nil
		}
		var jobs []MessageImageCleanupJob
		if retry {
			var err error
			jobs, err = s.deps.MessageImages.FindMessageImageCleanupRetries(ctx, now, *cursor, budget)
			if err != nil {
				return err
			}
		} else {
			candidates, err := s.deps.MessageImages.FindMessageImageCleanupCandidates(ctx, now, *cursor, budget)
			if err != nil {
				return err
			}
			jobs = make([]MessageImageCleanupJob, len(candidates))
			for i := range candidates {
				jobs[i].FileID = candidates[i].ID
			}
		}
		for _, job := range jobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			var item CleanupItemResult
			if retry {
				item = s.RetryMessageImageCleanup(ctx, job, now, rotation)
				cursor.AfterID = job.ID
			} else {
				item = s.CleanupMessageImage(ctx, job.FileID, now, rotation)
				cursor.AfterID = job.FileID
			}
			result.Processed++
			switch item.Status {
			case "succeeded":
				result.Succeeded++
			case "skipped":
				result.Skipped++
			default:
				result.Failed++
			}
			result.Items = append(result.Items, item)
		}
		if len(jobs) < budget || cursor.AfterID >= cursor.UpperID {
			*cursor = CleanupCursor{}
			exhausted[index] = true
		}
		return ctx.Err()
	}
	retryBudget := (limit + 1) / 2
	newBudget := limit / 2
	if limit == 1 && scan.NewFirst {
		retryBudget = 0
		newBudget = 1
	}
	if limit == 1 {
		scan.NewFirst = !scan.NewFirst
	}
	if err := process(true, retryBudget); err != nil {
		return result, err
	}
	if err := process(false, newBudget); err != nil {
		return result, err
	}
	if left := limit - result.Processed; left > 0 {
		if err := process(true, left); err != nil {
			return result, err
		}
	}
	if left := limit - result.Processed; left > 0 {
		if err := process(false, left); err != nil {
			return result, err
		}
	}
	return result, nil
}
