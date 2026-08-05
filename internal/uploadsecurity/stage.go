package uploadsecurity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
)

const stageBufferSize = 32 * 1024

// StagedFile is a bounded temporary upload that supports both streaming reads
// and random access required by ZIP validation.
type StagedFile struct {
	file   *os.File
	name   string
	size   int64
	digest string
	mu     sync.Mutex
	closed bool
}

// Stage copies at most maxBytes from reader into a temporary file.
func Stage(ctx context.Context, reader io.Reader, maxBytes int64) (*StagedFile, error) {
	if reader == nil {
		return nil, NewError(CodeUploadBodyInvalid, nil)
	}
	if maxBytes < 1 {
		return nil, NewError(CodeFileTooLarge, nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	file, err := os.CreateTemp("", "admin-upload-*")
	if err != nil {
		return nil, NewError(CodeStorageUnavailable, err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}

	size, digest, err := copyBounded(ctx, file, reader, maxBytes)
	if err != nil {
		cleanup()
		return nil, err
	}
	if size == 0 {
		cleanup()
		return nil, NewError(CodeFileEmpty, nil)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, NewError(CodeStorageUnavailable, err)
	}

	return &StagedFile{
		file:   file,
		name:   file.Name(),
		size:   size,
		digest: digest,
	}, nil
}

func copyBounded(ctx context.Context, destination io.Writer, source io.Reader, maxBytes int64) (int64, string, error) {
	buffer := make([]byte, stageBufferSize)
	var size int64
	hash := sha256.New()

	for {
		select {
		case <-ctx.Done():
			return size, "", NewError(CodeUploadBodyInvalid, ctx.Err())
		default:
		}

		readSize := int64(len(buffer))
		if remaining := maxBytes - size + 1; remaining < readSize {
			readSize = remaining
		}
		count, readErr := source.Read(buffer[:readSize])
		if count > 0 {
			if size+int64(count) > maxBytes {
				return size + int64(count), "", NewError(CodeFileTooLarge, nil)
			}
			written, writeErr := destination.Write(buffer[:count])
			if writeErr != nil {
				return size, "", NewError(CodeStorageUnavailable, writeErr)
			}
			if written != count {
				return size, "", NewError(CodeStorageUnavailable, io.ErrShortWrite)
			}
			if _, err := hash.Write(buffer[:written]); err != nil {
				return size, "", NewError(CodeInternalError, err)
			}
			size += int64(written)
		}
		if readErr == io.EOF {
			return size, hex.EncodeToString(hash.Sum(nil)), nil
		}
		if readErr != nil {
			if code, classified := CodeOf(readErr); classified &&
				(code == CodeStorageObjectNotFound ||
					code == CodeStorageUnavailable) {
				return size, "", readErr
			}
			return size, "", NewError(CodeUploadBodyInvalid, readErr)
		}
	}
}

func (f *StagedFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *StagedFile) ReadAt(p []byte, offset int64) (int, error) {
	return f.file.ReadAt(p, offset)
}

func (f *StagedFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

// Size returns the number of staged bytes.
func (f *StagedFile) Size() int64 {
	return f.size
}

// ContentSHA256 returns the service-computed digest of all staged bytes.
func (f *StagedFile) ContentSHA256() string {
	if f == nil {
		return ""
	}
	return f.digest
}

// Name returns the temporary path for low-level validators. It must never be
// included in API responses or audit metadata.
func (f *StagedFile) Name() string {
	return f.name
}

// Close closes and removes the temporary file. It is safe to call repeatedly.
func (f *StagedFile) Close() error {
	if f == nil {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true

	closeErr := f.file.Close()
	removeErr := os.Remove(f.name)
	if os.IsNotExist(removeErr) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}
