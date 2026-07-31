package service

import (
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"admin/model"
)

var ErrAccessVersionNotFound = errors.New("用户授权版本不存在")

type AccessVersionRepository struct {
	db      *gorm.DB
	metrics *AccessVersionMetrics
}

var defaultAccessVersionMetrics = NewAccessVersionMetrics()

func NewAccessVersionRepository(
	db *gorm.DB,
	metrics ...*AccessVersionMetrics,
) *AccessVersionRepository {
	accessVersionMetrics := defaultAccessVersionMetrics
	if len(metrics) > 0 && metrics[0] != nil {
		accessVersionMetrics = metrics[0]
	}
	return &AccessVersionRepository{
		db:      db,
		metrics: accessVersionMetrics,
	}
}

func (r *AccessVersionRepository) CurrentVersion(userID uint) (int, error) {
	var accessVersion model.UserAccessVersion
	if err := r.db.First(&accessVersion, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			r.metrics.unexpectedMissing.Add(1)
			return 0, ErrAccessVersionNotFound
		}
		return 0, err
	}
	return accessVersion.Version, nil
}

func (r *AccessVersionRepository) recordSuccessfulTokenIssuance(userID uint) {
	var accessVersion model.UserAccessVersion
	err := r.db.Select("user_id").
		First(&accessVersion, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		r.metrics.tokenIssuanceMissing.Add(1)
	}
}

func (r *AccessVersionRepository) EnsureVersion(userID uint) (int, error) {
	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var version int
		var initialized bool
		err := r.db.Transaction(func(tx *gorm.DB) error {
			var accessVersion model.UserAccessVersion
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&accessVersion, "user_id = ?", userID).Error
			if err == nil {
				version = accessVersion.Version
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&model.UserAccessVersion{
					UserID:  userID,
					Version: 1,
				})
			if createResult.Error != nil {
				return createResult.Error
			}
			initialized = createResult.RowsAffected == 1

			accessVersion = model.UserAccessVersion{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&accessVersion, "user_id = ?", userID).Error; err != nil {
				return err
			}
			version = accessVersion.Version
			return nil
		})
		if err == nil {
			if initialized {
				r.metrics.initializations.Add(1)
			}
			return version, nil
		}
		errorNumber, retryable := retryableMySQLTransactionErrorNumber(err)
		if attempt == maxAttempts || !retryable {
			return 0, err
		}
		r.metrics.transactionRetries.Add(1)
		if errorNumber == 1213 {
			r.metrics.deadlockRetries.Add(1)
		}
		time.Sleep(time.Duration(attempt) * 10 * time.Millisecond)
	}
	return 0, errors.New("授权版本初始化重试次数耗尽")
}

func (r *AccessVersionRepository) EnsureAndIncrement(tx *gorm.DB, userID uint) (int, error) {
	var accessVersion model.UserAccessVersion
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&accessVersion, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&model.UserAccessVersion{
				UserID:  userID,
				Version: 1,
			}).Error; err != nil {
			return 0, err
		}
		accessVersion = model.UserAccessVersion{}
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&accessVersion, "user_id = ?", userID).Error
	}
	if err != nil {
		return 0, err
	}

	result := tx.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", userID).
		UpdateColumn("version", gorm.Expr("version + ?", 1))
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		if result.RowsAffected == 0 {
			r.metrics.postIncrementMissing.Add(1)
		}
		return 0, ErrAccessVersionNotFound
	}

	accessVersion = model.UserAccessVersion{}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&accessVersion, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			r.metrics.postIncrementMissing.Add(1)
			return 0, ErrAccessVersionNotFound
		}
		return 0, err
	}
	r.metrics.increments.Add(1)
	return accessVersion.Version, nil
}

func isRetryableMySQLTransactionError(err error) bool {
	_, retryable := retryableMySQLTransactionErrorNumber(err)
	return retryable
}

func retryableMySQLTransactionErrorNumber(err error) (uint16, bool) {
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) {
		return 0, false
	}
	return mysqlError.Number,
		mysqlError.Number == 1205 || mysqlError.Number == 1213
}
