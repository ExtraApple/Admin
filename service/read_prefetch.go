package service

import (
	"bytes"
	"errors"
	"io"

	"admin/service/uploadsecurity"
)

type emptyStreamPolicy uint8

const (
	rejectEmptyStream emptyStreamPolicy = iota
	allowEmptyStream
)

// prefetchReadCloser verifies that a lazily opened object can produce its
// first byte before an HTTP handler commits a successful response. Callers
// explicitly decide whether a normal empty stream is valid. Any bytes read
// without an accompanying failure are replayed to preserve the complete stream.
func prefetchReadCloser(
	reader io.ReadCloser,
	emptyPolicy emptyStreamPolicy,
) (io.ReadCloser, error) {
	if reader == nil {
		return nil, io.ErrUnexpectedEOF
	}

	firstByte := make([]byte, 1)
	count, err := reader.Read(firstByte)
	if err != nil {
		if _, classified := uploadsecurity.CodeOf(err); classified {
			_ = reader.Close()
			return nil, err
		}
	}
	if count > 0 {
		if err != nil && !errors.Is(err, io.EOF) {
			_ = reader.Close()
			return nil, err
		}
		return &prefetchedReadCloser{
			Reader: io.MultiReader(bytes.NewReader(firstByte[:count]), reader),
			closer: reader,
		}, nil
	}

	if errors.Is(err, io.EOF) && emptyPolicy == allowEmptyStream {
		return &prefetchedReadCloser{
			Reader: bytes.NewReader(nil),
			closer: reader,
		}, nil
	}
	if err == nil {
		err = io.ErrNoProgress
	}
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

type prefetchedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *prefetchedReadCloser) Close() error {
	return r.closer.Close()
}
