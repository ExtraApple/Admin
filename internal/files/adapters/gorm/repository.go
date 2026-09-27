package gormadapter

import (
	"context"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"admin/internal/files"

	"admin/internal/files/application"
	"admin/internal/files/domain"
	platformdatabase "admin/internal/platform/database"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository                { return &Repository{db: db} }
func NewGORMRepository(db *gorm.DB) application.Repository { return NewRepository(db) }
func (r *Repository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, r.db)
}
func (r *Repository) Create(ctx context.Context, file *domain.File) error {
	record := fromDomain(*file)
	if err := r.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	*file = toDomain(record)
	return nil
}
func (r *Repository) FindByID(ctx context.Context, id uint) (domain.File, error) {
	var record files.File
	if err := r.connection(ctx).Where("purpose = ? OR purpose IS NULL OR purpose = ''", "managed_file").First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.File{}, application.ErrFileNotFound
		}
		return domain.File{}, err
	}
	return toDomain(record), nil
}
func (r *Repository) List(ctx context.Context, page, size int, prefix string) ([]domain.File, int64, error) {
	query := r.connection(ctx).Model(&files.File{}).Where("purpose = ? OR purpose IS NULL OR purpose = ''", "managed_file")
	if prefix != "" {
		query = query.Where("object_name LIKE ?", prefix+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []files.File
	if err := query.Order("created_at desc").Limit(size).Offset((page - 1) * size).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	result := make([]domain.File, len(records))
	for i := range records {
		result[i] = toDomain(records[i])
	}
	return result, total, nil
}
func (r *Repository) CreateMessageImage(ctx context.Context, file *domain.File) error {
	record := fromDomain(*file)
	if err := r.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	*file = toDomain(record)
	return nil
}

func (r *Repository) FindMessageImage(ctx context.Context, id uint) (domain.File, error) {
	var record files.File
	if err := r.connection(ctx).Where("purpose = ?", "message_image").Where("NOT EXISTS (SELECT 1 FROM message_image_cleanup_jobs AS job WHERE job.file_id = files.id)").First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.File{}, application.ErrFileNotFound
		}
		return domain.File{}, err
	}
	return toDomain(record), nil
}

func (r *Repository) BindMessageImages(ctx context.Context, request application.MessageImageBindRequest) error {
	if request.ActorID == 0 || request.MessageLogicalID == "" || len(request.ImageIDs) == 0 {
		return application.ErrStateConflict
	}
	ids := make([]uint, 0, len(request.ImageIDs))
	seen := make(map[uint]struct{}, len(request.ImageIDs))
	for _, id := range request.ImageIDs {
		if id == 0 {
			return application.ErrStateConflict
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return platformdatabase.NewTransactionRunner(r.db).Run(ctx, func(tx context.Context) error {
		db := r.connection(tx)
		for _, id := range ids {
			var file files.File
			if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&file, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return application.ErrFileNotFound
				}
				return err
			}
			now := db.NowFunc().UTC()
			if file.Purpose != "message_image" || file.UploaderID != request.ActorID || file.LogicalMessageID != "" || file.BindingExpiresAt == nil || !now.Before(*file.BindingExpiresAt) {
				return application.ErrStateConflict
			}
			var job files.MessageImageCleanupJob
			err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("file_id = ?", id).First(&job).Error
			if err == nil {
				return application.ErrStateConflict
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		result := db.Model(&files.File{}).Where("id IN ?", ids).Update("logical_message_id", request.MessageLogicalID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return application.ErrStateConflict
		}
		return nil
	})
}

func (r *Repository) UpdateName(ctx context.Context, id uint, name string) error {
	return r.connection(ctx).Model(&files.File{}).Where("id = ?", id).Update("name", name).Error
}
func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.connection(ctx).Unscoped().Delete(&files.File{}, id).Error
}
func (r *Repository) UpdateValidation(ctx context.Context, id uint, update application.ValidationUpdate) error {
	return r.connection(ctx).Model(&files.File{}).Where("id = ?", id).Updates(map[string]any{"content_type": update.ContentType, "detected_content_type": update.DetectedContentType, "content_sha256": update.ContentSHA256, "validation_status": update.Status, "validation_policy_version": update.PolicyVersion, "validation_error_code": update.ErrorCode, "validated_at": update.ValidatedAt}).Error
}
func (r *Repository) FindRotationCandidates(ctx context.Context, cutoff time.Time, bucket string, limit int) ([]domain.File, error) {
	var records []files.File
	if err := r.connection(ctx).Where("created_at < ? AND bucket = ?", cutoff, bucket).
		Where("purpose IS NULL OR purpose <> ? OR (logical_message_id IS NOT NULL AND logical_message_id <> '')", "message_image").
		Where("NOT EXISTS (SELECT 1 FROM message_image_cleanup_jobs AS job WHERE job.file_id = files.id)").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.File, len(records))
	for i := range records {
		result[i] = toDomain(records[i])
	}
	return result, nil
}
func (r *Repository) UpdateBucket(ctx context.Context, id uint, bucket string) error {
	return r.connection(ctx).Model(&files.File{}).Where("id = ?", id).Update("bucket", bucket).Error
}
func toDomain(record files.File) domain.File {
	return domain.File{ID: record.ID, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Name: record.Name, Bucket: record.Bucket, ObjectName: record.ObjectName, ContentType: record.ContentType, DetectedContentType: record.DetectedContentType, ContentSHA256: record.ContentSHA256, Size: record.Size, UploaderID: record.UploaderID, Purpose: record.Purpose, LogicalMessageID: record.LogicalMessageID, BindingExpiresAt: record.BindingExpiresAt, ValidationStatus: record.ValidationStatus, ValidationPolicyVersion: record.ValidationPolicyVersion, ValidationErrorCode: record.ValidationErrorCode, ValidatedAt: record.ValidatedAt}
}
func fromDomain(value domain.File) files.File {
	return files.File{Model: gorm.Model{ID: value.ID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, Name: value.Name, Bucket: value.Bucket, ObjectName: value.ObjectName, ContentType: value.ContentType, DetectedContentType: value.DetectedContentType, ContentSHA256: value.ContentSHA256, Size: value.Size, UploaderID: value.UploaderID, Purpose: value.Purpose, LogicalMessageID: value.LogicalMessageID, BindingExpiresAt: value.BindingExpiresAt, ValidationStatus: value.ValidationStatus, ValidationPolicyVersion: value.ValidationPolicyVersion, ValidationErrorCode: value.ValidationErrorCode, ValidatedAt: value.ValidatedAt}
}

var _ application.Repository = (*Repository)(nil)
var _ application.MessageImageRepository = (*Repository)(nil)
