package service

import (
	"errors"
	"testing"

	"admin/initialize/testutil"
	"admin/model"

	"gorm.io/gorm"
)

func TestAccessVersionRepositoryCurrentVersionReadsOnlyAuthorizationStorage(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}

	user := exitAccessVersionTestUser{
		Username:     "access-version-read",
		Password:     "secret",
		Email:        "access-version-read@example.com",
		TokenVersion: 9,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user with legacy token version: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 3,
	}).Error; err != nil {
		t.Fatalf("create authorization-owned access version: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	version, err := repository.CurrentVersion(user.ID)
	if err != nil {
		t.Fatalf("read authorization-owned access version: %v", err)
	}
	if version != 3 {
		t.Fatalf("current version = %d, want new-table value 3", version)
	}

	if err := db.Delete(
		&model.UserAccessVersion{},
		"user_id = ?",
		user.ID,
	).Error; err != nil {
		t.Fatalf("delete authorization-owned access version: %v", err)
	}
	_, err = repository.CurrentVersion(user.ID)
	if !errors.Is(err, ErrAccessVersionNotFound) {
		t.Fatalf(
			"missing new-table record error = %v, want ErrAccessVersionNotFound",
			err,
		)
	}
}

func TestAccessVersionRepositoryEnsureVersionCreatesMissingVersionOne(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username: "ensure-version-create",
		Password: "secret",
		Email:    "ensure-version-create@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for access-version initialization: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	version, err := repository.EnsureVersion(user.ID)
	if err != nil {
		t.Fatalf("ensure missing access version: %v", err)
	}
	if version != 1 {
		t.Fatalf("ensured version = %d, want 1", version)
	}

	storedVersion, err := repository.CurrentVersion(user.ID)
	if err != nil {
		t.Fatalf("read ensured access version: %v", err)
	}
	if storedVersion != 1 {
		t.Fatalf("stored version = %d, want 1", storedVersion)
	}

	var count int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count ensured access versions: %v", err)
	}
	if count != 1 {
		t.Fatalf("ensured row count = %d, want 1", count)
	}
}

func TestAccessVersionRepositoryEnsureVersionKeepsExistingHigherVersion(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&model.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  43,
		Version: 7,
	}).Error; err != nil {
		t.Fatalf("create existing access version: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_redundant_access_version_insert
		BEFORE INSERT ON user_access_versions
		BEGIN
			SELECT RAISE(ABORT, 'existing version must not be inserted again');
		END
	`).Error; err != nil {
		t.Fatalf("create redundant-insert guard: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	version, err := repository.EnsureVersion(43)
	if err != nil {
		t.Fatalf("ensure existing access version: %v", err)
	}
	if version != 7 {
		t.Fatalf("ensured version = %d, want existing version 7", version)
	}

	storedVersion, err := repository.CurrentVersion(43)
	if err != nil {
		t.Fatalf("read existing access version: %v", err)
	}
	if storedVersion != 7 {
		t.Fatalf("stored version = %d, want unchanged version 7", storedVersion)
	}
}

func TestAccessVersionRepositoryEnsureAndIncrementJoinsCallerTransaction(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username: "ensure-increment-transaction",
		Password: "secret",
		Email:    "ensure-increment-transaction@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for access-version increment: %v", err)
	}

	rollback := errors.New("rollback caller transaction")
	err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID); err != nil {
			return err
		}

		var accessVersion model.UserAccessVersion
		if err := tx.First(&accessVersion, "user_id = ?", user.ID).Error; err != nil {
			t.Fatalf("read incremented version inside caller transaction: %v", err)
		}
		if accessVersion.Version != 2 {
			t.Fatalf(
				"version inside caller transaction = %d, want created 1 then incremented to 2",
				accessVersion.Version,
			)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("caller transaction error = %v, want rollback sentinel", err)
	}

	_, err = NewAccessVersionRepository(db).CurrentVersion(user.ID)
	if !errors.Is(err, ErrAccessVersionNotFound) {
		t.Fatalf(
			"version after caller rollback error = %v, want ErrAccessVersionNotFound",
			err,
		)
	}
}

func TestAccessVersionRepositoryEnsureAndIncrementReturnsFinalVersion(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username: "ensure-increment-final",
		Password: "secret",
		Email:    "ensure-increment-final@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for access-version increment: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 5,
	}).Error; err != nil {
		t.Fatalf("create existing access version: %v", err)
	}

	var finalVersion int
	err := db.Transaction(func(tx *gorm.DB) error {
		version, err := NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID)
		if err != nil {
			return err
		}
		finalVersion = version
		return nil
	})
	if err != nil {
		t.Fatalf("increment existing access version: %v", err)
	}
	if finalVersion != 6 {
		t.Fatalf("returned final version = %d, want 6", finalVersion)
	}

	storedVersion, err := NewAccessVersionRepository(db).CurrentVersion(user.ID)
	if err != nil {
		t.Fatalf("read incremented access version: %v", err)
	}
	if storedVersion != 6 {
		t.Fatalf("stored final version = %d, want 6", storedVersion)
	}
}

func TestAccessVersionRepositoryEnsureVersionDoesNotWriteLegacyColumn(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "ensure-version-mirror",
		Password:     "secret",
		Email:        "ensure-version-mirror@example.com",
		TokenVersion: 9,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user with stale mirror version: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	version, err := repository.EnsureVersion(user.ID)
	if err != nil {
		t.Fatalf("ensure access version: %v", err)
	}
	if version != 1 {
		t.Fatalf("ensured version = %d, want 1", version)
	}

	if db.Migrator().HasColumn(&model.User{}, "token_version") {
		t.Fatal("exit-version repository test database unexpectedly has users.token_version")
	}
}

func TestAccessVersionRepositoryEnsureAndIncrementDoesNotWriteLegacyColumn(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "ensure-increment-mirror",
		Password:     "secret",
		Email:        "ensure-increment-mirror@example.com",
		TokenVersion: 13,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user with stale mirror version: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 5,
	}).Error; err != nil {
		t.Fatalf("create authorization access version: %v", err)
	}

	var finalVersion int
	err := db.Transaction(func(tx *gorm.DB) error {
		version, err := NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID)
		if err != nil {
			return err
		}
		finalVersion = version
		return nil
	})
	if err != nil {
		t.Fatalf("increment access version: %v", err)
	}
	if finalVersion != 6 {
		t.Fatalf("final authorization version = %d, want 6", finalVersion)
	}

	if db.Migrator().HasColumn(&model.User{}, "token_version") {
		t.Fatal("exit-version repository test database unexpectedly has users.token_version")
	}
}

func TestAccessVersionRepositoryEnsureVersionDoesNotRequireLegacyUserRow(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	version, err := repository.EnsureVersion(999)
	if err != nil {
		t.Fatalf("ensure version without legacy user row: %v", err)
	}
	if version != 1 {
		t.Fatalf("ensured version without legacy user row = %d, want 1", version)
	}

	storedVersion, err := repository.CurrentVersion(999)
	if err != nil {
		t.Fatalf("read version created without legacy user row: %v", err)
	}
	if storedVersion != 1 {
		t.Fatalf("stored version without legacy user row = %d, want 1", storedVersion)
	}
}

func TestAccessVersionRepositoryEnsureAndIncrementDoesNotRequireLegacyUserRow(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  1000,
		Version: 4,
	}).Error; err != nil {
		t.Fatalf("create access version without mirror target: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		version, err := NewAccessVersionRepository(tx).EnsureAndIncrement(1000)
		if err != nil {
			return err
		}
		if version != 5 {
			t.Fatalf(
				"incremented version without legacy user row = %d, want 5",
				version,
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("increment version without legacy user row: %v", err)
	}

	version, err := NewAccessVersionRepository(db).CurrentVersion(1000)
	if err != nil {
		t.Fatalf("read access version without legacy user row: %v", err)
	}
	if version != 5 {
		t.Fatalf("stored version without legacy user row = %d, want 5", version)
	}
}

func TestAccessVersionMetricsCountSuccessfulInitializationOnce(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username: "access-version-metrics-initialize",
		Password: "secret",
		Email:    "access-version-metrics-initialize@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for initialization metrics: %v", err)
	}

	metrics := NewAccessVersionMetrics()
	repository := NewAccessVersionRepository(db, metrics)
	if _, err := repository.EnsureVersion(user.ID); err != nil {
		t.Fatalf("first ensure access version: %v", err)
	}
	if _, err := repository.EnsureVersion(user.ID); err != nil {
		t.Fatalf("second ensure access version: %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.Initializations != 1 {
		t.Fatalf("initialization metric = %d, want 1", snapshot.Initializations)
	}
}

func TestAccessVersionMetricsCountUnexpectedMissingCurrentVersion(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&model.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}

	metrics := NewAccessVersionMetrics()
	repository := NewAccessVersionRepository(db, metrics)
	_, err := repository.CurrentVersion(1234)
	if !errors.Is(err, ErrAccessVersionNotFound) {
		t.Fatalf("current missing version error = %v, want ErrAccessVersionNotFound", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.UnexpectedMissing != 1 {
		t.Fatalf("unexpected-missing metric = %d, want 1", snapshot.UnexpectedMissing)
	}
}

func TestAccessVersionMetricsCountSuccessfulIncrement(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username: "access-version-metrics-increment",
		Password: "secret",
		Email:    "access-version-metrics-increment@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for increment metrics: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 3,
	}).Error; err != nil {
		t.Fatalf("create access version for increment metrics: %v", err)
	}

	metrics := NewAccessVersionMetrics()
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := NewAccessVersionRepository(tx, metrics).EnsureAndIncrement(user.ID)
		return err
	}); err != nil {
		t.Fatalf("increment access version: %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.Increments != 1 {
		t.Fatalf("increment metric = %d, want 1", snapshot.Increments)
	}
}

func TestAccessVersionMetricsCountMissingRowAfterEnsureAndIncrement(
	t *testing.T,
) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version repository database: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "access-version-metrics-post-increment-missing",
		Password:     "secret",
		Email:        "access-version-metrics-post-increment-missing@example.com",
		TokenVersion: 4,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for post-increment missing metric: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create access version for post-increment missing metric: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER remove_access_version_after_increment
		AFTER UPDATE OF version ON user_access_versions
		BEGIN
			DELETE FROM user_access_versions WHERE user_id = NEW.user_id;
		END
	`).Error; err != nil {
		t.Fatalf("create post-increment missing-row trigger: %v", err)
	}

	metrics := NewAccessVersionMetrics()
	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := NewAccessVersionRepository(tx, metrics).EnsureAndIncrement(user.ID)
		return err
	})
	if !errors.Is(err, ErrAccessVersionNotFound) {
		t.Fatalf(
			"post-increment missing-row error = %v, want ErrAccessVersionNotFound",
			err,
		)
	}

	snapshot := metrics.Snapshot()
	if snapshot.PostIncrementMissing != 1 {
		t.Fatalf(
			"post-increment-missing metric = %d, want 1",
			snapshot.PostIncrementMissing,
		)
	}
	if snapshot.Increments != 0 {
		t.Fatalf(
			"successful increment metric = %d, want 0 after rollback",
			snapshot.Increments,
		)
	}

	var stored model.UserAccessVersion
	if err := db.First(&stored, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("read access version after post-increment rollback: %v", err)
	}
	if stored.Version != user.TokenVersion {
		t.Fatalf(
			"access version after rollback = %d, want %d",
			stored.Version,
			user.TokenVersion,
		)
	}
}
