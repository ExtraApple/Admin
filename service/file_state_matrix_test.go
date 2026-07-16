package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"
	"admin/service/fileaccess"
	"admin/service/uploadsecurity"
)

func TestFileValidationStateMatrix(t *testing.T) {
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
		status                  string
		wantDetailDownloadURL   bool
		wantDetailPreviewURL    bool
		wantDownloadMIME        string
		wantDownloadCode        uploadsecurity.Code
		wantPreviewCode         uploadsecurity.Code
		wantRevalidationAllowed bool
	}{
		{
			status:                  model.FileValidationStatusValidated,
			wantDetailDownloadURL:   true,
			wantDetailPreviewURL:    true,
			wantDownloadMIME:        "image/png",
			wantRevalidationAllowed: false,
		},
		{
			status:                  model.FileValidationStatusLegacyUnverified,
			wantDetailDownloadURL:   true,
			wantDetailPreviewURL:    false,
			wantDownloadMIME:        "application/octet-stream",
			wantPreviewCode:         uploadsecurity.CodeFileStateConflict,
			wantRevalidationAllowed: true,
		},
		{
			status:                  model.FileValidationStatusValidationError,
			wantDetailDownloadURL:   true,
			wantDetailPreviewURL:    false,
			wantDownloadMIME:        "application/octet-stream",
			wantPreviewCode:         uploadsecurity.CodeFileStateConflict,
			wantRevalidationAllowed: true,
		},
		{
			status:                  model.FileValidationStatusBlocked,
			wantDetailDownloadURL:   false,
			wantDetailPreviewURL:    false,
			wantDownloadCode:        uploadsecurity.CodeFileStateBlocked,
			wantPreviewCode:         uploadsecurity.CodeFileStateBlocked,
			wantRevalidationAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			file := model.File{
				Name:                    "matrix-image.png",
				Bucket:                  "files-cold",
				ObjectName:              "private-object.png",
				ContentType:             "image/png",
				DetectedContentType:     "image/png",
				Size:                    128,
				ValidationStatus:        tt.status,
				ValidationPolicyVersion: uploadsecurity.PolicyVersionV1,
			}
			if err := db.Create(&file).Error; err != nil {
				t.Fatalf("create file: %v", err)
			}

			detail, err := details.Get(context.Background(), 42, file.ID)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got := detail.DownloadURL != ""; got != tt.wantDetailDownloadURL {
				t.Fatalf("detail download URL present = %v, want %v", got, tt.wantDetailDownloadURL)
			}
			if got := detail.PreviewURL != ""; got != tt.wantDetailPreviewURL {
				t.Fatalf("detail preview URL present = %v, want %v", got, tt.wantDetailPreviewURL)
			}

			download, err := ResolveFileDownloadAccess(&file)
			if tt.wantDownloadCode != "" {
				assertServiceUploadCode(t, err, tt.wantDownloadCode)
			} else {
				if err != nil {
					t.Fatalf("ResolveFileDownloadAccess() error = %v", err)
				}
				if download.ContentType != tt.wantDownloadMIME ||
					download.Disposition != FileDispositionAttachment {
					t.Fatalf("download decision = %#v, want MIME %q attachment",
						download, tt.wantDownloadMIME)
				}
			}

			preview, err := ResolveFilePreviewAccess(&file)
			if tt.wantPreviewCode != "" {
				assertServiceUploadCode(t, err, tt.wantPreviewCode)
			} else {
				if err != nil {
					t.Fatalf("ResolveFilePreviewAccess() error = %v", err)
				}
				if preview.ContentType != "image/png" ||
					preview.Disposition != FileDispositionInline {
					t.Fatalf("preview decision = %#v, want inline PNG", preview)
				}
			}

			repository := &recordingFileRevalidationRepository{file: file}
			store := &recordingFileStore{openContent: "historical image"}
			validator := &recordingFileValidator{
				result: uploadsecurity.Result{
					Purpose:            uploadsecurity.PurposeManagedFile,
					FileName:           file.Name,
					CanonicalType:      uploadsecurity.TypePNG,
					CanonicalExtension: ".png",
					CanonicalMIME:      "image/png",
					DetectedMIME:       "image/png",
					Size:               file.Size,
					PolicyVersion:      uploadsecurity.PolicyVersionV1,
					Reader:             strings.NewReader("validated image"),
				},
			}
			revalidation := NewFileRevalidationService(
				validator,
				store,
				repository,
				1024,
				func() time.Time { return time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC) },
			)

			result, err := revalidation.Revalidate(context.Background(), file.ID)
			if tt.wantRevalidationAllowed {
				if err != nil {
					t.Fatalf("Revalidate() error = %v", err)
				}
				if store.openCalls != 1 ||
					store.putCalls != 0 ||
					store.deleteCalls != 0 ||
					repository.updateCalls != 1 ||
					repository.update.Status != model.FileValidationStatusValidated ||
					result == nil ||
					result.ValidationStatus != model.FileValidationStatusValidated {
					t.Fatalf("revalidation result=%#v open=%d put=%d delete=%d update=%#v",
						result, store.openCalls, store.putCalls, store.deleteCalls, repository.update)
				}
			} else {
				assertServiceUploadCode(t, err, uploadsecurity.CodeFileStateConflict)
				if store.openCalls != 0 || repository.updateCalls != 0 {
					t.Fatalf("final-state revalidation side effects: open=%d update=%d, want 0 and 0",
						store.openCalls, repository.updateCalls)
				}
			}
		})
	}
}

func assertServiceUploadCode(t *testing.T, err error, want uploadsecurity.Code) {
	t.Helper()
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != want {
		t.Fatalf("error code = %q, classified=%v, want %q (error: %v)", code, ok, want, err)
	}
}
