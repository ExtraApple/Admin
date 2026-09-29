package app

import (
	apigorm "admin/internal/apimetadata/adapters/gorm"
	auditmodule "admin/internal/audit"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/dictionary"
	filesmodule "admin/internal/files"
	"admin/internal/identity"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/navigation"
	"admin/internal/organization"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

var migrationModels = func() []any {
	models := append([]any{}, identity.Models()...)
	models = append(models, authgorm.Models()...)
	models = append(models, navigation.Models()...)
	models = append(models, apigorm.Models()...)
	models = append(models, filesmodule.Models()...)
	models = append(models, auditmodule.Models()...)
	models = append(models, messaginggorm.Models()...)
	models = append(models,
		dictionary.Type{},
		dictionary.Item{},
		organization.Unit{},
		organization.Membership{},
	)
	return models
}()

const minimumSupportedMySQLVersion = "8.0.16"

func validateMySQLServerVersion(version string) error {
	value := strings.ToLower(strings.TrimSpace(version))
	if value == "" || strings.Contains(value, "mariadb") {
		return fmt.Errorf("unsupported or unknown MySQL server version %q", version)
	}
	parts := strings.SplitN(strings.SplitN(value, "-", 2)[0], ".", 4)
	if len(parts) < 3 {
		return fmt.Errorf("unable to parse MySQL server version %q", version)
	}
	parsed := make([]int, 3)
	for index := range parsed {
		value, err := strconv.Atoi(parts[index])
		if err != nil {
			return fmt.Errorf("unable to parse MySQL server version %q: %w", version, err)
		}
		parsed[index] = value
	}
	if parsed[0] < 8 || (parsed[0] == 8 && (parsed[1] < 0 || (parsed[1] == 0 && parsed[2] < 16))) {
		return fmt.Errorf("MySQL server version %q is below minimum %s", version, minimumSupportedMySQLVersion)
	}
	return nil
}

func validateMySQLMigrationServer(db *gorm.DB) error {
	if db == nil || strings.ToLower(db.Dialector.Name()) != "mysql" {
		return nil
	}
	var version string
	if err := db.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		return fmt.Errorf("read MySQL server version: %w", err)
	}
	return validateMySQLServerVersion(version)
}

func Migrate(db *gorm.DB) error {
	if err := validateMySQLMigrationServer(db); err != nil {
		return fmt.Errorf("validate MySQL migration server: %w", err)
	}
	// Normalize legacy NULLs before AutoMigrate restores the NOT NULL constraint.
	if db.Migrator().HasTable(&filesmodule.File{}) && db.Migrator().HasColumn(&filesmodule.File{}, "purpose") {
		if err := db.Table("files").Where("purpose IS NULL").Update("purpose", "managed_file").Error; err != nil {
			return fmt.Errorf("backfill legacy file purpose: %w", err)
		}
	}
	if err := db.AutoMigrate(migrationModels...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	if err := backfillUploadValidationStatus(db); err != nil {
		return fmt.Errorf("backfill upload validation status: %w", err)
	}
	if err := downgradeValidatedManagedFilesOutsideV1Policy(db); err != nil {
		return fmt.Errorf("downgrade validated managed files outside V1 policy: %w", err)
	}
	if err := backfillMembershipJoinedAt(db); err != nil {
		return fmt.Errorf("backfill organization membership joined at: %w", err)
	}
	if err := backfillMessagingDeadLetterOccurredAt(db); err != nil {
		return fmt.Errorf("backfill messaging dead letter occurred at: %w", err)
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

func backfillMembershipJoinedAt(db *gorm.DB) error {
	return db.Model(&organization.Membership{}).
		Where("created_at IS NULL").
		Update("created_at", time.Now().UTC()).Error
}

func backfillMessagingDeadLetterOccurredAt(db *gorm.DB) error {
	return db.Model(&messaginggorm.MessageConsumerDeadLetter{}).
		Where("occurred_at IS NULL").
		Update("occurred_at", gorm.Expr("created_at")).Error
}
func downgradeValidatedManagedFilesOutsideV1Policy(db *gorm.DB) error {
	allowedMIMEs := []string{
		"application/pdf",
		"text/plain",
		"text/csv",
	}
	if err := db.Unscoped().
		Model(&filesmodule.File{}).
		Where("purpose = ? OR purpose IS NULL OR purpose = ''", "managed_file").
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
