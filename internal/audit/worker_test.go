package audit

import (
	"context"
	"errors"
	"testing"
)

type failingRecorderRepository struct{ err error }

func (repository failingRecorderRepository) Create(context.Context, *AuditLog) error {
	return repository.err
}

type capturedError struct {
	message string
	err     error
}
type recordingLogger struct{ errors []capturedError }

func (*recordingLogger) Info(string, ...Field) {}
func (logger *recordingLogger) Error(message string, err error, _ ...Field) {
	logger.errors = append(logger.errors, capturedError{message: message, err: err})
}

func TestRecorderWorkerKeepsPersistenceFailureOutOfRequestPath(t *testing.T) {
	persistenceErr := errors.New("database unavailable")
	logger := &recordingLogger{}
	worker := NewRecorderWorker(failingRecorderRepository{err: persistenceErr}, logger, nil)
	worker.Record(context.Background(), AuditLog{UserID: 7, Path: "/api/failure"})
	if len(logger.errors) != 1 || logger.errors[0].message != "create audit log failed" || !errors.Is(logger.errors[0].err, persistenceErr) {
		t.Fatalf("recorded errors = %#v", logger.errors)
	}
}

type capturedRecordRepository struct{ log AuditLog }

func (repository *capturedRecordRepository) Create(_ context.Context, log *AuditLog) error {
	repository.log = *log
	return nil
}

type staticUsernameReader struct{}

func (staticUsernameReader) Username(context.Context, uint) (string, error) { return "alice", nil }

func TestRecorderWorkerUsesInjectedUsernameReader(t *testing.T) {
	repository := &capturedRecordRepository{}
	worker := NewRecorderWorker(repository, nil, nil, staticUsernameReader{})
	worker.Record(context.Background(), AuditLog{UserID: 7, Path: "/api/audit"})
	if repository.log.Username != "alice" {
		t.Fatalf("recorded username = %q, want alice", repository.log.Username)
	}
}
