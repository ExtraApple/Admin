package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"
	"admin/service/uploadsecurity"
)

type recordingAvatarRepository struct {
	events        *[]string
	findCalls     int
	updateCalls   int
	userID        uint
	update        AvatarUpdate
	user          model.User
	findErr       error
	updateOutcome AvatarUpdateOutcome
	updateErr     error
}

const normalizedAvatarSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func (r *recordingAvatarRepository) FindByID(
	_ context.Context,
	userID uint,
) (*model.User, error) {
	r.findCalls++
	r.userID = userID
	if r.findErr != nil {
		return nil, r.findErr
	}
	r.user.ID = userID
	copy := r.user
	return &copy, nil
}

func (r *recordingAvatarRepository) UpdateAvatar(
	_ context.Context,
	userID uint,
	update AvatarUpdate,
) (AvatarUpdateOutcome, error) {
	r.updateCalls++
	if r.events != nil {
		*r.events = append(*r.events, "update")
	}
	r.userID = userID
	r.update = update
	if r.updateErr != nil {
		return r.updateOutcome, r.updateErr
	}
	r.user.ID = userID
	applyAvatarUpdate(&r.user, update)
	return AvatarUpdateCommitted, nil
}

func TestAvatarServiceStoresNormalizedOutputAndTrustedMetadata(t *testing.T) {
	validatedAt := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/png",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			ContentSHA256:      normalizedAvatarSHA256,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{}
	users := &recordingAvatarRepository{
		user: model.User{
			Username: "alice",
			Avatar:   "https://legacy.example/avatar.png",
		},
	}
	avatars := NewAvatarService(
		validator,
		store,
		users,
		func() time.Time { return validatedAt },
	)

	result, err := avatars.UploadWithResult(context.Background(), UploadAvatarInput{
		UserID:      42,
		FileName:    "portrait.png",
		ContentType: "image/png",
		Size:        18,
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})
	if err != nil {
		t.Fatalf("UploadWithResult() error = %v", err)
	}

	if validator.input.Purpose != uploadsecurity.PurposeAvatar ||
		validator.input.FileName != "portrait.png" ||
		validator.input.DeclaredMIME != "image/png" ||
		validator.input.Size != 18 ||
		validator.input.MaxBytes != 2<<20 {
		t.Fatalf("validator input = %#v, want bounded avatar upload", validator.input)
	}
	if store.putCalls != 1 {
		t.Fatalf("storage Put() calls = %d, want 1", store.putCalls)
	}
	if store.putInput.Bucket != avatarBucket ||
		!strings.HasPrefix(store.putInput.Name, "avatars/42/") ||
		!strings.HasSuffix(store.putInput.Name, ".jpg") ||
		strings.Contains(store.putInput.Name, "portrait") ||
		store.putInput.ContentType != "image/jpeg" ||
		store.putInput.Size != int64(len("normalized-avatar")) ||
		store.putContent != "normalized-avatar" ||
		strings.Contains(store.putInput.Name, normalizedAvatarSHA256) {
		t.Fatalf("stored avatar = input %#v content %q, want normalized JPEG in user namespace",
			store.putInput, store.putContent)
	}
	if users.updateCalls != 1 ||
		users.userID != 42 ||
		users.update.ObjectName != store.putInput.Name ||
		users.update.ContentType != "image/jpeg" ||
		users.update.ContentSHA256 != normalizedAvatarSHA256 ||
		users.update.ValidationStatus != model.FileValidationStatusValidated ||
		users.update.ValidatedAt == nil ||
		!users.update.ValidatedAt.Equal(validatedAt) {
		t.Fatalf("avatar update = %#v for user %d, want trusted validation metadata",
			users.update, users.userID)
	}
	if result.User == nil ||
		result.User.ID != 42 ||
		result.User.AvatarObjectName != store.putInput.Name ||
		result.User.AvatarContentType != "image/jpeg" ||
		result.User.AvatarContentSHA256 != normalizedAvatarSHA256 ||
		result.User.AvatarValidationStatus != model.FileValidationStatusValidated ||
		result.User.AvatarValidatedAt == nil ||
		!result.User.AvatarValidatedAt.Equal(validatedAt) {
		t.Fatalf("UploadWithResult() user = %#v, want updated trusted avatar fields", result.User)
	}
	if result.User.Avatar != "https://legacy.example/avatar.png" {
		t.Fatalf("legacy avatar field = %q, want unchanged compatibility value", result.User.Avatar)
	}
	if result.FileName != "portrait.jpg" ||
		result.FileSize != int64(len("normalized-avatar")) ||
		result.DetectedMIME != "image/png" ||
		result.PolicyVersion != uploadsecurity.PolicyVersionV1 {
		t.Fatalf("UploadWithResult() audit result = %#v, want normalized validator metadata", result)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("legacy avatar cleanup calls = %d, want 0", store.deleteCalls)
	}
}

func TestAvatarServicePreservesOriginalWhenNewObjectWriteFails(t *testing.T) {
	const (
		oldObject    = "avatars/42/00000000-0000-0000-0000-000000000001.jpg"
		storageError = "minio secret=avatar-write"
	)
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/jpeg",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{putErr: errors.New(storageError)}
	users := &recordingAvatarRepository{
		user: model.User{
			AvatarObjectName:       oldObject,
			AvatarContentType:      "image/jpeg",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	avatars := NewAvatarService(validator, store, users, time.Now)

	_, err := avatars.Upload(context.Background(), UploadAvatarInput{
		UserID:      42,
		FileName:    "portrait.jpg",
		ContentType: "image/jpeg",
		Size:        18,
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeStorageUnavailable {
		t.Fatalf("Upload() error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodeStorageUnavailable, err)
	}
	if strings.Contains(err.Error(), storageError) {
		t.Fatalf("Upload() error %q leaked storage detail", err)
	}
	if users.updateCalls != 0 {
		t.Fatalf("database update calls = %d, want 0", users.updateCalls)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("object cleanup calls = %d, want 0 for failed write", store.deleteCalls)
	}
	if users.user.AvatarObjectName != oldObject {
		t.Fatalf("original avatar = %q, want unchanged %q",
			users.user.AvatarObjectName, oldObject)
	}
}

func TestAvatarServiceDeletesOldTrustedObjectAfterCommittedReplacement(t *testing.T) {
	const oldObject = "avatars/42/00000000-0000-0000-0000-000000000001.png"
	events := []string{}
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/png",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			ContentSHA256:      normalizedAvatarSHA256,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{events: &events}
	users := &recordingAvatarRepository{
		events: &events,
		user: model.User{
			AvatarObjectName:       oldObject,
			AvatarContentType:      "image/png",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	avatars := NewAvatarService(validator, store, users, time.Now)

	user, err := avatars.Upload(context.Background(), UploadAvatarInput{
		UserID:      42,
		FileName:    "portrait.png",
		ContentType: "image/png",
		Size:        18,
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if strings.Join(events, ",") != "put,update,delete" {
		t.Fatalf("replacement events = %v, want put, update, delete", events)
	}
	if store.deleteCalls != 1 ||
		store.deleteBucket != avatarBucket ||
		store.deleteName != oldObject {
		t.Fatalf("old avatar cleanup = %d calls for %q/%q, want %q/%q",
			store.deleteCalls, store.deleteBucket, store.deleteName, avatarBucket, oldObject)
	}
	if user.AvatarObjectName != store.putInput.Name {
		t.Fatalf("returned avatar = %q, want new object %q",
			user.AvatarObjectName, store.putInput.Name)
	}
}

func TestAvatarServiceDeletesNewObjectWhenDatabaseUpdateIsNotCommitted(t *testing.T) {
	const databaseDetail = "mysql password=avatar-database-secret"
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/png",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{}
	users := &recordingAvatarRepository{
		user: model.User{
			AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.jpg",
			AvatarContentType:      "image/jpeg",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
		updateOutcome: AvatarUpdateNotCommitted,
		updateErr:     errors.New(databaseDetail),
	}
	avatars := NewAvatarService(validator, store, users, time.Now)

	_, err := avatars.Upload(context.Background(), UploadAvatarInput{
		UserID:      42,
		FileName:    "portrait.png",
		ContentType: "image/png",
		Size:        18,
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodePersistenceFailed {
		t.Fatalf("Upload() error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodePersistenceFailed, err)
	}
	if strings.Contains(err.Error(), databaseDetail) {
		t.Fatalf("Upload() error %q leaked database detail", err)
	}
	if store.deleteCalls != 1 ||
		store.deleteBucket != avatarBucket ||
		store.deleteName != store.putInput.Name {
		t.Fatalf("compensation delete = %d calls for %q/%q, want new object %q/%q",
			store.deleteCalls, store.deleteBucket, store.deleteName,
			avatarBucket, store.putInput.Name)
	}
	if users.user.AvatarObjectName != "avatars/42/00000000-0000-0000-0000-000000000001.jpg" {
		t.Fatalf("original avatar = %q, want unchanged", users.user.AvatarObjectName)
	}
}

func TestAvatarServiceKeepsNewObjectWhenDatabaseCommitOutcomeIsZeroValue(t *testing.T) {
	const databaseDetail = "mysql connection lost while awaiting commit result"
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/jpeg",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			ContentSHA256:      normalizedAvatarSHA256,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{}
	users := &recordingAvatarRepository{
		user: model.User{
			AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.jpg",
			AvatarContentType:      "image/jpeg",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
		updateErr: errors.New(databaseDetail),
	}
	avatars := NewAvatarService(validator, store, users, time.Now)

	_, err := avatars.UploadWithResult(context.Background(), UploadAvatarInput{
		UserID:      42,
		FileName:    "portrait.jpg",
		ContentType: "image/jpeg",
		Size:        18,
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})

	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodePersistenceFailed {
		t.Fatalf("UploadWithResult() error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodePersistenceFailed, err)
	}
	if store.deleteCalls != 0 {
		t.Fatalf(
			"stable repro: zero-value database commit outcome must be unknown, "+
				"but service deleted "+
				"%d new object(s), last=%q",
			store.deleteCalls,
			store.deleteName,
		)
	}
}

func TestAvatarServiceDoesNotDeleteNewObjectAfterCommittedDatabaseUpdate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}

	const oldObject = "avatars/42/00000000-0000-0000-0000-000000000001.jpg"
	user := model.User{
		Model:                  gorm.Model{ID: 42},
		Username:               "post-commit-avatar",
		Password:               "test-password",
		Email:                  "post-commit-avatar@example.test",
		AvatarObjectName:       oldObject,
		AvatarContentType:      "image/jpeg",
		AvatarValidationStatus: model.FileValidationStatusValidated,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	postCommitReadErr := errors.New("forced post-commit user reload failure")
	queryCalls := 0
	injected := false
	armed := true
	if err := db.Callback().Query().Before("gorm:query").Register(
		"test:fail_avatar_post_commit_reload",
		func(tx *gorm.DB) {
			if !armed {
				return
			}
			queryCalls++
			if queryCalls == 2 {
				injected = true
				tx.AddError(postCommitReadErr)
			}
		},
	); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/jpeg",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			ContentSHA256:      normalizedAvatarSHA256,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{}
	avatars := NewAvatarService(
		validator,
		store,
		gormAvatarRepository{db: db},
		time.Now,
	)

	result, uploadErr := avatars.UploadWithResult(context.Background(), UploadAvatarInput{
		UserID:      user.ID,
		FileName:    "portrait.jpg",
		ContentType: "image/jpeg",
		Size:        int64(len("normalized-avatar")),
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})

	armed = false
	var persisted model.User
	if err := db.First(&persisted, user.ID).Error; err != nil {
		t.Fatalf("reload persisted user: %v", err)
	}
	if uploadErr != nil &&
		persisted.AvatarObjectName == store.putInput.Name &&
		store.deleteCalls == 1 &&
		store.deleteName == store.putInput.Name {
		t.Fatalf(
			"stable repro: avatar update committed object %q, but the post-commit "+
				"reload error %v caused that same new object to be deleted",
			persisted.AvatarObjectName,
			uploadErr,
		)
	}
	if uploadErr != nil {
		t.Fatalf("UploadWithResult() error = %v, want committed replacement to succeed", uploadErr)
	}
	if injected {
		t.Fatalf("avatar repository performed a post-commit query; query calls=%d", queryCalls)
	}
	if result == nil || result.User == nil ||
		result.User.AvatarObjectName != store.putInput.Name {
		t.Fatalf("UploadWithResult() result = %#v, want committed new avatar %q",
			result, store.putInput.Name)
	}
	if persisted.AvatarObjectName != store.putInput.Name {
		t.Fatalf("persisted avatar = %q, want new object %q",
			persisted.AvatarObjectName, store.putInput.Name)
	}
	if persisted.AvatarContentSHA256 != normalizedAvatarSHA256 {
		t.Fatalf("persisted avatar SHA-256 = %q, want %q",
			persisted.AvatarContentSHA256, normalizedAvatarSHA256)
	}
	if store.deleteCalls != 1 || store.deleteName != oldObject {
		t.Fatalf("cleanup deleted %d object(s), last=%q; want only old object %q",
			store.deleteCalls, store.deleteName, oldObject)
	}
}

func TestAvatarServiceKeepsSuccessfulReplacementWhenOldObjectCleanupFails(t *testing.T) {
	const (
		oldObject    = "avatars/42/00000000-0000-0000-0000-000000000001.jpg"
		deleteDetail = "minio access-key=old-avatar-secret"
	)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	originalLogger := global.Logger
	global.Logger = zap.New(logCore)
	t.Cleanup(func() {
		global.Logger = originalLogger
	})

	events := []string{}
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeAvatar,
			FileName:           "portrait.jpg",
			CanonicalType:      uploadsecurity.TypeJPEG,
			CanonicalExtension: ".jpg",
			CanonicalMIME:      "image/jpeg",
			DetectedMIME:       "image/png",
			Size:               int64(len("normalized-avatar")),
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("normalized-avatar"),
		},
	}
	store := &recordingFileStore{
		events:    &events,
		deleteErr: errors.New(deleteDetail),
	}
	users := &recordingAvatarRepository{
		events: &events,
		user: model.User{
			AvatarObjectName:       oldObject,
			AvatarContentType:      "image/jpeg",
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	avatars := NewAvatarService(validator, store, users, time.Now)

	user, err := avatars.Upload(context.Background(), UploadAvatarInput{
		UserID:      42,
		FileName:    "portrait.png",
		ContentType: "image/png",
		Size:        18,
		MaxBytes:    2 << 20,
		Reader:      strings.NewReader("untrusted-original"),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v, want successful replacement despite cleanup failure", err)
	}
	if strings.Join(events, ",") != "put,update,delete" {
		t.Fatalf("replacement events = %v, want put, update, delete", events)
	}
	if store.deleteCalls != 1 ||
		store.deleteBucket != avatarBucket ||
		store.deleteName != oldObject {
		t.Fatalf("old avatar cleanup = %d calls for %q/%q, want %q/%q",
			store.deleteCalls, store.deleteBucket, store.deleteName, avatarBucket, oldObject)
	}
	if user.AvatarObjectName != store.putInput.Name ||
		user.AvatarObjectName == oldObject {
		t.Fatalf("returned avatar = %q, want committed new object %q",
			user.AvatarObjectName, store.putInput.Name)
	}
	entries := observedLogs.FilterMessage("old avatar cleanup failed").All()
	if len(entries) != 1 {
		t.Fatalf("old avatar cleanup logs = %d, want 1", len(entries))
	}
	encoded := entries[0].Message
	for key, value := range entries[0].ContextMap() {
		encoded += key + "=" + strings.TrimSpace(value.(string))
	}
	if strings.Contains(encoded, deleteDetail) || strings.Contains(encoded, oldObject) {
		t.Fatalf("controlled cleanup log %q leaked storage detail", encoded)
	}
}

func TestAvatarServiceRestoresDefaultBeforeDeletingOldTrustedObject(t *testing.T) {
	const oldObject = "avatars/42/00000000-0000-0000-0000-000000000001.jpg"
	events := []string{}
	store := &recordingFileStore{events: &events}
	users := &recordingAvatarRepository{
		events: &events,
		user: model.User{
			AvatarObjectName:       oldObject,
			AvatarContentType:      "image/jpeg",
			AvatarContentSHA256:    normalizedAvatarSHA256,
			AvatarValidationStatus: model.FileValidationStatusValidated,
			AvatarValidatedAt: func() *time.Time {
				value := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
				return &value
			}(),
		},
	}
	avatars := NewAvatarService(
		uploadsecurity.NewAvatarValidator(),
		store,
		users,
		time.Now,
	)

	user, err := avatars.RestoreDefault(context.Background(), 42)
	if err != nil {
		t.Fatalf("RestoreDefault() error = %v", err)
	}
	if strings.Join(events, ",") != "update,delete" {
		t.Fatalf("restore events = %v, want update, delete", events)
	}
	if users.update.ObjectName != "" ||
		users.update.ContentType != "" ||
		users.update.ContentSHA256 != "" ||
		users.update.ValidationStatus != "" ||
		users.update.ValidatedAt != nil {
		t.Fatalf("default avatar update = %#v, want cleared trusted fields", users.update)
	}
	if store.deleteCalls != 1 ||
		store.deleteBucket != avatarBucket ||
		store.deleteName != oldObject {
		t.Fatalf("old avatar cleanup = %d calls for %q/%q, want %q/%q",
			store.deleteCalls, store.deleteBucket, store.deleteName, avatarBucket, oldObject)
	}
	if user.AvatarObjectName != "" ||
		user.AvatarContentType != "" ||
		user.AvatarContentSHA256 != "" ||
		user.AvatarValidationStatus != "" ||
		user.AvatarValidatedAt != nil {
		t.Fatalf("RestoreDefault() user = %#v, want cleared trusted avatar fields", user)
	}
}

func TestAvatarServiceRestoresDefaultWhenOnlyDigestRemains(t *testing.T) {
	store := &recordingFileStore{}
	users := &recordingAvatarRepository{
		user: model.User{
			AvatarContentSHA256: normalizedAvatarSHA256,
		},
	}
	avatars := NewAvatarService(
		uploadsecurity.NewAvatarValidator(),
		store,
		users,
		time.Now,
	)

	user, err := avatars.RestoreDefault(context.Background(), 42)
	if err != nil {
		t.Fatalf("RestoreDefault() error = %v", err)
	}
	if users.updateCalls != 1 {
		t.Fatalf("avatar update calls = %d, want 1 for residual digest cleanup", users.updateCalls)
	}
	if users.update.ContentSHA256 != "" {
		t.Fatalf("default avatar update digest = %q, want empty", users.update.ContentSHA256)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("avatar delete calls = %d, want 0 without a trusted object name", store.deleteCalls)
	}
	if user == nil || user.AvatarContentSHA256 != "" {
		t.Fatalf("RestoreDefault() user = %#v, want cleared residual digest", user)
	}
}

func TestAvatarServiceRestoreDefaultIsIdempotentWhenAlreadyDefault(t *testing.T) {
	store := &recordingFileStore{}
	users := &recordingAvatarRepository{
		user: model.User{
			Avatar: "https://legacy.example/avatar.png",
		},
		updateOutcome: AvatarUpdateNotCommitted,
		updateErr:     gorm.ErrRecordNotFound,
	}
	avatars := NewAvatarService(
		uploadsecurity.NewAvatarValidator(),
		store,
		users,
		time.Now,
	)

	user, err := avatars.RestoreDefault(context.Background(), 42)
	if err != nil {
		t.Fatalf(
			"stable repro: restoring an already-default avatar was treated as a persistence failure: %v",
			err,
		)
	}
	if users.updateCalls != 0 {
		t.Fatalf("avatar update calls = %d, want 0 for an already-default avatar", users.updateCalls)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("avatar delete calls = %d, want 0 for an already-default avatar", store.deleteCalls)
	}
	if user == nil ||
		user.AvatarObjectName != "" ||
		user.AvatarContentType != "" ||
		user.AvatarContentSHA256 != "" ||
		user.AvatarValidationStatus != "" ||
		user.AvatarValidatedAt != nil {
		t.Fatalf("RestoreDefault() user = %#v, want default avatar fields", user)
	}
}

func TestAvatarServiceKeepsDefaultWhenRestoreCleanupFails(t *testing.T) {
	const (
		oldObject    = "avatars/42/00000000-0000-0000-0000-000000000001.jpg"
		deleteDetail = "minio access-key=restore-avatar-secret"
	)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	originalLogger := global.Logger
	global.Logger = zap.New(logCore)
	t.Cleanup(func() {
		global.Logger = originalLogger
	})

	events := []string{}
	store := &recordingFileStore{
		events:    &events,
		deleteErr: errors.New(deleteDetail),
	}
	users := &recordingAvatarRepository{
		events: &events,
		user: model.User{
			AvatarObjectName:       oldObject,
			AvatarContentType:      "image/jpeg",
			AvatarContentSHA256:    normalizedAvatarSHA256,
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
	}
	avatars := NewAvatarService(
		uploadsecurity.NewAvatarValidator(),
		store,
		users,
		time.Now,
	)

	user, err := avatars.RestoreDefault(context.Background(), 42)
	if err != nil {
		t.Fatalf("RestoreDefault() error = %v, want committed default result", err)
	}
	if strings.Join(events, ",") != "update,delete" {
		t.Fatalf("restore events = %v, want update, delete", events)
	}
	if user.AvatarObjectName != "" ||
		user.AvatarContentType != "" ||
		user.AvatarContentSHA256 != "" ||
		user.AvatarValidationStatus != "" ||
		user.AvatarValidatedAt != nil {
		t.Fatalf("RestoreDefault() user = %#v, want default avatar despite cleanup failure", user)
	}
	entries := observedLogs.FilterMessage("old avatar cleanup failed").All()
	if len(entries) != 1 {
		t.Fatalf("old avatar cleanup logs = %d, want 1", len(entries))
	}
	encoded := entries[0].Message
	for key, value := range entries[0].ContextMap() {
		encoded += key + "=" + strings.TrimSpace(value.(string))
	}
	if strings.Contains(encoded, deleteDetail) || strings.Contains(encoded, oldObject) {
		t.Fatalf("controlled cleanup log %q leaked storage detail", encoded)
	}
}

func TestAvatarServicePreservesTrustedAvatarWhenRestoreUpdateFails(t *testing.T) {
	const (
		oldObject     = "avatars/42/00000000-0000-0000-0000-000000000001.png"
		databaseError = "mysql password=restore-avatar-secret"
	)
	store := &recordingFileStore{}
	users := &recordingAvatarRepository{
		user: model.User{
			AvatarObjectName:       oldObject,
			AvatarContentType:      "image/png",
			AvatarContentSHA256:    normalizedAvatarSHA256,
			AvatarValidationStatus: model.FileValidationStatusValidated,
		},
		updateErr: errors.New(databaseError),
	}
	avatars := NewAvatarService(
		uploadsecurity.NewAvatarValidator(),
		store,
		users,
		time.Now,
	)

	_, err := avatars.RestoreDefault(context.Background(), 42)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodePersistenceFailed {
		t.Fatalf("RestoreDefault() error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodePersistenceFailed, err)
	}
	if strings.Contains(err.Error(), databaseError) {
		t.Fatalf("RestoreDefault() error %q leaked database detail", err)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("old avatar cleanup calls = %d, want 0 before database commit", store.deleteCalls)
	}
	if users.user.AvatarObjectName != oldObject ||
		users.user.AvatarContentType != "image/png" ||
		users.user.AvatarContentSHA256 != normalizedAvatarSHA256 ||
		users.user.AvatarValidationStatus != model.FileValidationStatusValidated {
		t.Fatalf("original avatar = %#v, want unchanged after update failure", users.user)
	}
}
