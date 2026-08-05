package objectstorage

import (
	"context"
	"errors"
	"io"

	"admin/internal/identity/application"

	"github.com/minio/minio-go/v7"
)

type Store struct{ client *minio.Client }

func New(client *minio.Client) *Store { return &Store{client: client} }

func (store *Store) Put(ctx context.Context, object application.AvatarObject) error {
	_, err := store.client.PutObject(ctx, object.Bucket, object.Name, object.Reader, object.Size, minio.PutObjectOptions{ContentType: object.ContentType})
	return normalizeError(err)
}

func (store *Store) Open(ctx context.Context, bucket, name string) (io.ReadCloser, error) {
	reader, err := store.client.GetObject(ctx, bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, normalizeError(err)
	}
	return normalizingReadCloser{ReadCloser: reader}, nil
}

func (store *Store) Stat(ctx context.Context, bucket, name string) (application.AvatarObjectInfo, error) {
	info, err := store.client.StatObject(ctx, bucket, name, minio.StatObjectOptions{})
	if err != nil {
		return application.AvatarObjectInfo{}, normalizeError(err)
	}
	return application.AvatarObjectInfo{Size: info.Size, ContentType: info.ContentType}, nil
}

func (store *Store) Delete(ctx context.Context, bucket, name string) error {
	return normalizeError(store.client.RemoveObject(ctx, bucket, name, minio.RemoveObjectOptions{}))
}

func normalizeError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	if _, ok := application.AvatarErrorCode(err); ok {
		return err
	}
	var response minio.ErrorResponse
	if errors.As(err, &response) && response.Code == minio.NoSuchKey {
		return application.NewAvatarError("STORAGE_OBJECT_NOT_FOUND", err)
	}
	return application.NewAvatarError(application.AvatarCodeStorageUnavailable, err)
}

type normalizingReadCloser struct{ io.ReadCloser }

func (reader normalizingReadCloser) Read(buffer []byte) (int, error) {
	count, err := reader.ReadCloser.Read(buffer)
	return count, normalizeError(err)
}
func (reader normalizingReadCloser) Close() error { return normalizeError(reader.ReadCloser.Close()) }

var _ application.AvatarStorage = (*Store)(nil)
