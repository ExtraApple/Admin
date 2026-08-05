package app

import (
	"fmt"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	auditmodule "admin/internal/audit"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/dictionary"
	filesmodule "admin/internal/files"
	"admin/internal/identity"
	"admin/internal/navigation"
	"admin/internal/organization"

	"gorm.io/gorm"
)

var migrationModels = func() []any {
	models := append([]any{}, identity.Models()...)
	models = append(models, authgorm.Models()...)
	models = append(models, navigation.Models()...)
	models = append(models, apigorm.Models()...)
	models = append(models, filesmodule.Models()...)
	models = append(models, auditmodule.Models()...)
	models = append(models,
		dictionary.Type{},
		dictionary.Item{},
		organization.Unit{},
		organization.Membership{},
	)
	return models
}()

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(migrationModels...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	if err := backfillUploadValidationStatus(db); err != nil {
		return fmt.Errorf("backfill upload validation status: %w", err)
	}
	if err := downgradeValidatedManagedFilesOutsideV1Policy(db); err != nil {
		return fmt.Errorf("downgrade validated managed files outside V1 policy: %w", err)
	}
	return nil
}

func backfillUploadValidationStatus(db *gorm.DB) error {
	if err := db.Unscoped().
		Model(&filesmodule.File{}).
		Where("validation_status IS NULL OR validation_status = ?", "").
		Update("validation_status", filesmodule.FileValidationStatusLegacyUnverified).Error; err != nil {
		return fmt.Errorf("files: %w", err)
	}

	if err := db.Unscoped().
		Model(&identity.User{}).
		Where("(avatar_validation_status IS NULL OR avatar_validation_status = ?) AND avatar IS NOT NULL AND avatar <> ? AND avatar NOT LIKE ?", "", "", "%/browser/image/normal.png").
		Update("avatar_validation_status", filesmodule.FileValidationStatusLegacyUnverified).Error; err != nil {
		return fmt.Errorf("users: %w", err)
	}
	return nil
}

func downgradeValidatedManagedFilesOutsideV1Policy(db *gorm.DB) error {
	allowedMIMEs := []string{
		"application/pdf",
		"text/plain",
		"text/csv",
	}
	if err := db.Unscoped().
		Model(&filesmodule.File{}).
		Where(
			"validation_status = ? AND (content_type IS NULL OR content_type NOT IN ?)",
			filesmodule.FileValidationStatusValidated,
			allowedMIMEs,
		).
		Update("validation_status", filesmodule.FileValidationStatusLegacyUnverified).Error; err != nil {
		return fmt.Errorf("files: %w", err)
	}
	return nil
}
