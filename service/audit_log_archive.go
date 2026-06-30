package service

import (
	"time"

	"admin/global"
	"admin/initialize"
	"admin/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ArchiveAuditLogs 将超过保留期的审计日志搬到归档表，并从主表删除。
func ArchiveAuditLogs(conf *initialize.Config) {
	if !conf.AuditLogArchive.Enabled {
		global.Logger.Info("audit log archive skipped")
		return
	}

	retentionDays := conf.AuditLogArchive.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 90
	}

	batchSize := conf.AuditLogArchive.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	var logs []model.AuditLog
	if err := global.DB.Where("created_at < ?", cutoff).
		Order("created_at asc").
		Limit(batchSize).
		Find(&logs).Error; err != nil {
		global.Logger.Error("audit log archive query failed", zap.Error(err))
		return
	}

	if len(logs) == 0 {
		global.Logger.Info("audit log archive no records")
		return
	}

	archives := make([]model.AuditLogArchive, 0, len(logs))
	ids := make([]uint, 0, len(logs))
	now := time.Now()
	for _, item := range logs {
		archives = append(archives, toAuditLogArchive(item, now))
		ids = append(ids, item.ID)
	}

	err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&archives).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", ids).Delete(&model.AuditLog{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		global.Logger.Error("audit log archive failed", zap.Error(err))
		return
	}

	global.Logger.Info("audit log archive finished",
		zap.Int("count", len(logs)),
		zap.Time("cutoff", cutoff),
	)
}

// toAuditLogArchive 将主审计日志记录复制为归档记录，并写入归档时间。
func toAuditLogArchive(log model.AuditLog, archivedAt time.Time) model.AuditLogArchive {
	return model.AuditLogArchive{
		UserID:     log.UserID,
		Username:   log.Username,
		Method:     log.Method,
		Path:       log.Path,
		Query:      log.Query,
		Body:       log.Body,
		Status:     log.Status,
		Duration:   log.Duration,
		ClientIP:   log.ClientIP,
		UserAgent:  log.UserAgent,
		Category:   log.Category,
		CreatedAt:  log.CreatedAt,
		ArchivedAt: archivedAt,
	}
}
