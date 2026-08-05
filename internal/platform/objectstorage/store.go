package objectstorage

import (
	"context"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"
)

// MinIOStore adapts the provider client to the platform Store interface.
type MinIOStore struct{ client *minio.Client }

func NewStore(client *minio.Client) *MinIOStore { return &MinIOStore{client: client} }

var _ Store = (*MinIOStore)(nil)

func (s *MinIOStore) Put(ctx context.Context, input PutInput) (ObjectInfo, error) {
	if s == nil || s.client == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	info, err := s.client.PutObject(ctx, input.Bucket, input.Name, input.Reader, input.Size, minio.PutObjectOptions{ContentType: input.ContentType})
	if err != nil {
		return ObjectInfo{}, normalizePlatformError(err)
	}
	return ObjectInfo{Bucket: input.Bucket, Name: input.Name, Size: info.Size, ContentType: input.ContentType, LastModified: info.LastModified}, nil
}

func (s *MinIOStore) Open(ctx context.Context, bucket, name string) (io.ReadCloser, error) {
	if s == nil || s.client == nil {
		return nil, ErrUnavailable
	}
	reader, err := s.client.GetObject(ctx, bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, normalizePlatformError(err)
	}
	return platformReadCloser{ReadCloser: reader}, nil
}

func (s *MinIOStore) Stat(ctx context.Context, bucket, name string) (ObjectInfo, error) {
	if s == nil || s.client == nil {
		return ObjectInfo{}, ErrUnavailable
	}
	info, err := s.client.StatObject(ctx, bucket, name, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, normalizePlatformError(err)
	}
	return ObjectInfo{Bucket: bucket, Name: info.Key, Size: info.Size, ContentType: info.ContentType, LastModified: info.LastModified}, nil
}

func (s *MinIOStore) Delete(ctx context.Context, bucket, name string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	return normalizePlatformError(s.client.RemoveObject(ctx, bucket, name, minio.RemoveObjectOptions{}))
}

func (s *MinIOStore) List(ctx context.Context, bucket string, options ListOptions) ([]ObjectInfo, error) {
	if s == nil || s.client == nil {
		return nil, ErrUnavailable
	}
	objects := make([]ObjectInfo, 0)
	for info := range s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: options.Prefix, Recursive: options.Recursive, WithMetadata: true}) {
		if info.Err != nil {
			return nil, normalizePlatformError(info.Err)
		}
		objects = append(objects, ObjectInfo{Bucket: bucket, Name: info.Key, Size: info.Size, ContentType: info.ContentType, LastModified: info.LastModified})
	}
	return objects, nil
}

func (s *MinIOStore) Move(ctx context.Context, sourceBucket, targetBucket, name string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	if _, err := s.client.CopyObject(ctx, minio.CopyDestOptions{Bucket: targetBucket, Object: name}, minio.CopySrcOptions{Bucket: sourceBucket, Object: name}); err != nil {
		return normalizePlatformError(err)
	}
	return normalizePlatformError(s.client.RemoveObject(ctx, sourceBucket, name, minio.RemoveObjectOptions{}))
}

type platformReadCloser struct{ io.ReadCloser }

func (r platformReadCloser) Read(p []byte) (int, error) {
	returnCount, err := r.ReadCloser.Read(p)
	return returnCount, normalizePlatformError(err)
}
func (r platformReadCloser) Close() error { return normalizePlatformError(r.ReadCloser.Close()) }

func normalizePlatformError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	var response minio.ErrorResponse
	if errors.As(err, &response) && response.Code == minio.NoSuchKey {
		return errors.Join(ErrObjectNotFound, err)
	}
	return errors.Join(ErrUnavailable, err)
}
