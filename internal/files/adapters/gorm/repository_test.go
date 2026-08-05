package gormadapter_test

import (
	"context"
	"testing"
	"time"

	"admin/testsupport/testutil"
	"admin/internal/files"
	gormadapter "admin/internal/files/adapters/gorm"
	"admin/internal/files/application"
	"admin/internal/files/domain"
)

func TestRepositoryPersistsFilesWithoutLegacyModels(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(files.Models()...); err != nil {
		t.Fatalf("migrate Files models: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	file := domain.File{Name: "report.pdf", Bucket: "files", ObjectName: "managed.pdf", ContentType: "application/pdf", Size: 41, UploaderID: 9, ValidationStatus: domain.ValidationStatusLegacyUnverified, CreatedAt: createdAt}
	if err := repository.Create(ctx, &file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if file.ID == 0 {
		t.Fatal("created file ID is zero")
	}

	loaded, err := repository.FindByID(ctx, file.ID)
	if err != nil || loaded.ObjectName != file.ObjectName || loaded.UploaderID != 9 {
		t.Fatalf("loaded file = %#v err=%v", loaded, err)
	}
	list, total, err := repository.List(ctx, 1, 10, "managed")
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != file.ID {
		t.Fatalf("list = %#v total=%d err=%v", list, total, err)
	}
	if err := repository.UpdateName(ctx, file.ID, "renamed.pdf"); err != nil {
		t.Fatalf("update file name: %v", err)
	}
	validatedAt := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	if err := repository.UpdateValidation(ctx, file.ID, application.ValidationUpdate{ContentType: "application/pdf", DetectedContentType: "application/pdf", ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: domain.ValidationStatusValidated, PolicyVersion: files.FileUploadPolicyVersion, ValidatedAt: &validatedAt}); err != nil {
		t.Fatalf("update validation: %v", err)
	}
	candidates, err := repository.FindRotationCandidates(ctx, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), "files", 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("rotation candidates = %#v err=%v", candidates, err)
	}
	if err := repository.UpdateBucket(ctx, file.ID, "files-archive"); err != nil {
		t.Fatalf("update bucket: %v", err)
	}
	loaded, err = repository.FindByID(ctx, file.ID)
	if err != nil || loaded.Name != "renamed.pdf" || loaded.Bucket != "files-archive" || loaded.ValidationStatus != domain.ValidationStatusValidated || loaded.ContentSHA256 == "" {
		t.Fatalf("updated file = %#v err=%v", loaded, err)
	}
	if err := repository.Delete(ctx, file.ID); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if _, err := repository.FindByID(ctx, file.ID); err != application.ErrFileNotFound {
		t.Fatalf("find deleted error = %v", err)
	}
}
