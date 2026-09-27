package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"admin/internal/files"
	adapter "admin/internal/files/adapters/gorm"
	"admin/internal/files/application"
	"admin/internal/files/domain"
	database "admin/internal/platform/database"
	"admin/internal/uploadsecurity"
	"admin/testsupport/testutil"

	"github.com/google/uuid"
)

// Only the external object store is replaced; repository and transactions are real.
type imageTestStorage struct {
	objects   map[string][]byte
	deleteErr error
}

func (s *imageTestStorage) Stat(_ context.Context, bucket, name string) (application.Object, error) {
	data, ok := s.objects[bucket+"/"+name]
	if !ok {
		return application.Object{}, uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil)
	}
	return application.Object{Name: name, Size: int64(len(data))}, nil
}

func (s *imageTestStorage) Put(_ context.Context, in application.ObjectInput) error {
	data, err := io.ReadAll(in.Reader)
	if err != nil {
		return err
	}
	s.objects[in.Bucket+"/"+in.Name] = data
	return nil
}
func (s *imageTestStorage) Open(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	data, ok := s.objects[bucket+"/"+name]
	if !ok {
		return nil, uploadsecurity.NewError(uploadsecurity.CodeStorageObjectNotFound, nil)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *imageTestStorage) Delete(_ context.Context, bucket, name string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.objects, bucket+"/"+name)
	return nil
}
func (s *imageTestStorage) List(_ context.Context, bucket string, options application.ListOptions) ([]application.Object, error) {
	var objects []application.Object
	for key, data := range s.objects {
		if strings.HasPrefix(key, bucket+"/"+options.Prefix) {
			objects = append(objects, application.Object{Name: strings.TrimPrefix(key, bucket+"/"), Size: int64(len(data))})
		}
	}
	return objects, nil
}
func (s *imageTestStorage) Move(_ context.Context, from, to, name string) error {
	data, ok := s.objects[from+"/"+name]
	if !ok {
		return errors.New("source missing")
	}
	s.objects[to+"/"+name] = data
	delete(s.objects, from+"/"+name)
	return nil
}

type imageTestValidator struct{}

func (imageTestValidator) Validate(_ context.Context, input uploadsecurity.Input) (uploadsecurity.Result, error) {
	return uploadsecurity.Result{Purpose: uploadsecurity.PurposeMessageImage, FileName: "notice.png", CanonicalType: uploadsecurity.TypePNG, CanonicalExtension: ".png", CanonicalMIME: "image/png", DetectedMIME: "image/png", Size: input.Size, ContentSHA256: "digest", PolicyVersion: uploadsecurity.PolicyVersionV1, Reader: bytes.NewReader([]byte("validated image"))}, nil
}

func TestMessageImageServiceUploadsBindsAndReadsOnlyBoundImage(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	db.Config.NowFunc = func() time.Time { return now }
	repository := adapter.NewRepository(db)
	storage := &imageTestStorage{objects: map[string][]byte{}}
	service := application.NewService(application.Dependencies{Repository: repository, MessageImages: repository, Storage: storage, MessageImageValidator: imageTestValidator{}, Transactions: database.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now })})
	content := []byte("validated image")
	image, err := service.UploadMessageImage(context.Background(), application.MessageImageUploadInput{UploaderID: 7, FileName: "notice.png", ContentType: "image/png", Size: int64(len(content)), Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatal(err)
	}
	if image.ID == 0 || !image.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("temporary image: %+v", image)
	}
	if err := service.BindMessageImages(context.Background(), application.MessageImageBindRequest{ActorID: 7, MessageLogicalID: "logical-1", ImageIDs: []uint{image.ID}, Now: now}); err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenMessageImage(context.Background(), application.MessageImageOpenRequest{ID: image.ID, MessageLogicalID: "logical-1"})
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(opened.Reader)
	closeErr := opened.Reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(data, content) || opened.ContentType != "image/png" {
		t.Fatalf("read image: %q %v %v", data, readErr, closeErr)
	}
	_, err = service.OpenMessageImage(context.Background(), application.MessageImageOpenRequest{ID: image.ID, MessageLogicalID: "other"})
	if code, _ := uploadsecurity.CodeOf(err); code != uploadsecurity.CodeFileAccessInvalid {
		t.Fatalf("wrong logical message: %v", err)
	}
}

func TestMessageImageServiceCleansExpiredUnboundImages(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	repository := adapter.NewRepository(db)
	name := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"files/" + name: []byte("image")}}
	service := application.NewService(application.Dependencies{Repository: repository, MessageImages: repository, Storage: storage, Transactions: database.NewTransactionRunner(db)})
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: name, BindingExpiresAt: &expired}
	if err := repository.CreateMessageImage(context.Background(), &file); err != nil {
		t.Fatal(err)
	}
	if result := service.CleanupMessageImage(context.Background(), file.ID, now, application.RotationConfig{}); result.Status != "succeeded" {
		t.Fatalf("cleanup: %+v", result)
	}
	if _, err := repository.FindMessageImage(context.Background(), file.ID); !errors.Is(err, application.ErrFileNotFound) {
		t.Fatalf("expired record still accessible: %v", err)
	}
	if _, err := storage.Open(context.Background(), file.Bucket, file.ObjectName); err == nil {
		t.Fatal("expired object remains")
	}
}

func TestMessageImageCleanupRetainsLocationAfterStorageFailure(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	r := adapter.NewRepository(db)
	name := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"files/" + name: []byte("image")}, deleteErr: errors.New("store unavailable")}
	s := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storage, Transactions: database.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now })})
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: name, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(context.Background(), &file); err != nil {
		t.Fatal(err)
	}
	result := s.CleanupMessageImage(context.Background(), file.ID, now, application.RotationConfig{})
	if result.Status != "failed" {
		t.Fatalf("result: %+v", result)
	}
	jobs, err := r.FindMessageImageCleanupRetries(context.Background(), now.Add(time.Hour), application.CleanupCursor{UpperID: result.CleanupJobID}, 1)
	if err != nil || len(jobs) != 1 || jobs[0].ObjectName != name || jobs[0].RetryCount != 0 {
		t.Fatalf("lost recovery location: %+v %v", jobs, err)
	}
	storage.deleteErr = nil
	now = now.Add(time.Hour)
	result = s.RetryMessageImageCleanup(context.Background(), jobs[0], now, application.RotationConfig{})
	if result.Status != "succeeded" {
		t.Fatalf("retry: %+v", result)
	}
	if _, err := storage.Open(context.Background(), "files", name); err == nil {
		t.Fatal("object remains")
	}
}

func TestMessageImageCleanupDeletesOnlyTrustedConfiguredLocations(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	name := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"old/" + name: []byte("old"), "hot/" + name: []byte("hot"), "cold/" + name: []byte("cold"), "unrelated/" + name: []byte("keep")}}
	s := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storage})
	file := domain.File{Purpose: "message_image", Bucket: "old", ObjectName: name, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	result := s.CleanupMessageImage(ctx, file.ID, now, application.RotationConfig{Enabled: true, HotBucket: "hot", ColdBucket: "cold"})
	if result.Status != "succeeded" {
		t.Fatalf("cleanup: %+v", result)
	}
	for _, bucket := range []string{"old", "hot", "cold"} {
		if _, err := storage.Open(ctx, bucket, name); err == nil {
			t.Fatalf("%s copy remains", bucket)
		}
	}
	if _, err := storage.Open(ctx, "unrelated", name); err != nil {
		t.Fatal("unrelated bucket touched")
	}
}

func TestMessageImageCleanupRejectsConflictingFileOwnership(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	name := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"files/" + name: []byte("retain")}}
	s := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storage})
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: name, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	other := file
	other.ID = 0
	other.LogicalMessageID = "bound"
	if err := r.CreateMessageImage(ctx, &other); err != nil {
		t.Fatal(err)
	}
	result := s.CleanupMessageImage(ctx, file.ID, now, application.RotationConfig{})
	if result.Status != "failed" || result.FailureCode != "message_image_cleanup_scope_untrusted" {
		t.Fatalf("conflicting ownership: %+v", result)
	}
	if _, err := storage.Open(ctx, "files", name); err != nil {
		t.Fatal("conflicting object deleted")
	}
}

func TestMessageImageCleanupDisabledRotationLeavesColdCopy(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	name := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"files/" + name: []byte("old"), "cold/" + name: []byte("keep")}}
	s := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storage})
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: name, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	if result := s.CleanupMessageImage(ctx, file.ID, now, application.RotationConfig{ColdBucket: "cold"}); result.Status != "succeeded" {
		t.Fatalf("cleanup: %+v", result)
	}
	if _, err := storage.Open(ctx, "cold", name); err != nil {
		t.Fatal("disabled cold location touched")
	}
	file.ID = 0
	file.ObjectName = "message-images/not-a-generated-uuid.png"
	storage.objects["files/"+file.ObjectName] = []byte("keep")
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	result := s.CleanupMessageImage(ctx, file.ID, now, application.RotationConfig{})
	if result.FailureCode != "message_image_cleanup_scope_untrusted" {
		t.Fatalf("untrusted key: %+v", result)
	}
	if _, err := storage.Open(ctx, "files", file.ObjectName); err != nil {
		t.Fatal("untrusted key deleted")
	}
}

func TestMessageImageCleanupBatchContinuesAfterUnsafeObject(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	valid := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"files/unsafe.png": []byte("keep"), "files/" + valid: []byte("delete")}}
	s := application.NewService(application.Dependencies{Repository: r, MessageImages: r, Storage: storage})
	for _, name := range []string{"unsafe.png", valid} {
		file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: name, BindingExpiresAt: &expired}
		if err := r.CreateMessageImage(ctx, &file); err != nil {
			t.Fatal(err)
		}
	}
	state := application.MessageImageCleanupScan{}
	result, err := s.CleanupMessageImages(ctx, now, 2, &state, application.RotationConfig{})
	if err != nil || result.Processed != 2 || result.Failed != 1 || result.Succeeded != 1 {
		t.Fatalf("batch: %+v %v", result, err)
	}
	if _, err := storage.Open(ctx, "files", valid); err == nil {
		t.Fatal("later valid object not cleaned")
	}
}

func TestMessageImageCleanupRegistrationFailureCodes(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	name := "message-images/61d62896-ed6f-4676-926c-b7cce1e8bb10.png"
	storage := &imageTestStorage{objects: map[string][]byte{"files/" + name: []byte("retain")}, deleteErr: errors.New("unavailable")}
	s := application.NewService(application.Dependencies{MessageImages: r, Storage: storage})
	first := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: name, BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(context.Background(), &first); err != nil {
		t.Fatal(err)
	}
	if result := s.CleanupMessageImage(context.Background(), first.ID, now, application.RotationConfig{}); result.FailureCode != "message_image_cleanup_storage_unavailable" {
		t.Fatalf("first cleanup: %+v", result)
	}
	second := first
	second.ID = 0
	if err := r.CreateMessageImage(context.Background(), &second); err != nil {
		t.Fatal(err)
	}
	if result := s.CleanupMessageImage(context.Background(), second.ID, now, application.RotationConfig{}); result.Stage != "register" || result.FailureCode != "message_image_cleanup_hash_conflict" || result.CleanupJobID != 0 {
		t.Fatalf("path conflict: %+v", result)
	}
	if result := s.CleanupMessageImage(context.Background(), 999999, now, application.RotationConfig{}); result.Stage != "register" || result.FailureCode != "message_image_cleanup_register_failed" {
		t.Fatalf("missing record: %+v", result)
	}
	if _, err := storage.Open(context.Background(), "files", name); err != nil {
		t.Fatal("conflicting object deleted")
	}
}

func TestMessageImageCleanupSharedBudgetLendsAndAlternates(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Hour)
	storage := &imageTestStorage{objects: map[string][]byte{}, deleteErr: errors.New("unavailable")}
	s := application.NewService(application.Dependencies{MessageImages: r, Storage: storage, Clock: application.ClockFunc(func() time.Time { return now })})
	var newIDs, retryIDs []uint
	for range 4 {
		newFile := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
		if err := r.CreateMessageImage(ctx, &newFile); err != nil {
			t.Fatal(err)
		}
		newIDs = append(newIDs, newFile.ID)
		retryFile := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
		if err := r.CreateMessageImage(ctx, &retryFile); err != nil {
			t.Fatal(err)
		}
		retryIDs = append(retryIDs, retryFile.ID)
		storage.objects["files/"+newFile.ObjectName] = []byte("image")
		storage.objects["files/"+retryFile.ObjectName] = []byte("image")
		if _, created, err := r.RegisterMessageImageCleanup(ctx, retryFile.ID, now, now.Add(-time.Hour)); err != nil || !created {
			t.Fatalf("prepare retry: %v %v", created, err)
		}
	}
	scan := &application.MessageImageCleanupScan{}
	for round := range 4 {
		result, err := s.CleanupMessageImages(ctx, now, 1, scan, application.RotationConfig{})
		if err != nil || result.Processed != 1 || result.Failed != 1 || len(result.Items) != 1 {
			t.Fatalf("round %d: %+v %v", round, result, err)
		}
		want := retryIDs[round/2]
		if round%2 == 1 {
			want = newIDs[round/2]
		}
		if result.Items[0].FileID != want {
			t.Fatalf("round %d selected file %d, want %d", round, result.Items[0].FileID, want)
		}
	}
	more := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &more); err != nil {
		t.Fatal(err)
	}
	storage.objects["files/"+more.ObjectName] = []byte("image")
	additional := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &additional); err != nil {
		t.Fatal(err)
	}
	storage.objects["files/"+additional.ObjectName] = []byte("image")
	if scan.New.UpperID != retryIDs[3] || scan.New.AfterID != newIDs[1] {
		t.Fatalf("new scan upper changed before rollover: %+v", scan.New)
	}
	result, err := s.CleanupMessageImages(ctx, now, 4, scan, application.RotationConfig{})
	if err != nil || result.Processed != 4 || result.Failed != 4 || len(result.Items) != 4 || result.Items[0].FileID != retryIDs[2] || result.Items[1].FileID != retryIDs[3] || result.Items[2].FileID != newIDs[2] || result.Items[3].FileID != newIDs[3] {
		t.Fatalf("split budget: %+v %v", result, err)
	}
	if scan.New.UpperID != retryIDs[3] || scan.New.AfterID != newIDs[3] {
		t.Fatalf("new scan did not retain fixed upper after full page: %+v", scan.New)
	}
	result, err = s.CleanupMessageImages(ctx, now, 4, scan, application.RotationConfig{})
	if err != nil || result.Processed != 0 || scan.New.UpperID != 0 || scan.Retry.UpperID != 0 {
		t.Fatalf("new scan should roll over before seeing later IDs: %+v %+v %v", result, scan.New, err)
	}
	result, err = s.CleanupMessageImages(ctx, now, 4, scan, application.RotationConfig{})
	if err != nil || result.Processed != 2 || result.Failed != 2 || len(result.Items) != 2 || result.Items[0].FileID != more.ID || result.Items[1].FileID != additional.ID || result.Items[0].RetryAttempt != 0 || result.Items[1].RetryAttempt != 0 {
		t.Fatalf("empty retries should lend full budget after rollover: %+v %v", result, err)
	}
	oneMore := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &oneMore); err != nil {
		t.Fatal(err)
	}
	storage.objects["files/"+oneMore.ObjectName] = []byte("image")
	scan.NewFirst = false
	result, err = s.CleanupMessageImages(ctx, now, 1, scan, application.RotationConfig{})
	if err != nil || result.Processed != 1 || len(result.Items) != 1 || result.Items[0].FileID != oneMore.ID {
		t.Fatalf("N=1 retry-empty lending: %+v %v", result, err)
	}
	due := now.Add(time.Hour)
	s = application.NewService(application.Dependencies{MessageImages: r, Storage: storage, Clock: application.ClockFunc(func() time.Time { return due })})
	scan = &application.MessageImageCleanupScan{NewFirst: true}
	result, err = s.CleanupMessageImages(ctx, due, 1, scan, application.RotationConfig{})
	if err != nil || result.Processed != 1 || len(result.Items) != 1 || result.Items[0].RetryAttempt == 0 {
		t.Fatalf("N=1 new-empty lending to due retries: %+v %v", result, err)
	}
}

type cancelCleanupStorage struct {
	*imageTestStorage
	cancel context.CancelFunc
}

func (s *cancelCleanupStorage) Stat(ctx context.Context, bucket, name string) (application.Object, error) {
	object, err := s.imageTestStorage.Stat(ctx, bucket, name)
	s.cancel()
	return object, err
}

func TestMessageImageCleanupCancellationKeepsUnattemptedCursor(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	storage := &imageTestStorage{objects: map[string][]byte{}}
	ids := make([]uint, 0, 2)
	for range 2 {
		file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
		if err := r.CreateMessageImage(context.Background(), &file); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, file.ID)
		storage.objects["files/"+file.ObjectName] = []byte("image")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := application.NewService(application.Dependencies{MessageImages: r, Storage: &cancelCleanupStorage{imageTestStorage: storage, cancel: cancel}})
	scan := &application.MessageImageCleanupScan{}
	result, err := s.CleanupMessageImages(ctx, now, 2, scan, application.RotationConfig{})
	if !errors.Is(err, context.Canceled) || result.Processed != 1 || len(result.Items) != 1 || result.Items[0].FileID != ids[0] || scan.New.AfterID != ids[0] || scan.New.UpperID != ids[1] {
		t.Fatalf("canceled scan skipped unattempted item: %+v %+v %v", result, scan, err)
	}
	s = application.NewService(application.Dependencies{MessageImages: r, Storage: storage})
	result, err = s.CleanupMessageImages(context.Background(), now, 2, scan, application.RotationConfig{})
	if err != nil || result.Processed != 1 || len(result.Items) != 1 || result.Items[0].FileID != ids[1] || result.Succeeded != 1 {
		t.Fatalf("resume unattempted item: %+v %v", result, err)
	}
}

func TestMessageImageCleanupMissingObjectFinalizesSuccessfully(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(context.Background(), &file); err != nil {
		t.Fatal(err)
	}
	s := application.NewService(application.Dependencies{MessageImages: r, Storage: &imageTestStorage{objects: map[string][]byte{}}})
	scan := &application.MessageImageCleanupScan{}
	result, err := s.CleanupMessageImages(context.Background(), now, 1, scan, application.RotationConfig{})
	if err != nil || result.Processed != 1 || result.Succeeded != 1 || result.Failed != 0 || len(result.Items) != 1 || result.Items[0].FailureCode != "" {
		t.Fatalf("NoSuchKey should be idempotent success: %+v %v", result, err)
	}
	if _, err := r.FindMessageImage(context.Background(), file.ID); !errors.Is(err, application.ErrFileNotFound) {
		t.Fatalf("orphan record after idempotent cleanup: %v", err)
	}
	var fileCount, jobCount int64
	if err := db.Unscoped().Model(&files.File{}).Where("id = ?", file.ID).Count(&fileCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&files.MessageImageCleanupJob{}).Where("file_id = ?", file.ID).Count(&jobCount).Error; err != nil {
		t.Fatal(err)
	}
	if fileCount != 0 || jobCount != 0 {
		t.Fatalf("missing object left file/queue behind: %d/%d", fileCount, jobCount)
	}
}

func TestMessageImageCleanupNeverRetriesNewJobInSameRound(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatal(err)
	}
	r := adapter.NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Minute)
	file := domain.File{Purpose: "message_image", Bucket: "files", ObjectName: "message-images/" + uuid.NewString() + ".png", BindingExpiresAt: &expired}
	if err := r.CreateMessageImage(ctx, &file); err != nil {
		t.Fatal(err)
	}
	storage := &imageTestStorage{objects: map[string][]byte{"files/" + file.ObjectName: []byte("image")}, deleteErr: errors.New("unavailable")}
	s := application.NewService(application.Dependencies{MessageImages: r, Storage: storage, Clock: application.ClockFunc(func() time.Time { return now.Add(-2 * time.Hour) })})
	scan := &application.MessageImageCleanupScan{NewFirst: true}
	result, err := s.CleanupMessageImages(ctx, now, 2, scan, application.RotationConfig{})
	if err != nil || result.Processed != 1 || result.Failed != 1 || len(result.Items) != 1 || result.Items[0].RetryAttempt != 0 {
		t.Fatalf("new job consumed retry budget in same round: %+v %v", result, err)
	}
	jobs, err := r.FindMessageImageCleanupRetries(ctx, now, application.CleanupCursor{UpperID: result.Items[0].CleanupJobID}, 1)
	if err != nil || len(jobs) != 1 || jobs[0].RetryCount != 0 {
		t.Fatalf("new job was reserved for retry: %+v %v", jobs, err)
	}
	if scan.Retry.UpperID != 0 || scan.Retry.AfterID != 0 {
		t.Fatalf("same-round retry cursor picked up a new registration: %+v", scan.Retry)
	}
}
