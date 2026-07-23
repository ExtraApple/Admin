package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
	"admin/service/fileaccess"
	"admin/service/objectstorage"
	"admin/service/uploadsecurity"
)

type rejectingFileValidator struct {
	err error
}

func (v rejectingFileValidator) Validate(
	_ context.Context,
	_ uploadsecurity.Input,
) (uploadsecurity.Result, error) {
	return uploadsecurity.Result{}, v.err
}

type acceptingFileValidator struct {
	result uploadsecurity.Result
}

func (v acceptingFileValidator) Validate(
	_ context.Context,
	_ uploadsecurity.Input,
) (uploadsecurity.Result, error) {
	return v.result, nil
}

type recordingFileValidator struct {
	input  uploadsecurity.Input
	result uploadsecurity.Result
	err    error
}

func (v *recordingFileValidator) Validate(
	_ context.Context,
	input uploadsecurity.Input,
) (uploadsecurity.Result, error) {
	v.input = input
	return v.result, v.err
}

type recordingFileStore struct {
	events       *[]string
	putCalls     int
	putInput     objectstorage.PutInput
	putContent   string
	putErr       error
	openCalls    int
	openBucket   string
	openName     string
	openContent  string
	openReader   io.ReadCloser
	openErr      error
	deleteCalls  int
	deleteBucket string
	deleteName   string
	deleteErr    error
	listCalls    int
	listBucket   string
	listOptions  objectstorage.ListOptions
	listObjects  []objectstorage.ObjectInfo
	listErr      error
}

func (s *recordingFileStore) Put(
	_ context.Context,
	input objectstorage.PutInput,
) (objectstorage.ObjectInfo, error) {
	s.putCalls++
	if s.events != nil {
		*s.events = append(*s.events, "put")
	}
	s.putInput = input
	if s.putErr != nil {
		return objectstorage.ObjectInfo{}, s.putErr
	}
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	s.putContent = string(content)
	return objectstorage.ObjectInfo{
		Bucket:      input.Bucket,
		Name:        input.Name,
		Size:        input.Size,
		ContentType: input.ContentType,
	}, nil
}

func (s *recordingFileStore) Open(
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

type firstReadErrorCloser struct {
	err    error
	closed bool
}

type firstReadDataErrorCloser struct {
	err    error
	read   bool
	closed bool
}

func (r *firstReadErrorCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r *firstReadErrorCloser) Close() error {
	r.closed = true
	return nil
}

func (r *firstReadDataErrorCloser) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	if len(buffer) == 0 {
		return 0, nil
	}
	buffer[0] = 'x'
	return 1, r.err
}

func (r *firstReadDataErrorCloser) Close() error {
	r.closed = true
	return nil
}

func (s *recordingFileStore) Stat(
	_ context.Context,
	_, _ string,
) (objectstorage.ObjectInfo, error) {
	panic("unexpected Stat call")
}

func (s *recordingFileStore) Delete(
	_ context.Context,
	bucket, name string,
) error {
	s.deleteCalls++
	if s.events != nil {
		*s.events = append(*s.events, "delete")
	}
	s.deleteBucket = bucket
	s.deleteName = name
	return s.deleteErr
}

func (s *recordingFileStore) List(
	_ context.Context,
	bucket string,
	options objectstorage.ListOptions,
) ([]objectstorage.ObjectInfo, error) {
	s.listCalls++
	s.listBucket = bucket
	s.listOptions = options
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]objectstorage.ObjectInfo(nil), s.listObjects...), nil
}

type recordingFileRepository struct {
	createCalls int
	file        *model.File
	createErr   error
}

func (r *recordingFileRepository) Create(_ context.Context, file *model.File) error {
	r.createCalls++
	copy := *file
	r.file = &copy
	if r.createErr != nil {
		return r.createErr
	}
	copy.ID = 7
	file.ID = copy.ID
	return nil
}

type recordingFileRevalidationRepository struct {
	file        model.File
	findCalls   int
	findErr     error
	updateCalls int
	update      FileValidationUpdate
	updateErr   error
}

func (r *recordingFileRevalidationRepository) FindByID(
	_ context.Context,
	fileID uint,
) (*model.File, error) {
	r.findCalls++
	if r.findErr != nil {
		return nil, r.findErr
	}
	if r.file.ID != fileID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := r.file
	return &copy, nil
}

func (r *recordingFileRevalidationRepository) UpdateValidation(
	_ context.Context,
	fileID uint,
	update FileValidationUpdate,
) error {
	r.updateCalls++
	r.update = update
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.file.ID != fileID {
		return gorm.ErrRecordNotFound
	}
	r.file.ContentType = update.ContentType
	r.file.DetectedContentType = update.DetectedContentType
	r.file.ContentSHA256 = update.ContentSHA256
	r.file.ValidationStatus = update.Status
	r.file.ValidationPolicyVersion = update.PolicyVersion
	r.file.ValidationErrorCode = update.ErrorCode
	r.file.ValidatedAt = update.ValidatedAt
	return nil
}

func TestFileServiceRejectsInvalidUploadBeforeStorageOrPersistence(t *testing.T) {
	validationErr := uploadsecurity.NewError(uploadsecurity.CodeFileContentInvalid, nil)
	store := &recordingFileStore{}
	files := &recordingFileRepository{}
	fileService := NewFileService(
		rejectingFileValidator{err: validationErr},
		store,
		files,
	)

	_, err := fileService.Upload(context.Background(), UploadFileInput{
		UploaderID:  42,
		FileName:    "report.pdf",
		ContentType: "application/pdf",
		Size:        12,
		MaxBytes:    1024,
		Reader:      strings.NewReader("not-a-pdf"),
	})

	if err != validationErr {
		t.Fatalf("Upload() error = %v, want original validation error %v", err, validationErr)
	}
	if store.putCalls != 0 {
		t.Fatalf("storage Put() calls = %d, want 0", store.putCalls)
	}
	if files.createCalls != 0 {
		t.Fatalf("repository Create() calls = %d, want 0", files.createCalls)
	}
}

func TestFileServiceStoresOnlyValidatedUploadResult(t *testing.T) {
	store := &recordingFileStore{}
	files := &recordingFileRepository{}
	fileService := NewFileService(
		acceptingFileValidator{result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeManagedFile,
			FileName:           "report.pdf",
			CanonicalType:      uploadsecurity.TypePDF,
			CanonicalExtension: ".pdf",
			CanonicalMIME:      "application/pdf",
			DetectedMIME:       "application/pdf",
			Size:               13,
			ContentSHA256:      "0c339e699e5ff5f6f8f8d710b4d2e8053cb3dddf32980830f9e64bbd94c7eb2e",
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("validated-pdf"),
		}},
		store,
		files,
	)

	startedAt := time.Now()
	info, err := fileService.Upload(context.Background(), UploadFileInput{
		UploaderID:  42,
		FileName:    `C:\fakepath\unsafe-name.PDF`,
		ContentType: "application/pdf",
		Size:        9,
		MaxBytes:    1024,
		Reader:      strings.NewReader("untrusted"),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	finishedAt := time.Now()

	if store.putCalls != 1 {
		t.Fatalf("storage Put() calls = %d, want 1", store.putCalls)
	}
	if store.putInput.Bucket != fileBucket {
		t.Fatalf("storage bucket = %q, want %q", store.putInput.Bucket, fileBucket)
	}
	if store.putInput.ContentType != "application/pdf" || store.putInput.Size != 13 {
		t.Fatalf("storage metadata = MIME %q size %d, want validated MIME and size",
			store.putInput.ContentType, store.putInput.Size)
	}
	if !strings.HasSuffix(store.putInput.Name, ".pdf") ||
		strings.Contains(store.putInput.Name, "unsafe-name") {
		t.Fatalf("storage object name = %q, want server-generated .pdf name", store.putInput.Name)
	}
	if strings.Contains(
		store.putInput.Name,
		"0c339e699e5ff5f6f8f8d710b4d2e8053cb3dddf32980830f9e64bbd94c7eb2e",
	) {
		t.Fatalf("storage object name = %q, must not contain content digest", store.putInput.Name)
	}
	if store.putContent != "validated-pdf" {
		t.Fatalf("storage content = %q, want validated reader content", store.putContent)
	}
	if files.createCalls != 1 || files.file == nil {
		t.Fatalf("repository Create() calls = %d file = %#v, want one file record",
			files.createCalls, files.file)
	}
	if files.file.Name != "report.pdf" ||
		files.file.ContentType != "application/pdf" ||
		files.file.DetectedContentType != "application/pdf" ||
		files.file.Size != 13 ||
		files.file.UploaderID != 42 ||
		files.file.ValidationStatus != model.FileValidationStatusValidated ||
		files.file.ContentSHA256 != "0c339e699e5ff5f6f8f8d710b4d2e8053cb3dddf32980830f9e64bbd94c7eb2e" ||
		files.file.ValidationPolicyVersion != uploadsecurity.PolicyVersionV1 ||
		files.file.ValidationErrorCode != "" ||
		files.file.ValidatedAt == nil {
		t.Fatalf("created file metadata = %#v, want complete trusted validation metadata", files.file)
	}
	if files.file.ValidatedAt.Before(startedAt) || files.file.ValidatedAt.After(finishedAt) {
		t.Fatalf("validated_at = %v, want between %v and %v",
			files.file.ValidatedAt, startedAt, finishedAt)
	}
	if info == nil ||
		info.ID != 7 ||
		info.Name != "report.pdf" ||
		info.ContentSHA256 != "0c339e699e5ff5f6f8f8d710b4d2e8053cb3dddf32980830f9e64bbd94c7eb2e" {
		t.Fatalf("Upload() info = %#v, want created validated file", info)
	}
}

func TestListFilesReturnsValidationMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	validatedAt := time.Date(2026, 7, 14, 9, 10, 11, 0, time.UTC)
	file := model.File{
		Name:                    "blocked.pdf",
		Bucket:                  "files",
		ObjectName:              "private-object.pdf",
		ContentType:             "application/pdf",
		DetectedContentType:     "application/pdf",
		Size:                    128,
		UploaderID:              42,
		ValidationStatus:        model.FileValidationStatusBlocked,
		ValidationPolicyVersion: model.FileUploadPolicyVersion,
		ValidationErrorCode:     string(uploadsecurity.CodeFileContentInvalid),
		ValidatedAt:             &validatedAt,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}

	list, total, err := ListFiles(1, 10, "")
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("ListFiles() total=%d len=%d, want 1 and 1", total, len(list))
	}

	got := list[0]
	if got.DetectedContentType != file.DetectedContentType ||
		got.ValidationStatus != file.ValidationStatus ||
		got.ValidationPolicyVersion != file.ValidationPolicyVersion ||
		got.ValidationErrorCode != file.ValidationErrorCode ||
		got.ValidatedAt != "2026-07-14 09:10:11" {
		t.Fatalf("ListFiles() validation metadata = %#v, want values from file record", got)
	}
}

func TestListFilesOrdersPaginatesFiltersAndPreservesValidationStates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	baseTime := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	files := []model.File{
		{
			Model:                   gorm.Model{CreatedAt: baseTime},
			Name:                    "legacy.pdf",
			Bucket:                  "files",
			ObjectName:              "team/legacy.pdf",
			ContentType:             "application/pdf",
			DetectedContentType:     "application/pdf",
			ValidationStatus:        model.FileValidationStatusLegacyUnverified,
			ValidationErrorCode:     "",
			ValidationPolicyVersion: "",
		},
		{
			Model:                   gorm.Model{CreatedAt: baseTime.Add(time.Hour)},
			Name:                    "retry.pdf",
			Bucket:                  "files",
			ObjectName:              "other/retry.pdf",
			ContentType:             "application/pdf",
			DetectedContentType:     "application/pdf",
			ValidationStatus:        model.FileValidationStatusValidationError,
			ValidationErrorCode:     string(uploadsecurity.CodeStorageUnavailable),
			ValidationPolicyVersion: model.FileUploadPolicyVersion,
		},
		{
			Model:                   gorm.Model{CreatedAt: baseTime.Add(2 * time.Hour)},
			Name:                    "blocked.pdf",
			Bucket:                  "files",
			ObjectName:              "team/blocked.pdf",
			ContentType:             "application/pdf",
			DetectedContentType:     "application/pdf",
			ValidationStatus:        model.FileValidationStatusBlocked,
			ValidationErrorCode:     string(uploadsecurity.CodeFileContentInvalid),
			ValidationPolicyVersion: model.FileUploadPolicyVersion,
		},
		{
			Model:                   gorm.Model{CreatedAt: baseTime.Add(3 * time.Hour)},
			Name:                    "validated.png",
			Bucket:                  "files",
			ObjectName:              "team/validated.png",
			ContentType:             "image/png",
			DetectedContentType:     "image/png",
			ValidationStatus:        model.FileValidationStatusValidated,
			ValidationPolicyVersion: model.FileUploadPolicyVersion,
		},
	}
	for i := range files {
		if err := db.Create(&files[i]).Error; err != nil {
			t.Fatalf("create file %q: %v", files[i].ObjectName, err)
		}
	}

	firstPage, total, err := ListFiles(1, 2, "team/")
	if err != nil {
		t.Fatalf("ListFiles(first page) error = %v", err)
	}
	if total != 3 || len(firstPage) != 2 {
		t.Fatalf("ListFiles(first page) total=%d len=%d, want 3 and 2", total, len(firstPage))
	}
	if firstPage[0].Name != "validated.png" ||
		firstPage[0].ValidationStatus != model.FileValidationStatusValidated ||
		firstPage[1].Name != "blocked.pdf" ||
		firstPage[1].ValidationStatus != model.FileValidationStatusBlocked ||
		firstPage[1].ValidationErrorCode != string(uploadsecurity.CodeFileContentInvalid) {
		t.Fatalf("ListFiles(first page) = %#v, want newest matching records with exact states", firstPage)
	}

	secondPage, total, err := ListFiles(2, 2, "team/")
	if err != nil {
		t.Fatalf("ListFiles(second page) error = %v", err)
	}
	if total != 3 || len(secondPage) != 1 {
		t.Fatalf("ListFiles(second page) total=%d len=%d, want 3 and 1", total, len(secondPage))
	}
	if secondPage[0].Name != "legacy.pdf" ||
		secondPage[0].ValidationStatus != model.FileValidationStatusLegacyUnverified {
		t.Fatalf("ListFiles(second page) = %#v, want legacy matching record", secondPage)
	}

	allFiles, total, err := ListFiles(1, 10, "")
	if err != nil {
		t.Fatalf("ListFiles(all) error = %v", err)
	}
	if total != 4 || len(allFiles) != 4 {
		t.Fatalf("ListFiles(all) total=%d len=%d, want 4 and 4", total, len(allFiles))
	}
	if allFiles[2].Name != "retry.pdf" ||
		allFiles[2].ValidationStatus != model.FileValidationStatusValidationError ||
		allFiles[2].ValidationErrorCode != string(uploadsecurity.CodeStorageUnavailable) {
		t.Fatalf("ListFiles(all) retry record = %#v, want validation_error preserved", allFiles[2])
	}
}

func TestGetFileReturnsValidationMetadataWithoutStorageURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	previousMinIO := global.Minio
	global.DB = db
	global.Minio = nil
	t.Cleanup(func() {
		global.DB = previousDB
		global.Minio = previousMinIO
	})

	validatedAt := time.Date(2026, 7, 14, 9, 10, 11, 0, time.UTC)
	file := model.File{
		Name:                    "legacy.pdf",
		Bucket:                  "files",
		ObjectName:              "private-object.pdf",
		ContentType:             "application/pdf",
		DetectedContentType:     "application/pdf",
		Size:                    128,
		UploaderID:              42,
		ValidationStatus:        model.FileValidationStatusLegacyUnverified,
		ValidationPolicyVersion: model.FileUploadPolicyVersion,
		ValidationErrorCode:     string(uploadsecurity.CodeStorageUnavailable),
		ValidatedAt:             &validatedAt,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}

	info, err := GetFile(file.ID)
	if err != nil {
		t.Fatalf("GetFile() error = %v", err)
	}
	if info.ValidationStatus != file.ValidationStatus ||
		info.DetectedContentType != file.DetectedContentType ||
		info.ValidationPolicyVersion != file.ValidationPolicyVersion ||
		info.ValidationErrorCode != file.ValidationErrorCode ||
		info.ValidatedAt != "2026-07-14 09:10:11" {
		t.Fatalf("GetFile() validation metadata = %#v, want values from file record", info)
	}
}

func TestFileDetailServiceGeneratesDownloadURLWithoutPreviewForValidatedFile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	file := model.File{
		Name:             "report.pdf",
		Bucket:           "files",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusValidated,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}

	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	now := time.Unix(1_700_000_000, 0)
	details := NewFileDetailService(signer, 300, func() time.Time { return now })

	result, err := details.Get(context.Background(), 42, file.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.File == nil || result.File.ID != file.ID {
		t.Fatalf("Get() file = %#v, want file ID %d", result.File, file.ID)
	}

	for _, access := range []struct {
		name   string
		rawURL string
		mode   fileaccess.Mode
		path   string
	}{
		{"download", result.DownloadURL, fileaccess.ModeDownload, "/api/admin/files/" + strconv.FormatUint(uint64(file.ID), 10) + "/download"},
	} {
		t.Run(access.name, func(t *testing.T) {
			parsed, err := url.Parse(access.rawURL)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			if parsed.Path != access.path {
				t.Fatalf("URL path = %q, want %q", parsed.Path, access.path)
			}

			expires, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
			if err != nil {
				t.Fatalf("parse expires: %v", err)
			}
			if expires != now.Add(300*time.Second).Unix() {
				t.Fatalf("expires = %d, want %d", expires, now.Add(300*time.Second).Unix())
			}
			claims := fileaccess.Claims{
				UserID:           42,
				FileID:           file.ID,
				Mode:             access.mode,
				ExpiresAt:        expires,
				ValidationStatus: file.ValidationStatus,
			}
			if err := signer.Verify(claims, parsed.Query().Get("signature"), now); err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
		})
	}
}

func TestFileDetailServiceClassifiesMissingRecordWithoutExposingQueryDetails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	details := NewFileDetailService(signer, 300, time.Now)

	_, err = details.Get(context.Background(), 42, 999)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileNotFound {
		t.Fatalf("Get() code = %q, classified=%v, want %q; error=%v",
			code, ok, uploadsecurity.CodeFileNotFound, err)
	}
}

func TestFileDetailServiceRestrictsURLsByValidationStatusAndType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	details := NewFileDetailService(
		signer,
		300,
		func() time.Time { return time.Unix(1_700_000_000, 0) },
	)

	tests := []struct {
		name         string
		status       string
		contentType  string
		wantDownload bool
	}{
		{"validated PDF", model.FileValidationStatusValidated, "application/pdf", true},
		{"validated TXT", model.FileValidationStatusValidated, "text/plain", true},
		{"validated CSV", model.FileValidationStatusValidated, "text/csv", true},
		{"stale validated image", model.FileValidationStatusValidated, "image/png", false},
		{"legacy file", model.FileValidationStatusLegacyUnverified, "image/png", true},
		{"temporary validation error", model.FileValidationStatusValidationError, "image/png", true},
		{"blocked image", model.FileValidationStatusBlocked, "image/png", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := model.File{
				Name:             "document",
				Bucket:           "files",
				ObjectName:       "private-object",
				ContentType:      tt.contentType,
				ValidationStatus: tt.status,
			}
			if err := db.Create(&file).Error; err != nil {
				t.Fatalf("create file: %v", err)
			}

			result, err := details.Get(context.Background(), 42, file.ID)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got := result.DownloadURL != ""; got != tt.wantDownload {
				t.Fatalf("download URL present = %v, want %v (%q)", got, tt.wantDownload, result.DownloadURL)
			}
		})
	}
}

func TestResolveDownloadAccessUsesCanonicalMIMEForValidatedFile(t *testing.T) {
	file := model.File{
		Name:             "report.pdf",
		Bucket:           "files-cold",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusValidated,
	}

	decision, err := ResolveDownloadAccess(&file)
	if err != nil {
		t.Fatalf("ResolveDownloadAccess() error = %v", err)
	}
	if decision.ContentType != "application/pdf" {
		t.Fatalf("ContentType = %q, want application/pdf", decision.ContentType)
	}
	if decision.Disposition != FileDispositionAttachment {
		t.Fatalf("Disposition = %q, want %q", decision.Disposition, FileDispositionAttachment)
	}
	if decision.FileName != file.Name ||
		decision.Bucket != file.Bucket ||
		decision.ObjectName != file.ObjectName ||
		decision.ValidationStatus != file.ValidationStatus {
		t.Fatalf("decision = %#v, want trusted file access metadata", decision)
	}
}

func TestResolveDownloadAccessRejectsValidatedTypeOutsideManagedWhitelist(t *testing.T) {
	for _, contentType := range []string{
		"image/jpeg",
		"image/png",
		"image/webp",
	} {
		t.Run(contentType, func(t *testing.T) {
			_, err := ResolveDownloadAccess(&model.File{
				Name:             "stale-validated-image",
				Bucket:           "files",
				ObjectName:       "private-object",
				ContentType:      contentType,
				ValidationStatus: model.FileValidationStatusValidated,
			})
			assertServiceUploadCode(t, err, uploadsecurity.CodeFileStateConflict)
		})
	}
}

func TestResolveDownloadAccessForcesUnverifiedFilesToBinaryAttachment(t *testing.T) {
	for _, status := range []string{
		model.FileValidationStatusLegacyUnverified,
		model.FileValidationStatusValidationError,
	} {
		t.Run(status, func(t *testing.T) {
			file := model.File{
				Name:             "historical-image.png",
				Bucket:           "files-cold",
				ObjectName:       "private-object.png",
				ContentType:      "image/png",
				ValidationStatus: status,
			}

			decision, err := ResolveDownloadAccess(&file)
			if err != nil {
				t.Fatalf("ResolveDownloadAccess() error = %v", err)
			}
			if decision.ContentType != "application/octet-stream" {
				t.Fatalf("ContentType = %q, want application/octet-stream", decision.ContentType)
			}
			if decision.Disposition != FileDispositionAttachment {
				t.Fatalf("Disposition = %q, want %q", decision.Disposition, FileDispositionAttachment)
			}
			if decision.ValidationStatus != status {
				t.Fatalf("ValidationStatus = %q, want %q", decision.ValidationStatus, status)
			}
		})
	}
}

func TestResolveDownloadAccessRejectsBlockedFile(t *testing.T) {
	_, err := ResolveDownloadAccess(&model.File{
		Name:             "blocked.pdf",
		Bucket:           "files",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusBlocked,
	})

	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileStateBlocked {
		t.Fatalf("error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodeFileStateBlocked, err)
	}
}

func TestResolvePreviewAccessRejectsAllManagedFiles(t *testing.T) {
	for _, contentType := range []string{
		"application/pdf",
		"text/plain",
		"text/csv",
		"image/jpeg",
		"image/png",
		"image/webp",
	} {
		t.Run(contentType, func(t *testing.T) {
			file := model.File{
				Name:             "preview-candidate",
				Bucket:           "files",
				ObjectName:       "private-object",
				ContentType:      contentType,
				ValidationStatus: model.FileValidationStatusValidated,
			}

			_, err := ResolvePreviewAccess(&file)
			assertServiceUploadCode(t, err, uploadsecurity.CodeFileStateConflict)
		})
	}
}

func TestResolvePreviewAccessRejectsNonImagesAndDisallowedStates(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		contentType string
		wantCode    uploadsecurity.Code
	}{
		{"validated PDF", model.FileValidationStatusValidated, "application/pdf", uploadsecurity.CodeFileStateConflict},
		{"legacy image", model.FileValidationStatusLegacyUnverified, "image/png", uploadsecurity.CodeFileStateConflict},
		{"validation error image", model.FileValidationStatusValidationError, "image/png", uploadsecurity.CodeFileStateConflict},
		{"blocked image", model.FileValidationStatusBlocked, "image/png", uploadsecurity.CodeFileStateConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolvePreviewAccess(&model.File{
				Name:             "preview-candidate",
				Bucket:           "files",
				ObjectName:       "private-object",
				ContentType:      tt.contentType,
				ValidationStatus: tt.status,
			})
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("error code = %q, classified=%v, want %q (error: %v)",
					code, ok, tt.wantCode, err)
			}
		})
	}
}

func TestFileContentServiceVerifiesSignedDownloadBeforeOpeningTrustedObject(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 31},
		Name:             "季度 报告.pdf",
		Bucket:           "files-cold",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusValidated,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{openContent: "trusted object content"}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signature, err := signer.Sign(fileaccess.Claims{
		UserID:           42,
		FileID:           file.ID,
		Mode:             fileaccess.ModeDownload,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	content, err := contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signature,
		Mode:      fileaccess.ModeDownload,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer content.Reader.Close()

	if repository.findCalls != 1 {
		t.Fatalf("FindByID() calls = %d, want 1", repository.findCalls)
	}
	if store.openCalls != 1 ||
		store.openBucket != file.Bucket ||
		store.openName != file.ObjectName {
		t.Fatalf("storage Open() calls=%d bucket=%q name=%q, want trusted record location",
			store.openCalls, store.openBucket, store.openName)
	}
	if content.FileName != file.Name ||
		content.ContentType != "application/pdf" ||
		content.Disposition != FileDispositionAttachment {
		t.Fatalf("FileContent = %#v, want trusted attachment metadata", content)
	}
	body, err := io.ReadAll(content.Reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != store.openContent {
		t.Fatalf("content = %q, want %q", body, store.openContent)
	}
}

func TestFileContentServiceRejectsPreviewBeforeOpeningObject(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 32},
		Name:             "historical-image.png",
		Bucket:           "files-cold",
		ObjectName:       "private-object.png",
		ContentType:      "image/png",
		ValidationStatus: model.FileValidationStatusValidated,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{openContent: "should not be opened"}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signature, err := signer.Sign(fileaccess.Claims{
		UserID:           42,
		FileID:           file.ID,
		Mode:             fileaccess.ModePreview,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	_, err = contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signature,
		Mode:      fileaccess.ModePreview,
	})
	assertServiceUploadCode(t, err, uploadsecurity.CodeFileStateConflict)
	if store.openCalls != 0 {
		t.Fatalf("storage Open() calls = %d, want 0", store.openCalls)
	}
}

func TestFileContentServiceAllowsEmptyHistoricalDownload(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 33},
		Name:             "empty-history.bin",
		Bucket:           "files-cold",
		ObjectName:       "empty-history-object",
		ContentType:      "application/octet-stream",
		ValidationStatus: model.FileValidationStatusLegacyUnverified,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signature, err := signer.Sign(fileaccess.Claims{
		UserID:           42,
		FileID:           file.ID,
		Mode:             fileaccess.ModeDownload,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	content, err := contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signature,
		Mode:      fileaccess.ModeDownload,
	})
	if err != nil {
		t.Fatalf("stable repro: Open() rejected a zero-byte historical object: %v", err)
	}
	defer content.Reader.Close()

	body, err := io.ReadAll(content.Reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("download body = %q, want empty attachment", body)
	}
	if content.ContentType != "application/octet-stream" ||
		content.Disposition != FileDispositionAttachment {
		t.Fatalf("FileContent = %#v, want binary attachment metadata", content)
	}
}

func TestFileContentServicePreservesErrorReturnedWithFirstByte(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 34},
		Name:             "report.pdf",
		Bucket:           "files",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusValidated,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	providerErr := uploadsecurity.NewError(
		uploadsecurity.CodeStorageUnavailable,
		errors.New("provider failed after returning first byte"),
	)
	reader := &firstReadDataErrorCloser{err: providerErr}
	store := &recordingFileStore{openReader: reader}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signature, err := signer.Sign(fileaccess.Claims{
		UserID:           42,
		FileID:           file.ID,
		Mode:             fileaccess.ModeDownload,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	content, err := contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signature,
		Mode:      fileaccess.ModeDownload,
	})
	if content != nil && content.Reader != nil {
		_ = content.Reader.Close()
	}
	code, classified := uploadsecurity.CodeOf(err)
	if !classified || code != uploadsecurity.CodeStorageUnavailable {
		t.Fatalf(
			"stable repro: first Read() returned data with a storage error, "+
				"but Open() returned code=%q classified=%v content=%#v error=%v",
			code,
			classified,
			content,
			err,
		)
	}
	if !reader.closed {
		t.Fatal("object reader was not closed after first-byte storage failure")
	}
}

func TestFileContentServiceRejectsClassifiedEOFReturnedWithFirstByte(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 35},
		Name:             "report.pdf",
		Bucket:           "files",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusValidated,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	providerErr := uploadsecurity.NewError(
		uploadsecurity.CodeStorageUnavailable,
		io.EOF,
	)
	reader := &firstReadDataErrorCloser{err: providerErr}
	store := &recordingFileStore{openReader: reader}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signature, err := signer.Sign(fileaccess.Claims{
		UserID:           42,
		FileID:           file.ID,
		Mode:             fileaccess.ModeDownload,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	content, err := contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signature,
		Mode:      fileaccess.ModeDownload,
	})
	if content != nil && content.Reader != nil {
		_ = content.Reader.Close()
	}
	code, classified := uploadsecurity.CodeOf(err)
	if !classified || code != uploadsecurity.CodeStorageUnavailable {
		t.Fatalf(
			"classified EOF returned with data was treated as success: "+
				"code=%q classified=%v content=%#v error=%v",
			code,
			classified,
			content,
			err,
		)
	}
	if !reader.closed {
		t.Fatal("object reader was not closed after classified EOF storage failure")
	}
}

func TestFileContentServiceRejectsClassifiedEOFForEmptyHistoricalObject(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 36},
		Name:             "empty-history.bin",
		Bucket:           "files-cold",
		ObjectName:       "empty-history-object",
		ContentType:      "application/octet-stream",
		ValidationStatus: model.FileValidationStatusLegacyUnverified,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	providerErr := uploadsecurity.NewError(
		uploadsecurity.CodeStorageUnavailable,
		io.EOF,
	)
	reader := &firstReadErrorCloser{err: providerErr}
	store := &recordingFileStore{openReader: reader}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signature, err := signer.Sign(fileaccess.Claims{
		UserID:           42,
		FileID:           file.ID,
		Mode:             fileaccess.ModeDownload,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	content, err := contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signature,
		Mode:      fileaccess.ModeDownload,
	})
	if content != nil && content.Reader != nil {
		_ = content.Reader.Close()
	}
	code, classified := uploadsecurity.CodeOf(err)
	if !classified || code != uploadsecurity.CodeStorageUnavailable {
		t.Fatalf(
			"classified EOF returned without data was treated as an empty file: "+
				"code=%q classified=%v content=%#v error=%v",
			code,
			classified,
			content,
			err,
		)
	}
	if !reader.closed {
		t.Fatal("object reader was not closed after classified EOF storage failure")
	}
}

func TestFileContentServiceRejectsInvalidSignatureBeforeOpeningObject(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	file := model.File{
		Model:            gorm.Model{ID: 32},
		Name:             "report.pdf",
		Bucket:           "files",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ValidationStatus: model.FileValidationStatusValidated,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{openContent: "must not be opened"}
	signer, err := fileaccess.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	expiresAt := now.Add(5 * time.Minute).Unix()
	signatureForAnotherUser, err := signer.Sign(fileaccess.Claims{
		UserID:           99,
		FileID:           file.ID,
		Mode:             fileaccess.ModeDownload,
		ExpiresAt:        expiresAt,
		ValidationStatus: file.ValidationStatus,
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	contents := NewFileContentService(
		signer,
		store,
		repository,
		func() time.Time { return now },
	)

	_, err = contents.Open(context.Background(), FileAccessInput{
		UserID:    42,
		FileID:    file.ID,
		ExpiresAt: expiresAt,
		Signature: signatureForAnotherUser,
		Mode:      fileaccess.ModeDownload,
	})
	code, classified := uploadsecurity.CodeOf(err)
	if !classified || code != uploadsecurity.CodeFileAccessInvalid {
		t.Fatalf("error code = %q, classified=%v, want %q (error: %v)",
			code, classified, uploadsecurity.CodeFileAccessInvalid, err)
	}
	if store.openCalls != 0 {
		t.Fatalf("storage Open() calls = %d, want 0", store.openCalls)
	}
}

func TestFileRevalidationServiceMarksPassingHistoricalFileValidatedWithoutRewritingObject(t *testing.T) {
	const (
		oldDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		newDigest = "1111111111111111111111111111111111111111111111111111111111111111"
	)
	validatedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	file := model.File{
		Model:            gorm.Model{ID: 21},
		Name:             "historical.pdf",
		Bucket:           "files-cold",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ContentSHA256:    oldDigest,
		Size:             15,
		ValidationStatus: model.FileValidationStatusLegacyUnverified,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{openContent: "historical-pdf"}
	validator := &recordingFileValidator{
		result: uploadsecurity.Result{
			FileName:           "historical.pdf",
			CanonicalType:      uploadsecurity.TypePDF,
			CanonicalExtension: ".pdf",
			CanonicalMIME:      "application/pdf",
			DetectedMIME:       "application/pdf",
			Size:               file.Size,
			ContentSHA256:      newDigest,
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("validated-copy"),
		},
	}
	revalidation := NewFileRevalidationService(
		validator,
		store,
		repository,
		1024,
		func() time.Time { return validatedAt },
	)

	info, err := revalidation.Revalidate(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("Revalidate() error = %v", err)
	}
	if store.openCalls != 1 ||
		store.openBucket != file.Bucket ||
		store.openName != file.ObjectName {
		t.Fatalf("storage Open() calls=%d bucket=%q name=%q, want trusted record location",
			store.openCalls, store.openBucket, store.openName)
	}
	if store.putCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("storage mutations: put=%d delete=%d, want 0 and 0",
			store.putCalls, store.deleteCalls)
	}
	if validator.input.Purpose != uploadsecurity.PurposeManagedFile ||
		validator.input.FileName != file.Name ||
		validator.input.DeclaredMIME != file.ContentType ||
		validator.input.Size != file.Size ||
		validator.input.MaxBytes != 1024 ||
		validator.input.Reader == nil {
		t.Fatalf("validator input = %#v, want historical record metadata and opened object", validator.input)
	}
	if repository.updateCalls != 1 ||
		repository.update.ContentType != "application/pdf" ||
		repository.update.DetectedContentType != "application/pdf" ||
		repository.update.ContentSHA256 != newDigest ||
		repository.update.Status != model.FileValidationStatusValidated ||
		repository.update.PolicyVersion != uploadsecurity.PolicyVersionV1 ||
		repository.update.ErrorCode != "" ||
		repository.update.ValidatedAt == nil ||
		!repository.update.ValidatedAt.Equal(validatedAt) {
		t.Fatalf("validation update = %#v, want validated metadata", repository.update)
	}
	if info == nil ||
		info.ValidationStatus != model.FileValidationStatusValidated ||
		info.ContentType != "application/pdf" ||
		info.DetectedContentType != "application/pdf" ||
		info.ContentSHA256 != newDigest ||
		info.ValidationPolicyVersion != uploadsecurity.PolicyVersionV1 ||
		info.ValidationErrorCode != "" ||
		info.ValidatedAt != "2026-07-14 12:00:00" {
		t.Fatalf("Revalidate() info = %#v, want updated validated file", info)
	}
}

func TestFileRevalidationServicePreservesStorageFailureFromObjectRead(t *testing.T) {
	const oldDigest = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	validatedAt := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	file := model.File{
		Model:            gorm.Model{ID: 22},
		Name:             "historical.pdf",
		Bucket:           "files-cold",
		ObjectName:       "private-object.pdf",
		ContentType:      "application/pdf",
		ContentSHA256:    oldDigest,
		Size:             14,
		ValidationStatus: model.FileValidationStatusLegacyUnverified,
	}
	repository := &recordingFileRevalidationRepository{file: file}
	providerErr := uploadsecurity.NewError(
		uploadsecurity.CodeStorageUnavailable,
		errors.New("provider unavailable during first object read"),
	)
	reader := &firstReadErrorCloser{err: providerErr}
	store := &recordingFileStore{openReader: reader}
	revalidation := NewFileRevalidationService(
		uploadsecurity.NewManagedFileValidator(),
		store,
		repository,
		1024,
		func() time.Time { return validatedAt },
	)

	_, err := revalidation.Revalidate(context.Background(), file.ID)

	code, classified := uploadsecurity.CodeOf(err)
	httpError := uploadsecurity.ToHTTPError(err)
	if !classified ||
		code != uploadsecurity.CodeStorageUnavailable ||
		httpError.Status != http.StatusServiceUnavailable {
		t.Fatalf(
			"first object read failure mapped to code=%q status=%d classified=%v; "+
				"want %q/%d (persisted status=%q reason=%q)",
			code,
			httpError.Status,
			classified,
			uploadsecurity.CodeStorageUnavailable,
			http.StatusServiceUnavailable,
			repository.file.ValidationStatus,
			repository.file.ValidationErrorCode,
		)
	}
	if repository.updateCalls != 1 ||
		repository.file.ValidationStatus != model.FileValidationStatusValidationError ||
		repository.file.ContentSHA256 != oldDigest ||
		repository.file.ValidationErrorCode != string(uploadsecurity.CodeStorageUnavailable) {
		t.Fatalf(
			"validation failure update = %#v, persisted status=%q reason=%q; "+
				"want validation_error/%q",
			repository.update,
			repository.file.ValidationStatus,
			repository.file.ValidationErrorCode,
			uploadsecurity.CodeStorageUnavailable,
		)
	}
	if !reader.closed {
		t.Fatal("object reader was not closed after revalidation failure")
	}
}

func TestFileRevalidationServiceMarksPolicyRejectionBlockedWithoutDeletingObject(t *testing.T) {
	const oldDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	validatedAt := time.Date(2026, 7, 14, 12, 30, 0, 0, time.UTC)
	file := model.File{
		Model:                   gorm.Model{ID: 22},
		Name:                    "historical.pdf",
		Bucket:                  "files",
		ObjectName:              "private-object.pdf",
		ContentType:             "application/pdf",
		DetectedContentType:     "application/pdf",
		ContentSHA256:           oldDigest,
		Size:                    128,
		ValidationStatus:        model.FileValidationStatusLegacyUnverified,
		ValidationErrorCode:     "",
		ValidationPolicyVersion: "",
	}
	policyErr := uploadsecurity.NewError(uploadsecurity.CodeFileContentInvalid, errors.New("invalid PDF structure"))
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{openContent: "invalid-pdf"}
	validator := &recordingFileValidator{err: policyErr}
	revalidation := NewFileRevalidationService(
		validator,
		store,
		repository,
		1024,
		func() time.Time { return validatedAt },
	)

	_, err := revalidation.Revalidate(context.Background(), file.ID)
	if err != policyErr {
		t.Fatalf("Revalidate() error = %v, want policy error %v", err, policyErr)
	}
	if repository.updateCalls != 1 ||
		repository.update.ContentType != file.ContentType ||
		repository.update.DetectedContentType != file.DetectedContentType ||
		repository.update.ContentSHA256 != oldDigest ||
		repository.update.Status != model.FileValidationStatusBlocked ||
		repository.update.PolicyVersion != uploadsecurity.PolicyVersionV1 ||
		repository.update.ErrorCode != string(uploadsecurity.CodeFileContentInvalid) ||
		repository.update.ValidatedAt == nil ||
		!repository.update.ValidatedAt.Equal(validatedAt) {
		t.Fatalf("validation update = %#v, want blocked policy result", repository.update)
	}
	if store.putCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("storage mutations: put=%d delete=%d, want 0 and 0",
			store.putCalls, store.deleteCalls)
	}
}

func TestFileRevalidationServiceBlocksHistoricalTypesOutsideManagedWhitelist(t *testing.T) {
	const oldDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	tests := []struct {
		name        string
		fileName    string
		contentType string
	}{
		{
			name:        "historical image",
			fileName:    "historical.png",
			contentType: "image/png",
		},
		{
			name:        "historical office document",
			fileName:    "historical.docx",
			contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validatedAt := time.Date(2026, 7, 14, 12, 45, 0, 0, time.UTC)
			file := model.File{
				Model:               gorm.Model{ID: 26},
				Name:                tt.fileName,
				Bucket:              "files",
				ObjectName:          "private-object",
				ContentType:         tt.contentType,
				ContentSHA256:       oldDigest,
				Size:                16,
				ValidationStatus:    model.FileValidationStatusLegacyUnverified,
				ValidationErrorCode: "",
			}
			repository := &recordingFileRevalidationRepository{file: file}
			store := &recordingFileStore{openContent: "historical-data"}
			revalidation := NewFileRevalidationService(
				uploadsecurity.NewManagedFileValidator(),
				store,
				repository,
				1024,
				func() time.Time { return validatedAt },
			)

			_, err := revalidation.Revalidate(context.Background(), file.ID)
			assertServiceUploadCode(t, err, uploadsecurity.CodeFileTypeNotAllowed)
			if repository.updateCalls != 1 ||
				repository.update.ContentSHA256 != oldDigest ||
				repository.update.Status != model.FileValidationStatusBlocked ||
				repository.update.ErrorCode != string(uploadsecurity.CodeFileTypeNotAllowed) ||
				repository.update.ValidatedAt == nil ||
				!repository.update.ValidatedAt.Equal(validatedAt) {
				t.Fatalf("validation update = %#v, want blocked result preserving prior digest", repository.update)
			}
			if store.openCalls != 1 || store.putCalls != 0 || store.deleteCalls != 0 {
				t.Fatalf("storage calls: open=%d put=%d delete=%d, want 1, 0 and 0",
					store.openCalls, store.putCalls, store.deleteCalls)
			}
		})
	}
}

func TestFileRevalidationServiceMarksStorageFailureValidationErrorForRetry(t *testing.T) {
	const oldDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	validatedAt := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	file := model.File{
		Model:                   gorm.Model{ID: 23},
		Name:                    "historical.pdf",
		Bucket:                  "files",
		ObjectName:              "private-object.pdf",
		ContentType:             "application/pdf",
		DetectedContentType:     "application/pdf",
		ContentSHA256:           oldDigest,
		Size:                    128,
		ValidationStatus:        model.FileValidationStatusLegacyUnverified,
		ValidationPolicyVersion: "",
	}
	repository := &recordingFileRevalidationRepository{file: file}
	store := &recordingFileStore{openErr: errors.New("provider unavailable")}
	validator := &recordingFileValidator{}
	revalidation := NewFileRevalidationService(
		validator,
		store,
		repository,
		1024,
		func() time.Time { return validatedAt },
	)

	_, err := revalidation.Revalidate(context.Background(), file.ID)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeStorageUnavailable {
		t.Fatalf("error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodeStorageUnavailable, err)
	}
	if repository.updateCalls != 1 ||
		repository.update.ContentType != file.ContentType ||
		repository.update.DetectedContentType != file.DetectedContentType ||
		repository.update.ContentSHA256 != oldDigest ||
		repository.update.Status != model.FileValidationStatusValidationError ||
		repository.update.PolicyVersion != uploadsecurity.PolicyVersionV1 ||
		repository.update.ErrorCode != string(uploadsecurity.CodeStorageUnavailable) ||
		repository.update.ValidatedAt == nil ||
		!repository.update.ValidatedAt.Equal(validatedAt) {
		t.Fatalf("validation update = %#v, want retryable validation error", repository.update)
	}
	if validator.input.Reader != nil {
		t.Fatalf("validator called after storage Open failure: %#v", validator.input)
	}
	if store.putCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("storage mutations: put=%d delete=%d, want 0 and 0",
			store.putCalls, store.deleteCalls)
	}
}

func TestFileRevalidationServiceMarksTemporaryValidationFailureForRetry(t *testing.T) {
	for _, errorCode := range []uploadsecurity.Code{
		uploadsecurity.CodeStorageUnavailable,
		uploadsecurity.CodeUploadBodyInvalid,
	} {
		t.Run(string(errorCode), func(t *testing.T) {
			validatedAt := time.Date(2026, 7, 14, 13, 30, 0, 0, time.UTC)
			file := model.File{
				Model:               gorm.Model{ID: 24},
				Name:                "historical.pdf",
				Bucket:              "files",
				ObjectName:          "private-object.pdf",
				ContentType:         "application/pdf",
				ContentSHA256:       "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				Size:                128,
				ValidationStatus:    model.FileValidationStatusValidationError,
				ValidationErrorCode: string(uploadsecurity.CodeStorageUnavailable),
			}
			repository := &recordingFileRevalidationRepository{file: file}
			store := &recordingFileStore{openContent: "partial-object"}
			validationErr := uploadsecurity.NewError(errorCode, errors.New("temporary read failure"))
			validator := &recordingFileValidator{err: validationErr}
			revalidation := NewFileRevalidationService(
				validator,
				store,
				repository,
				1024,
				func() time.Time { return validatedAt },
			)

			_, err := revalidation.Revalidate(context.Background(), file.ID)
			if err != validationErr {
				t.Fatalf("Revalidate() error = %v, want temporary error %v", err, validationErr)
			}
			if repository.updateCalls != 1 ||
				repository.update.ContentSHA256 != file.ContentSHA256 ||
				repository.update.Status != model.FileValidationStatusValidationError ||
				repository.update.PolicyVersion != uploadsecurity.PolicyVersionV1 ||
				repository.update.ErrorCode != string(errorCode) ||
				repository.update.ValidatedAt == nil ||
				!repository.update.ValidatedAt.Equal(validatedAt) {
				t.Fatalf("validation update = %#v, want retryable validation error", repository.update)
			}
			if store.putCalls != 0 || store.deleteCalls != 0 {
				t.Fatalf("storage mutations: put=%d delete=%d, want 0 and 0",
					store.putCalls, store.deleteCalls)
			}
		})
	}
}

func TestFileRevalidationServiceRejectsFinalStatesBeforeOpeningObject(t *testing.T) {
	for _, status := range []string{
		model.FileValidationStatusValidated,
		model.FileValidationStatusBlocked,
	} {
		t.Run(status, func(t *testing.T) {
			file := model.File{
				Model:            gorm.Model{ID: 25},
				Name:             "final-state.pdf",
				Bucket:           "files",
				ObjectName:       "private-object.pdf",
				ContentType:      "application/pdf",
				Size:             128,
				ValidationStatus: status,
			}
			repository := &recordingFileRevalidationRepository{file: file}
			store := &recordingFileStore{}
			revalidation := NewFileRevalidationService(
				&recordingFileValidator{},
				store,
				repository,
				1024,
				time.Now,
			)

			_, err := revalidation.Revalidate(context.Background(), file.ID)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeFileStateConflict {
				t.Fatalf("error code = %q, classified=%v, want %q (error: %v)",
					code, ok, uploadsecurity.CodeFileStateConflict, err)
			}
			if store.openCalls != 0 || repository.updateCalls != 0 {
				t.Fatalf("side effects: open=%d update=%d, want 0 and 0",
					store.openCalls, repository.updateCalls)
			}
		})
	}
}

func TestFileBrowseServiceReturnsOnlyObjectMetadataWithoutClaimingObjects(t *testing.T) {
	lastModified := time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC)
	store := &recordingFileStore{
		listObjects: []objectstorage.ObjectInfo{
			{
				Bucket:       fileBucket,
				Name:         "orphan/private-object.pdf",
				Size:         128,
				ContentType:  "application/pdf",
				LastModified: lastModified,
			},
		},
	}
	browse := NewFileBrowseService(store)

	objects, err := browse.List(context.Background(), "orphan/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if store.listCalls != 1 ||
		store.listBucket != fileBucket ||
		store.listOptions.Prefix != "orphan/" ||
		!store.listOptions.Recursive {
		t.Fatalf("storage List() calls=%d bucket=%q options=%#v, want recursive files metadata listing",
			store.listCalls, store.listBucket, store.listOptions)
	}
	if store.putCalls != 0 || store.openCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("storage side effects: put=%d open=%d delete=%d, want 0",
			store.putCalls, store.openCalls, store.deleteCalls)
	}
	if len(objects) != 1 ||
		objects[0].Name != "orphan/private-object.pdf" ||
		objects[0].Size != 128 ||
		objects[0].ContentType != "application/pdf" ||
		!objects[0].LastModified.Equal(lastModified) {
		t.Fatalf("List() objects = %#v, want storage metadata only", objects)
	}

	body, err := json.Marshal(objects)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, forbidden := range []string{
		"bucket",
		"validation_status",
		"validation_policy_version",
		"download_url",
		"preview_url",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("serialized browse metadata contains forbidden field %q: %s", forbidden, body)
		}
	}
}

func TestUpdateFileSanitizesNameBodyAndPreservesTrustedStorageMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.File{}); err != nil {
		t.Fatalf("migrate files: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	file := model.File{
		Name:                    "old.pdf",
		Bucket:                  "files-cold",
		ObjectName:              "private-object.pdf",
		ContentType:             "application/pdf",
		DetectedContentType:     "application/pdf",
		ValidationStatus:        model.FileValidationStatusValidated,
		ValidationPolicyVersion: model.FileUploadPolicyVersion,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}

	info, err := UpdateFile(file.ID, dto.UpdateFileReq{Name: "  季度   报表  "})
	if err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}
	if info.Name != "季度 报表.pdf" {
		t.Fatalf("UpdateFile() name = %q, want %q", info.Name, "季度 报表.pdf")
	}

	var stored model.File
	if err := db.First(&stored, file.ID).Error; err != nil {
		t.Fatalf("reload file: %v", err)
	}
	if stored.Name != "季度 报表.pdf" ||
		stored.Bucket != file.Bucket ||
		stored.ObjectName != file.ObjectName ||
		stored.ContentType != file.ContentType ||
		stored.ValidationStatus != file.ValidationStatus {
		t.Fatalf("stored file = %#v, want renamed display name with trusted metadata unchanged", stored)
	}
}

func TestUpdateFileRejectsUnsafeNamesWithoutMutatingRecord(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCode uploadsecurity.Code
	}{
		{"changes canonical extension", "renamed.txt", uploadsecurity.CodeFileNameInvalid},
		{"contains path", "../renamed", uploadsecurity.CodeFileNameInvalid},
		{"contains control character", "bad\nname", uploadsecurity.CodeFileNameInvalid},
		{"contains bidi control", "safe\u202Ename", uploadsecurity.CodeFileNameInvalid},
		{"contains dangerous extension", "payload.exe", uploadsecurity.CodeFileTypeNotAllowed},
		{"becomes empty after sanitizing", "  ...  ", uploadsecurity.CodeFileNameInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			if err := db.AutoMigrate(&model.File{}); err != nil {
				t.Fatalf("migrate files: %v", err)
			}

			previousDB := global.DB
			global.DB = db
			t.Cleanup(func() {
				global.DB = previousDB
			})

			file := model.File{
				Name:                    "old.pdf",
				Bucket:                  "files-cold",
				ObjectName:              "private-object.pdf",
				ContentType:             "application/pdf",
				DetectedContentType:     "application/pdf",
				ValidationStatus:        model.FileValidationStatusValidated,
				ValidationPolicyVersion: model.FileUploadPolicyVersion,
			}
			if err := db.Create(&file).Error; err != nil {
				t.Fatalf("create file: %v", err)
			}

			_, err = UpdateFile(file.ID, dto.UpdateFileReq{Name: tt.input})
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("UpdateFile() code = %q, classified=%v, want %q (error: %v)",
					code, ok, tt.wantCode, err)
			}

			var stored model.File
			if err := db.First(&stored, file.ID).Error; err != nil {
				t.Fatalf("reload file: %v", err)
			}
			if stored.Name != file.Name ||
				stored.Bucket != file.Bucket ||
				stored.ObjectName != file.ObjectName ||
				stored.ContentType != file.ContentType ||
				stored.ValidationStatus != file.ValidationStatus {
				t.Fatalf("stored file changed after rejected rename: %#v", stored)
			}
		})
	}
}

func TestManagedFileOperationsClassifyMissingRecordsAndPersistenceFailures(t *testing.T) {
	t.Run("update missing file", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		if err := db.AutoMigrate(&model.File{}); err != nil {
			t.Fatalf("migrate files: %v", err)
		}

		previousDB := global.DB
		global.DB = db
		t.Cleanup(func() {
			global.DB = previousDB
		})

		_, err = UpdateFile(999, dto.UpdateFileReq{Name: "report"})
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodeFileNotFound {
			t.Fatalf("UpdateFile() code = %q, classified=%v, want %q; error=%v",
				code, ok, uploadsecurity.CodeFileNotFound, err)
		}
	})

	t.Run("delete missing file", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		if err := db.AutoMigrate(&model.File{}); err != nil {
			t.Fatalf("migrate files: %v", err)
		}

		previousDB := global.DB
		global.DB = db
		t.Cleanup(func() {
			global.DB = previousDB
		})

		err = DeleteFile(999)
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodeFileNotFound {
			t.Fatalf("DeleteFile() code = %q, classified=%v, want %q; error=%v",
				code, ok, uploadsecurity.CodeFileNotFound, err)
		}
	})

	t.Run("list persistence failure", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		if err := db.AutoMigrate(&model.File{}); err != nil {
			t.Fatalf("migrate files: %v", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("unwrap sqlite DB: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close sqlite DB: %v", err)
		}

		previousDB := global.DB
		global.DB = db
		t.Cleanup(func() {
			global.DB = previousDB
		})

		_, _, err = ListFiles(1, 10, "")
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodePersistenceFailed {
			t.Fatalf("ListFiles() code = %q, classified=%v, want %q; error=%v",
				code, ok, uploadsecurity.CodePersistenceFailed, err)
		}
	})
}

func TestFileServiceUsesManagedFileValidationBeforeStorage(t *testing.T) {
	validPDF := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	tests := []struct {
		name        string
		fileName    string
		contentType string
		content     []byte
		maxBytes    int64
		wantCode    uploadsecurity.Code
	}{
		{
			name:        "size exceeds configured limit",
			fileName:    "report.pdf",
			contentType: "application/pdf",
			content:     validPDF,
			maxBytes:    int64(len(validPDF) - 1),
			wantCode:    uploadsecurity.CodeFileTooLarge,
		},
		{
			name:        "filename has dangerous double extension",
			fileName:    "payload.exe.pdf",
			contentType: "application/pdf",
			content:     validPDF,
			maxBytes:    1024,
			wantCode:    uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name:        "declared type disagrees with content",
			fileName:    "report.pdf",
			contentType: "image/png",
			content:     validPDF,
			maxBytes:    1024,
			wantCode:    uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name:        "content is malformed",
			fileName:    "report.pdf",
			contentType: "application/pdf",
			content:     []byte("%PDF-1.7\nmissing EOF marker"),
			maxBytes:    1024,
			wantCode:    uploadsecurity.CodeFileContentInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingFileStore{}
			files := &recordingFileRepository{}
			fileService := NewFileService(
				uploadsecurity.NewManagedFileValidator(),
				store,
				files,
			)

			_, err := fileService.Upload(context.Background(), UploadFileInput{
				UploaderID:  42,
				FileName:    tt.fileName,
				ContentType: tt.contentType,
				Size:        int64(len(tt.content)),
				MaxBytes:    tt.maxBytes,
				Reader:      strings.NewReader(string(tt.content)),
			})

			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("Upload() error code = %q, classified=%v, want %q (error: %v)",
					code, ok, tt.wantCode, err)
			}
			if store.putCalls != 0 {
				t.Fatalf("storage Put() calls = %d, want 0", store.putCalls)
			}
			if files.createCalls != 0 {
				t.Fatalf("repository Create() calls = %d, want 0", files.createCalls)
			}
		})
	}
}

func TestManagedFileUploadEndToEndUsesOnlyValidatedSystemObjectKeys(t *testing.T) {
	validPDF := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	tests := []struct {
		name        string
		fileName    string
		contentType string
		content     []byte
		wantCode    uploadsecurity.Code
	}{
		{
			name:        "valid upload is persisted under a system UUID",
			fileName:    `C:\fakepath\quarterly-report.PDF`,
			contentType: "application/pdf",
			content:     validPDF,
		},
		{
			name:        "unsupported archive is rejected before persistence",
			fileName:    "archive.zip",
			contentType: "application/zip",
			content:     []byte("PK\x03\x04"),
			wantCode:    uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name:        "dangerous double extension is rejected before persistence",
			fileName:    "payload.exe.pdf",
			contentType: "application/pdf",
			content:     validPDF,
			wantCode:    uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name:        "mismatched declaration is rejected before persistence",
			fileName:    "quarterly-report.pdf",
			contentType: "image/png",
			content:     validPDF,
			wantCode:    uploadsecurity.CodeFileTypeNotAllowed,
		},
		{
			name:        "damaged allowed content is rejected before persistence",
			fileName:    "quarterly-report.pdf",
			contentType: "application/pdf",
			content:     []byte("%PDF-1.7\nmissing EOF marker"),
			wantCode:    uploadsecurity.CodeFileContentInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingFileStore{}
			files := &recordingFileRepository{}
			fileService := NewFileService(
				uploadsecurity.NewManagedFileValidator(),
				store,
				files,
			)

			info, err := fileService.Upload(context.Background(), UploadFileInput{
				UploaderID:  42,
				FileName:    tt.fileName,
				ContentType: tt.contentType,
				Size:        int64(len(tt.content)),
				MaxBytes:    1024,
				Reader:      strings.NewReader(string(tt.content)),
			})

			if tt.wantCode != "" {
				code, ok := uploadsecurity.CodeOf(err)
				if !ok || code != tt.wantCode {
					t.Fatalf("Upload() error code = %q, classified=%v, want %q (error: %v)",
						code, ok, tt.wantCode, err)
				}
				if store.putCalls != 0 || files.createCalls != 0 {
					t.Fatalf("rejected upload side effects: put=%d create=%d, want 0 and 0",
						store.putCalls, files.createCalls)
				}
				return
			}

			if err != nil {
				t.Fatalf("Upload() error = %v", err)
			}
			if store.putCalls != 1 || files.createCalls != 1 || info == nil {
				t.Fatalf("accepted upload side effects: put=%d create=%d info=%#v, want 1, 1 and result",
					store.putCalls, files.createCalls, info)
			}
			if strings.Contains(store.putInput.Name, "quarterly-report") ||
				strings.Contains(store.putInput.Name, "fakepath") ||
				!strings.HasSuffix(store.putInput.Name, ".pdf") {
				t.Fatalf("object key = %q, want server UUID plus canonical .pdf extension", store.putInput.Name)
			}
			objectID := strings.TrimSuffix(store.putInput.Name, ".pdf")
			if _, err := uuid.Parse(objectID); err != nil {
				t.Fatalf("object key UUID = %q, parse error: %v", objectID, err)
			}
			if files.file == nil ||
				files.file.ObjectName != store.putInput.Name ||
				files.file.Name != "quarterly-report.pdf" ||
				files.file.ValidationStatus != model.FileValidationStatusValidated {
				t.Fatalf("persisted file = %#v, want trusted metadata and generated object key", files.file)
			}
		})
	}
}

func TestFileServiceDoesNotCreateRecordWhenStorageWriteFails(t *testing.T) {
	const providerDetail = "minio access-key=private-secret"
	store := &recordingFileStore{putErr: errors.New(providerDetail)}
	files := &recordingFileRepository{}
	fileService := NewFileService(
		acceptingFileValidator{result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeManagedFile,
			FileName:           "report.pdf",
			CanonicalType:      uploadsecurity.TypePDF,
			CanonicalExtension: ".pdf",
			CanonicalMIME:      "application/pdf",
			DetectedMIME:       "application/pdf",
			Size:               13,
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("validated-pdf"),
		}},
		store,
		files,
	)

	_, err := fileService.Upload(context.Background(), UploadFileInput{
		UploaderID:  42,
		FileName:    "report.pdf",
		ContentType: "application/pdf",
		Size:        13,
		MaxBytes:    1024,
		Reader:      strings.NewReader("untrusted"),
	})

	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeStorageUnavailable {
		t.Fatalf("Upload() error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodeStorageUnavailable, err)
	}
	if strings.Contains(err.Error(), providerDetail) {
		t.Fatalf("Upload() error %q leaked provider detail", err)
	}
	if files.createCalls != 0 {
		t.Fatalf("repository Create() calls = %d, want 0", files.createCalls)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("storage Delete() calls = %d, want 0", store.deleteCalls)
	}
}

func TestFileServiceDeletesStoredObjectWhenRecordCreationFails(t *testing.T) {
	const (
		databaseDetail = "mysql password=database-secret"
		deleteDetail   = "minio access-key=delete-secret"
		contentDigest  = "4974036fd654294eb5da8a89f1c4ce8863c7c3e3b1c8f5e1f3a00d6e52a446f8"
	)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	originalLogger := global.Logger
	global.Logger = zap.New(logCore)
	t.Cleanup(func() {
		global.Logger = originalLogger
	})

	store := &recordingFileStore{deleteErr: errors.New(deleteDetail)}
	files := &recordingFileRepository{createErr: errors.New(databaseDetail)}
	fileService := NewFileService(
		acceptingFileValidator{result: uploadsecurity.Result{
			Purpose:            uploadsecurity.PurposeManagedFile,
			FileName:           "report.pdf",
			CanonicalType:      uploadsecurity.TypePDF,
			CanonicalExtension: ".pdf",
			CanonicalMIME:      "application/pdf",
			DetectedMIME:       "application/pdf",
			Size:               13,
			ContentSHA256:      contentDigest,
			PolicyVersion:      uploadsecurity.PolicyVersionV1,
			Reader:             strings.NewReader("validated-pdf"),
		}},
		store,
		files,
	)

	_, err := fileService.Upload(context.Background(), UploadFileInput{
		UploaderID:  42,
		FileName:    "report.pdf",
		ContentType: "application/pdf",
		Size:        13,
		MaxBytes:    1024,
		Reader:      strings.NewReader("untrusted"),
	})

	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodePersistenceFailed {
		t.Fatalf("Upload() error code = %q, classified=%v, want %q (error: %v)",
			code, ok, uploadsecurity.CodePersistenceFailed, err)
	}
	if strings.Contains(err.Error(), databaseDetail) ||
		strings.Contains(err.Error(), deleteDetail) {
		t.Fatalf("Upload() error %q leaked infrastructure detail", err)
	}
	if store.deleteCalls != 1 {
		t.Fatalf("storage Delete() calls = %d, want 1", store.deleteCalls)
	}
	if store.deleteBucket != fileBucket || store.deleteName != store.putInput.Name {
		t.Fatalf("deleted object = %q/%q, want uploaded object %q/%q",
			store.deleteBucket, store.deleteName, fileBucket, store.putInput.Name)
	}

	if observedLogs.FilterMessage("managed file record creation failed").Len() != 1 {
		t.Fatalf("record creation failure logs = %d, want 1",
			observedLogs.FilterMessage("managed file record creation failed").Len())
	}
	if observedLogs.FilterMessage("managed file compensation delete failed").Len() != 1 {
		t.Fatalf("compensation failure logs = %d, want 1",
			observedLogs.FilterMessage("managed file compensation delete failed").Len())
	}
	for _, entry := range observedLogs.All() {
		encoded := entry.Message
		for key, value := range entry.ContextMap() {
			encoded += key + "=" + strings.TrimSpace(value.(string))
		}
		if strings.Contains(encoded, databaseDetail) ||
			strings.Contains(encoded, deleteDetail) ||
			strings.Contains(encoded, contentDigest) ||
			strings.Contains(encoded, store.putInput.Name) {
			t.Fatalf("controlled log %q leaked sensitive detail", encoded)
		}
	}
}
