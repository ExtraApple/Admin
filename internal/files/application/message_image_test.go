package application

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"admin/internal/files/domain"
	"admin/internal/uploadsecurity"
)

type messageImageRepositoryFake struct {
	nextID uint
	files  map[uint]domain.File
}

type messageImageValidatorFake struct{}

func (messageImageValidatorFake) Validate(_ context.Context, input uploadsecurity.Input) (uploadsecurity.Result, error) {
	return uploadsecurity.Result{Purpose: uploadsecurity.PurposeMessageImage, FileName: "notice.png", CanonicalType: uploadsecurity.TypePNG, CanonicalExtension: ".png", CanonicalMIME: "image/png", DetectedMIME: "image/png", Size: input.Size, ContentSHA256: "digest", PolicyVersion: uploadsecurity.PolicyVersionV1, Reader: bytes.NewReader([]byte("validated image"))}, nil
}

func (repository *messageImageRepositoryFake) CreateMessageImage(_ context.Context, file *domain.File) error {
	if repository.nextID == 0 {
		repository.nextID = 1
	}
	file.ID = repository.nextID
	repository.nextID++
	if repository.files == nil {
		repository.files = make(map[uint]domain.File)
	}
	repository.files[file.ID] = *file
	return nil
}

func (repository *messageImageRepositoryFake) FindMessageImage(_ context.Context, id uint) (domain.File, error) {
	file, ok := repository.files[id]
	if !ok {
		return domain.File{}, ErrFileNotFound
	}
	return file, nil
}

func (repository *messageImageRepositoryFake) BindMessageImages(_ context.Context, request MessageImageBindRequest) error {
	for _, id := range request.ImageIDs {
		file, ok := repository.files[id]
		if !ok {
			return ErrFileNotFound
		}
		if file.Purpose != string(uploadsecurity.PurposeMessageImage) || file.UploaderID != request.ActorID || file.LogicalMessageID != "" || file.BindingExpiresAt == nil || !request.Now.Before(*file.BindingExpiresAt) {
			return ErrStateConflict
		}
		file.LogicalMessageID = request.MessageLogicalID
		repository.files[id] = file
	}
	return nil
}

func (repository *messageImageRepositoryFake) DeleteExpiredMessageImages(_ context.Context, now time.Time, _ int) ([]domain.File, error) {
	var expired []domain.File
	for id, file := range repository.files {
		if file.Purpose == string(uploadsecurity.PurposeMessageImage) && file.LogicalMessageID == "" && file.BindingExpiresAt != nil && !now.Before(*file.BindingExpiresAt) {
			expired = append(expired, file)
			delete(repository.files, id)
		}
	}
	return expired, nil
}

func TestMessageImageServiceUploadsBindsAndReadsOnlyBoundImage(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	repository := &messageImageRepositoryFake{}
	storage := newMemoryStorage()
	service := NewService(Dependencies{
		Repository:            newMemoryRepository(),
		Storage:               storage,
		MessageImages:         repository,
		MessageImageValidator: messageImageValidatorFake{},
		Clock:                 fixedClock{now: now},
	})
	content := []byte("validated image")
	image, err := service.UploadMessageImage(context.Background(), MessageImageUploadInput{UploaderID: 7, FileName: "notice.png", ContentType: "image/png", Size: int64(len(content)), Reader: strings.NewReader(string(content))})
	if err != nil {
		t.Fatalf("UploadMessageImage() = %v", err)
	}
	if image.ID == 0 || !image.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("temporary image = %#v", image)
	}
	if err := service.BindMessageImages(context.Background(), MessageImageBindRequest{ActorID: 7, MessageLogicalID: "logical-1", ImageIDs: []uint{image.ID}, Now: now}); err != nil {
		t.Fatalf("BindMessageImages() = %v", err)
	}
	opened, err := service.OpenMessageImage(context.Background(), MessageImageOpenRequest{ID: image.ID, MessageLogicalID: "logical-1"})
	if err != nil {
		t.Fatalf("OpenMessageImage() = %v", err)
	}
	data, readErr := io.ReadAll(opened.Reader)
	closeErr := opened.Reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(data, content) || opened.ContentType != "image/png" {
		t.Fatalf("opened image data=%q type=%q read=%v close=%v", data, opened.ContentType, readErr, closeErr)
	}
	if _, err := service.OpenMessageImage(context.Background(), MessageImageOpenRequest{ID: image.ID, MessageLogicalID: "other"}); uploadCode(err) != uploadsecurity.CodeFileAccessInvalid {
		t.Fatalf("wrong logical message error = %v", err)
	}
}

func TestMessageImageServiceCleansExpiredUnboundImages(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	repository := &messageImageRepositoryFake{}
	storage := newMemoryStorage()
	service := NewService(Dependencies{Repository: newMemoryRepository(), Storage: storage, MessageImages: repository, Clock: fixedClock{now: now}})
	repository.files = map[uint]domain.File{1: {ID: 1, Purpose: string(uploadsecurity.PurposeMessageImage), Bucket: "files", ObjectName: "message-images/expired.png", BindingExpiresAt: timePtr(now.Add(-time.Minute))}}
	storage.objects[storageKey("files", "message-images/expired.png")] = []byte("image")
	if err := service.CleanupExpiredMessageImages(context.Background(), now, 10); err != nil {
		t.Fatalf("CleanupExpiredMessageImages() = %v", err)
	}
	if _, ok := repository.files[1]; ok {
		t.Fatal("expired image record remains")
	}
	if _, ok := storage.objects[storageKey("files", "message-images/expired.png")]; ok {
		t.Fatal("expired image object remains")
	}
}

func timePtr(value time.Time) *time.Time { return &value }
