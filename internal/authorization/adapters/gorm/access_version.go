package gormadapter

import (
	"context"
	"errors"

	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAccessVersionNotFound = errors.New("用户授权版本不存在")

type AccessVersions struct{ db *gorm.DB }

func NewAccessVersions(db *gorm.DB) *AccessVersions { return &AccessVersions{db: db} }

func (versions *AccessVersions) Current(ctx context.Context, userID uint) (int, error) {
	var record UserAccessVersion
	if err := platformdatabase.FromContext(ctx, versions.db).First(&record, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrAccessVersionNotFound
		}
		return 0, err
	}
	return record.Version, nil
}

// Ensure creates the authorization version at 1 when absent and preserves it when present.
func (versions *AccessVersions) Ensure(ctx context.Context, userID uint) (int, error) {
	db := platformdatabase.FromContext(ctx, versions.db)
	if err := ensureAccessVersion(db, userID); err != nil {
		return 0, err
	}
	return versions.Current(ctx, userID)
}

func (versions *AccessVersions) EnsureAndIncrement(ctx context.Context, userID uint) (int, error) {
	db := platformdatabase.FromContext(ctx, versions.db)
	if err := ensureAccessVersion(db, userID); err != nil {
		return 0, err
	}
	result := db.Model(&UserAccessVersion{}).Where("user_id = ?", userID).
		UpdateColumn("version", gorm.Expr("version + ?", 1))
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, ErrAccessVersionNotFound
	}
	var record UserAccessVersion
	if err := db.First(&record, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrAccessVersionNotFound
		}
		return 0, err
	}
	return record.Version, nil
}

func (versions *AccessVersions) Increment(ctx context.Context, userIDs []uint) error {
	for _, userID := range uniqueIDs(userIDs) {
		if _, err := versions.EnsureAndIncrement(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

func ensureAccessVersion(db *gorm.DB, userID uint) error {
	var record UserAccessVersion
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "user_id = ?", userID).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&UserAccessVersion{UserID: userID, Version: 1}).Error; err != nil {
		return err
	}
	return db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "user_id = ?", userID).Error
}
