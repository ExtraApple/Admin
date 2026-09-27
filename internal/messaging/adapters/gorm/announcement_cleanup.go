package gormadapter

import (
	"context"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

func (repository *Repository) AnnouncementCleanupUpperID(ctx context.Context) (uint, error) {
	var upper uint
	err := repository.connection(ctx).Model(&Message{}).
		Where("kind = ?", domain.MessageKindAnnouncement).
		Select("COALESCE(MAX(id), 0)").Scan(&upper).Error
	return upper, err
}

func (repository *Repository) FindDueAnnouncements(ctx context.Context, query application.DueAnnouncementQuery) ([]domain.Message, error) {
	if query.Limit < 1 || query.Limit > 1000 || query.Now.IsZero() {
		return nil, application.ErrStateConflict
	}
	now := query.Now.UTC()
	db := repository.connection(ctx).Model(&Message{}).
		Where("kind = ? AND id > ? AND id <= ?", domain.MessageKindAnnouncement, query.AfterID, query.UpperID)
	if query.Expiring {
		db = db.Where("expires_at IS NOT NULL AND expires_at <= ?", now).
			Where("status = ? OR (status = ? AND publish_at IS NOT NULL AND publish_at <= ?)", domain.MessageStatusPublished, domain.MessageStatusScheduled, now)
	} else {
		db = db.Where("status = ? AND publish_at IS NOT NULL AND publish_at <= ?", domain.MessageStatusScheduled, now).
			Where("expires_at IS NULL OR expires_at > ?", now)
	}
	var records []Message
	if err := db.Order("id ASC").Limit(query.Limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return messagesToDomain(records), nil
}
