package objectstorage

import (
	"context"
	"io"
	"time"
)

// Store is the object-storage boundary used by file and user-avatar services.
// Implementations must not expose provider-specific clients, options, or errors
// through this interface.
type Store interface {
	Put(ctx context.Context, input PutInput) (ObjectInfo, error)
	Open(ctx context.Context, bucket, name string) (io.ReadCloser, error)
	Stat(ctx context.Context, bucket, name string) (ObjectInfo, error)
	Delete(ctx context.Context, bucket, name string) error
	List(ctx context.Context, bucket string, options ListOptions) ([]ObjectInfo, error)
}

// PutInput describes a streaming object write. Name is supplied by the service,
// not derived by the storage implementation from an untrusted display name.
type PutInput struct {
	Bucket      string
	Name        string
	Reader      io.Reader
	Size        int64
	ContentType string
}

// ListOptions controls object metadata listing without exposing provider types.
type ListOptions struct {
	Prefix    string
	Recursive bool
}

// ObjectInfo is provider-neutral object metadata returned to services.
type ObjectInfo struct {
	Bucket       string
	Name         string
	Size         int64
	ContentType  string
	LastModified time.Time
}
