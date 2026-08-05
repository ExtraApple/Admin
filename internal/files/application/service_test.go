package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"admin/internal/files/domain"
	"admin/internal/uploadsecurity"
)

type memoryRepository struct {
	nextID    uint
	files     map[uint]domain.File
	createErr error
	deleteErr error
	updateErr error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{nextID: 1, files: make(map[uint]domain.File)}
}

func (repository *memoryRepository) Create(_ context.Context, file *domain.File) error {
	if repository.createErr != nil {
		return repository.createErr
	}
	file.ID = repository.nextID
	repository.nextID++
	repository.files[file.ID] = *file
	return nil
}

func (repository *memoryRepository) FindByID(_ context.Context, id uint) (domain.File, error) {
	file, ok := repository.files[id]
	if !ok {
		return domain.File{}, ErrFileNotFound
	}
	return file, nil
}

func (repository *memoryRepository) List(_ context.Context, page, size int, prefix string) ([]domain.File, int64, error) {
	files := make([]domain.File, 0, len(repository.files))
	for _, file := range repository.files {
		if prefix == "" || strings.HasPrefix(file.ObjectName, prefix) {
			files = append(files, file)
		}
	}
	sort.Slice(files, func(left, right int) bool { return files[left].ID > files[right].ID })
	total := int64(len(files))
	start := (page - 1) * size
	if start >= len(files) {
		return nil, total, nil
	}
	end := start + size
	if end > len(files) {
		end = len(files)
	}
	return files[start:end], total, nil
}

func (repository *memoryRepository) UpdateName(_ context.Context, id uint, name string) error {
	if repository.updateErr != nil {
		return repository.updateErr
	}
	file, ok := repository.files[id]
	if !ok {
		return ErrFileNotFound
	}
	file.Name = name
	repository.files[id] = file
	return nil
}

func (repository *memoryRepository) Delete(_ context.Context, id uint) error {
	if repository.deleteErr != nil {
		return repository.deleteErr
	}
	delete(repository.files, id)
	return nil
}

func (repository *memoryRepository) UpdateValidation(_ context.Context, id uint, update ValidationUpdate) error {
	if repository.updateErr != nil {
		return repository.updateErr
	}
	file, ok := repository.files[id]
	if !ok {
		return ErrFileNotFound
	}
	applyValidationUpdate(&file, update)
	repository.files[id] = file
	return nil
}

func (repository *memoryRepository) FindRotationCandidates(_ context.Context, cutoff time.Time, bucket string, limit int) ([]domain.File, error) {
	result := make([]domain.File, 0, limit)
	for _, file := range repository.files {
		if file.Bucket == bucket && file.CreatedAt.Before(cutoff) {
			result = append(result, file)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (repository *memoryRepository) UpdateBucket(_ context.Context, id uint, bucket string) error {
	if repository.updateErr != nil {
		return repository.updateErr
	}
	file, ok := repository.files[id]
	if !ok {
		return ErrFileNotFound
	}
	file.Bucket = bucket
	repository.files[id] = file
	return nil
}

type memoryStorage struct {
	objects     map[string][]byte
	putErr      error
	deleteCalls int
	moveCalls   [][3]string
}

func newMemoryStorage() *memoryStorage      { return &memoryStorage{objects: make(map[string][]byte)} }
func storageKey(bucket, name string) string { return bucket + "/" + name }

func (storage *memoryStorage) Put(_ context.Context, input ObjectInput) error {
	if storage.putErr != nil {
		return storage.putErr
	}
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return err
	}
	storage.objects[storageKey(input.Bucket, input.Name)] = content
	return nil
}

func (storage *memoryStorage) Open(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	content, ok := storage.objects[storageKey(bucket, name)]
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil)
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (storage *memoryStorage) Delete(_ context.Context, bucket, name string) error {
	storage.deleteCalls++
	delete(storage.objects, storageKey(bucket, name))
	return nil
}

func (storage *memoryStorage) List(_ context.Context, bucket string, options ListOptions) ([]Object, error) {
	result := make([]Object, 0)
	prefix := storageKey(bucket, options.Prefix)
	for key, content := range storage.objects {
		if strings.HasPrefix(key, prefix) {
			result = append(result, Object{Name: strings.TrimPrefix(key, bucket+"/"), Size: int64(len(content)), ContentType: "application/pdf"})
		}
	}
	return result, nil
}

func (storage *memoryStorage) Move(_ context.Context, sourceBucket, targetBucket, name string) error {
	storage.moveCalls = append(storage.moveCalls, [3]string{sourceBucket, targetBucket, name})
	source := storageKey(sourceBucket, name)
	content, ok := storage.objects[source]
	if !ok {
		return errors.New("source object is missing")
	}
	storage.objects[storageKey(targetBucket, name)] = content
	delete(storage.objects, source)
	return nil
}

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type fixedObjectNames struct{}

func (fixedObjectNames) ManagedFileName(uploadsecurity.CanonicalType, string) (string, error) {
	return "managed.pdf", nil
}

func newLifecycleService(t *testing.T, repository Repository, storage ObjectStorage, now time.Time) *Service {
	t.Helper()
	signer, err := NewHMACSigner([]byte("files-application-test-signing-key"))
	if err != nil {
		t.Fatalf("create file signer: %v", err)
	}
	return NewService(Dependencies{
		Repository:               repository,
		Storage:                  storage,
		Validator:                uploadsecurity.NewManagedFileValidator(),
		Signer:                   signer,
		Clock:                    fixedClock{now: now},
		ObjectNames:              fixedObjectNames{},
		DownloadURLExpireSeconds: 300,
	})
}

func TestServicePreservesManagedFileLifecycleContracts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	storage := newMemoryStorage()
	service := newLifecycleService(t, repository, storage, now)
	content := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\r\n")

	uploaded, err := service.Upload(ctx, UploadInput{UploaderID: 7, FileName: "report.pdf", ContentType: "application/pdf", Size: int64(len(content)), MaxBytes: 1024, Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatalf("upload managed file: %v", err)
	}
	if uploaded.ID != 1 || uploaded.Name != "report.pdf" || uploaded.ValidationStatus != domain.ValidationStatusValidated || len(uploaded.ContentSHA256) != 64 {
		t.Fatalf("uploaded file = %#v", uploaded)
	}
	if got := storage.objects[storageKey("files", "managed.pdf")]; !bytes.Equal(got, content) {
		t.Fatalf("stored content = %q", got)
	}

	list, total, err := service.List(ctx, 7, 1, 10, "managed")
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != uploaded.ID {
		t.Fatalf("list = %#v total=%d err=%v", list, total, err)
	}
	detail, err := service.Get(ctx, 7, uploaded.ID)
	if err != nil {
		t.Fatalf("get file detail: %v", err)
	}
	accessURL, err := url.Parse(detail.DownloadURL)
	if err != nil || accessURL.Path != "/api/admin/files/1/download" || accessURL.Query().Get("signature") == "" {
		t.Fatalf("download URL = %q err=%v", detail.DownloadURL, err)
	}

	expires, err := parseExpiry(accessURL.Query().Get("expires"))
	if err != nil {
		t.Fatalf("parse expiry: %v", err)
	}
	download, err := service.Open(ctx, FileAccessInput{UserID: 7, FileID: uploaded.ID, ExpiresAt: expires, Signature: accessURL.Query().Get("signature"), Mode: ModeDownload})
	if err != nil {
		t.Fatalf("open download: %v", err)
	}
	downloaded, readErr := io.ReadAll(download.Reader)
	closeErr := download.Reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(downloaded, content) || download.Disposition != DispositionAttachment {
		t.Fatalf("download content=%q readErr=%v closeErr=%v disposition=%q", downloaded, readErr, closeErr, download.Disposition)
	}
	if _, err := service.Open(ctx, FileAccessInput{UserID: 7, FileID: uploaded.ID, Mode: ModePreview}); uploadCode(err) != uploadsecurity.CodeFileStateConflict {
		t.Fatalf("preview error = %v", err)
	}

	updated, err := service.Update(ctx, 7, uploaded.ID, UpdateFileRequest{Name: "quarterly"})
	if err != nil || updated.Name != "quarterly.pdf" {
		t.Fatalf("update = %#v err=%v", updated, err)
	}
	file := repository.files[uploaded.ID]
	file.ValidationStatus = domain.ValidationStatusLegacyUnverified
	file.CreatedAt = now.AddDate(0, 0, -60)
	repository.files[uploaded.ID] = file
	revalidated, err := service.Revalidate(ctx, 7, uploaded.ID, 1024)
	if err != nil || revalidated.ValidationStatus != domain.ValidationStatusValidated {
		t.Fatalf("revalidate = %#v err=%v", revalidated, err)
	}

	objects, err := service.Browse(ctx, 7, "managed")
	if err != nil || len(objects) != 1 || objects[0].Name != "managed.pdf" {
		t.Fatalf("browse = %#v err=%v", objects, err)
	}
	rotation, err := service.Rotate(ctx, RotationConfig{Enabled: true, Days: 30, HotBucket: "files", ColdBucket: "files-archive", BatchSize: 10})
	if err != nil || rotation.Examined != 1 || rotation.Moved != 1 || len(rotation.Failures) != 0 {
		t.Fatalf("rotation = %#v err=%v", rotation, err)
	}
	if _, ok := storage.objects[storageKey("files-archive", "managed.pdf")]; !ok || repository.files[uploaded.ID].Bucket != "files-archive" {
		t.Fatal("rotation did not move object and persistence record together")
	}
	if err := service.Delete(ctx, 7, uploaded.ID); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if len(repository.files) != 0 || len(storage.objects) != 0 {
		t.Fatalf("delete left repository=%#v storage=%#v", repository.files, storage.objects)
	}
}

func TestUploadCompensatesObjectWhenPersistenceFails(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.createErr = errors.New("database unavailable")
	storage := newMemoryStorage()
	service := newLifecycleService(t, repository, storage, now)
	content := []byte("plain UTF-8 文本\n")

	_, err := service.Upload(context.Background(), UploadInput{UploaderID: 7, FileName: "notes.txt", ContentType: "text/plain", Size: int64(len(content)), MaxBytes: 1024, Reader: bytes.NewReader(content)})
	if uploadCode(err) != uploadsecurity.CodePersistenceFailed {
		t.Fatalf("upload error = %v", err)
	}
	if storage.deleteCalls != 1 || len(storage.objects) != 0 || len(repository.files) != 0 {
		t.Fatalf("compensation deleteCalls=%d storage=%#v repository=%#v", storage.deleteCalls, storage.objects, repository.files)
	}
}

func uploadCode(err error) uploadsecurity.Code {
	code, _ := uploadsecurity.CodeOf(err)
	return code
}

func parseExpiry(value string) (int64, error) {
	var expires int64
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, errors.New("expiry is not numeric")
		}
		expires = expires*10 + int64(digit-'0')
	}
	return expires, nil
}
