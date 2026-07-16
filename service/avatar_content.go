package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"

	"admin/global"
	"admin/model"
	"admin/service/objectstorage"
)

const defaultAvatarPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

var defaultAvatarPNG = func() []byte {
	content, err := base64.StdEncoding.DecodeString(defaultAvatarPNGBase64)
	if err != nil {
		panic("decode built-in default avatar: " + err.Error())
	}
	return content
}()

type AvatarContentRepository interface {
	FindByID(ctx context.Context, userID uint) (*model.User, error)
}

type AvatarContentStore interface {
	Stat(ctx context.Context, bucket, name string) (objectstorage.ObjectInfo, error)
	Open(ctx context.Context, bucket, name string) (io.ReadCloser, error)
}

// AvatarContent contains only the safe response information needed by the
// public avatar handler. Storage object names remain private to this service.
type AvatarContent struct {
	ContentType string
	Reader      io.ReadCloser
}

type AvatarContentService struct {
	users   AvatarContentRepository
	storage AvatarContentStore
}

func NewAvatarContentService(
	users AvatarContentRepository,
	storage AvatarContentStore,
) *AvatarContentService {
	return &AvatarContentService{
		users:   users,
		storage: storage,
	}
}

func NewManagedAvatarContentService() *AvatarContentService {
	return NewAvatarContentService(
		gormAvatarRepository{db: global.DB},
		objectstorage.NewMinIOStore(global.Minio),
	)
}

// Open returns a trusted normalized avatar or the built-in default PNG. All
// lookup and storage failures intentionally collapse to the same default
// response so callers cannot infer user or object-storage state.
func (s *AvatarContentService) Open(
	ctx context.Context,
	userID uint,
) AvatarContent {
	if s == nil || s.users == nil || s.storage == nil || userID == 0 {
		return DefaultAvatarContent()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return DefaultAvatarContent()
	}
	objectName, trusted := trustedAvatarObjectName(user, userID)
	if !trusted {
		return DefaultAvatarContent()
	}

	info, err := s.storage.Stat(ctx, avatarBucket, objectName)
	if err != nil ||
		info.Size < 1 ||
		info.ContentType != user.AvatarContentType {
		return DefaultAvatarContent()
	}
	reader, err := s.storage.Open(ctx, avatarBucket, objectName)
	if err != nil || reader == nil {
		return DefaultAvatarContent()
	}
	reader, err = prefetchReadCloser(reader, rejectEmptyStream)
	if err != nil {
		return DefaultAvatarContent()
	}
	return AvatarContent{
		ContentType: user.AvatarContentType,
		Reader:      reader,
	}
}

// DefaultAvatarContent returns a fresh reader over the immutable application
// default image.
func DefaultAvatarContent() AvatarContent {
	return AvatarContent{
		ContentType: "image/png",
		Reader:      io.NopCloser(bytes.NewReader(defaultAvatarPNG)),
	}
}
