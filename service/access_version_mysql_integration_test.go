//go:build mysql_integration

package service

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"admin/global"
	"admin/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type accessVersionDeadlockWeight struct {
	ID    uint `gorm:"primaryKey"`
	Value int
}

func TestMySQLAccessVersionRepositoryConcurrentEnsureVersionCreatesOneRow(t *testing.T) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version service tables: %v", err)
	}
	user := model.User{
		Username: fmt.Sprintf("concurrent-ensure-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("concurrent-ensure-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for concurrent ensure: %v", err)
	}

	const workers = 8
	var createWaiters atomic.Int32
	releaseCreates := make(chan struct{})
	var releaseOnce sync.Once
	timeout := time.AfterFunc(5*time.Second, func() {
		releaseOnce.Do(func() {
			close(releaseCreates)
		})
	})
	defer timeout.Stop()

	if err := db.Callback().Create().
		Before("gorm:create").
		Register("test:align-access-version-inserts", func(tx *gorm.DB) {
			if tx.Statement.Schema == nil ||
				tx.Statement.Schema.Name != "UserAccessVersion" {
				return
			}
			if createWaiters.Add(1) == workers {
				releaseOnce.Do(func() {
					close(releaseCreates)
				})
			}
			<-releaseCreates
		}); err != nil {
		t.Fatalf("register concurrent create barrier: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	start := make(chan struct{})
	type result struct {
		version int
		err     error
	}
	results := make(chan result, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			version, err := repository.EnsureVersion(user.ID)
			results <- result{version: version, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	if got := createWaiters.Load(); got < workers {
		t.Fatalf("aligned insert waiters = %d, want at least %d", got, workers)
	}
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent EnsureVersion returned error: %v", result.err)
		}
		if result.version != 1 {
			t.Fatalf("concurrent EnsureVersion returned version %d, want 1", result.version)
		}
	}

	var count int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count concurrent access versions: %v", err)
	}
	if count != 1 {
		t.Fatalf("concurrent access-version rows = %d, want 1", count)
	}
}

func TestMySQLAccessVersionRepositoryConcurrentIncrementsSerializeWithoutLosingVersions(t *testing.T) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version service tables: %v", err)
	}
	const initialVersion = 5
	user := model.User{
		Username: fmt.Sprintf("concurrent-increment-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("concurrent-increment-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for concurrent increments: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: initialVersion,
	}).Error; err != nil {
		t.Fatalf("create access version for concurrent increments: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	type result struct {
		version int
		err     error
	}
	results := make(chan result, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			var version int
			err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				version, err = NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID)
				return err
			})
			results <- result{version: version, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	versions := make([]int, 0, workers)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent EnsureAndIncrement returned error: %v", result.err)
		}
		versions = append(versions, result.version)
	}
	sort.Ints(versions)
	for index, version := range versions {
		want := 6 + index
		if version != want {
			t.Fatalf("sorted incremented versions = %v, want each value from 6 through 13", versions)
		}
	}

	finalVersion, err := NewAccessVersionRepository(db).CurrentVersion(user.ID)
	if err != nil {
		t.Fatalf("read final access version: %v", err)
	}
	if finalVersion != 13 {
		t.Fatalf("final access version = %d, want 13", finalVersion)
	}
}

func TestMySQLAccessVersionRepositoryIncrementWaitsForLockedVersionRow(t *testing.T) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version service tables: %v", err)
	}
	user := model.User{
		Username: fmt.Sprintf("locked-increment-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("locked-increment-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for row-lock test: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 5,
	}).Error; err != nil {
		t.Fatalf("create access version for row-lock test: %v", err)
	}

	firstTx := db.Begin()
	if firstTx.Error != nil {
		t.Fatalf("begin first increment transaction: %v", firstTx.Error)
	}
	t.Cleanup(func() {
		_ = firstTx.Rollback().Error
	})
	firstVersion, err := NewAccessVersionRepository(firstTx).EnsureAndIncrement(user.ID)
	if err != nil {
		t.Fatalf("increment access version while holding transaction: %v", err)
	}
	if firstVersion != 6 {
		t.Fatalf("first locked increment version = %d, want 6", firstVersion)
	}

	secondQueryStarted := make(chan struct{})
	var queryStartedOnce sync.Once
	if err := db.Callback().Query().
		Before("gorm:query").
		Register("test:observe-locked-access-version-query", func(tx *gorm.DB) {
			if tx.Statement.Schema == nil ||
				tx.Statement.Schema.Name != "UserAccessVersion" {
				return
			}
			queryStartedOnce.Do(func() {
				close(secondQueryStarted)
			})
		}); err != nil {
		t.Fatalf("register row-lock query observer: %v", err)
	}

	type incrementResult struct {
		version int
		err     error
	}
	secondResult := make(chan incrementResult, 1)
	go func() {
		var version int
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			version, err = NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID)
			return err
		})
		secondResult <- incrementResult{version: version, err: err}
	}()

	select {
	case <-secondQueryStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("second increment did not reach the locked version row")
	}
	select {
	case result := <-secondResult:
		t.Fatalf(
			"second increment completed before first transaction committed: version=%d error=%v",
			result.version,
			result.err,
		)
	case <-time.After(150 * time.Millisecond):
	}

	if err := firstTx.Commit().Error; err != nil {
		t.Fatalf("commit first increment transaction: %v", err)
	}
	result := <-secondResult
	if result.err != nil {
		t.Fatalf("second increment after lock release returned error: %v", result.err)
	}
	if result.version != 7 {
		t.Fatalf("second increment version = %d, want 7", result.version)
	}

	finalVersion, err := NewAccessVersionRepository(db).CurrentVersion(user.ID)
	if err != nil {
		t.Fatalf("read version after serialized increments: %v", err)
	}
	if finalVersion != 7 {
		t.Fatalf("final serialized version = %d, want 7", finalVersion)
	}
}

func TestMySQLAccessVersionRepositoryConcurrentEnsureAndIncrementNeverRegressesVersion(t *testing.T) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version service tables: %v", err)
	}
	user := model.User{
		Username: fmt.Sprintf("ensure-increment-race-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("ensure-increment-race-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for ensure/increment race: %v", err)
	}

	const competingCreates = 2
	var createWaiters atomic.Int32
	releaseCreates := make(chan struct{})
	var releaseOnce sync.Once
	timeout := time.AfterFunc(5*time.Second, func() {
		releaseOnce.Do(func() {
			close(releaseCreates)
		})
	})
	defer timeout.Stop()
	if err := db.Callback().Create().
		Before("gorm:create").
		Register("test:align-ensure-and-increment-inserts", func(tx *gorm.DB) {
			if tx.Statement.Schema == nil ||
				tx.Statement.Schema.Name != "UserAccessVersion" {
				return
			}
			if createWaiters.Add(1) == competingCreates {
				releaseOnce.Do(func() {
					close(releaseCreates)
				})
			}
			<-releaseCreates
		}); err != nil {
		t.Fatalf("register ensure/increment create barrier: %v", err)
	}

	repository := NewAccessVersionRepository(db)
	start := make(chan struct{})
	type versionResult struct {
		version int
		err     error
	}
	ensureResult := make(chan versionResult, 1)
	incrementResult := make(chan versionResult, 1)
	go func() {
		<-start
		version, err := repository.EnsureVersion(user.ID)
		ensureResult <- versionResult{version: version, err: err}
	}()
	go func() {
		<-start
		version, err := runRetryingAccessVersionTransaction(
			db,
			func(tx *gorm.DB) (int, error) {
				return NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID)
			},
		)
		incrementResult <- versionResult{version: version, err: err}
	}()
	close(start)

	ensured := <-ensureResult
	if ensured.err != nil {
		t.Fatalf("concurrent EnsureVersion returned error: %v", ensured.err)
	}
	incremented := <-incrementResult
	if incremented.err != nil {
		t.Fatalf("concurrent EnsureAndIncrement returned error: %v", incremented.err)
	}
	if incremented.version != 2 {
		t.Fatalf("concurrent increment returned version %d, want 2", incremented.version)
	}
	if ensured.version < 1 || ensured.version > incremented.version {
		t.Fatalf(
			"concurrent ensure returned version %d, want a committed version between 1 and %d",
			ensured.version,
			incremented.version,
		)
	}

	var count int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count access versions after ensure/increment race: %v", err)
	}
	if count != 1 {
		t.Fatalf("access-version rows after ensure/increment race = %d, want 1", count)
	}
	finalVersion, err := repository.CurrentVersion(user.ID)
	if err != nil {
		t.Fatalf("read final version after ensure/increment race: %v", err)
	}
	if finalVersion != 2 {
		t.Fatalf("final version after ensure/increment race = %d, want 2", finalVersion)
	}
}

func TestMySQLAccessVersionRepositoryEnsureVersionRetriesAfterRealDeadlock(
	t *testing.T,
) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
		&accessVersionDeadlockWeight{},
	); err != nil {
		t.Fatalf("migrate access-version deadlock tables: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Migrator().DropTable(&accessVersionDeadlockWeight{}); err != nil {
			t.Errorf("drop access-version deadlock weight table: %v", err)
		}
	})

	user := model.User{
		Username: fmt.Sprintf("deadlock-retry-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("deadlock-retry-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for deadlock retry: %v", err)
	}
	weights := make([]accessVersionDeadlockWeight, 128)
	for index := range weights {
		weights[index] = accessVersionDeadlockWeight{
			ID:    uint(index + 1),
			Value: index,
		}
	}
	if err := db.Create(&weights).Error; err != nil {
		t.Fatalf("create deadlock rollback weights: %v", err)
	}

	blocker := db.Begin()
	if blocker.Error != nil {
		t.Fatalf("begin deadlock blocker transaction: %v", blocker.Error)
	}
	t.Cleanup(func() {
		_ = blocker.Rollback().Error
	})
	if result := blocker.Model(&accessVersionDeadlockWeight{}).
		Where("id > ?", 0).
		UpdateColumn("value", gorm.Expr("value + 1")); result.Error != nil {
		t.Fatalf("increase deadlock blocker rollback cost: %v", result.Error)
	} else if result.RowsAffected != int64(len(weights)) {
		t.Fatalf(
			"deadlock blocker updated %d weight rows, want %d",
			result.RowsAffected,
			len(weights),
		)
	}
	var lockedUser model.User
	if err := blocker.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&lockedUser, user.ID).Error; err != nil {
		t.Fatalf("lock user before EnsureVersion: %v", err)
	}

	var createAttempts atomic.Int32
	firstInsertCompleted := make(chan struct{})
	var firstInsertOnce sync.Once
	weightsTable := quoteAccessVersionMySQLTestIdentifier(
		db.NamingStrategy.TableName("AccessVersionDeadlockWeight"),
	)
	if err := db.Callback().Create().
		After("gorm:create").
		Register("test:observe-deadlock-retry-inserts", func(tx *gorm.DB) {
			if tx.Statement.Schema == nil ||
				tx.Statement.Schema.Name != "UserAccessVersion" ||
				tx.Error != nil {
				return
			}
			createAttempts.Add(1)
			firstInsertOnce.Do(func() {
				close(firstInsertCompleted)
			})
			if err := tx.Session(&gorm.Session{NewDB: true}).Exec(
				"UPDATE " + weightsTable + " SET value = value + 1 WHERE id = 1",
			).Error; err != nil {
				tx.AddError(err)
			}
		}); err != nil {
		t.Fatalf("register deadlock retry insert observer: %v", err)
	}

	metrics := NewAccessVersionMetrics()
	repository := NewAccessVersionRepository(db, metrics)
	type ensureResult struct {
		version int
		err     error
	}
	resultChannel := make(chan ensureResult, 1)
	go func() {
		version, err := repository.EnsureVersion(user.ID)
		resultChannel <- ensureResult{version: version, err: err}
	}()

	select {
	case <-firstInsertCompleted:
	case <-time.After(5 * time.Second):
		t.Fatal("EnsureVersion did not insert before deadlock was constructed")
	}

	var blockedVersion model.UserAccessVersion
	err := blocker.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&blockedVersion, "user_id = ?", user.ID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf(
			"deadlock blocker access-version lookup error = %v, want rolled-back record not found",
			err,
		)
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatalf("commit surviving deadlock blocker transaction: %v", err)
	}

	var result ensureResult
	select {
	case result = <-resultChannel:
	case <-time.After(5 * time.Second):
		t.Fatal("EnsureVersion did not finish after deadlock victim retry")
	}
	if result.err != nil {
		t.Fatalf("EnsureVersion after real deadlock error = %v", result.err)
	}
	if result.version != 1 {
		t.Fatalf("EnsureVersion after real deadlock version = %d, want 1", result.version)
	}
	if attempts := createAttempts.Load(); attempts != 2 {
		t.Fatalf("access-version insert attempts = %d, want one deadlocked attempt and one retry", attempts)
	}
	metricsSnapshot := metrics.Snapshot()
	if metricsSnapshot.TransactionRetries != 1 {
		t.Fatalf(
			"transaction retries after deadlock = %d, want 1",
			metricsSnapshot.TransactionRetries,
		)
	}
	if metricsSnapshot.DeadlockRetries != 1 {
		t.Fatalf(
			"deadlock retries after constructed cycle = %d, want 1",
			metricsSnapshot.DeadlockRetries,
		)
	}

	var accessVersion model.UserAccessVersion
	if err := db.First(&accessVersion, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("read access version after deadlock retry: %v", err)
	}
	if accessVersion.Version != 1 {
		t.Fatalf("access version after deadlock retry = %d, want 1", accessVersion.Version)
	}
}

func TestMySQLAccessVersionRepositoryPrimaryWriteFailureRollsBackVersion(
	t *testing.T,
) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate access-version rollback tables: %v", err)
	}
	const initialVersion = 3
	user := model.User{
		Username: fmt.Sprintf("primary-write-rollback-%d", time.Now().UnixNano()),
		Password: "secret",
		Email:    fmt.Sprintf("primary-write-rollback-%d@example.com", time.Now().UnixNano()),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user for primary-write rollback: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: initialVersion,
	}).Error; err != nil {
		t.Fatalf("create access version for primary-write rollback: %v", err)
	}

	accessVersionsTable := quoteAccessVersionMySQLTestIdentifier(
		db.NamingStrategy.TableName("UserAccessVersion"),
	)
	constraintName := fmt.Sprintf("it_avs_primary_%d", time.Now().UnixNano())
	if err := db.Exec(fmt.Sprintf(`
		ALTER TABLE %s
		ADD CONSTRAINT %s CHECK (version <> %d)
	`,
		accessVersionsTable,
		quoteAccessVersionMySQLTestIdentifier(constraintName),
		initialVersion+1,
	)).Error; err != nil {
		t.Fatalf("create forced primary-write failure constraint: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := NewAccessVersionRepository(tx).EnsureAndIncrement(user.ID)
		return err
	})
	if err == nil {
		t.Fatal("forced MySQL primary-write failure returned nil error")
	}

	var accessVersion model.UserAccessVersion
	if err := db.First(&accessVersion, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("read access version after primary-write rollback: %v", err)
	}
	if accessVersion.Version != initialVersion {
		t.Fatalf(
			"authorization version after primary-write rollback = %d, want %d",
			accessVersion.Version,
			initialVersion,
		)
	}
}

func TestMySQLDeletingUserSoftDeletesAndIncrementsAccessVersionInOneTransaction(
	t *testing.T,
) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
		&model.Role{},
		&model.UserRole{},
	); err != nil {
		t.Fatalf("migrate user-deletion service tables: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	adminRole := model.Role{
		Name:      fmt.Sprintf("administrator-%d", time.Now().UnixNano()),
		Code:      "admin",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&adminRole).Error; err != nil {
		t.Fatalf("create all-data administrator role: %v", err)
	}
	operator := model.User{
		Username: fmt.Sprintf("delete-operator-%d", time.Now().UnixNano()),
		Password: "not-used",
		Email:    fmt.Sprintf("delete-operator-%d@example.com", time.Now().UnixNano()),
		Role:     "admin",
		Status:   1,
	}
	const initialVersion = 3
	target := model.User{
		Username: fmt.Sprintf("delete-target-%d", time.Now().UnixNano()),
		Password: "not-used",
		Email:    fmt.Sprintf("delete-target-%d@example.com", time.Now().UnixNano()),
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create delete operator: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create delete target: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: adminRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign administrator role: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: initialVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}

	if err := DeleteUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf("DeleteUserByAdmin() error = %v", err)
	}

	var deletedUser model.User
	if err := db.Unscoped().First(&deletedUser, target.ID).Error; err != nil {
		t.Fatalf("reload soft-deleted target: %v", err)
	}
	if !deletedUser.DeletedAt.Valid {
		t.Fatal("target user was not soft deleted")
	}

	var accessVersion model.UserAccessVersion
	if err := db.First(&accessVersion, "user_id = ?", target.ID).Error; err != nil {
		t.Fatalf("reload target access version: %v", err)
	}
	if accessVersion.Version != initialVersion+1 {
		t.Fatalf(
			"authorization version after soft delete = %d, want %d",
			accessVersion.Version,
			initialVersion+1,
		)
	}
}

func TestMySQLDeletingUserRollsBackWhenAccessVersionIncrementFails(
	t *testing.T,
) {
	db := openAccessVersionServiceMySQL(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
		&model.Role{},
		&model.UserRole{},
	); err != nil {
		t.Fatalf("migrate user-deletion rollback tables: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	adminRole := model.Role{
		Name:      fmt.Sprintf("rollback-administrator-%d", time.Now().UnixNano()),
		Code:      "admin",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&adminRole).Error; err != nil {
		t.Fatalf("create rollback administrator role: %v", err)
	}
	operator := model.User{
		Username: fmt.Sprintf("rollback-delete-operator-%d", time.Now().UnixNano()),
		Password: "not-used",
		Email:    fmt.Sprintf("rollback-delete-operator-%d@example.com", time.Now().UnixNano()),
		Role:     "admin",
		Status:   1,
	}
	const initialVersion = 3
	target := model.User{
		Username: fmt.Sprintf("rollback-delete-target-%d", time.Now().UnixNano()),
		Password: "not-used",
		Email:    fmt.Sprintf("rollback-delete-target-%d@example.com", time.Now().UnixNano()),
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create rollback delete operator: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create rollback delete target: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: adminRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign rollback administrator role: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: initialVersion,
	}).Error; err != nil {
		t.Fatalf("create rollback target access version: %v", err)
	}

	accessVersionsTable := quoteAccessVersionMySQLTestIdentifier(
		db.NamingStrategy.TableName("UserAccessVersion"),
	)
	constraintName := fmt.Sprintf("it_avs_delete_primary_%d", time.Now().UnixNano())
	if err := db.Exec(fmt.Sprintf(`
		ALTER TABLE %s
		ADD CONSTRAINT %s CHECK (version <> %d)
	`,
		accessVersionsTable,
		quoteAccessVersionMySQLTestIdentifier(constraintName),
		initialVersion+1,
	)).Error; err != nil {
		t.Fatalf("create forced delete version failure constraint: %v", err)
	}

	err := DeleteUserByAdmin(operator.ID, target.ID)
	if err == nil {
		t.Fatal("DeleteUserByAdmin() returned nil after forced access-version failure")
	}

	var retainedUser model.User
	if err := db.First(&retainedUser, target.ID).Error; err != nil {
		t.Fatalf("target user should remain active after rollback: %v", err)
	}
	if retainedUser.DeletedAt.Valid {
		t.Fatal("target user remained soft deleted after version rollback")
	}
	var accessVersion model.UserAccessVersion
	if err := db.First(&accessVersion, "user_id = ?", target.ID).Error; err != nil {
		t.Fatalf("read access version after delete rollback: %v", err)
	}
	if accessVersion.Version != initialVersion {
		t.Fatalf(
			"authorization version after delete rollback = %d, want %d",
			accessVersion.Version,
			initialVersion,
		)
	}
}

func quoteAccessVersionMySQLTestIdentifier(identifier string) string {
	return "`" + identifier + "`"
}

func runRetryingAccessVersionTransaction(
	db *gorm.DB,
	work func(tx *gorm.DB) (int, error),
) (int, error) {
	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var version int
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			version, err = work(tx)
			return err
		})
		if err == nil {
			return version, nil
		}
		if attempt == maxAttempts || !isRetryableMySQLTransactionError(err) {
			return 0, err
		}
		time.Sleep(time.Duration(attempt) * 10 * time.Millisecond)
	}
	return 0, fmt.Errorf("access-version transaction retries exhausted")
}

func openAccessVersionServiceMySQL(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required for the mandatory MySQL gate")
	}
	tablePrefix := fmt.Sprintf("it_avs_%d_", time.Now().UnixNano())
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: tablePrefix,
		},
	})
	if err != nil {
		t.Fatalf("open MySQL integration database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL integration connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(16)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close MySQL integration database: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := db.Migrator().DropTable(
			&model.UserRole{},
			&model.Role{},
			&model.UserAccessVersion{},
			&model.User{},
		); err != nil {
			t.Errorf("drop isolated MySQL access-version tables: %v", err)
		}
	})
	return db
}
