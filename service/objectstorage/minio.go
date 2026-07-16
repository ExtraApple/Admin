package objectstorage

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
)

// MinIOStore adapts the existing MinIO client to the provider-neutral Store
// contract used by services.
type MinIOStore struct {
	client *minio.Client
}

var _ Store = (*MinIOStore)(nil)

// NewMinIOStore creates an object store backed by the existing MinIO client.
func NewMinIOStore(client *minio.Client) *MinIOStore {
	return &MinIOStore{client: client}
}

func (s *MinIOStore) Put(ctx context.Context, input PutInput) (ObjectInfo, error) {
	uploaded, err := s.client.PutObject(
		ctx,
		input.Bucket,
		input.Name,
		input.Reader,
		input.Size,
		minio.PutObjectOptions{ContentType: input.ContentType},
	)
	if err != nil {
		return ObjectInfo{}, normalizeError(err)
	}

	return ObjectInfo{
		Bucket:       input.Bucket,
		Name:         input.Name,
		Size:         uploaded.Size,
		ContentType:  input.ContentType,
		LastModified: uploaded.LastModified,
	}, nil
}

func (s *MinIOStore) Open(ctx context.Context, bucket, name string) (io.ReadCloser, error) {
	reader, err := s.client.GetObject(ctx, bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, normalizeError(err)
	}
	return normalizingReadCloser{ReadCloser: reader}, nil
}

func (s *MinIOStore) Stat(ctx context.Context, bucket, name string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, bucket, name, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, normalizeError(err)
	}
	return objectInfoFromMinIO(bucket, info), nil
}

func (s *MinIOStore) Delete(ctx context.Context, bucket, name string) error {
	return normalizeError(s.client.RemoveObject(ctx, bucket, name, minio.RemoveObjectOptions{}))
}

func (s *MinIOStore) List(ctx context.Context, bucket string, options ListOptions) ([]ObjectInfo, error) {
	objects := make([]ObjectInfo, 0)
	for info := range s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:       options.Prefix,
		Recursive:    options.Recursive,
		WithMetadata: true,
	}) {
		if info.Err != nil {
			return nil, normalizeError(info.Err)
		}
		objects = append(objects, objectInfoFromMinIO(bucket, info))
	}
	return objects, nil
}

func objectInfoFromMinIO(bucket string, info minio.ObjectInfo) ObjectInfo {
	return ObjectInfo{
		Bucket:       bucket,
		Name:         info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}
}
