package uploadsecurity_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"admin/service/uploadsecurity"
)

var _ interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
} = (*uploadsecurity.StagedFile)(nil)

func TestStageProvidesBoundedReaderAtInputAndRemovesItOnClose(t *testing.T) {
	content := []byte("0123456789")
	staged, err := uploadsecurity.Stage(context.Background(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	path := staged.Name()

	if staged.Size() != int64(len(content)) {
		t.Fatalf("size: got %d", staged.Size())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("temporary file should exist before close: %v", err)
	}

	part := make([]byte, 4)
	if _, err := staged.ReadAt(part, 3); err != nil {
		t.Fatalf("read at: %v", err)
	}
	if string(part) != "3456" {
		t.Fatalf("read at content: got %q", part)
	}

	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	all, err := io.ReadAll(staged)
	if err != nil {
		t.Fatalf("read staged content: %v", err)
	}
	if !bytes.Equal(all, content) {
		t.Fatalf("content: got %q", all)
	}

	if err := staged.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary file should be removed, stat error: %v", err)
	}
	if err := staged.Close(); err != nil {
		t.Fatalf("second close should be safe: %v", err)
	}
}

func TestStageRejectsContentBeyondLimit(t *testing.T) {
	_, err := uploadsecurity.Stage(context.Background(), bytes.NewReader([]byte("12345")), 4)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileTooLarge {
		t.Fatalf("code: got %q, classified=%v", code, ok)
	}
}

func TestStageRejectsEmptyContent(t *testing.T) {
	_, err := uploadsecurity.Stage(context.Background(), bytes.NewReader(nil), 4)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileEmpty {
		t.Fatalf("code: got %q, classified=%v", code, ok)
	}
}

func TestStagePreservesClassifiedStorageReadFailures(t *testing.T) {
	tests := []uploadsecurity.Code{
		uploadsecurity.CodeStorageObjectNotFound,
		uploadsecurity.CodeStorageUnavailable,
	}

	for _, wantCode := range tests {
		t.Run(string(wantCode), func(t *testing.T) {
			sourceErr := uploadsecurity.NewError(
				wantCode,
				errors.New("provider read failed"),
			)
			_, err := uploadsecurity.Stage(
				context.Background(),
				readerFunc(func(_ []byte) (int, error) {
					return 0, sourceErr
				}),
				4,
			)

			code, classified := uploadsecurity.CodeOf(err)
			if !classified || code != wantCode {
				t.Fatalf("Stage() code = %q classified=%v, want preserved %q",
					code, classified, wantCode)
			}
		})
	}
}

type readerFunc func([]byte) (int, error)

func (read readerFunc) Read(buffer []byte) (int, error) {
	return read(buffer)
}
