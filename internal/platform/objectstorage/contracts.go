package objectstorage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrObjectNotFound = errors.New("object storage object not found")
	ErrUnavailable    = errors.New("object storage unavailable")
)

// Store is the provider-neutral object-storage contract exposed by the
// platform. Business modules should consume it through their own caller-owned
// contracts and adapters.
type Store interface {
	Put(context.Context, PutInput) (ObjectInfo, error)
	Open(context.Context, string, string) (io.ReadCloser, error)
	Stat(context.Context, string, string) (ObjectInfo, error)
	Delete(context.Context, string, string) error
	List(context.Context, string, ListOptions) ([]ObjectInfo, error)
	Move(context.Context, string, string, string) error
}

type PutInput struct {
	Bucket      string
	Name        string
	Reader      io.Reader
	Size        int64
	ContentType string
}

type ListOptions struct {
	Prefix    string
	Recursive bool
}

type ObjectInfo struct {
	Bucket       string
	Name         string
	Size         int64
	ContentType  string
	LastModified time.Time
}
