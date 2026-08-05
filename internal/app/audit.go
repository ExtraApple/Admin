package app

import (
	"context"
	"time"

	auditmodule "admin/internal/audit"
	auditgorm "admin/internal/audit/adapters/gorm"
	audithttp "admin/internal/audit/adapters/http"
	identityapplication "admin/internal/identity/application"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type auditComposition struct {
	recorder auditRecorder
	query    *auditmodule.QueryService
	routes   []routecatalog.Descriptor
	jobs     []BackgroundJob
}

type auditRecorderQueue struct{ worker *auditmodule.RecorderWorker }

func (queue auditRecorderQueue) Submit(log auditmodule.AuditLog) {
	if queue.worker == nil {
		return
	}
	go queue.worker.Record(context.Background(), log)
}

type auditRecorder interface{ Submit(auditmodule.AuditLog) }

// auditZapLogger keeps zap confined to the App composition root.
type auditZapLogger struct{ logger *zap.Logger }

func (logger auditZapLogger) Info(message string, fields ...auditmodule.Field) {
	values := make([]zap.Field, len(fields))
	for index, field := range fields {
		values[index] = zap.Any(field.Key, field.Value)
	}
	logger.logger.Info(message, values...)
}
func (logger auditZapLogger) Error(message string, err error, fields ...auditmodule.Field) {
	values := make([]zap.Field, 0, len(fields)+1)
	values = append(values, zap.Error(err))
	for _, field := range fields {
		values = append(values, zap.Any(field.Key, field.Value))
	}
	logger.logger.Error(message, values...)
}

func newAuditComposition(resources Resources, config platformconfig.Config, users identityapplication.UserRepository) auditComposition {
	repository := auditgorm.NewRepository(resources.DB)
	username := auditIdentityReader{users: users}
	worker := auditmodule.NewRecorderWorker(repository, auditZapLogger{logger: resources.Logger}, nil, username)
	queue := auditRecorderQueue{worker: worker}
	recorder := auditmodule.NewAsyncRecorder(queue)
	query := auditmodule.NewQueryService(repository)
	composition := auditComposition{recorder: recorder, query: query, routes: audithttp.Routes(query)}
	if config.AuditLogArchive.Enabled {
		archive := auditmodule.NewArchiveService(repository, auditZapLogger{logger: resources.Logger}, nil)
		options := auditmodule.ArchiveOptions{Enabled: true, RetentionDays: config.AuditLogArchive.RetentionDays, BatchSize: config.AuditLogArchive.BatchSize}
		composition.jobs = []BackgroundJob{{Name: "audit-log-archive", Run: func(ctx context.Context) {
			_ = archive.ArchiveAuditLogs(ctx, options)
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = archive.ArchiveAuditLogs(ctx, options)
				}
			}
		}}}
	}
	return composition
}

type auditIdentityReader struct {
	users identityapplication.UserRepository
}

func (reader auditIdentityReader) Username(ctx context.Context, userID uint) (string, error) {
	user, err := reader.users.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

var _ auditmodule.UsernameReader = auditIdentityReader{}

func auditMiddleware(recorder auditRecorder) gin.HandlerFunc {
	return audithttp.NewMiddleware(recorder)
}
