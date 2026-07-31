package initialize

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"admin/global"
	"admin/model"

	"go.uber.org/zap"
)

// InitMysql 初始化 MySQL 连接，并执行模型自动迁移。
func InitMysql(conf *Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.Mysql.User,
		conf.Mysql.Password,
		conf.Mysql.Host,
		conf.Mysql.Port,
		conf.Mysql.DB,
	)

	var err error
	global.DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		global.Logger.Fatal("open mysql failed", zap.Error(err))
	}

	if err := migrateDatabase(global.DB); err != nil {
		global.Logger.Fatal("database migrate failed", zap.Error(err))
	}
}

func migrateDatabase(db *gorm.DB) error {
	return migrateDatabaseWithoutMySQLLock(db)
}

func migrateDatabaseWithoutMySQLLock(db *gorm.DB) error {
	if err := db.AutoMigrate(model.Models...); err != nil {
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
		Model(&model.File{}).
		Where("validation_status IS NULL OR validation_status = ?", "").
		Update("validation_status", model.FileValidationStatusLegacyUnverified).Error; err != nil {
		return fmt.Errorf("files: %w", err)
	}

	if err := db.Unscoped().
		Model(&model.User{}).
		Where("(avatar_validation_status IS NULL OR avatar_validation_status = ?) AND avatar IS NOT NULL AND avatar <> ? AND avatar NOT LIKE ?", "", "", "%/browser/image/normal.png").
		Update("avatar_validation_status", model.FileValidationStatusLegacyUnverified).Error; err != nil {
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
		Model(&model.File{}).
		Where(
			"validation_status = ? AND (content_type IS NULL OR content_type NOT IN ?)",
			model.FileValidationStatusValidated,
			allowedMIMEs,
		).
		Update("validation_status", model.FileValidationStatusLegacyUnverified).Error; err != nil {
		return fmt.Errorf("files: %w", err)
	}
	return nil
}
