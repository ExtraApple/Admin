package audit

import (
	"context"
	"time"
)

// RecorderRepository owns only audit-log persistence. User identity lookup is
// supplied separately through UsernameReader by the composition root.
type RecorderRepository interface {
	Create(context.Context, *AuditLog) error
}

type UsernameReader interface {
	Username(context.Context, uint) (string, error)
}

type QueryRepository interface {
	List(context.Context, AuditLogListRequest, []string) ([]AuditLog, int64, error)
}

type ArchiveRepository interface {
	Expired(context.Context, time.Time, int) ([]AuditLog, error)
	Archive(context.Context, []AuditLogArchive, []uint) error
}

// Repository is the complete persistence contract for the module. Adapters may
// implement only the narrower contracts when a process does not use archiving.
type Repository interface {
	RecorderRepository
	QueryRepository
	ArchiveRepository
}

// Logger intentionally carries no zap or provider-specific fields.
type Field struct {
	Key   string
	Value any
}

type Logger interface {
	Info(string, ...Field)
	Error(string, error, ...Field)
}

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// RecorderQueue is caller-owned. Submit MUST return promptly; queue workers
// provide the asynchronous durability guarantee after the HTTP response.
type RecorderQueue interface{ Submit(AuditLog) }

// ArchiveOptions are normalized by ArchiveService before calling the adapter.
type ArchiveOptions struct {
	Enabled       bool
	RetentionDays int
	BatchSize     int
}

// ArchiveService moves one bounded batch atomically through ArchiveRepository.
type ArchiveService struct {
	repository ArchiveRepository
	logger     Logger
	clock      Clock
}

func NewArchiveService(repository ArchiveRepository, logger Logger, clock Clock) *ArchiveService {
	if clock == nil {
		clock = systemClock{}
	}
	return &ArchiveService{repository: repository, logger: logger, clock: clock}
}

func (service *ArchiveService) Archive(ctx context.Context, options ArchiveOptions) error {
	if !options.Enabled {
		if service.logger != nil {
			service.logger.Info("audit log archive skipped")
		}
		return nil
	}
	retention := options.RetentionDays
	if retention <= 0 {
		retention = 90
	}
	batch := options.BatchSize
	if batch <= 0 {
		batch = 1000
	}
	now := service.clock.Now()
	cutoff := now.AddDate(0, 0, -retention)
	logs, err := service.repository.Expired(ctx, cutoff, batch)
	if err != nil {
		if service.logger != nil {
			service.logger.Error("audit log archive query failed", err)
		}
		return err
	}
	if len(logs) == 0 {
		if service.logger != nil {
			service.logger.Info("audit log archive no records")
		}
		return nil
	}
	archives := make([]AuditLogArchive, 0, len(logs))
	ids := make([]uint, 0, len(logs))
	for _, log := range logs {
		archives = append(archives, ToArchive(log, now))
		ids = append(ids, log.ID)
	}
	if err := service.repository.Archive(ctx, archives, ids); err != nil {
		if service.logger != nil {
			service.logger.Error("audit log archive failed", err)
		}
		return err
	}
	if service.logger != nil {
		service.logger.Info("audit log archive finished", Field{Key: "count", Value: len(logs)}, Field{Key: "cutoff", Value: cutoff})
	}
	return nil
}

func ToArchive(log AuditLog, archivedAt time.Time) AuditLogArchive {
	return AuditLogArchive{UserID: log.UserID, Username: log.Username, Method: log.Method, Path: log.Path,
		Query: log.Query, Body: log.Body, Metadata: log.Metadata, Status: log.Status, Duration: log.Duration,
		ClientIP: log.ClientIP, UserAgent: log.UserAgent, Category: log.Category, CreatedAt: log.CreatedAt, ArchivedAt: archivedAt}
}
