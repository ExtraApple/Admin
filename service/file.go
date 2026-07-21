package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
	"admin/service/fileaccess"
	"admin/service/objectstorage"
	"admin/service/uploadsecurity"
	"admin/utils"
)

const (
	fileBucket                 = "files"
	defaultManagedFileMaxBytes = int64(50 * 1024 * 1024)
)

type FileRepository interface {
	Create(ctx context.Context, file *model.File) error
}

type FileService struct {
	validator uploadsecurity.Validator
	storage   objectstorage.Store
	files     FileRepository
}

type FileBrowseService struct {
	storage objectstorage.Store
}

type FileRevalidationRepository interface {
	FindByID(ctx context.Context, fileID uint) (*model.File, error)
	UpdateValidation(ctx context.Context, fileID uint, update FileValidationUpdate) error
}

type FileValidationUpdate struct {
	ContentType         string
	DetectedContentType string
	Status              string
	PolicyVersion       string
	ErrorCode           string
	ValidatedAt         *time.Time
}

type FileRevalidationService struct {
	validator uploadsecurity.Validator
	storage   objectstorage.Store
	files     FileRevalidationRepository
	maxBytes  int64
	now       func() time.Time
}

type FileDetailService struct {
	signer        *fileaccess.Signer
	expireSeconds int
	now           func() time.Time
}

type FileDisposition string

const (
	FileDispositionAttachment FileDisposition = "attachment"
	FileDispositionInline     FileDisposition = "inline"
)

// FileAccessDecision contains trusted internal metadata needed to proxy a
// stored object without exposing object-storage details to API responses.
type FileAccessDecision struct {
	FileName         string
	Bucket           string
	ObjectName       string
	ContentType      string
	ValidationStatus string
	Disposition      FileDisposition
}

type gormFileRepository struct {
	db *gorm.DB
}

type gormFileRevalidationRepository struct {
	db *gorm.DB
}

func (r gormFileRepository) Create(ctx context.Context, file *model.File) error {
	return r.db.WithContext(ctx).Create(file).Error
}

func (r gormFileRevalidationRepository) FindByID(
	ctx context.Context,
	fileID uint,
) (*model.File, error) {
	var file model.File
	if err := r.db.WithContext(ctx).First(&file, fileID).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (r gormFileRevalidationRepository) UpdateValidation(
	ctx context.Context,
	fileID uint,
	update FileValidationUpdate,
) error {
	return r.db.WithContext(ctx).
		Model(&model.File{}).
		Where("id = ?", fileID).
		Updates(map[string]any{
			"content_type":              update.ContentType,
			"detected_content_type":     update.DetectedContentType,
			"validation_status":         update.Status,
			"validation_policy_version": update.PolicyVersion,
			"validation_error_code":     update.ErrorCode,
			"validated_at":              update.ValidatedAt,
		}).Error
}

type UploadFileInput struct {
	UploaderID  uint
	FileName    string
	ContentType string
	Size        int64
	MaxBytes    int64
	Reader      io.Reader
}

func NewFileService(
	validator uploadsecurity.Validator,
	storage objectstorage.Store,
	files FileRepository,
) *FileService {
	return &FileService{
		validator: validator,
		storage:   storage,
		files:     files,
	}
}

func NewManagedFileService() *FileService {
	return NewFileService(
		uploadsecurity.NewManagedFileValidator(),
		objectstorage.NewMinIOStore(global.Minio),
		gormFileRepository{db: global.DB},
	)
}

func NewFileBrowseService(storage objectstorage.Store) *FileBrowseService {
	return &FileBrowseService{storage: storage}
}

func NewFileRevalidationService(
	validator uploadsecurity.Validator,
	storage objectstorage.Store,
	files FileRevalidationRepository,
	maxBytes int64,
	now func() time.Time,
) *FileRevalidationService {
	if now == nil {
		now = time.Now
	}
	return &FileRevalidationService{
		validator: validator,
		storage:   storage,
		files:     files,
		maxBytes:  maxBytes,
		now:       now,
	}
}

func NewManagedFileRevalidationService(
	maxBytes int64,
) *FileRevalidationService {
	return NewFileRevalidationService(
		uploadsecurity.NewManagedFileValidator(),
		objectstorage.NewMinIOStore(global.Minio),
		gormFileRevalidationRepository{db: global.DB},
		maxBytes,
		time.Now,
	)
}

func NewFileDetailService(
	signer *fileaccess.Signer,
	expireSeconds int,
	now func() time.Time,
) *FileDetailService {
	if now == nil {
		now = time.Now
	}
	return &FileDetailService{
		signer:        signer,
		expireSeconds: expireSeconds,
		now:           now,
	}
}

func (s *FileDetailService) Get(
	ctx context.Context,
	userID, fileID uint,
) (*dto.FileDetailResp, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var file model.File
	if err := global.DB.WithContext(ctx).First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	result := &dto.FileDetailResp{File: toFileInfo(&file)}
	allowDownload := false
	allowPreview := false
	switch file.ValidationStatus {
	case model.FileValidationStatusValidated:
		allowDownload = true
		allowPreview = isPreviewableImageMIME(file.ContentType)
	case model.FileValidationStatusLegacyUnverified,
		model.FileValidationStatusValidationError:
		allowDownload = true
	case model.FileValidationStatusBlocked:
		return result, nil
	default:
		return result, nil
	}

	expiresAt := s.now().Add(time.Duration(s.expireSeconds) * time.Second).Unix()
	if allowDownload {
		downloadURL, err := s.signURL(userID, file.ID, file.ValidationStatus, fileaccess.ModeDownload, expiresAt)
		if err != nil {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
		}
		result.DownloadURL = downloadURL
	}
	if allowPreview {
		previewURL, err := s.signURL(userID, file.ID, file.ValidationStatus, fileaccess.ModePreview, expiresAt)
		if err != nil {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
		}
		result.PreviewURL = previewURL
	}
	return result, nil
}

func (s *FileDetailService) signURL(
	userID, fileID uint,
	status string,
	mode fileaccess.Mode,
	expiresAt int64,
) (string, error) {
	if s == nil || s.signer == nil || s.expireSeconds < 1 {
		return "", errors.New("file detail signing is not configured")
	}

	signature, err := s.signer.Sign(fileaccess.Claims{
		UserID:           userID,
		FileID:           fileID,
		Mode:             mode,
		ExpiresAt:        expiresAt,
		ValidationStatus: status,
	})
	if err != nil {
		return "", err
	}

	query := url.Values{}
	query.Set("expires", fmt.Sprintf("%d", expiresAt))
	query.Set("signature", signature)
	return (&url.URL{
		Path:     fmt.Sprintf("/api/admin/files/%d/%s", fileID, mode),
		RawQuery: query.Encode(),
	}).String(), nil
}

func isPreviewableImageMIME(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

// ResolveDownloadAccess applies the current validation-state policy before
// a download handler opens or streams the stored object.
func ResolveDownloadAccess(file *model.File) (*FileAccessDecision, error) {
	if file == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}

	contentType := ""
	switch file.ValidationStatus {
	case model.FileValidationStatusValidated:
		canonicalType, ok := uploadsecurity.LookupTypeByMIME(file.ContentType)
		if !ok {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
		}
		definition, ok := uploadsecurity.DefinitionForType(canonicalType)
		if !ok {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
		}
		contentType = definition.MIME
	case model.FileValidationStatusLegacyUnverified,
		model.FileValidationStatusValidationError:
		contentType = "application/octet-stream"
	case model.FileValidationStatusBlocked:
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateBlocked, nil)
	default:
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}

	return &FileAccessDecision{
		FileName:         file.Name,
		Bucket:           file.Bucket,
		ObjectName:       file.ObjectName,
		ContentType:      contentType,
		ValidationStatus: file.ValidationStatus,
		Disposition:      FileDispositionAttachment,
	}, nil
}

// ResolvePreviewAccess permits inline access only for validated V1 image
// types. All other files remain download-only or inaccessible.
func ResolvePreviewAccess(file *model.File) (*FileAccessDecision, error) {
	if file == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if file.ValidationStatus == model.FileValidationStatusBlocked {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateBlocked, nil)
	}
	if file.ValidationStatus != model.FileValidationStatusValidated {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}

	canonicalType, ok := uploadsecurity.LookupTypeByMIME(file.ContentType)
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	definition, ok := uploadsecurity.DefinitionForType(canonicalType)
	if !ok || !isPreviewableImageMIME(definition.MIME) {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}

	return &FileAccessDecision{
		FileName:         file.Name,
		Bucket:           file.Bucket,
		ObjectName:       file.ObjectName,
		ContentType:      definition.MIME,
		ValidationStatus: file.ValidationStatus,
		Disposition:      FileDispositionInline,
	}, nil
}

func (s *FileRevalidationService) Revalidate(
	ctx context.Context,
	fileID uint,
) (*dto.FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	file, err := s.files.FindByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	if file.ValidationStatus != model.FileValidationStatusLegacyUnverified &&
		file.ValidationStatus != model.FileValidationStatusValidationError {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}

	reader, err := s.storage.Open(ctx, file.Bucket, file.ObjectName)
	if err != nil {
		storageErr := classifyStorageError(err)
		code, _ := uploadsecurity.CodeOf(storageErr)
		if updateErr := s.saveRevalidationFailure(
			ctx,
			file,
			model.FileValidationStatusValidationError,
			code,
		); updateErr != nil {
			return nil, updateErr
		}
		return nil, storageErr
	}
	defer reader.Close()

	result, err := s.validator.Validate(ctx, uploadsecurity.Input{
		Purpose:      uploadsecurity.PurposeManagedFile,
		FileName:     file.Name,
		DeclaredMIME: file.ContentType,
		Size:         file.Size,
		MaxBytes:     s.maxBytes,
		Reader:       reader,
	})
	if err != nil {
		code, classified := uploadsecurity.CodeOf(err)
		if classified && isFilePolicyRejection(code) {
			validatedAt := s.now().UTC()
			update := FileValidationUpdate{
				ContentType:         file.ContentType,
				DetectedContentType: file.DetectedContentType,
				Status:              model.FileValidationStatusBlocked,
				PolicyVersion:       uploadsecurity.PolicyVersionV1,
				ErrorCode:           string(code),
				ValidatedAt:         &validatedAt,
			}
			if updateErr := s.files.UpdateValidation(ctx, file.ID, update); updateErr != nil {
				return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, updateErr)
			}
		} else if classified && isRevalidationRetryable(code) {
			if updateErr := s.saveRevalidationFailure(
				ctx,
				file,
				model.FileValidationStatusValidationError,
				code,
			); updateErr != nil {
				return nil, updateErr
			}
		}
		return nil, err
	}
	if closer, ok := result.Reader.(io.Closer); ok {
		defer closer.Close()
	}

	validatedAt := s.now().UTC()
	update := FileValidationUpdate{
		ContentType:         result.CanonicalMIME,
		DetectedContentType: result.DetectedMIME,
		Status:              model.FileValidationStatusValidated,
		PolicyVersion:       result.PolicyVersion,
		ErrorCode:           "",
		ValidatedAt:         &validatedAt,
	}
	if err := s.files.UpdateValidation(ctx, file.ID, update); err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	applyValidationUpdate(file, update)
	return toFileInfo(file), nil
}

func (s *FileBrowseService) List(
	ctx context.Context,
	prefix string,
) ([]dto.FileObjectInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	objects, err := s.storage.List(ctx, fileBucket, objectstorage.ListOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	if err != nil {
		return nil, classifyStorageError(err)
	}

	result := make([]dto.FileObjectInfo, len(objects))
	for index, object := range objects {
		result[index] = dto.FileObjectInfo{
			Name:         object.Name,
			Size:         object.Size,
			ContentType:  object.ContentType,
			LastModified: object.LastModified,
		}
	}
	return result, nil
}

func (s *FileRevalidationService) saveRevalidationFailure(
	ctx context.Context,
	file *model.File,
	status string,
	code uploadsecurity.Code,
) error {
	validatedAt := s.now().UTC()
	update := FileValidationUpdate{
		ContentType:         file.ContentType,
		DetectedContentType: file.DetectedContentType,
		Status:              status,
		PolicyVersion:       uploadsecurity.PolicyVersionV1,
		ErrorCode:           string(code),
		ValidatedAt:         &validatedAt,
	}
	if err := s.files.UpdateValidation(ctx, file.ID, update); err != nil {
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return nil
}

func isFilePolicyRejection(code uploadsecurity.Code) bool {
	switch code {
	case uploadsecurity.CodeFileEmpty,
		uploadsecurity.CodeFileTooLarge,
		uploadsecurity.CodeFileNameInvalid,
		uploadsecurity.CodeFileTypeNotAllowed,
		uploadsecurity.CodeFileTypeMismatch,
		uploadsecurity.CodeFileEncodingInvalid,
		uploadsecurity.CodeFileContentInvalid,
		uploadsecurity.CodeImageDimensionLimit,
		uploadsecurity.CodeImageDecodeInvalid,
		uploadsecurity.CodeOOXMLInvalid,
		uploadsecurity.CodeOOXMLDangerousContent,
		uploadsecurity.CodeOOXMLResourceLimit:
		return true
	default:
		return false
	}
}

func isRevalidationRetryable(code uploadsecurity.Code) bool {
	switch code {
	case uploadsecurity.CodeUploadBodyInvalid,
		uploadsecurity.CodeStorageObjectNotFound,
		uploadsecurity.CodeStorageUnavailable,
		uploadsecurity.CodeInternalError:
		return true
	default:
		return false
	}
}

func applyValidationUpdate(file *model.File, update FileValidationUpdate) {
	file.ContentType = update.ContentType
	file.DetectedContentType = update.DetectedContentType
	file.ValidationStatus = update.Status
	file.ValidationPolicyVersion = update.PolicyVersion
	file.ValidationErrorCode = update.ErrorCode
	file.ValidatedAt = update.ValidatedAt
}

func (s *FileService) Upload(
	ctx context.Context,
	input UploadFileInput,
) (*dto.FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := s.validator.Validate(ctx, uploadsecurity.Input{
		Purpose:      uploadsecurity.PurposeManagedFile,
		FileName:     input.FileName,
		DeclaredMIME: input.ContentType,
		Size:         input.Size,
		MaxBytes:     input.MaxBytes,
		Reader:       input.Reader,
	})
	if err != nil {
		return nil, err
	}
	if result.Reader == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, nil)
	}
	if closer, ok := result.Reader.(io.Closer); ok {
		defer closer.Close()
	}

	objectName, err := objectstorage.NewManagedFileObjectName(result.CanonicalType)
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
	}
	if _, err := s.storage.Put(ctx, objectstorage.PutInput{
		Bucket:      fileBucket,
		Name:        objectName,
		Reader:      result.Reader,
		Size:        result.Size,
		ContentType: result.CanonicalMIME,
	}); err != nil {
		storageErr := classifyStorageError(err)
		code, _ := uploadsecurity.CodeOf(storageErr)
		global.Logger.Error(
			"managed file storage write failed",
			zap.String("error_code", string(code)),
		)
		return nil, storageErr
	}

	validatedAt := time.Now().UTC()
	file := model.File{
		Name:                    result.FileName,
		Bucket:                  fileBucket,
		ObjectName:              objectName,
		ContentType:             result.CanonicalMIME,
		DetectedContentType:     result.DetectedMIME,
		Size:                    result.Size,
		UploaderID:              input.UploaderID,
		ValidationStatus:        model.FileValidationStatusValidated,
		ValidationPolicyVersion: result.PolicyVersion,
		ValidationErrorCode:     "",
		ValidatedAt:             &validatedAt,
	}
	if err := s.files.Create(ctx, &file); err != nil {
		persistenceErr := uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
		global.Logger.Error(
			"managed file record creation failed",
			zap.String("error_code", string(uploadsecurity.CodePersistenceFailed)),
		)
		if deleteErr := s.storage.Delete(ctx, fileBucket, objectName); deleteErr != nil {
			deleteErr = classifyStorageError(deleteErr)
			code, _ := uploadsecurity.CodeOf(deleteErr)
			global.Logger.Error(
				"managed file compensation delete failed",
				zap.String("error_code", string(code)),
			)
		}
		return nil, persistenceErr
	}

	return toFileInfo(&file), nil
}

func classifyStorageError(err error) error {
	if _, classified := uploadsecurity.CodeOf(err); classified {
		return err
	}
	return uploadsecurity.NewError(uploadsecurity.CodeStorageUnavailable, err)
}

// UploadFile 上传文件，存入 MinIO 并在 files 表记录元数据
func UploadFile(uploaderID uint, fileName, contentType string, size int64, reader io.Reader) (*dto.FileInfo, error) {
	return NewManagedFileService().Upload(context.Background(), UploadFileInput{
		UploaderID:  uploaderID,
		FileName:    fileName,
		ContentType: contentType,
		Size:        size,
		MaxBytes:    defaultManagedFileMaxBytes,
		Reader:      reader,
	})
}

// GetFile 获取不包含对象存储访问地址的文件详情。
func GetFile(fileID uint) (*dto.FileInfo, error) {
	var file model.File
	if err := global.DB.First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	return toFileInfo(&file), nil
}

// ListFiles 获取文件列表（分页，支持按前缀筛选）
func ListFiles(page, pageSize int, prefix string) ([]dto.FileInfo, int64, error) {
	var files []model.File
	var total int64

	query := global.DB.Model(&model.File{})
	if prefix != "" {
		query = query.Where("object_name LIKE ?", prefix+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&files).Error; err != nil {
		return nil, 0, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	list := make([]dto.FileInfo, len(files))
	for i, f := range files {
		list[i] = *toFileInfo(&f)
	}
	return list, total, nil
}

// UpdateFile 修改文件元信息（仅文件名）
func UpdateFile(fileID uint, req dto.UpdateFileReq) (*dto.FileInfo, error) {
	var file model.File
	if err := global.DB.First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	canonicalType, ok := uploadsecurity.LookupTypeByMIME(file.ContentType)
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	definition, ok := uploadsecurity.DefinitionForType(canonicalType)
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeFileStateConflict, nil)
	}
	safeName, err := uploadsecurity.SanitizeManagedFileRename(
		req.Name,
		definition.CanonicalExtension,
	)
	if err != nil {
		return nil, err
	}
	if err := global.DB.Model(&file).Update("name", safeName).Error; err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	file.Name = safeName
	return toFileInfo(&file), nil
}

// DeleteFile 删除文件（MinIO + DB 双删）
func DeleteFile(fileID uint) error {
	var file model.File
	if err := global.DB.First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uploadsecurity.NewError(uploadsecurity.CodeFileNotFound, err)
		}
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	if err := utils.RemoveFile(file.Bucket, file.ObjectName); err != nil {
		return classifyStorageError(err)
	}
	if err := global.DB.Unscoped().Delete(&file).Error; err != nil {
		return uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	return nil
}

// BrowseFiles 按目录浏览 MinIO 中的文件
func BrowseFiles(prefix string) ([]dto.FileObjectInfo, error) {
	return NewFileBrowseService(objectstorage.NewMinIOStore(global.Minio)).
		List(context.Background(), prefix)
}

// toFileInfo 将文件模型转换为接口返回结构，并格式化创建时间。
func toFileInfo(f *model.File) *dto.FileInfo {
	validatedAt := ""
	if f.ValidatedAt != nil {
		validatedAt = f.ValidatedAt.Format("2006-01-02 15:04:05")
	}

	return &dto.FileInfo{
		ID:                      f.ID,
		Name:                    f.Name,
		ContentType:             f.ContentType,
		DetectedContentType:     f.DetectedContentType,
		Size:                    f.Size,
		UploaderID:              f.UploaderID,
		ValidationStatus:        f.ValidationStatus,
		ValidationPolicyVersion: f.ValidationPolicyVersion,
		ValidationErrorCode:     f.ValidationErrorCode,
		ValidatedAt:             validatedAt,
		CreatedAt:               f.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
