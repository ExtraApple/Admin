package objectstorageadapter

import (
	"context"
	"errors"
	"io"

	"admin/internal/files/application"
	platformstorage "admin/internal/platform/objectstorage"
	"admin/internal/uploadsecurity"
)

type Store struct{ provider platformstorage.Store }

func New(provider platformstorage.Store) *Store { return &Store{provider: provider} }

var _ application.ObjectStorage = (*Store)(nil)

func (s *Store) Put(ctx context.Context, input application.ObjectInput) error {
	if s == nil || s.provider == nil {
		return uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	_, err := s.provider.Put(ctx, platformstorage.PutInput{Bucket: input.Bucket, Name: input.Name, Reader: input.Reader, Size: input.Size, ContentType: input.ContentType})
	return normalize(err)
}
func (s *Store) Open(ctx context.Context, bucket, name string) (io.ReadCloser, error) {
	if s == nil || s.provider == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	reader, err := s.provider.Open(ctx, bucket, name)
	return reader, normalize(err)
}
func (s *Store) Delete(ctx context.Context, bucket, name string) error {
	if s == nil || s.provider == nil {
		return uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	return normalize(s.provider.Delete(ctx, bucket, name))
}
func (s *Store) List(ctx context.Context, bucket string, options application.ListOptions) ([]application.Object, error) {
	if s == nil || s.provider == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	objects, err := s.provider.List(ctx, bucket, platformstorage.ListOptions{Prefix: options.Prefix, Recursive: options.Recursive})
	if err != nil {
		return nil, normalize(err)
	}
	result := make([]application.Object, len(objects))
	for i := range objects {
		result[i] = application.Object{Name: objects[i].Name, ContentType: objects[i].ContentType, Size: objects[i].Size, LastModified: objects[i].LastModified}
	}
	return result, nil
}
func (s *Store) Move(ctx context.Context, from, to, name string) error {
	if s == nil || s.provider == nil {
		return uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	return normalize(s.provider.Move(ctx, from, to, name))
}
func normalize(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := uploadsecurity.CodeOf(err); ok {
		return err
	}
	if errors.Is(err, platformstorage.ErrObjectNotFound) {
		return uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, err)
	}
	return uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, err)
}
