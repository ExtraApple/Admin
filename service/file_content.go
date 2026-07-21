package service

import (
	"context"
	"errors"
	"io"
	"time"

	"gorm.io/gorm"

	"admin/global"
	"admin/model"
	"admin/service/fileaccess"
	"admin/service/objectstorage"
	"admin/service/uploadsecurity"
)

type FileAccessRepository interface {
	FindByID(ctx context.Context, fileID uint) (*model.File, error)
}

type FileContentService struct {
	signer  *fileaccess.Signer
	storage objectstorage.Store
	files   FileAccessRepository
	now     func() time.Time
}

// FileAccessInput contains the signed application access request supplied by
// an authenticated file handler.
type FileAccessInput struct {
	UserID    uint
	FileID    uint
	ExpiresAt int64
	Signature string
	Mode      fileaccess.Mode
}

// FileContent exposes only trusted response metadata and a streaming reader.
// Object-storage locations remain private to the service layer.
type FileContent struct {
	FileName    string
	ContentType string
	Disposition FileDisposition
	Reader      io.ReadCloser
}

func NewFileContentService(
	signer *fileaccess.Signer,
	storage objectstorage.Store,
	files FileAccessRepository,
	now func() time.Time,
) *FileContentService {
	if now == nil {
		now = time.Now
	}
	return &FileContentService{
		signer:  signer,
		storage: storage,
		files:   files,
		now:     now,
	}
}

func NewManagedFileContentService(
	signer *fileaccess.Signer,
) *FileContentService {
	return NewFileContentService(
		signer,
		objectstorage.NewMinIOStore(global.Minio),
		gormFileRevalidationRepository{db: global.DB},
		time.Now,
	)
}

func (s *FileContentService) Open(
	ctx context.Context,
	input FileAccessInput,
) (*FileContent, error) {
	if s == nil || s.signer == nil || s.storage == nil || s.files == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	file, err := s.files.FindByID(ctx, input.FileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	var decision *FileAccessDecision
	switch input.Mode {
	case fileaccess.ModeDownload:
		decision, err = ResolveDownloadAccess(file)
	case fileaccess.ModePreview:
		decision, err = ResolvePreviewAccess(file)
	default:
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, nil)
	}
	if err != nil {
		return nil, err
	}

	err = s.signer.Verify(fileaccess.Claims{
		UserID:           input.UserID,
		FileID:           input.FileID,
		Mode:             input.Mode,
		ExpiresAt:        input.ExpiresAt,
		ValidationStatus: decision.ValidationStatus,
	}, input.Signature, s.now())
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, err)
	}

	reader, err := s.storage.Open(ctx, decision.Bucket, decision.ObjectName)
	if err != nil {
		return nil, classifyStorageError(err)
	}
	reader, err = prefetchReadCloser(reader, allowEmptyStream)
	if err != nil {
		return nil, classifyStorageError(err)
	}
	return &FileContent{
		FileName:    decision.FileName,
		ContentType: decision.ContentType,
		Disposition: decision.Disposition,
		Reader:      reader,
	}, nil
}
