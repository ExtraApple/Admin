package application_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
)

type avatarRepositoryFake struct {
	user   domain.User
	update application.AvatarUpdate
}

func (fake *avatarRepositoryFake) FindByID(context.Context, uint) (domain.User, error) {
	return fake.user, nil
}
func (fake *avatarRepositoryFake) UpdateAvatar(_ context.Context, _ uint, update application.AvatarUpdate) (application.AvatarUpdateOutcome, error) {
	fake.update = update
	return application.AvatarUpdateCommitted, nil
}

type avatarValidatorFake struct{}

func (avatarValidatorFake) Validate(context.Context, application.AvatarValidationInput) (application.AvatarValidationResult, error) {
	return application.AvatarValidationResult{FileName: "face.png", Size: 10, DetectedMIME: "image/png", CanonicalMIME: "image/png", CanonicalExtension: ".png", ContentSHA256: "sha", PolicyVersion: "avatar-v1", Reader: strings.NewReader("normalized")}, nil
}

type avatarStorageFake struct {
	put     application.AvatarObject
	deleted []string
}

func (fake *avatarStorageFake) Put(_ context.Context, object application.AvatarObject) error {
	fake.put = object
	return nil
}
func (fake *avatarStorageFake) Delete(_ context.Context, _, name string) error {
	fake.deleted = append(fake.deleted, name)
	return nil
}
func (*avatarStorageFake) Stat(context.Context, string, string) (application.AvatarObjectInfo, error) {
	return application.AvatarObjectInfo{Size: 10, ContentType: "image/png"}, nil
}
func (*avatarStorageFake) Open(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte("avatar"))), nil
}

func TestAvatarServiceUploadsValidatedContentAndPersistsTrustedMetadata(t *testing.T) {
	repository := &avatarRepositoryFake{user: domain.User{ID: 7}}
	storage := &avatarStorageFake{}
	service := application.NewAvatarService(avatarValidatorFake{}, storage, repository, nil)
	result, err := service.Upload(context.Background(), application.UploadAvatarInput{UserID: 7, FileName: "face.png", ContentType: "image/png", Size: 10, MaxBytes: 1024, Reader: strings.NewReader("untrusted")})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if result.User.ID != 7 || !strings.HasPrefix(repository.update.ObjectName, "avatars/7/") || repository.update.ContentType != "image/png" || repository.update.ValidationStatus != application.AvatarValidated || storage.put.Name != repository.update.ObjectName {
		t.Fatalf("upload result=%#v update=%#v object=%#v", result, repository.update, storage.put)
	}
}
