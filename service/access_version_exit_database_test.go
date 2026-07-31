package service

import (
	"testing"

	"admin/model"
)

func TestExitVersionRegressionDatabaseHasNoLegacyUserColumn(t *testing.T) {
	db := openTokenVersionRegressionDB(t)

	if db.Migrator().HasColumn(&model.User{}, "token_version") {
		t.Fatal("exit-version regression database still has users.token_version")
	}
}
