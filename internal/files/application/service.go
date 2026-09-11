package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"admin/internal/files/domain"
	"admin/internal/uploadsecurity"
)

const defaultManagedFileMaxBytes int64 = 50 * 1024 * 1024

type Service struct{ deps Dependencies }

func NewService(deps Dependencies) *Service {
	if deps.Clock == nil {
		deps.Clock = ClockFunc(time.Now)
	}
	if deps.DownloadURLExpireSeconds == 0 {
		deps.DownloadURLExpireSeconds = 300
	}
	if deps.ObjectNames == nil {
		deps.ObjectNames = UUIDObjectNames{}
	}
	if deps.Transactions == nil {
		deps.Transactions = directTransactionRunner{}
	}
	return &Service{deps: deps}
}
func DefaultManagedFileMaxBytes() int64 { return defaultManagedFileMaxBytes }

type UUIDObjectNames struct{}

func (UUIDObjectNames) ManagedFileName(_ uploadsecurity.CanonicalType, extension string) (string, error) {
	if strings.TrimSpace(extension) == "" || !strings.HasPrefix(extension, ".") {
		return "", errors.New("invalid canonical extension")
	}
	return uuid.NewString() + extension, nil
}

type directTransactionRunner struct{}

func (directTransactionRunner) Run(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func (s *Service) repository() error {
	if s == nil || s.deps.Repository == nil || s.deps.Storage == nil {
		return uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	return nil
}
func (s *Service) tx(ctx context.Context, operation func(context.Context) error) error {
	if s == nil || s.deps.Transactions == nil {
		return uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	return s.deps.Transactions.Run(ctx, operation)
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (*FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, err
	}
	if s.deps.Validator == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	result, err := s.deps.Validator.Validate(ctx, uploadsecurity.Input{Purpose: uploadsecurity.PurposeManagedFile, FileName: input.FileName, DeclaredMIME: input.ContentType, Size: input.Size, MaxBytes: input.MaxBytes, Reader: input.Reader})
	if err != nil {
		return nil, err
	}
	if result.Reader == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if closer, ok := result.Reader.(io.Closer); ok {
		defer closer.Close()
	}
	name, err := s.deps.ObjectNames.ManagedFileName(result.CanonicalType, result.CanonicalExtension)
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
	}
	if err := s.deps.Storage.Put(ctx, ObjectInput{Bucket: "files", Name: name, Reader: result.Reader, Size: result.Size, ContentType: result.CanonicalMIME}); err != nil {
		return nil, classifyStorageError(err)
	}
	validatedAt := s.deps.Clock.Now().UTC()
	file := domain.File{Name: result.FileName, Bucket: "files", ObjectName: name, ContentType: result.CanonicalMIME, DetectedContentType: result.DetectedMIME, ContentSHA256: result.ContentSHA256, Size: result.Size, UploaderID: input.UploaderID, Purpose: string(uploadsecurity.PurposeManagedFile), ValidationStatus: domain.ValidationStatusValidated, ValidationPolicyVersion: result.PolicyVersion, ValidatedAt: &validatedAt}
	if err := s.tx(ctx, func(tx context.Context) error { return s.deps.Repository.Create(tx, &file) }); err != nil {
		_ = s.deps.Storage.Delete(ctx, file.Bucket, file.ObjectName)
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	info := toFileInfo(&file)
	return info, nil
}
func (s *Service) UploadMessageImage(ctx context.Context, input MessageImageUploadInput) (TemporaryMessageImage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return TemporaryMessageImage{}, err
	}
	if s.deps.MessageImages == nil || s.deps.MessageImageValidator == nil {
		return TemporaryMessageImage{}, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	result, err := s.deps.MessageImageValidator.Validate(ctx, uploadsecurity.Input{Purpose: uploadsecurity.PurposeMessageImage, FileName: input.FileName, DeclaredMIME: input.ContentType, Size: input.Size, MaxBytes: uploadsecurity.MaxMessageImageBytes, Reader: input.Reader})
	if err != nil {
		return TemporaryMessageImage{}, err
	}
	if result.Reader == nil {
		return TemporaryMessageImage{}, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if closer, ok := result.Reader.(io.Closer); ok {
		defer closer.Close()
	}
	name, err := s.deps.ObjectNames.ManagedFileName(result.CanonicalType, result.CanonicalExtension)
	if err != nil {
		return TemporaryMessageImage{}, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
	}
	name = "message-images/" + name
	if err := s.deps.Storage.Put(ctx, ObjectInput{Bucket: "files", Name: name, Reader: result.Reader, Size: result.Size, ContentType: result.CanonicalMIME}); err != nil {
		return TemporaryMessageImage{}, classifyStorageError(err)
	}
	now := s.deps.Clock.Now().UTC()
	expiresAt := now.Add(15 * time.Minute)
	file := domain.File{Name: result.FileName, Bucket: "files", ObjectName: name, ContentType: result.CanonicalMIME, DetectedContentType: result.DetectedMIME, ContentSHA256: result.ContentSHA256, Size: result.Size, UploaderID: input.UploaderID, Purpose: string(uploadsecurity.PurposeMessageImage), BindingExpiresAt: &expiresAt, ValidationStatus: domain.ValidationStatusValidated, ValidationPolicyVersion: result.PolicyVersion, ValidatedAt: &now}
	if err := s.tx(ctx, func(tx context.Context) error { return s.deps.MessageImages.CreateMessageImage(tx, &file) }); err != nil {
		_ = s.deps.Storage.Delete(ctx, file.Bucket, file.ObjectName)
		return TemporaryMessageImage{}, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return TemporaryMessageImage{ID: file.ID, ExpiresAt: expiresAt}, nil
}

func (s *Service) BindMessageImages(ctx context.Context, request MessageImageBindRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.deps.MessageImages == nil || request.ActorID == 0 || request.MessageLogicalID == "" || len(request.ImageIDs) == 0 {
		return uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, nil)
	}
	if request.Now.IsZero() {
		request.Now = s.deps.Clock.Now().UTC()
	}
	if err := s.tx(ctx, func(tx context.Context) error { return s.deps.MessageImages.BindMessageImages(tx, request) }); err != nil {
		if errors.Is(err, ErrFileNotFound) || errors.Is(err, ErrStateConflict) {
			return uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, err)
		}
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return nil
}

func (s *Service) OpenMessageImage(ctx context.Context, request MessageImageOpenRequest) (MessageImageContent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.deps.MessageImages == nil || request.ID == 0 || request.MessageLogicalID == "" {
		return MessageImageContent{}, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, nil)
	}
	file, err := s.deps.MessageImages.FindMessageImage(ctx, request.ID)
	if err != nil {
		if errors.Is(err, ErrFileNotFound) {
			return MessageImageContent{}, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, err)
		}
		return MessageImageContent{}, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	if file.Purpose != string(uploadsecurity.PurposeMessageImage) || file.LogicalMessageID != request.MessageLogicalID || file.ValidationStatus != domain.ValidationStatusValidated || !isMessageImageMIME(file.ContentType) {
		return MessageImageContent{}, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, nil)
	}
	reader, err := s.deps.Storage.Open(ctx, file.Bucket, file.ObjectName)
	if err != nil {
		return MessageImageContent{}, classifyStorageError(err)
	}
	return MessageImageContent{Reader: reader, ContentType: file.ContentType, Size: file.Size}, nil
}

func (s *Service) CleanupExpiredMessageImages(ctx context.Context, now time.Time, limit int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.deps.MessageImages == nil {
		return uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if now.IsZero() {
		now = s.deps.Clock.Now().UTC()
	}
	files, err := s.deps.MessageImages.DeleteExpiredMessageImages(ctx, now, limit)
	if err != nil {
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	for _, file := range files {
		if err := s.deps.Storage.Delete(ctx, file.Bucket, file.ObjectName); err != nil {
			return classifyStorageError(err)
		}
	}
	return nil
}

func isMessageImageMIME(value string) bool {
	canonicalType, ok := uploadsecurity.LookupTypeByMIME(value)
	return ok && isMessageImageCanonicalType(canonicalType)
}

func isMessageImageCanonicalType(value uploadsecurity.CanonicalType) bool {
	switch value {
	case uploadsecurity.TypeJPEG, uploadsecurity.TypePNG, uploadsecurity.TypeWebP:
		return true
	default:
		return false
	}
}

func (s *Service) List(ctx context.Context, actor uint, page, size int, prefix string) ([]FileInfo, int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, 0, err
	}
	if page < 1 || size < 1 {
		return nil, 0, uploadsecurity.NewError(uploadsecurity.CodeRequestInvalid, nil)
	}
	files, total, err := s.deps.Repository.List(ctx, page, size, prefix)
	if err != nil {
		return nil, 0, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	list := make([]FileInfo, len(files))
	for i := range files {
		list[i] = *toFileInfo(&files[i])
	}
	return list, total, nil
}

func (s *Service) Get(ctx context.Context, actor, id uint) (*FileDetailResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, err
	}
	file, err := s.find(ctx, id)
	if err != nil {
		return nil, err
	}
	result := &FileDetailResponse{File: toFileInfo(&file)}
	allow := false
	switch file.ValidationStatus {
	case domain.ValidationStatusValidated:
		_, allow = managedDownloadMIME(file.ContentType)
	case domain.ValidationStatusLegacyUnverified, domain.ValidationStatusValidationError:
		allow = true
	}
	if allow {
		if s.deps.Signer == nil || s.deps.DownloadURLExpireSeconds < 1 {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
		}
		expires := s.deps.Clock.Now().Add(time.Duration(s.deps.DownloadURLExpireSeconds) * time.Second).Unix()
		signature, err := s.deps.Signer.Sign(Claims{UserID: actor, FileID: id, Mode: ModeDownload, ExpiresAt: expires, ValidationStatus: file.ValidationStatus})
		if err != nil {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
		}
		query := url.Values{}
		query.Set("expires", fmt.Sprintf("%d", expires))
		query.Set("signature", signature)
		result.DownloadURL = (&url.URL{Path: fmt.Sprintf("/api/admin/files/%d/download", id), RawQuery: query.Encode()}).String()
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, actor, id uint, request UpdateFileRequest) (*FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, err
	}
	file, err := s.find(ctx, id)
	if err != nil {
		return nil, err
	}
	canonical, ok := uploadsecurity.LookupTypeByMIME(file.ContentType)
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	definition, ok := uploadsecurity.DefinitionForType(canonical)
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	name, err := uploadsecurity.SanitizeManagedFileRename(request.Name, definition.CanonicalExtension)
	if err != nil {
		return nil, err
	}
	if err := s.tx(ctx, func(tx context.Context) error { return s.deps.Repository.UpdateName(tx, id, name) }); err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	file.Name = name
	return toFileInfo(&file), nil
}

func (s *Service) Delete(ctx context.Context, actor, id uint) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return err
	}
	file, err := s.find(ctx, id)
	if err != nil {
		return err
	}
	if err := s.deps.Storage.Delete(ctx, file.Bucket, file.ObjectName); err != nil {
		return classifyStorageError(err)
	}
	if err := s.tx(ctx, func(tx context.Context) error { return s.deps.Repository.Delete(tx, id) }); err != nil {
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return nil
}

func (s *Service) Browse(ctx context.Context, actor uint, prefix string) ([]FileObjectInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, err
	}
	objects, err := s.deps.Storage.List(ctx, "files", ListOptions{Prefix: prefix, Recursive: true})
	if err != nil {
		return nil, classifyStorageError(err)
	}
	result := make([]FileObjectInfo, 0, len(objects))
	for i := range objects {
		if strings.HasPrefix(objects[i].Name, "message-images/") {
			continue
		}
		result = append(result, FileObjectInfo{Name: objects[i].Name, Size: objects[i].Size, ContentType: objects[i].ContentType, LastModified: objects[i].LastModified})
	}
	return result, nil
}

func (s *Service) Revalidate(ctx context.Context, actor, id uint, maxBytes int64) (*FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, err
	}
	if s.deps.Validator == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	file, err := s.find(ctx, id)
	if err != nil {
		return nil, err
	}
	if file.ValidationStatus != domain.ValidationStatusLegacyUnverified && file.ValidationStatus != domain.ValidationStatusValidationError {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	reader, err := s.deps.Storage.Open(ctx, file.Bucket, file.ObjectName)
	if err != nil {
		classified := classifyStorageError(err)
		code, _ := uploadsecurity.CodeOf(classified)
		if updateErr := s.saveFailure(ctx, &file, code); updateErr != nil {
			return nil, updateErr
		}
		return nil, classified
	}
	defer reader.Close()
	result, err := s.deps.Validator.Validate(ctx, uploadsecurity.Input{Purpose: uploadsecurity.PurposeManagedFile, FileName: file.Name, DeclaredMIME: file.ContentType, Size: file.Size, MaxBytes: maxBytes, Reader: reader})
	if err != nil {
		code, classified := uploadsecurity.CodeOf(err)
		if classified && isPolicyRejection(code) {
			now := s.deps.Clock.Now().UTC()
			update := ValidationUpdate{ContentType: file.ContentType, DetectedContentType: file.DetectedContentType, ContentSHA256: file.ContentSHA256, Status: domain.ValidationStatusBlocked, PolicyVersion: uploadsecurity.PolicyVersionV1, ErrorCode: string(code), ValidatedAt: &now}
			if updateErr := s.updateValidation(ctx, id, update); updateErr != nil {
				return nil, updateErr
			}
			return nil, err
		}
		if classified && isRetryable(code) {
			if updateErr := s.saveFailure(ctx, &file, code); updateErr != nil {
				return nil, updateErr
			}
		}
		return nil, err
	}
	if closer, ok := result.Reader.(io.Closer); ok {
		defer closer.Close()
	}
	now := s.deps.Clock.Now().UTC()
	update := ValidationUpdate{ContentType: result.CanonicalMIME, DetectedContentType: result.DetectedMIME, ContentSHA256: result.ContentSHA256, Status: domain.ValidationStatusValidated, PolicyVersion: result.PolicyVersion, ValidatedAt: &now}
	if err := s.updateValidation(ctx, id, update); err != nil {
		return nil, err
	}
	applyValidationUpdate(&file, update)
	return toFileInfo(&file), nil
}

func (s *Service) revalidationReader(ctx context.Context, file *domain.File) (io.ReadCloser, error) {
	reader, err := s.deps.Storage.Open(ctx, file.Bucket, file.ObjectName)
	if err != nil {
		classified := classifyStorageError(err)
		code, _ := uploadsecurity.CodeOf(classified)
		if updateErr := s.saveFailure(ctx, file, code); updateErr != nil {
			return nil, updateErr
		}
		return nil, classified
	}
	return reader, nil
}

func (s *Service) Open(ctx context.Context, input FileAccessInput) (*Content, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.repository(); err != nil {
		return nil, err
	}
	file, err := s.find(ctx, input.FileID)
	if err != nil {
		return nil, err
	}
	decision, err := resolveAccess(file, input.Mode)
	if err != nil {
		return nil, err
	}
	if s.deps.Signer == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if err := s.deps.Signer.Verify(Claims{UserID: input.UserID, FileID: input.FileID, Mode: input.Mode, ExpiresAt: input.ExpiresAt, ValidationStatus: decision.ValidationStatus}, input.Signature, s.deps.Clock.Now()); err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, err)
	}
	reader, err := s.deps.Storage.Open(ctx, decision.Bucket, decision.ObjectName)
	if err != nil {
		return nil, classifyStorageError(err)
	}
	reader, err = prefetchReadCloser(reader, input.Mode == ModeDownload)
	if err != nil {
		return nil, classifyStorageError(err)
	}
	return &Content{FileName: decision.FileName, ContentType: decision.ContentType, Disposition: decision.Disposition, Reader: reader}, nil
}
func (s *Service) find(ctx context.Context, id uint) (domain.File, error) {
	file, err := s.deps.Repository.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrFileNotFound) {
			return domain.File{}, uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return domain.File{}, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return file, nil
}
func (s *Service) updateValidation(ctx context.Context, id uint, update ValidationUpdate) error {
	if err := s.tx(ctx, func(tx context.Context) error { return s.deps.Repository.UpdateValidation(tx, id, update) }); err != nil {
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return nil
}
func (s *Service) saveFailure(ctx context.Context, file *domain.File, code uploadsecurity.Code) error {
	now := s.deps.Clock.Now().UTC()
	return s.updateValidation(ctx, file.ID, ValidationUpdate{ContentType: file.ContentType, DetectedContentType: file.DetectedContentType, ContentSHA256: file.ContentSHA256, Status: domain.ValidationStatusValidationError, PolicyVersion: uploadsecurity.PolicyVersionV1, ErrorCode: string(code), ValidatedAt: &now})
}
func classifyStorageError(err error) error {
	if _, ok := uploadsecurity.CodeOf(err); ok {
		return err
	}
	return uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, err)
}
func isPolicyRejection(code uploadsecurity.Code) bool {
	switch code {
	case uploadsecurity.CodeFileEmpty, uploadsecurity.CodeFileTooLarge, uploadsecurity.CodeFileNameInvalid, uploadsecurity.CodeFileTypeNotAllowed, uploadsecurity.CodeFileTypeMismatch, uploadsecurity.CodeFileEncodingInvalid, uploadsecurity.CodeFileContentInvalid, uploadsecurity.CodeImageDimensionLimit, uploadsecurity.CodeImageDecodeInvalid:
		return true
	}
	return false
}
func isRetryable(code uploadsecurity.Code) bool {
	switch code {
	case uploadsecurity.CodeUploadBodyInvalid, uploadsecurity.CodeStorageObjectNotFound, uploadsecurity.CodeStorageUnavailable, uploadsecurity.CodeInternalError:
		return true
	}
	return false
}
func managedDownloadMIME(value string) (string, bool) {
	typ, ok := uploadsecurity.LookupTypeByMIME(value)
	if !ok || !uploadsecurity.IsManagedFileType(typ) {
		return "", false
	}
	def, ok := uploadsecurity.DefinitionForType(typ)
	if !ok {
		return "", false
	}
	return def.MIME, true
}
func resolveAccess(file domain.File, mode AccessMode) (*AccessDecision, error) {
	if mode == ModePreview {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	if mode != ModeDownload {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileAccessInvalid, nil)
	}
	contentType := ""
	switch file.ValidationStatus {
	case domain.ValidationStatusValidated:
		var ok bool
		contentType, ok = managedDownloadMIME(file.ContentType)
		if !ok {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
		}
	case domain.ValidationStatusLegacyUnverified, domain.ValidationStatusValidationError:
		contentType = "application/octet-stream"
	case domain.ValidationStatusBlocked:
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateBlocked, nil)
	default:
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	return &AccessDecision{FileName: file.Name, Bucket: file.Bucket, ObjectName: file.ObjectName, ContentType: contentType, ValidationStatus: file.ValidationStatus, Disposition: DispositionAttachment}, nil
}
func applyValidationUpdate(file *domain.File, update ValidationUpdate) {
	file.ContentType = update.ContentType
	file.DetectedContentType = update.DetectedContentType
	file.ContentSHA256 = update.ContentSHA256
	file.ValidationStatus = update.Status
	file.ValidationPolicyVersion = update.PolicyVersion
	file.ValidationErrorCode = update.ErrorCode
	file.ValidatedAt = update.ValidatedAt
}
func toFileInfo(file *domain.File) *FileInfo {
	validated := ""
	if file.ValidatedAt != nil {
		validated = file.ValidatedAt.Format("2006-01-02 15:04:05")
	}
	return &FileInfo{ID: file.ID, Name: file.Name, ContentType: file.ContentType, DetectedContentType: file.DetectedContentType, ContentSHA256: file.ContentSHA256, Size: file.Size, UploaderID: file.UploaderID, ValidationStatus: file.ValidationStatus, ValidationPolicyVersion: file.ValidationPolicyVersion, ValidationErrorCode: file.ValidationErrorCode, ValidatedAt: validated, CreatedAt: file.CreatedAt.Format("2006-01-02 15:04:05")}
}

func prefetchReadCloser(reader io.ReadCloser, allowEmpty bool) (io.ReadCloser, error) {
	if reader == nil {
		return nil, io.ErrUnexpectedEOF
	}
	first := make([]byte, 1)
	n, err := reader.Read(first)
	if _, classified := uploadsecurity.CodeOf(err); classified {
		_ = reader.Close()
		return nil, err
	}
	if n > 0 {
		if err != nil && !errors.Is(err, io.EOF) {
			_ = reader.Close()
			return nil, err
		}
		return &prefetchedReadCloser{Reader: io.MultiReader(bytes.NewReader(first[:n]), reader), closer: reader}, nil
	}
	if errors.Is(err, io.EOF) && allowEmpty {
		return &prefetchedReadCloser{Reader: bytes.NewReader(nil), closer: reader}, nil
	}
	if err == nil {
		err = io.ErrNoProgress
	}
	_ = reader.Close()
	return nil, err
}

type prefetchedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *prefetchedReadCloser) Close() error { return r.closer.Close() }
