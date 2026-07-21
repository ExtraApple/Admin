package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"
	"admin/service/objectstorage"
	"admin/service/uploadsecurity"
)

const avatarBucket = "image"

type AvatarRepository interface {
	FindByID(ctx context.Context, userID uint) (*model.User, error)
	UpdateAvatar(
		ctx context.Context,
		userID uint,
		update AvatarUpdate,
	) (AvatarUpdateOutcome, error)
}

// AvatarUpdateOutcome tells the service whether compensating object deletion
// is safe after a persistence error.
type AvatarUpdateOutcome uint8

const (
	AvatarUpdateUnknown AvatarUpdateOutcome = iota
	AvatarUpdateNotCommitted
	AvatarUpdateCommitted
)

type AvatarUpdate struct {
	ObjectName       string
	ContentType      string
	ValidationStatus string
	ValidatedAt      *time.Time
}

type UploadAvatarInput struct {
	UserID      uint
	FileName    string
	ContentType string
	Size        int64
	MaxBytes    int64
	Reader      io.Reader
}

// UploadAvatarResult returns the updated user together with normalized,
// non-sensitive validation metadata needed by the HTTP audit layer.
type UploadAvatarResult struct {
	User          *model.User
	FileName      string
	FileSize      int64
	DetectedMIME  string
	PolicyVersion string
}

type AvatarService struct {
	validator uploadsecurity.Validator
	storage   objectstorage.Store
	users     AvatarRepository
	now       func() time.Time
}

type gormAvatarRepository struct {
	db *gorm.DB
}

func (r gormAvatarRepository) FindByID(
	ctx context.Context,
	userID uint,
) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r gormAvatarRepository) UpdateAvatar(
	ctx context.Context,
	userID uint,
	update AvatarUpdate,
) (AvatarUpdateOutcome, error) {
	result := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"avatar_object_name":       update.ObjectName,
			"avatar_content_type":      update.ContentType,
			"avatar_validation_status": update.ValidationStatus,
			"avatar_validated_at":      update.ValidatedAt,
		})
	if result.Error != nil {
		return AvatarUpdateUnknown, result.Error
	}
	if result.RowsAffected != 1 {
		return AvatarUpdateNotCommitted, gorm.ErrRecordNotFound
	}
	return AvatarUpdateCommitted, nil
}

func NewAvatarService(
	validator uploadsecurity.Validator,
	storage objectstorage.Store,
	users AvatarRepository,
	now func() time.Time,
) *AvatarService {
	if now == nil {
		now = time.Now
	}
	return &AvatarService{
		validator: validator,
		storage:   storage,
		users:     users,
		now:       now,
	}
}

func NewManagedAvatarService() *AvatarService {
	return NewAvatarService(
		uploadsecurity.NewAvatarValidator(),
		objectstorage.NewMinIOStore(global.Minio),
		gormAvatarRepository{db: global.DB},
		time.Now,
	)
}

func (s *AvatarService) Upload(
	ctx context.Context,
	input UploadAvatarInput,
) (*model.User, error) {
	result, err := s.UploadWithResult(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.User, nil
}

func (s *AvatarService) UploadWithResult(
	ctx context.Context,
	input UploadAvatarInput,
) (*UploadAvatarResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := s.validator.Validate(ctx, uploadsecurity.Input{
		Purpose:      uploadsecurity.PurposeAvatar,
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

	previousUser, err := s.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	if previousUser == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, nil)
	}
	oldObjectName, hasOldObject := validAvatarObjectName(previousUser, input.UserID)

	objectName, err := objectstorage.NewAvatarObjectName(input.UserID, result.CanonicalType)
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeInternalError, err)
	}
	if _, err := s.storage.Put(ctx, objectstorage.PutInput{
		Bucket:      avatarBucket,
		Name:        objectName,
		Reader:      result.Reader,
		Size:        result.Size,
		ContentType: result.CanonicalMIME,
	}); err != nil {
		return nil, classifyStorageError(err)
	}

	validatedAt := s.now().UTC()
	update := AvatarUpdate{
		ObjectName:       objectName,
		ContentType:      result.CanonicalMIME,
		ValidationStatus: model.FileValidationStatusValidated,
		ValidatedAt:      &validatedAt,
	}
	updateOutcome, err := s.users.UpdateAvatar(ctx, input.UserID, update)
	if err != nil || updateOutcome != AvatarUpdateCommitted {
		persistenceErr := uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
		global.Logger.Error(
			"avatar record update failed",
			zap.String("error_code", string(uploadsecurity.CodePersistenceFailed)),
		)
		if updateOutcome == AvatarUpdateNotCommitted {
			if deleteErr := s.storage.Delete(ctx, avatarBucket, objectName); deleteErr != nil {
				deleteErr = classifyStorageError(deleteErr)
				code, _ := uploadsecurity.CodeOf(deleteErr)
				global.Logger.Error(
					"avatar compensation delete failed",
					zap.String("error_code", string(code)),
				)
			}
		}
		return nil, persistenceErr
	}

	user := *previousUser
	applyAvatarUpdate(&user, update)
	if hasOldObject &&
		oldObjectName != objectName {
		if deleteErr := s.storage.Delete(ctx, avatarBucket, oldObjectName); deleteErr != nil {
			deleteErr = classifyStorageError(deleteErr)
			code, _ := uploadsecurity.CodeOf(deleteErr)
			global.Logger.Error(
				"old avatar cleanup failed",
				zap.String("error_code", string(code)),
			)
		}
	}
	return &UploadAvatarResult{
		User:          &user,
		FileName:      result.FileName,
		FileSize:      result.Size,
		DetectedMIME:  result.DetectedMIME,
		PolicyVersion: result.PolicyVersion,
	}, nil
}

func (s *AvatarService) RestoreDefault(
	ctx context.Context,
	userID uint,
) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	previousUser, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}
	if previousUser == nil {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, nil)
	}
	oldObjectName, hasOldObject := validAvatarObjectName(previousUser, userID)
	if !hasAvatarMetadata(previousUser) {
		user := *previousUser
		return &user, nil
	}

	update := AvatarUpdate{}
	updateOutcome, err := s.users.UpdateAvatar(ctx, userID, update)
	if err != nil || updateOutcome != AvatarUpdateCommitted {
		return nil, uploadsecurity.NewError(uploadsecurity.CodePersistenceFailed, err)
	}

	user := *previousUser
	applyAvatarUpdate(&user, update)
	if hasOldObject {
		if deleteErr := s.storage.Delete(ctx, avatarBucket, oldObjectName); deleteErr != nil {
			deleteErr = classifyStorageError(deleteErr)
			code, _ := uploadsecurity.CodeOf(deleteErr)
			global.Logger.Error(
				"old avatar cleanup failed",
				zap.String("error_code", string(code)),
			)
		}
	}
	return &user, nil
}

func applyAvatarUpdate(user *model.User, update AvatarUpdate) {
	if user == nil {
		return
	}
	user.AvatarObjectName = update.ObjectName
	user.AvatarContentType = update.ContentType
	user.AvatarValidationStatus = update.ValidationStatus
	user.AvatarValidatedAt = update.ValidatedAt
}

func hasAvatarMetadata(user *model.User) bool {
	return user != nil &&
		(user.AvatarObjectName != "" ||
			user.AvatarContentType != "" ||
			user.AvatarValidationStatus != "" ||
			user.AvatarValidatedAt != nil)
}

func validAvatarObjectName(user *model.User, userID uint) (string, bool) {
	if user == nil ||
		userID == 0 ||
		user.AvatarValidationStatus != model.FileValidationStatusValidated {
		return "", false
	}

	prefix := fmt.Sprintf("avatars/%d/", userID)
	if !strings.HasPrefix(user.AvatarObjectName, prefix) {
		return "", false
	}
	fileName := strings.TrimPrefix(user.AvatarObjectName, prefix)
	if fileName == "" || strings.Contains(fileName, "/") || strings.Contains(fileName, `\`) {
		return "", false
	}

	var expectedContentType string
	var idText string
	switch {
	case strings.HasSuffix(fileName, ".jpg"):
		expectedContentType = "image/jpeg"
		idText = strings.TrimSuffix(fileName, ".jpg")
	case strings.HasSuffix(fileName, ".png"):
		expectedContentType = "image/png"
		idText = strings.TrimSuffix(fileName, ".png")
	default:
		return "", false
	}
	if user.AvatarContentType != expectedContentType {
		return "", false
	}

	id, err := uuid.Parse(idText)
	if err != nil || id.String() != idText {
		return "", false
	}
	return user.AvatarObjectName, true
}
