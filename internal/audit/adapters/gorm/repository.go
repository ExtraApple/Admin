package gormadapter

import (
	"context"
	"time"

	"admin/internal/audit"
	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
)

func Models() []any { return audit.Models() }

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (repository *Repository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *Repository) Create(ctx context.Context, log *audit.AuditLog) error {
	return repository.connection(ctx).Create(log).Error
}

func (repository *Repository) List(ctx context.Context, request audit.AuditLogListRequest, categories []string) ([]audit.AuditLog, int64, error) {
	request = audit.NormalizePage(request)
	query := repository.connection(ctx).Model(&audit.AuditLog{})
	if request.UserID > 0 {
		query = query.Where("user_id = ?", request.UserID)
	}
	if request.Method != "" {
		query = query.Where("method = ?", request.Method)
	}
	if request.Path != "" {
		query = query.Where("path LIKE ?", "%"+request.Path+"%")
	}
	if request.Status > 0 {
		query = query.Where("status = ?", request.Status)
	}
	if request.Category != "" {
		query = query.Where("category = ?", request.Category)
	}
	if len(categories) == 1 {
		query = query.Where("category = ?", categories[0])
	}
	if len(categories) > 1 {
		query = query.Where("category IN ?", categories)
	}
	if start, err := time.Parse("2006-01-02 15:04:05", request.StartTime); request.StartTime != "" && err == nil {
		query = query.Where("created_at >= ?", start)
	}
	if end, err := time.Parse("2006-01-02 15:04:05", request.EndTime); request.EndTime != "" && err == nil {
		query = query.Where("created_at <= ?", end)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []audit.AuditLog
	if err := query.Order("created_at desc").Limit(request.Size).Offset((request.Page - 1) * request.Size).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (repository *Repository) Expired(ctx context.Context, cutoff time.Time, limit int) ([]audit.AuditLog, error) {
	var logs []audit.AuditLog
	err := repository.connection(ctx).Where("created_at < ?", cutoff).Order("created_at asc").Limit(limit).Find(&logs).Error
	return logs, err
}

func (repository *Repository) Archive(ctx context.Context, archives []audit.AuditLogArchive, ids []uint) error {
	if len(archives) == 0 {
		return nil
	}
	return repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&archives).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Delete(&audit.AuditLog{}).Error
	})
}

var _ audit.Repository = (*Repository)(nil)
