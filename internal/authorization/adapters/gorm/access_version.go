package gormadapter

import (
	"context"
	"errors"
	"sort"

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
	record, err := ensureAccessVersion(db, userID)
	return record.Version, err
}

// EnsureMany initializes missing rows and reads a current, locked page in one batch.
func (versions *AccessVersions) EnsureMany(ctx context.Context, userIDs []uint) (map[uint]int, error) {
	ids := uniqueIDs(userIDs)
	result := make(map[uint]int, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	records := make([]UserAccessVersion, len(ids))
	for index, id := range ids {
		records[index] = UserAccessVersion{UserID: id, Version: 1}
	}
	db := platformdatabase.FromContext(ctx, versions.db)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&records).Error; err != nil {
		return nil, err
	}
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id IN ?", ids).Order("user_id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		result[record.UserID] = record.Version
	}
	return result, nil
}

func (versions *AccessVersions) EnsureAndIncrement(ctx context.Context, userID uint) (int, error) {
	db := platformdatabase.FromContext(ctx, versions.db)
	if _, err := ensureAccessVersion(db, userID); err != nil {
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
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "user_id = ?", userID).Error; err != nil {
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

func ensureAccessVersion(db *gorm.DB, userID uint) (UserAccessVersion, error) {
	var record UserAccessVersion
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "user_id = ?", userID).Error
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return record, err
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&UserAccessVersion{UserID: userID, Version: 1}).Error; err != nil {
		return record, err
	}
	err = db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "user_id = ?", userID).Error
	return record, err
}
