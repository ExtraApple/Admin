package service

import (
	"context"
	"errors"
	"image"
	"io"
	"strings"
	"testing"

	"admin/model"
	"admin/service/objectstorage"
)

type recordingAvatarContentRepository struct {
	userID uint
	user   *model.User
	err    error
}

func (r *recordingAvatarContentRepository) FindByID(
	_ context.Context,
	userID uint,
) (*model.User, error) {
	r.userID = userID
	return r.user, r.err
}

type recordingAvatarContentStore struct {
	statCalls   int
	statBucket  string
	statName    string
	statResult  objectstorage.ObjectInfo
	statErr     error
	openCalls   int
	openBucket  string
	openName    string
	openContent string
	openReader  io.ReadCloser
	openErr     error
}

func (s *recordingAvatarContentStore) Stat(
	_ context.Context,
	bucket, name string,
) (objectstorage.ObjectInfo, error) {
	s.statCalls++
	s.statBucket = bucket
	s.statName = name
	return s.statResult, s.statErr
}

func (s *recordingAvatarContentStore) Open(
	_ context.Context,
	bucket, name string,
) (io.ReadCloser, error) {
	s.openCalls++
	s.openBucket = bucket
	s.openName = name
	if s.openErr != nil {
		return nil, s.openErr
	}
	if s.openReader != nil {
		return s.openReader, nil
	}
	return io.NopCloser(strings.NewReader(s.openContent)), nil
}

func TestAvatarContentServiceOpensOnlyTrustedNormalizedAvatar(t *testing.T) {
	const objectName = "avatars/42/00000000-0000-0000-0000-000000000001.jpg"
	repository := &recordingAvatarContentRepository{
		user: &model.User{
			AvatarObjectName:       objectName,
			AvatarContentType:      "image/jpeg",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	repository.user.ID = 42
	store := &recordingAvatarContentStore{
		statResult: objectstorage.ObjectInfo{
			Bucket:      avatarBucket,
			Name:        objectName,
			Size:        int64(len("normalized-jpeg")),
			ContentType: "image/jpeg",
		},
		openContent: "normalized-jpeg",
	}
	avatars := NewAvatarContentService(repository, store)

	content := avatars.Open(context.Background(), 42)

	if repository.userID != 42 {
		t.Fatalf("FindByID() userID = %d, want 42", repository.userID)
	}
	if store.statCalls != 1 ||
		store.statBucket != avatarBucket ||
		store.statName != objectName {
		t.Fatalf("Stat() = %d calls for %q/%q, want trusted avatar object",
			store.statCalls, store.statBucket, store.statName)
	}
	if store.openCalls != 1 ||
		store.openBucket != avatarBucket ||
		store.openName != objectName {
		t.Fatalf("Open() = %d calls for %q/%q, want trusted avatar object",
			store.openCalls, store.openBucket, store.openName)
	}
	if content.ContentType != "image/jpeg" || content.Reader == nil {
		t.Fatalf("content = %#v, want normalized JPEG reader", content)
	}
	defer content.Reader.Close()
	body, err := io.ReadAll(content.Reader)
	if err != nil {
		t.Fatalf("read avatar content: %v", err)
	}
	if string(body) != "normalized-jpeg" {
		t.Fatalf("body = %q, want normalized-jpeg", body)
	}
}

func TestAvatarContentServiceFallsBackWhenFirstByteHasStorageError(t *testing.T) {
	const objectName = "avatars/42/00000000-0000-0000-0000-000000000001.png"
	repository := &recordingAvatarContentRepository{
		user: &model.User{
			AvatarObjectName:       objectName,
			AvatarContentType:      "image/png",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	repository.user.ID = 42
	reader := &firstReadDataErrorCloser{
		err: errors.New("provider failed after returning first avatar byte"),
	}
	store := &recordingAvatarContentStore{
		statResult: objectstorage.ObjectInfo{
			Size:        12,
			ContentType: "image/png",
		},
		openReader: reader,
	}
	avatars := NewAvatarContentService(repository, store)

	content := avatars.Open(context.Background(), 42)

	defer content.Reader.Close()
	_, format, err := image.Decode(content.Reader)
	if err != nil || format != "png" {
		t.Fatalf(
			"stable repro: avatar first Read() returned data with an error, "+
				"but service did not return the built-in PNG: format=%q error=%v",
			format,
			err,
		)
	}
	if !reader.closed {
		t.Fatal("avatar object reader was not closed after first-byte storage failure")
	}
}

func TestAvatarContentServiceFallsBackToSameBuiltInPNG(t *testing.T) {
	const objectName = "avatars/42/00000000-0000-0000-0000-000000000001.png"

	tests := []struct {
		name       string
		repository *recordingAvatarContentRepository
		store      *recordingAvatarContentStore
	}{
		{
			name: "user lookup fails",
			repository: &recordingAvatarContentRepository{
				err: errors.New("database internal detail"),
			},
			store: &recordingAvatarContentStore{},
		},
		{
			name: "historical URL is not trusted",
			repository: &recordingAvatarContentRepository{
				user: &model.User{
					Avatar: "https://legacy.example/private-avatar.png",
				},
			},
			store: &recordingAvatarContentStore{},
		},
		{
			name: "object metadata is unavailable",
			repository: &recordingAvatarContentRepository{
				user: &model.User{
					AvatarObjectName:       objectName,
					AvatarContentType:      "image/png",
					AvatarValidationStatus: model.FileValidationStatusValidated,
				},
			},
			store: &recordingAvatarContentStore{
				statErr: errors.New("storage credential detail"),
			},
		},
		{
			name: "object metadata MIME does not match",
			repository: &recordingAvatarContentRepository{
				user: &model.User{
					AvatarObjectName:       objectName,
					AvatarContentType:      "image/png",
					AvatarValidationStatus: model.FileValidationStatusValidated,
				},
			},
			store: &recordingAvatarContentStore{
				statResult: objectstorage.ObjectInfo{
					Size:        12,
					ContentType: "image/jpeg",
				},
			},
		},
		{
			name: "object cannot be opened",
			repository: &recordingAvatarContentRepository{
				user: &model.User{
					AvatarObjectName:       objectName,
					AvatarContentType:      "image/png",
					AvatarValidationStatus: model.FileValidationStatusValidated,
				},
			},
			store: &recordingAvatarContentStore{
				statResult: objectstorage.ObjectInfo{
					Size:        12,
					ContentType: "image/png",
				},
				openErr: errors.New("object not found detail"),
			},
		},
		{
			name: "object stream is empty",
			repository: &recordingAvatarContentRepository{
				user: &model.User{
					AvatarObjectName:       objectName,
					AvatarContentType:      "image/png",
					AvatarValidationStatus: model.FileValidationStatusValidated,
				},
			},
			store: &recordingAvatarContentStore{
				statResult: objectstorage.ObjectInfo{
					Size:        12,
					ContentType: "image/png",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.repository.user != nil {
				tt.repository.user.ID = 42
			}
			avatars := NewAvatarContentService(tt.repository, tt.store)

			content := avatars.Open(context.Background(), 42)

			if content.ContentType != "image/png" || content.Reader == nil {
				t.Fatalf("fallback content = %#v, want built-in PNG", content)
			}
			defer content.Reader.Close()
			_, format, err := image.Decode(content.Reader)
			if err != nil {
				t.Fatalf("decode built-in default avatar: %v", err)
			}
			if format != "png" {
				t.Fatalf("default avatar format = %q, want png", format)
			}
			if tt.repository.user == nil ||
				tt.repository.user.AvatarValidationStatus != model.FileValidationStatusValidated {
				if tt.store.statCalls != 0 || tt.store.openCalls != 0 {
					t.Fatalf("untrusted avatar accessed storage: stat=%d open=%d",
						tt.store.statCalls, tt.store.openCalls)
				}
			}
			if tt.store.statErr != nil && tt.store.openCalls != 0 {
				t.Fatalf("Open() calls = %d after failed Stat(), want 0", tt.store.openCalls)
			}
		})
	}
}
