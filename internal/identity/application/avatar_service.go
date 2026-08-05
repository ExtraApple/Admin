package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"admin/internal/identity/domain"

	"github.com/google/uuid"
)

const (
	AvatarBucket    = "image"
	AvatarValidated = "validated"

	AvatarCodeStorageUnavailable = "STORAGE_UNAVAILABLE"
	AvatarCodePersistenceFailed  = "PERSISTENCE_FAILED"
	AvatarCodeInternalError      = "INTERNAL_ERROR"
)

type AvatarError struct {
	Code  string
	cause error
}

func NewAvatarError(code string, cause error) *AvatarError {
	return &AvatarError{Code: code, cause: cause}
}
func (avatarError *AvatarError) Error() string {
	if avatarError == nil {
		return ""
	}
	return avatarError.Code
}
func (avatarError *AvatarError) Unwrap() error {
	if avatarError == nil {
		return nil
	}
	return avatarError.cause
}
func AvatarErrorCode(err error) (string, bool) {
	var target *AvatarError
	if !errors.As(err, &target) || target == nil {
		return "", false
	}
	return target.Code, true
}

type AvatarValidationInput struct {
	FileName     string
	DeclaredMIME string
	Size         int64
	MaxBytes     int64
	Reader       io.Reader
}

type AvatarValidationResult struct {
	FileName           string
	Size               int64
	DetectedMIME       string
	CanonicalMIME      string
	CanonicalExtension string
	ContentSHA256      string
	PolicyVersion      string
	Reader             io.Reader
}

type AvatarValidator interface {
	Validate(context.Context, AvatarValidationInput) (AvatarValidationResult, error)
}

type AvatarObject struct {
	Bucket      string
	Name        string
	Reader      io.Reader
	Size        int64
	ContentType string
}

type AvatarObjectInfo struct {
	Size        int64
	ContentType string
}

type AvatarStorage interface {
	Put(context.Context, AvatarObject) error
	Open(context.Context, string, string) (io.ReadCloser, error)
	Stat(context.Context, string, string) (AvatarObjectInfo, error)
	Delete(context.Context, string, string) error
}

type AvatarUpdate struct {
	ObjectName       string
	ContentType      string
	ContentSHA256    string
	ValidationStatus string
	ValidatedAt      *time.Time
}

type AvatarUpdateOutcome uint8

const (
	AvatarUpdateUnknown AvatarUpdateOutcome = iota
	AvatarUpdateNotCommitted
	AvatarUpdateCommitted
)

type AvatarRepository interface {
	FindByID(context.Context, uint) (domain.User, error)
	UpdateAvatar(context.Context, uint, AvatarUpdate) (AvatarUpdateOutcome, error)
}

type UploadAvatarInput struct {
	UserID      uint
	FileName    string
	ContentType string
	Size        int64
	MaxBytes    int64
	Reader      io.Reader
}

type UploadAvatarResult struct {
	User          domain.User
	FileName      string
	FileSize      int64
	DetectedMIME  string
	PolicyVersion string
}

type AvatarContent struct {
	ContentType string
	Reader      io.ReadCloser
}

type AvatarService struct {
	validator AvatarValidator
	storage   AvatarStorage
	users     AvatarRepository
	now       func() time.Time
}

func NewAvatarService(validator AvatarValidator, storage AvatarStorage, users AvatarRepository, now func() time.Time) *AvatarService {
	if now == nil {
		now = time.Now
	}
	return &AvatarService{validator: validator, storage: storage, users: users, now: now}
}

func (service *AvatarService) Upload(ctx context.Context, input UploadAvatarInput) (UploadAvatarResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	validated, err := service.validator.Validate(ctx, AvatarValidationInput{FileName: input.FileName, DeclaredMIME: input.ContentType, Size: input.Size, MaxBytes: input.MaxBytes, Reader: input.Reader})
	if err != nil {
		return UploadAvatarResult{}, err
	}
	if validated.Reader == nil {
		return UploadAvatarResult{}, NewAvatarError(AvatarCodeInternalError, nil)
	}
	if closer, ok := validated.Reader.(io.Closer); ok {
		defer closer.Close()
	}
	previous, err := service.users.FindByID(ctx, input.UserID)
	if err != nil {
		return UploadAvatarResult{}, NewAvatarError(AvatarCodePersistenceFailed, err)
	}
	oldObjectName, hasOldObject := TrustedAvatarObjectName(previous, input.UserID)
	objectName, err := newAvatarObjectName(input.UserID, validated.CanonicalExtension)
	if err != nil {
		return UploadAvatarResult{}, NewAvatarError(AvatarCodeInternalError, err)
	}
	if err := service.storage.Put(ctx, AvatarObject{Bucket: AvatarBucket, Name: objectName, Reader: validated.Reader, Size: validated.Size, ContentType: validated.CanonicalMIME}); err != nil {
		return UploadAvatarResult{}, ensureAvatarError(err, AvatarCodeStorageUnavailable)
	}
	validatedAt := service.now().UTC()
	update := AvatarUpdate{ObjectName: objectName, ContentType: validated.CanonicalMIME, ContentSHA256: validated.ContentSHA256, ValidationStatus: AvatarValidated, ValidatedAt: &validatedAt}
	outcome, err := service.users.UpdateAvatar(ctx, input.UserID, update)
	if err != nil || outcome != AvatarUpdateCommitted {
		if outcome == AvatarUpdateNotCommitted {
			_ = service.storage.Delete(ctx, AvatarBucket, objectName)
		}
		return UploadAvatarResult{}, NewAvatarError(AvatarCodePersistenceFailed, err)
	}
	applyAvatarUpdate(&previous, update)
	if hasOldObject && oldObjectName != objectName {
		_ = service.storage.Delete(ctx, AvatarBucket, oldObjectName)
	}
	return UploadAvatarResult{User: previous, FileName: validated.FileName, FileSize: validated.Size, DetectedMIME: validated.DetectedMIME, PolicyVersion: validated.PolicyVersion}, nil
}

func (service *AvatarService) RestoreDefault(ctx context.Context, userID uint) (domain.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	previous, err := service.users.FindByID(ctx, userID)
	if err != nil {
		return domain.User{}, NewAvatarError(AvatarCodePersistenceFailed, err)
	}
	oldObjectName, hasOldObject := TrustedAvatarObjectName(previous, userID)
	if !hasAvatarMetadata(previous) {
		return previous, nil
	}
	outcome, err := service.users.UpdateAvatar(ctx, userID, AvatarUpdate{})
	if err != nil || outcome != AvatarUpdateCommitted {
		return domain.User{}, NewAvatarError(AvatarCodePersistenceFailed, err)
	}
	applyAvatarUpdate(&previous, AvatarUpdate{})
	if hasOldObject {
		_ = service.storage.Delete(ctx, AvatarBucket, oldObjectName)
	}
	return previous, nil
}

func (service *AvatarService) Open(ctx context.Context, userID uint) AvatarContent {
	if service == nil || service.users == nil || service.storage == nil || userID == 0 {
		return DefaultAvatarContent()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	user, err := service.users.FindByID(ctx, userID)
	if err != nil {
		return DefaultAvatarContent()
	}
	objectName, trusted := TrustedAvatarObjectName(user, userID)
	if !trusted {
		return DefaultAvatarContent()
	}
	info, err := service.storage.Stat(ctx, AvatarBucket, objectName)
	if err != nil || info.Size < 1 || info.ContentType != user.AvatarContentType {
		return DefaultAvatarContent()
	}
	reader, err := service.storage.Open(ctx, AvatarBucket, objectName)
	if err != nil || reader == nil {
		return DefaultAvatarContent()
	}
	buffer := make([]byte, 1)
	count, readErr := reader.Read(buffer)
	if count == 0 {
		_ = reader.Close()
		return DefaultAvatarContent()
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = reader.Close()
		return DefaultAvatarContent()
	}
	return AvatarContent{ContentType: user.AvatarContentType, Reader: &prefetchedAvatarReader{Reader: io.MultiReader(bytes.NewReader(buffer[:count]), reader), closer: reader}}
}

func TrustedAvatarObjectName(user domain.User, userID uint) (string, bool) {
	if userID == 0 || user.AvatarValidationStatus != AvatarValidated {
		return "", false
	}
	prefix := fmt.Sprintf("avatars/%d/", userID)
	if !strings.HasPrefix(user.AvatarObjectName, prefix) {
		return "", false
	}
	fileName := strings.TrimPrefix(user.AvatarObjectName, prefix)
	if fileName == "" || strings.ContainsAny(fileName, `/\\`) {
		return "", false
	}
	extension := ""
	expectedMIME := ""
	switch {
	case strings.HasSuffix(fileName, ".jpg"):
		extension, expectedMIME = ".jpg", "image/jpeg"
	case strings.HasSuffix(fileName, ".png"):
		extension, expectedMIME = ".png", "image/png"
	default:
		return "", false
	}
	if user.AvatarContentType != expectedMIME {
		return "", false
	}
	idText := strings.TrimSuffix(fileName, extension)
	id, err := uuid.Parse(idText)
	return user.AvatarObjectName, err == nil && id.String() == idText
}

func newAvatarObjectName(userID uint, extension string) (string, error) {
	if userID == 0 || (extension != ".jpg" && extension != ".png") {
		return "", errors.New("unsupported normalized avatar extension")
	}
	return fmt.Sprintf("avatars/%d/%s%s", userID, uuid.NewString(), extension), nil
}

func hasAvatarMetadata(user domain.User) bool {
	return user.AvatarObjectName != "" || user.AvatarContentType != "" || user.AvatarContentSHA256 != "" || user.AvatarValidationStatus != "" || user.AvatarValidatedAt != nil
}
func applyAvatarUpdate(user *domain.User, update AvatarUpdate) {
	user.AvatarObjectName = update.ObjectName
	user.AvatarContentType = update.ContentType
	user.AvatarContentSHA256 = update.ContentSHA256
	user.AvatarValidationStatus = update.ValidationStatus
	user.AvatarValidatedAt = update.ValidatedAt
}
func ensureAvatarError(err error, fallback string) error {
	if _, ok := AvatarErrorCode(err); ok {
		return err
	}
	return NewAvatarError(fallback, err)
}

type prefetchedAvatarReader struct {
	io.Reader
	closer io.Closer
}

func (reader *prefetchedAvatarReader) Close() error { return reader.closer.Close() }

const defaultAvatarPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

var defaultAvatarPNG = func() []byte {
	content, err := base64.StdEncoding.DecodeString(defaultAvatarPNGBase64)
	if err != nil {
		panic(err)
	}
	return content
}()

func DefaultAvatarContent() AvatarContent {
	return AvatarContent{ContentType: "image/png", Reader: io.NopCloser(bytes.NewReader(defaultAvatarPNG))}
}
