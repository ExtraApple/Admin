package objectstorage

import (
	"errors"
	"io"

	"github.com/minio/minio-go/v7"

	"admin/service/uploadsecurity"
)

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := uploadsecurity.CodeOf(err); ok &&
		(code == uploadsecurity.CodeStorageObjectNotFound ||
			code == uploadsecurity.CodeStorageUnavailable) {
		return err
	}
	if errors.Is(err, io.EOF) {
		return err
	}

	var response minio.ErrorResponse
	if errors.As(err, &response) &&
		response.Code == minio.NoSuchKey {
		return uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, err)
	}
	return uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, err)
}

type normalizingReadCloser struct {
	io.ReadCloser
}

func (r normalizingReadCloser) Read(buffer []byte) (int, error) {
	n, err := r.ReadCloser.Read(buffer)
	return n, normalizeError(err)
}

func (r normalizingReadCloser) Close() error {
	return normalizeError(r.ReadCloser.Close())
}
