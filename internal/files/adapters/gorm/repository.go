package gormadapter

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

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
	if err := r.connection(ctx).First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.File{}, application.ErrFileNotFound
		}
		return domain.File{}, err
	}
	return toDomain(record), nil
}
func (r *Repository) List(ctx context.Context, page, size int, prefix string) ([]domain.File, int64, error) {
	query := r.connection(ctx).Model(&files.File{})
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
	if err := r.connection(ctx).Where("created_at < ? AND bucket = ?", cutoff, bucket).Limit(limit).Find(&records).Error; err != nil {
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
	return domain.File{ID: record.ID, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Name: record.Name, Bucket: record.Bucket, ObjectName: record.ObjectName, ContentType: record.ContentType, DetectedContentType: record.DetectedContentType, ContentSHA256: record.ContentSHA256, Size: record.Size, UploaderID: record.UploaderID, ValidationStatus: record.ValidationStatus, ValidationPolicyVersion: record.ValidationPolicyVersion, ValidationErrorCode: record.ValidationErrorCode, ValidatedAt: record.ValidatedAt}
}
func fromDomain(value domain.File) files.File {
	return files.File{Model: gorm.Model{ID: value.ID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, Name: value.Name, Bucket: value.Bucket, ObjectName: value.ObjectName, ContentType: value.ContentType, DetectedContentType: value.DetectedContentType, ContentSHA256: value.ContentSHA256, Size: value.Size, UploaderID: value.UploaderID, ValidationStatus: value.ValidationStatus, ValidationPolicyVersion: value.ValidationPolicyVersion, ValidationErrorCode: value.ValidationErrorCode, ValidatedAt: value.ValidatedAt}
}

var _ application.Repository = (*Repository)(nil)
