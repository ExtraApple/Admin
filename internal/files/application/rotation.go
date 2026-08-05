package application

import (
	"context"
	"errors"

	"admin/internal/uploadsecurity"
)

// Rotate moves one bounded batch from hot to cold storage. A database failure
// triggers the same best-effort object move rollback as the legacy job while
// allowing the caller to own scheduling and logging.
func (s *Service) Rotate(ctx context.Context, config RotationConfig) (RotationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !config.Enabled {
		return RotationResult{}, nil
	}
	if err := s.repository(); err != nil {
		return RotationResult{}, err
	}
	batch := config.BatchSize
	if batch <= 0 {
		batch = 100
	}
	cutoff := s.deps.Clock.Now().AddDate(0, 0, -config.Days)
	candidates, err := s.deps.Repository.FindRotationCandidates(ctx, cutoff, config.HotBucket, batch)
	if err != nil {
		return RotationResult{}, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	result := RotationResult{Examined: len(candidates), Failures: make([]error, 0)}
	for i := range candidates {
		file := candidates[i]
		if err := s.deps.Storage.Move(ctx, config.HotBucket, config.ColdBucket, file.ObjectName); err != nil {
			result.Failures = append(result.Failures, classifyStorageError(err))
			continue
		}
		err := s.tx(ctx, func(tx context.Context) error { return s.deps.Repository.UpdateBucket(tx, file.ID, config.ColdBucket) })
		if err != nil {
			rollbackErr := s.deps.Storage.Move(ctx, config.ColdBucket, config.HotBucket, file.ObjectName)
			persistenceErr := uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
			if rollbackErr != nil {
				result.Failures = append(result.Failures, errors.Join(persistenceErr, classifyStorageError(rollbackErr)))
			} else {
				result.Failures = append(result.Failures, persistenceErr)
			}
			continue
		}
		result.Moved++
	}
	return result, nil
}
