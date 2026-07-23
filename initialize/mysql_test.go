package initialize

import (
	"testing"

	"admin/global"
	"admin/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyFileRow struct {
	gorm.Model
	Name        string
	Bucket      string
	ObjectName  string
	ContentType string
	Size        int64
	UploaderID  uint
}

func (legacyFileRow) TableName() string {
	return "files"
}

type preDigestFileRow struct {
	gorm.Model
	Name                    string
	Bucket                  string
	ObjectName              string
	ContentType             string
	DetectedContentType     string
	Size                    int64
	UploaderID              uint
	ValidationStatus        string
	ValidationPolicyVersion string
	ValidationErrorCode     string
}

func (preDigestFileRow) TableName() string {
	return "files"
}

type legacyUserRow struct {
	gorm.Model
	Username     string
	Password     string
	Nickname     string
	Avatar       string
	Role         string
	Status       int
	TokenVersion int
	Email        string
}

func (legacyUserRow) TableName() string {
	return "users"
}

func TestMigrateDatabaseBackfillsUploadValidationStatusIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&legacyFileRow{}, &legacyUserRow{}); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	legacyFile := legacyFileRow{
		Name:        "legacy.pdf",
		Bucket:      "files",
		ObjectName:  "legacy-object",
		ContentType: "application/pdf",
	}
	if err := db.Create(&legacyFile).Error; err != nil {
		t.Fatalf("create legacy file: %v", err)
	}

	customAvatar := legacyUserRow{
		Username: "custom",
		Password: "secret",
		Email:    "custom@example.com",
		Avatar:   "https://legacy.example/avatar.png",
	}
	defaultAvatar := legacyUserRow{
		Username: "default",
		Password: "secret",
		Email:    "default@example.com",
		Avatar:   "http://127.0.0.1:9001/browser/image/normal.png",
	}
	if err := db.Create(&customAvatar).Error; err != nil {
		t.Fatalf("create custom avatar user: %v", err)
	}
	if err := db.Create(&defaultAvatar).Error; err != nil {
		t.Fatalf("create default avatar user: %v", err)
	}

	if err := migrateDatabase(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	var file model.File
	if err := db.First(&file, legacyFile.ID).Error; err != nil {
		t.Fatalf("query migrated file: %v", err)
	}
	if file.ValidationStatus != model.FileValidationStatusLegacyUnverified {
		t.Fatalf("legacy file status: got %q", file.ValidationStatus)
	}

	var custom model.User
	if err := db.First(&custom, customAvatar.ID).Error; err != nil {
		t.Fatalf("query custom avatar user: %v", err)
	}
	if custom.AvatarValidationStatus != model.FileValidationStatusLegacyUnverified {
		t.Fatalf("custom avatar status: got %q", custom.AvatarValidationStatus)
	}

	var systemDefault model.User
	if err := db.First(&systemDefault, defaultAvatar.ID).Error; err != nil {
		t.Fatalf("query default avatar user: %v", err)
	}
	if systemDefault.AvatarValidationStatus != "" {
		t.Fatalf("default avatar status: got %q, want empty", systemDefault.AvatarValidationStatus)
	}

	if err := db.Model(&file).Update("validation_status", model.FileValidationStatusValidated).Error; err != nil {
		t.Fatalf("mark file validated: %v", err)
	}
	if err := db.Model(&custom).Update("avatar_validation_status", model.FileValidationStatusValidated).Error; err != nil {
		t.Fatalf("mark avatar validated: %v", err)
	}

	if err := migrateDatabase(db); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}
	if err := db.First(&file, legacyFile.ID).Error; err != nil {
		t.Fatalf("reload migrated file: %v", err)
	}
	if file.ValidationStatus != model.FileValidationStatusValidated {
		t.Fatalf("rerun overwrote file status: got %q", file.ValidationStatus)
	}
	if err := db.First(&custom, customAvatar.ID).Error; err != nil {
		t.Fatalf("reload custom avatar user: %v", err)
	}
	if custom.AvatarValidationStatus != model.FileValidationStatusValidated {
		t.Fatalf("rerun overwrote avatar status: got %q", custom.AvatarValidationStatus)
	}
}

func TestMigrateDatabaseCreatesUploadSecurityFieldsAndSafeDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := migrateDatabase(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	fields := []struct {
		model any
		field string
	}{
		{&model.File{}, "ContentSHA256"},
		{&model.File{}, "DetectedContentType"},
		{&model.File{}, "ValidationStatus"},
		{&model.File{}, "ValidationPolicyVersion"},
		{&model.File{}, "ValidationErrorCode"},
		{&model.File{}, "ValidatedAt"},
		{&model.User{}, "AvatarObjectName"},
		{&model.User{}, "AvatarContentType"},
		{&model.User{}, "AvatarContentSHA256"},
		{&model.User{}, "AvatarValidationStatus"},
		{&model.User{}, "AvatarValidatedAt"},
		{&model.AuditLog{}, "Metadata"},
		{&model.AuditLogArchive{}, "Metadata"},
	}
	for _, field := range fields {
		if !db.Migrator().HasColumn(field.model, field.field) {
			t.Fatalf("missing migrated field %T.%s", field.model, field.field)
		}
	}

	file := model.File{
		Name:       "safe-default.pdf",
		Bucket:     "files",
		ObjectName: "safe-default-object",
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file with default status: %v", err)
	}
	if file.ValidationStatus != model.FileValidationStatusLegacyUnverified {
		t.Fatalf("file default validation status: got %q", file.ValidationStatus)
	}
	if file.ContentSHA256 != "" {
		t.Fatalf("new file should have empty content sha256 by default: got %q", file.ContentSHA256)
	}
	if file.ValidatedAt != nil {
		t.Fatalf("unverified file should not have validated_at: %v", file.ValidatedAt)
	}

	user := model.User{
		Username: "system-default",
		Password: "secret",
		Email:    "system-default@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user with default avatar: %v", err)
	}
	if user.AvatarValidationStatus != "" || user.AvatarObjectName != "" || user.AvatarValidatedAt != nil {
		t.Fatalf("default avatar should remain unbound: %+v", user)
	}
	if user.AvatarContentSHA256 != "" {
		t.Fatalf("default avatar should have empty sha256: got %q", user.AvatarContentSHA256)
	}
}

func TestMigrateDatabaseDowngradesValidatedManagedFilesOutsideV1WhitelistIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&preDigestFileRow{}); err != nil {
		t.Fatalf("create pre-digest schema: %v", err)
	}

	rows := []preDigestFileRow{
		{
			Name:             "report.pdf",
			Bucket:           "files",
			ObjectName:       "objects/report",
			ContentType:      "application/pdf",
			ValidationStatus: model.FileValidationStatusValidated,
		},
		{
			Name:             "notes.txt",
			Bucket:           "files",
			ObjectName:       "objects/notes",
			ContentType:      "text/plain",
			ValidationStatus: model.FileValidationStatusValidated,
		},
		{
			Name:             "data.csv",
			Bucket:           "files",
			ObjectName:       "objects/data",
			ContentType:      "text/csv",
			ValidationStatus: model.FileValidationStatusValidated,
		},
		{
			Name:             "old-image.png",
			Bucket:           "files",
			ObjectName:       "objects/old-image",
			ContentType:      "image/png",
			ValidationStatus: model.FileValidationStatusValidated,
		},
		{
			Name:             "old-office.docx",
			Bucket:           "files",
			ObjectName:       "objects/old-office",
			ContentType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			ValidationStatus: model.FileValidationStatusValidated,
		},
		{
			Name:             "already-blocked.png",
			Bucket:           "files",
			ObjectName:       "objects/already-blocked",
			ContentType:      "image/png",
			ValidationStatus: model.FileValidationStatusBlocked,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed pre-digest files: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := migrateDatabase(db); err != nil {
			t.Fatalf("migrate database run %d: %v", i+1, err)
		}
	}

	expectedStatuses := map[string]string{
		"report.pdf":          model.FileValidationStatusValidated,
		"notes.txt":           model.FileValidationStatusValidated,
		"data.csv":            model.FileValidationStatusValidated,
		"old-image.png":       model.FileValidationStatusLegacyUnverified,
		"old-office.docx":     model.FileValidationStatusLegacyUnverified,
		"already-blocked.png": model.FileValidationStatusBlocked,
	}
	for name, want := range expectedStatuses {
		var file model.File
		if err := db.Where("name = ?", name).First(&file).Error; err != nil {
			t.Fatalf("query %s: %v", name, err)
		}
		if file.ValidationStatus != want {
			t.Fatalf("%s status: got %q, want %q", name, file.ValidationStatus, want)
		}
		if file.ContentSHA256 != "" {
			t.Fatalf("%s historical digest should remain empty: got %q", name, file.ContentSHA256)
		}
		if file.ObjectName == "" {
			t.Fatalf("%s object name should be preserved", name)
		}
	}
}

func TestMigrateDatabaseDoesNotRequireObjectStorage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	previousMinio := global.Minio
	global.Minio = nil
	t.Cleanup(func() {
		global.Minio = previousMinio
	})

	if err := migrateDatabase(db); err != nil {
		t.Fatalf("migration should not require object storage: %v", err)
	}
	if global.Minio != nil {
		t.Fatal("migration should not initialize or replace the object storage client")
	}
}
