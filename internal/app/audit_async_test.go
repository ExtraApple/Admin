package app

import (
	"context"
	"testing"
	"time"

	"admin/internal/audit"
)

type blockingAuditRepository struct {
	started chan struct{}
	release chan struct{}
}

func (repository *blockingAuditRepository) Create(context.Context, *audit.AuditLog) error {
	close(repository.started)
	<-repository.release
	return nil
}

func (*blockingAuditRepository) Username(context.Context, uint) (string, error) { return "", nil }

func TestAuditRecorderQueueReturnsBeforePersistenceCompletes(t *testing.T) {
	repository := &blockingAuditRepository{started: make(chan struct{}), release: make(chan struct{})}
	queue := auditRecorderQueue{worker: audit.NewRecorderWorker(repository, nil, nil)}
	returned := make(chan struct{})
	go func() {
		queue.Submit(audit.AuditLog{Path: "/api/async"})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("audit submission blocked on persistence")
	}
	select {
	case <-repository.started:
	case <-time.After(time.Second):
		t.Fatal("audit persistence did not start asynchronously")
	}
	close(repository.release)
}
